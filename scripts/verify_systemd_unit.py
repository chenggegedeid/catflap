#!/usr/bin/env python3
"""校验 deploy/systemd/catflap.service 的结构与安全基线。

为什么不直接用 systemd-analyze verify：
它会连带检查 User= 引用的账户、ExecStart= 指向的可执行文件在当前机器上是否存在，
在 CI 容器里必然报一堆环境噪音，反倒把真正的配置问题盖住了。
这里只看「这份 unit 自己该满足什么」，在任何机器上结论一致。

用法：
    python3 scripts/verify_systemd_unit.py [unit 文件路径]
不传参数时默认校验 deploy/systemd/catflap.service。
退出码非 0 表示不合规。
"""

from __future__ import annotations

import re
import sys
from pathlib import Path

DEFAULT_UNIT = "deploy/systemd/catflap.service"

# [Service] 段里必须有、且值必须精确等于期望的项。
# 每一条都对应 SECURITY.md 部署清单里的一个承诺，改动前先想清楚。
REQUIRED_EXACT: dict[str, str] = {
    "User": "catflap",
    "Group": "catflap",
    "NoNewPrivileges": "yes",
    "ProtectSystem": "strict",
    "UMask": "0077",
    "CapabilityBoundingSet": "",
}

# 只要求「存在且非空」
REQUIRED_NONEMPTY = ("WorkingDirectory", "ExecStart")


def parse(text: str) -> tuple[dict[str, dict[str, str]], list[str]]:
    """返回 ({段名: {键: 值}}, 语法错误列表)。后出现的同名键覆盖先出现的。"""
    sections: dict[str, dict[str, str]] = {}
    errors: list[str] = []
    current: str | None = None

    for lineno, raw in enumerate(text.splitlines(), 1):
        line = raw.strip()
        if not line or line.startswith("#") or line.startswith(";"):
            continue
        if line.startswith("["):
            if not re.fullmatch(r"\[[A-Za-z]+\]", line):
                errors.append(f"{lineno}: 段名格式不对：{line}")
                continue
            current = line
            sections.setdefault(current, {})
            continue
        if "=" not in line:
            errors.append(f"{lineno}: 不是 Key=Value：{line}")
            continue
        if current is None:
            errors.append(f"{lineno}: 键出现在任何 [段] 之前：{line}")
            continue
        key, value = line.split("=", 1)
        key = key.strip()
        if not re.fullmatch(r"[A-Za-z][A-Za-z0-9]*", key):
            errors.append(f"{lineno}: 键名不合法：{key}")
            continue
        sections[current][key] = value.strip()

    return sections, errors


def check(path: Path) -> list[str]:
    errors: list[str] = []
    if not path.is_file():
        return [f"找不到 unit 文件：{path}"]

    sections, parse_errors = parse(path.read_text(encoding="utf-8"))
    errors.extend(parse_errors)

    service = sections.get("[Service]", {})
    unit = sections.get("[Unit]", {})
    install = sections.get("[Install]", {})

    for section, name in ((unit, "[Unit]"), (service, "[Service]"), (install, "[Install]")):
        if not section:
            errors.append(f"缺少 {name} 段")

    for key, want in REQUIRED_EXACT.items():
        got = service.get(key, "<缺失>")
        if got != want:
            errors.append(f"[Service] {key}={got!r}，期望 {want!r}")

    for key in REQUIRED_NONEMPTY:
        if not service.get(key):
            errors.append(f"[Service] {key} 缺失或为空")

    if service.get("User") == "root":
        errors.append("[Service] User=root：面板能读写明文凭据，绝不能用 root 跑")

    # 工作目录必须可写，否则程序建不了 config/ 与 data/，表现为「配置存了重启就丢」
    workdir = service.get("WorkingDirectory", "")
    writable = [d.strip() for d in service.get("ReadWritePaths", "").split() if d.strip()]
    if workdir and writable and workdir not in writable:
        errors.append(
            f"WorkingDirectory={workdir} 不在 ReadWritePaths={writable} 里，"
            "程序将无法写配置"
        )

    if not unit.get("After"):
        errors.append("[Unit] 缺少 After=network-online.target")

    if install.get("WantedBy") != "multi-user.target":
        errors.append("[Install] WantedBy 应为 multi-user.target")

    return errors


def main() -> int:
    path = Path(sys.argv[1] if len(sys.argv) > 1 else DEFAULT_UNIT)
    errors = check(path)
    if errors:
        print(f"✗ {path} 未通过校验：")
        for e in errors:
            print(f"  - {e}")
        return 1
    print(f"✓ {path} 通过校验（安全基线与必填项齐全）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
