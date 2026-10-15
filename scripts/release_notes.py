#!/usr/bin/env python3
"""从 CHANGELOG.md 抽出某个版本的段落，作为 Release 的说明正文。

为什么需要它：以前 Release 的正文是一段硬编码模板（校验和 + 安装方式），
**完全没有"这个版本改了什么"**。用户在 GitHub 上点开每个 Release 都只看到
一堆资产文件名，不知道该不该升级。交接这个活儿给脚本，以后每次发布自动带上。

用法：
    python scripts/release_notes.py 1.7.0          # 输出到标准输出
    python scripts/release_notes.py 1.7.0 -o body.md
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CHANGELOG = ROOT / "CHANGELOG.md"


def extract(version: str) -> str | None:
    """抽指定版本的段落。

    认两种标题写法：`## [1.7.0] - 2026-10-15` 和 `## 1.7.0 - ...`，
    版本号前面的 `v` 可带可不带。
    """
    if not CHANGELOG.exists():
        return None

    text = CHANGELOG.read_text(encoding="utf-8")
    lines = text.splitlines()

    wanted = version.lstrip("vV")
    # 匹配 "## [1.7.0]" / "## v1.7.0" / "## 1.7.0" 开头
    pattern = re.compile(
        r"^##\s+\[?v?V?" + re.escape(wanted) + r"\]?\b.*$", re.IGNORECASE
    )

    start = None
    for i, line in enumerate(lines):
        if pattern.match(line.strip()):
            start = i
            break
    if start is None:
        return None

    # 收集到下一个同级标题（## ）或分隔线为止
    body: list[str] = []
    for line in lines[start + 1 :]:
        stripped = line.strip()
        if stripped.startswith("## ") or stripped.startswith("##\t"):
            break
        # CHANGELOG 里用 --- 分隔各版本，遇到就停（连续两个以上）
        if re.fullmatch(r"-{3,}", stripped):
            break
        body.append(line)

    # 去掉尾部空行
    while body and not body[-1].strip():
        body.pop()

    return "\n".join(body).strip()


def main() -> int:
    ap = argparse.ArgumentParser(description="从 CHANGELOG.md 生成 Release 说明")
    ap.add_argument("version", help="版本号，如 1.7.0 或 v1.7.0")
    ap.add_argument("-o", "--output", help="写入文件（默认输出到标准输出）")
    args = ap.parse_args()

    body = extract(args.version)
    if body is None:
        print(
            f"警告：CHANGELOG.md 里没有 {args.version} 这一段，Release 说明将只有模板部分",
            file=sys.stderr,
        )
        body = ""

    if args.output:
        Path(args.output).write_text(body + "\n", encoding="utf-8")
        print(f"已写入 {args.output}（{len(body)} 字符）", file=sys.stderr)
    else:
        print(body)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
