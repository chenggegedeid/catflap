import type { LucideIcon } from 'lucide-react'
import { enabledIn, useComponents, type ComponentsState } from '../lib/components'
import { pageGroups, pages, type NavGroupDef, type PageKey, type PageMeta } from '../pages/registry'

/**
 * 侧栏导航 —— 从页面注册表（pages/registry.tsx）推导，不再手工维护列表。
 *
 * 本文件不再「定义」有哪些页面（那是注册表的事），只负责两件呈现层的事：
 *
 *   1. 把注册表的扁平结构（pages + pageGroups）组装成侧栏要的嵌套结构；
 *   2. 按「功能管理」里的开关把已关闭组件的入口隐藏掉。
 *
 * 新增页面的流程因此变成：registry.tsx 里加一条 + 挂到某个分组 —— 这里和
 * Sidebar / TopBar / App 都不用动。
 */
export interface NavItem {
  key: PageKey
  label: string
  icon: LucideIcon
  badge?: string
}

export interface NavGroup {
  title?: string
  items: NavItem[]
}

/**
 * 组装导航并按组件开关过滤。
 *
 * 纯函数（不碰 React），便于单测与复用；useNavGroups 只是它的钩子包装。
 *
 * 过滤语义与 App.tsx 的路由回落一致：**拿不准就放行**（enabledIn 对「没数据 /
 * 找不到该组件」一律返回 true），宁可多点一个 404 的入口，也不要在信息不全时
 * 把功能悄悄藏起来。
 *
 * 一个分组里的页面全被关掉时，整个分组（连标题）一起消失 —— 留一个空标题
 * 在侧栏里比没有它更让人困惑。
 */
export function navGroupsFor(state: ComponentsState): NavGroup[] {
  // 放宽成 NavGroupDef 再遍历：pageGroups 是 as const，第一个分组没写 title，
  // 直接在字面量联合上取 .title 会 TS2339。
  const defs: readonly NavGroupDef[] = pageGroups
  const groups: NavGroup[] = []
  for (const g of defs) {
    const items: NavItem[] = []
    for (const key of g.keys) {
      // 显式标注成 PageMeta：pages 用了 satisfies，没写 component 的那几条
      // 在联合类型上取不到这个可选属性（TS2339）。
      const meta: PageMeta = pages[key]
      if (!enabledIn(state, meta.component)) continue
      items.push({ key, label: meta.label, icon: meta.icon, badge: meta.badge })
    }
    if (items.length > 0) groups.push({ title: g.title, items })
  }
  return groups
}

/** 订阅组件状态并给出当前应显示的导航。侧栏用它 */
export function useNavGroups(): NavGroup[] {
  return navGroupsFor(useComponents())
}
