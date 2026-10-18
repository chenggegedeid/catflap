#!/usr/bin/env python3
"""安装脚本守门：语法 + 头部注释块完整性。

为什么需要它（2026-10-18 事故的教训）：

    曾经给 install.sh 的头部注释块补了几行说明、漏写行首的 `#`，
    bash 就把中文当命令执行。报错长这样 ——

        /root/install.sh: line 11: $'\350\257\264\346\230\216': command not found
        /root/install.sh: line 14: `curl: (23)'

    里面混着 `curl: (23)`、`raw.githubusercontent.com`、`代理` 这些字样，
    看着完全是网络故障，实际是纯语法错误 —— 换域名、换镜像、加代理都绕不开。

    ⚠️ 关键点：`bash -n` **抓不到**这种情况。一串中文在 shell 语法里是合法的
    命令名（执行时才 command not found）。真正灵敏的信号是 `--help` 的输出行数：
    头部注释块一旦被非 `#` 行或空行截断，行数会从 70+ 掉到 10 以下。

CI 与本地都跑这一份，避免两边规则漂移。
"""
import subprocess
import sys

SCRIPT = "scripts/install.sh"
MIN_HELP_LINES = 30


def run(cmd):
    return subprocess.run(cmd, capture_output=True, text=True)


def main():
    # 1. shell 语法（能抓真正的语法错误，比如反引号没闭合）
    r = run(["bash", "-n", SCRIPT])
    if r.returncode != 0:
        print("FAIL  bash -n 未通过：")
        print(r.stderr.strip())
        return 1

    # 2. --help 行数（能抓漏 # / 注释块被空行截断）
    r = run(["bash", SCRIPT, "--help"])
    n = len([ln for ln in r.stdout.splitlines() if ln.strip()])
    if n < MIN_HELP_LINES:
        print(f"FAIL  {SCRIPT} --help 只有 {n} 行（应 >= {MIN_HELP_LINES}）")
        print("      多半是有行漏写了行首的 # ；注释块里的空行也要写成 #，不能留空。")
        return 1

    print(f"OK  {SCRIPT} 语法通过，--help {n} 行")
    return 0


if __name__ == "__main__":
    sys.exit(main())
