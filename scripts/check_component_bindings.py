#!/usr/bin/env python3
"""检查「页面 ↔ 组件」的绑定没有漂移。

为什么需要它：功能管理页关掉一个组件之后，侧栏上对应的入口要跟着消失。
这条链路上有两个真相源：

  - 后端 modules/<x>/component.go 里的 `const Name = "<x>"`
  - 前端 pages/registry.tsx 里每条页面的 `component: '<x>'`

两边对不上不会报错，只是**静默失效**：侧栏留着一个点了就 404 的入口。
访问审计 / Tunnel / 通知 三个页面就曾经漏绑，关掉功能后入口还在。

本脚本只查三种确定能判的情形，宁可漏报也不误报 —— 误报会让人开始习惯性
忽略这个检查（这个取舍见 check_path_refs.py 里的同一段说明）：

  1. 前端写了 `component: 'x'`，但后端没有叫 x 的组件；
  2. 页面键就叫 x、后端确实有个可开关的组件叫 x，但这条页面没写绑定；
  3. 组件没实现 core/component.Describer —— 功能管理页上会退化成机器名。

第 2 条是漏绑的特征。只在「页面键 == 组件名」时才判：forwards /
reverse-proxy 都挂在 proxy 上，那种「键与组件不同名」的写法脚本认不出来，
也不该去猜。

用法：
    python scripts/check_component_bindings.py
"""

from __future__ import annotations

import pathlib
import re
import sys

ROOT = pathlib.Path(__file__).resolve().parent.parent

REGISTRY = ROOT / "web" / "admin" / "src" / "pages" / "registry.tsx"
MODULES_DIR = ROOT / "modules"

# 嵌入了 component.LocalSwitch 的组件才可以被用户关掉。
# 不可开关的组件（auth / system / backup）无所谓绑定 —— 它们永远开着。
ENABLABLE_MARKER = "component.LocalSwitch"

# `const Name = "ddns"` —— 组件名常量。限定在 component.go / config.go 这类
# 文件里找，避免把 cert 模块里「证书名字」那种同名常量算进来。
NAME_RE = re.compile(r'^\s*const\s+Name\s*=\s*"([a-z0-9_-]+)"', re.MULTILINE)

# `func (xComponent) Title() string` —— 实现了 core/component.Describer。
# 没有对应页面的组件靠它显示中文名，否则界面上只剩机器名。
TITLE_RE = re.compile(r"func\s*\(\s*\w+\s*\)\s*Title\(\)\s*string")
SUMMARY_RE = re.compile(r"func\s*\(\s*\w+\s*\)\s*Summary\(\)\s*string")

# `component: 'ddns',`
BINDING_RE = re.compile(r"^\s*component:\s*'([a-z0-9_-]+)'", re.MULTILINE)

# 页面条目：`  audit: {`
PAGE_RE = re.compile(r"^\s{2}'?([a-z0-9-]+)'?:\s*\{", re.MULTILINE)


def collect_components() -> dict[str, dict]:
    """返回 {组件名: {"enableable": bool, "title": bool, "dir": str}}。

    扫的是模块顶层的所有 .go（不含 _test）：多数模块把 `const Name` 写在
    component.go，但 tunnel 写在 config.go —— 只认 component.go 会把 tunnel
    整个漏掉，表现是「后端少了这个组件」，方向完全反了。

    「可开关」的判据是模块里嵌了 component.LocalSwitch：注册表就是照这个
    接口来决定给不给路由组套 404 闸门的（core/component/routes.go）。
    """
    out: dict[str, dict] = {}
    if not MODULES_DIR.is_dir():
        return out

    for mod in sorted(MODULES_DIR.iterdir()):
        if not mod.is_dir():
            continue
        name: str | None = None
        info = {"enableable": False, "title": False, "summary": False, "dir": mod.name}
        for go in sorted(mod.glob("*.go")):
            if go.name.endswith("_test.go"):
                continue
            text = go.read_text(encoding="utf-8", errors="ignore")
            if name is None:
                m = NAME_RE.search(text)
                if m:
                    name = m.group(1)
            if ENABLABLE_MARKER in text:
                info["enableable"] = True
            if TITLE_RE.search(text):
                info["title"] = True
            if SUMMARY_RE.search(text):
                info["summary"] = True
        if name:
            out[name] = info
    return out


def collect_pages() -> dict[str, str | None]:
    """返回 {页面键: 绑定的组件名或 None}。

    做法是按 `pages = { ... } satisfies` 这一段里出现的顺序配对：页面键后面
    跟着的下一个 component: 就属于它。比按正则分别抓两个列表再猜对应关系稳。
    """
    text = REGISTRY.read_text(encoding="utf-8", errors="ignore")
    start = text.find("export const pages = {")
    if start < 0:
        return {}
    # 到 satisfies 为止，别把 pageGroups 里的键也算进来
    end = text.find("satisfies Record<string, PageMeta>", start)
    if end < 0:
        end = len(text)
    body = text[start:end]

    pages: dict[str, str | None] = {}
    current: str | None = None
    for line in body.splitlines():
        m = PAGE_RE.match(line)
        if m:
            current = m.group(1)
            pages[current] = None
            continue
        if current is None:
            continue
        b = BINDING_RE.match(line)
        if b:
            pages[current] = b.group(1)
            current = None
        elif line.rstrip() == "},":
            current = None
    return pages


def main() -> int:
    if not REGISTRY.is_file():
        print(f"[跳过] 找不到 {REGISTRY.relative_to(ROOT)}")
        return 0

    comps = collect_components()
    pages = collect_pages()
    if not comps:
        print("[失败] 没从 modules/*/component.go 里读到任何组件名，检查是不是改名了")
        return 1

    errors: list[str] = []

    for key, bound in sorted(pages.items()):
        if bound is not None and bound not in comps:
            errors.append(
                f"页面 {key!r} 绑定了组件 {bound!r}，但后端没有叫这个名字的组件"
                f"（已知：{', '.join(sorted(comps))}）"
            )
            continue

        # 页面键与组件同名、且那个组件可开关 → 期待有绑定
        if bound is None and comps.get(key, {}).get("enableable") is True:
            errors.append(
                f"页面 {key!r} 没有绑定组件，但后端有一个可开关的组件就叫 {key!r}。"
                f"关掉它之后侧栏入口不会消失 —— 在这一条里补 component: '{key}'，"
                f"或者在注释里说明为什么不绑。"
            )

    # 组件缺 Describer：功能管理页上会退化成机器名（webhook / traffic / auth…），
    # 用户看不出那是什么。有页面的组件还能靠页面名兜住，没页面的就只能靠自己。
    for name, info in sorted(comps.items()):
        if not (info["title"] and info["summary"]):
            missing = [k for k in ("title", "summary") if not info[k]]
            errors.append(
                f"组件 {name!r}（modules/{info['dir']}/）没实现 "
                f"core/component.Describer 的 {'/'.join(missing)}()，"
                f"功能管理页上会显示机器名。补上 Title() 与 Summary() 两个方法。"
            )

    if errors:
        print("[失败] 页面与组件的绑定对不上：\n")
        for e in errors:
            print(f"  - {e}")
        print("\n功能管理关掉某个功能后，侧栏上的入口要跟着消失；")
        print("页面与组件的对应关系错了不会报错，只是静默失效。")
        return 1

    bound = sum(1 for v in pages.values() if v)
    print(f"[通过] {len(pages)} 个页面 / {len(comps)} 个组件，其中 {bound} 条页面绑定了组件")
    return 0


if __name__ == "__main__":
    sys.exit(main())
