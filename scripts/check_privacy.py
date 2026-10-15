#!/usr/bin/env python3
"""防止把会话记录 / 本机私人信息提交进仓库。

为什么需要它：这个仓库是公开的，而开发过程中很容易顺手把不该进的带进去 ——
把 AI 会话记录贴进 issue 草稿、把本机内网 IP 写进示例、把带代理口令的命令
粘进文档。一旦提交，即使后面删掉，历史里也还在。

所以这里做两道检查：
1. 不许出现会话/笔记类文件（路径层面）
2. 不许出现已知的私人标识与本机信息（内容层面）

用法：
    python scripts/check_privacy.py
退出码非 0 表示发现问题。
"""

from __future__ import annotations

import pathlib
import re
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent

# ── 1. 不允许进仓库的路径 ────────────────────────────────────────────────
# 路径片段，命中即报错。都是「本地工作产物」或「AI 会话痕迹」。
FORBIDDEN_PATH_PARTS = [
    ".workbuddy/",
    "memory/",  # 工作区记忆目录
    "clipboard-images/",
    "conversation",
    "transcript",
    "_diag/",  # 本地诊断脚本
]

# 后缀：会话导出常见格式
FORBIDDEN_SUFFIX = {".chatlog", ".transcript"}

# ── 2. 不允许出现的内容 ──────────────────────────────────────────────────
# 只放「确定是隐私或本机信息」的模式。宁可少而准 —— 误报会让人开始
# 习惯性忽略这个检查，那时它就等于没有。
FORBIDDEN_CONTENT = [
    # 只认「像本机绝对路径」的写法（带盘符或家目录前缀），
    # 纯提一句 .workbuddy 目录名不算问题——那可能只是在说明某个工具在哪
    (r"[A-Za-z]:[\\\\/]Users[\\\\/][^\\\\/\s]+", "出现了本机 Windows 用户目录"),
    (r"/Users/[a-z][^/\s]*/", "出现了本机 macOS 用户目录"),
    (r"/home/[a-z][^/\s]*/\.workbuddy", "出现了本机 Linux 用户目录"),
    (r"XVAiEIEB", "出现了代理口令"),
    (r"Clipboard_Screenshot", "出现了剪贴板截图文件名"),
]

# 扫描的文件类型：只扫会进仓库的文本
SCAN_SUFFIX = {
    ".go", ".ts", ".tsx", ".js", ".jsx", ".py", ".sh", ".md",
    ".json", ".yml", ".yaml", ".toml", ".css", ".html", ".txt",
}


def tracked_files() -> list[str]:
    out = subprocess.run(
        ["git", "ls-files"], cwd=ROOT, capture_output=True, text=True, check=False
    )
    if out.returncode != 0:
        print("无法列出仓库文件（不在 git 仓库里？）")
        return []
    return [f for f in out.stdout.splitlines() if f]


def main() -> int:
    problems: list[str] = []
    files = tracked_files()

    for rel in files:
        posix = rel.replace("\\", "/")
        if any(part in posix for part in FORBIDDEN_PATH_PARTS):
            problems.append(f"{rel}：这类文件不该进仓库（本地工作产物/会话痕迹）")
            continue
        if pathlib.Path(rel).suffix in FORBIDDEN_SUFFIX:
            problems.append(f"{rel}：会话导出文件不该进仓库")
            continue

        if pathlib.Path(rel).suffix.lower() not in SCAN_SUFFIX:
            continue
        p = ROOT / rel
        try:
            text = p.read_text(encoding="utf-8")
        except (UnicodeDecodeError, OSError):
            continue
        for pat, why in FORBIDDEN_CONTENT:
            m = re.search(pat, text)
            if m:
                line = text[: m.start()].count("\n") + 1
                problems.append(f"{rel}:{line} {why}：{m.group(0)[:40]}")

    if problems:
        print("发现不该进仓库的内容：")
        for p in problems:
            print("  ✗", p)
        print("\n如确认无隐私问题，请修正后再提交。")
        return 1

    print(f"OK  仓库未发现会话痕迹与本机隐私信息（检查了 {len(files)} 个文件）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
