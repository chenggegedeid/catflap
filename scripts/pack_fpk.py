#!/usr/bin/env python3
"""把 Catflap 打成飞牛 fnOS 的第三方应用安装包（.fpk）。

用法（在仓库根目录）：

    python scripts/pack_fpk.py                    # 版本取 deploy/fnos/manifest
    python scripts/pack_fpk.py --version 1.0.1    # 覆盖版本号
    python scripts/pack_fpk.py --arch arm64       # 其它架构
    python scripts/pack_fpk.py --binary <path>    # 用已经编好的二进制

# fpk 的结构（对照一份确定能装的参照包核对出来的）

    外层 fpk = tar.gz
      app.tgz          ← 内层，**必须是真 gzip**，写成裸 tar 飞牛会报
                         「应用包格式不符合系统版本要求」
      cmd/*            ← 生命周期脚本，必须带可执行位
      config/privilege
      config/resource
      wizard/          ← 参照包里是空目录，保持一致
      ICON.PNG
      ICON_256.PNG
      manifest         ← checksum 字段 = md5(app.tgz)

    内层 app.tgz = tar.gz
      server/catflap   ← Go 二进制
      ui/config        ← 桌面图标定义（决定装完有没有桌面入口）
      ui/images/*.png
      config/*

# 两个最容易踩的坑，脚本里都做了断言

1. 内层没 gzip → 飞牛拒装，而且从 manifest 层面完全看不出来。
2. `cmd/*` 没有可执行位 → 装上起不来（Windows 上打 tar 会丢权限位）。
"""

from __future__ import annotations

import argparse
import gzip
import hashlib
import io
import json
import os
import pathlib
import re
import shutil
import subprocess
import sys
import tarfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
FNOS = ROOT / "deploy" / "fnos"
BRANDING = ROOT / "branding"
OUT_DIR = ROOT / "dist"

# manifest 里跟端口相关的字段，必须与 ui/config、cmd 保持一致
SERVICE_PORT = "3791"


def build_binary(version: str, arch: str, dest: pathlib.Path) -> None:
    """交叉编译 linux 二进制。CGO 关掉，产物不依赖 libc 版本。"""
    env = dict(os.environ)
    env.update(
        CGO_ENABLED="0",
        GOOS="linux",
        GOARCH=arch,
    )
    cmd = [
        "go",
        "build",
        "-trimpath",
        "-ldflags",
        f"-s -w -X github.com/chenggegedeid/catflap/core.Version={version}",
        "-o",
        str(dest),
        ".",
    ]
    print(f"  交叉编译 linux/{arch} → {dest.name}")
    r = subprocess.run(cmd, cwd=str(ROOT), env=env, capture_output=True, text=True)
    if r.returncode != 0:
        raise SystemExit(f"go build 失败：\n{r.stdout}\n{r.stderr}")


def add_tree(tar: tarfile.TarFile, src: pathlib.Path, arcname: str, mode: int) -> None:
    """把一个目录树加进 tar。

    recursive=False + 手动遍历，是为了避免 tar.add(dir) 默认递归造成条目重复，
    并且成员名不带 './' 前缀（带了飞牛解包后路径会错）。
    """
    ti = tar.gettarinfo(str(src), arcname=arcname)
    ti.mode = 0o755
    tar.addfile(ti)
    for p in sorted(src.rglob("*")):
        rel = p.relative_to(src).as_posix()
        t = tar.gettarinfo(str(p), arcname=f"{arcname}/{rel}")
        t.mode = 0o755 if p.is_dir() else mode
        if p.is_dir():
            tar.addfile(t)
        else:
            with open(p, "rb") as f:
                tar.addfile(t, f)


def write_gz_tar(dest: pathlib.Path, build) -> None:
    """写一个可复现的 .tar.gz：gzip 头的 mtime 固定为 0。

    tarfile.open(mode="w:gz") 会把当前时间写进 gzip 头，同一个输入每次打出来
    sha256 都不同，导致「本地包 == 线上包」永远判断不了。
    """
    with open(dest, "wb") as fp:
        gz = gzip.GzipFile(filename="", mode="wb", fileobj=fp, compresslevel=9, mtime=0)
        tar = tarfile.open(fileobj=gz, mode="w", format=tarfile.USTAR_FORMAT)
        try:
            build(tar)
        finally:
            tar.close()
            gz.close()


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--version", default="")
    ap.add_argument("--arch", default="amd64")
    ap.add_argument("--binary", default="", help="已编译好的 linux 二进制，跳过交叉编译")
    args = ap.parse_args()

    manifest_src = (FNOS / "manifest").read_text(encoding="utf-8")

    version = args.version
    if not version:
        m = re.search(r"^version\s*=\s*(\S+)", manifest_src, re.M)
        if not m:
            raise SystemExit("manifest 里没有 version")
        version = m.group(1)
    print(f"打包 Catflap {version}（linux/{args.arch}）")

    # 图标从品牌资产目录取。以前是根目录的 icon_256.png / icon_64.png，
    # 品牌资产搬进 branding/ 之后根目录那两份就删了——这里必须跟着改，
    # 否则打包会在"缺少图标"处炸掉，而 CI 上表现为整个 Release 被跳过。
    icon_256 = BRANDING / "icon" / "icon-256.png"
    icon_64 = BRANDING / "icon" / "tray-64.png"
    for p in (icon_256, icon_64):
        if not p.is_file():
            raise SystemExit(f"缺少图标：{p}")

    tmp = OUT_DIR / "_fpk_work"
    if tmp.exists():
        shutil.rmtree(tmp)
    tmp.mkdir(parents=True)

    # ── 1. 二进制 ──────────────────────────────────────────────────────────
    if args.binary:
        binary = pathlib.Path(args.binary)
        if not binary.is_file():
            raise SystemExit(f"找不到二进制：{binary}")
    else:
        binary = tmp / "catflap"
        build_binary(version, args.arch, binary)
    print(f"  二进制 {binary.stat().st_size / 1024 / 1024:.1f} MB")

    # ── 2. 内层 app.tgz ────────────────────────────────────────────────────
    inner = tmp / "app.tgz"

    def build_inner(tar: tarfile.TarFile) -> None:
        ti = tar.gettarinfo(str(binary), arcname="server/catflap")
        ti.mode = 0o755
        with open(binary, "rb") as f:
            tar.addfile(ti, f)

        # ui/config：桌面入口定义。缺了它装完没有桌面图标。
        t = tar.gettarinfo(str(FNOS / "ui" / "config"), arcname="ui/config")
        t.mode = 0o644
        with open(FNOS / "ui" / "config", "rb") as f:
            tar.addfile(t, f)

        # 两个图标直接取仓库根目录的自有资产，不在 deploy/fnos 里再存一份副本
        # （二进制文件重复入库只会让两边慢慢不一致）。
        for arc, src in (
            ("ui/images/icon_64.png", icon_64),
            ("ui/images/icon_256.png", icon_256),
        ):
            t = tar.gettarinfo(str(src), arcname=arc)
            t.mode = 0o644
            with open(src, "rb") as f:
                tar.addfile(t, f)

        add_tree(tar, FNOS / "config", "config", 0o644)

    write_gz_tar(inner, build_inner)
    inner_bytes = inner.read_bytes()
    checksum = hashlib.md5(inner_bytes).hexdigest()
    print(f"  内层 app.tgz {len(inner_bytes) / 1024 / 1024:.1f} MB，md5={checksum}")

    # ── 3. manifest（填版本与 checksum）────────────────────────────────────
    manifest = re.sub(r"^version\s*=.*$", f"version               = {version}",
                      manifest_src, flags=re.M)
    manifest = re.sub(r"^checksum\s*=.*$", f"checksum              = {checksum}",
                      manifest, flags=re.M)
    (tmp / "manifest").write_text(manifest, encoding="utf-8")

    # ── 4. 外层 fpk ────────────────────────────────────────────────────────
    OUT_DIR.mkdir(exist_ok=True)
    out = OUT_DIR / f"catflap-{version}.fpk"

    def build_outer(tar: tarfile.TarFile) -> None:
        # app.tgz
        t = tar.gettarinfo(str(inner), arcname="app.tgz")
        t.mode = 0o644
        with open(inner, "rb") as f:
            tar.addfile(t, f)
        # cmd/*
        add_tree(tar, FNOS / "cmd", "cmd", 0o755)
        # config/*
        add_tree(tar, FNOS / "config", "config", 0o666)
        # wizard/（参照包里是空目录）
        tw = tarfile.TarInfo("wizard")
        tw.type = tarfile.DIRTYPE
        tw.mode = 0o755
        tar.addfile(tw)
        # 图标与 manifest
        for arc, src in (("ICON.PNG", icon_256), ("ICON_256.PNG", icon_256)):
            t = tar.gettarinfo(str(src), arcname=arc)
            t.mode = 0o644
            with open(src, "rb") as f:
                tar.addfile(t, f)
        t = tar.gettarinfo(str(tmp / "manifest"), arcname="manifest")
        t.mode = 0o644
        with open(tmp / "manifest", "rb") as f:
            tar.addfile(t, f)

    write_gz_tar(out, build_outer)
    print(f"\n产物：{out}（{out.stat().st_size / 1024 / 1024:.1f} MB）")

    # ── 5. 断言 ────────────────────────────────────────────────────────────
    print("校验：")
    problems: list[str] = []

    outer = tarfile.open(out, "r:*")
    names = outer.getmembers()
    for need in ["app.tgz", "cmd/main", "config/privilege", "manifest", "ICON.PNG", "ICON_256.PNG"]:
        if not any(m.name == need for m in names):
            problems.append(f"外层缺少 {need}")
    for m in names:
        if m.name.startswith("cmd/") and m.isfile() and not m.mode & 0o111:
            problems.append(f"{m.name} 没有可执行位（mode={oct(m.mode)}），装上起不来")
        if m.name.startswith("./"):
            problems.append(f"成员名带 ./ 前缀：{m.name}")

    app = outer.extractfile("app.tgz").read()
    if app[:2] != b"\x1f\x8b":
        problems.append("内层 app.tgz 不是 gzip 流，飞牛会报「应用包格式不符合系统版本要求」")
    if hashlib.md5(app).hexdigest() != checksum:
        problems.append("manifest 的 checksum 与 app.tgz 的 md5 不一致")

    itf = tarfile.open(fileobj=io.BytesIO(app), mode="r:*")
    inames = [m.name for m in itf.getmembers()]
    for need in ["server/catflap", "ui/config", "ui/images/icon_256.png", "ui/images/icon_64.png"]:
        if need not in inames:
            problems.append(f"内层缺少 {need}")

    # 端口三处一致
    mf = outer.extractfile("manifest").read().decode("utf-8")
    port_mf = re.search(r"^service_port\s*=\s*(\S+)", mf, re.M).group(1)
    ui = json.loads(itf.extractfile("ui/config").read().decode("utf-8"))
    entry = ui[".url"]["Catflap.Application"]
    if port_mf != SERVICE_PORT or entry["port"] != SERVICE_PORT or port_mf != entry["port"]:
        problems.append(
            f"端口不一致：manifest={port_mf}、ui/config={entry['port']}、脚本约定={SERVICE_PORT}"
        )
    launch = re.search(r"^desktop_applaunchname\s*=\s*(\S+)", mf, re.M).group(1)
    if launch not in ui[".url"]:
        problems.append(f"ui/config 的键必须等于 manifest 的 desktop_applaunchname（{launch}）")

    if problems:
        for p in problems:
            print(f"  FAIL {p}")
        return 1

    print(f"  OK  外层 {len(names)} 项，内层 {len(inames)} 项")
    print(f"  OK  内层是 gzip、checksum 一致、cmd/* 有可执行位、端口三处均为 {SERVICE_PORT}")
    print(f"  OK  桌面入口键 {launch} 与 manifest 一致")
    print(f"\nsha256: {hashlib.sha256(out.read_bytes()).hexdigest()}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
