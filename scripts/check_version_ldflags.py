#!/usr/bin/env python3
"""校验「版本号注入点」没有被改错地方。

# 为什么需要这个脚本

版本号由 CI 在编译期注入：

    go build -ldflags "-X github.com/chenggegedeid/catflap/core.Version=v1.0.0"

Go 的链接器有一个很坑的行为：**`-X` 指向的符号不存在时不报错**，只是静静
地什么都不做。于是「把 Version 变量挪了个包」这种重构，编译照样通过、
测试照样全绿，只有发布出去的二进制会显示 `dev` —— 而版本号还参与更新
检查（modules/update），等于连自动更新也一起坏掉。

所以这里把「注入点写在哪」变成一条可被 CI 检查的断言：
扫描发布流水线里的 `-X github.com/chenggegedeid/catflap/<pkg>.Version`，要求 pkg 就是 core。

用法：

    python scripts/check_version_ldflags.py          # 校验，不一致则退出码 1
"""

from __future__ import annotations

import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent

# 版本号变量所在的包（相对 module 路径）
EXPECTED_PACKAGE = "core"
EXPECTED_SYMBOL = "Version"

# 要扫描的文件：只有真正编译发布产物的地方才需要检查
TARGETS = [
    ".github/workflows/release.yml",
    ".github/workflows/ci.yml",
]

def module_path() -> str:
    """从 go.mod 读出 module 路径，不把前缀写死在脚本里。

    写死过一次：项目改名时脚本没跟着改，于是它一直在找旧前缀、找不到就报
    「没有找到任何注入」—— 而真正的问题（注入点写错）反而被这条假警报盖住。
    """
    gomod = ROOT / "go.mod"
    if not gomod.is_file():
        raise SystemExit("找不到 go.mod，请在仓库根目录执行")
    for line in gomod.read_text(encoding="utf-8").splitlines():
        line = line.strip()
        if line.startswith("module "):
            return line[len("module ") :].strip()
    raise SystemExit("go.mod 里没有 module 行")


def pattern() -> re.Pattern[str]:
    """匹配 -X <module>/<pkg>.<Symbol>，例如 -X github.com/x/catflap/core.Version=v1.0.0"""
    return re.compile(
        r"-X\s+" + re.escape(module_path()) + r"/([A-Za-z0-9_/]+)\.(" + EXPECTED_SYMBOL + r")\b"
    )


def main() -> int:
    problems: list[str] = []
    found = 0
    pat = pattern()
    mod = module_path()

    for rel in TARGETS:
        path = ROOT / rel
        if not path.exists():
            continue
        for lineno, line in enumerate(path.read_text(encoding="utf-8").splitlines(), 1):
            for m in pat.finditer(line):
                found += 1
                pkg = m.group(1)
                if pkg != EXPECTED_PACKAGE:
                    problems.append(
                        f"{rel}:{lineno} 注入的是 {mod}/{pkg}.{EXPECTED_SYMBOL}，"
                        f"实际应该注入 {mod}/{EXPECTED_PACKAGE}.{EXPECTED_SYMBOL}"
                    )

    if found == 0:
        problems.append(
            f"没有在 {', '.join(TARGETS)} 里找到任何 -X github.com/chenggegedeid/catflap/*.{EXPECTED_SYMBOL} 注入；"
            "是不是流水线改过格式，或者版本号忘了注入？（发布出去的二进制会显示 dev）"
        )

    if problems:
        for p in problems:
            print(f"FAIL {p}")
        return 1

    print(f"OK  版本号注入点一致：{mod}/{EXPECTED_PACKAGE}.{EXPECTED_SYMBOL}（{found} 处）")
    return 0


if __name__ == "__main__":
    sys.exit(main())
