#!/usr/bin/env python3
"""检查代码/脚本里引用的仓库文件是否真的存在。

为什么需要它：清理仓库（比如把图标从根目录挪进 branding/）时，很容易漏改
某个脚本里的路径。这类问题的表现很隐蔽——本地跑得通，直到 CI 上的某一步
才炸，而打包步骤一炸，后面「发布 Release」就被跳过，表现为"tag 打了但
一条资产都没有"。

本脚本把这类引用扫出来，在推送前就拦住。

用法：
    python scripts/check_path_refs.py
"""

from __future__ import annotations

import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent

# 只扫这些类型：它们的路径错了会直接让构建/打包/发布失败
SCAN_EXT = {".py", ".sh", ".yml", ".yaml", ".toml", ".cfg", ".ini"}
SCAN_NAMES = {"Dockerfile", "Taskfile.yml", "Taskfile.yaml", "Makefile"}

# 明确要检查的关键资产：这些是"被挪走过"或"容易被顺手删掉"的文件。
# 不在这里的文件不查——全量扫路径会淹在噪声里（正则、URL、注释里的例子）。
WATCHED = [
    "branding/icon/icon-256.png",
    "branding/icon/tray-64.png",
    "deploy/fnos/manifest",
    "deploy/systemd/catflap.service",
    "scripts/install.sh",
    "docs/guides/快速上手.md",
    "docs/guides/部署与升级.md",
    # IP 归属地库：被 //go:embed 引用，删掉会直接编译失败；
    # 许可证是 Apache-2.0 的要求，删掉就是许可不合规
    "modules/audit/geo/ip2region_v4.xdb",
    # IPv6 的库大（解压后 37MB），存的是 gzip 版
    "modules/audit/geo/ip2region_v6.xdb.gz",
    "modules/audit/geo/LICENSE.ip2region",
]

# 已经不存在、且不该再被引用的历史路径。出现即报错。
# 都是这次仓库清理（ADR-0006）删掉的。
#
# 注意每个模式的上下文：`icon_256.png` 这种名字在 fpk 包里也会出现
# （包内的 ui/images/icon_256.png），那不是在引用仓库文件。所以这里用
# 前后文把"从仓库根读"的写法才认出来——宁可漏报也别误报，
# 误报会让人开始习惯性忽略这个检查。
FORBIDDEN = [
    r"ROOT\s*/\s*[\"']icon_256\.png[\"']",
    r"ROOT\s*/\s*[\"']icon_64\.png[\"']",
    r"\blICON\.PNG\b",
    r"[\"'`]\.?/?README\.en-US\.md[\"'`]",
    r"[\"'`]docs/about\.md[\"'`]",
    r"[\"'`]docs/快速上手\.md[\"'`]",
    r"[\"'`]docs/部署与升级\.md[\"'`]",
    # 旧位置：文档曾经在 docs/ 根下，现在归到 docs/guides/
    r"[\"'`](?:\.\./)?docs/img/(?:home|stun|qq|wx)\.(?:jpg|png)[\"'`]",
]


def iter_files():
    for p in ROOT.rglob("*"):
        if not p.is_file():
            continue
        rel = p.relative_to(ROOT).as_posix()
        if rel.startswith((".git/", "node_modules/")) or "/node_modules/" in rel:
            continue
        if rel.startswith("dist/") or "/dist/" in rel:
            continue
        if p.suffix in SCAN_EXT or p.name in SCAN_NAMES:
            yield p, rel


def main() -> int:
    problems: list[str] = []

    for path, rel in iter_files():
        try:
            text = path.read_text(encoding="utf-8")
        except (UnicodeDecodeError, OSError):
            continue

        for pat in FORBIDDEN:
            for m in re.finditer(pat, text):
                line = text[: m.start()].count("\n") + 1
                problems.append(f"{rel}:{line} 引用了已删除的路径 {m.group(0)}")

    for rel in WATCHED:
        if not (ROOT / rel).is_file():
            problems.append(f"关键文件缺失：{rel}")

    if problems:
        print("发现失效引用：")
        for p in problems:
            print("  ✗", p)
        return 1

    print(f"OK  未发现失效路径引用（检查了 {len(WATCHED)} 个关键资产与 {len(FORBIDDEN)} 条禁用模式）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
