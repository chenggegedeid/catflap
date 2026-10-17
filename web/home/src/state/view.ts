/**
 * 首页该显示什么。
 *
 * 这里定了一条产品上的规矩：**没有「全部」这一页**。
 *
 * 理由（用户提的，说得对）：首页是"我每天要点的东西"，不是"我所有东西的清单"。
 * 用户既然分了组，就说明他心里有优先级 —— 第一个分组就是他最常用的那一组。
 * 塞一个「全部」在最前面，等于每次都先给他看一遍全量列表，再多点一次才到
 * 他真正要的地方。
 *
 * 所以：
 *   - 未选中任何分组（首次进入）→ 显示**第一个**分组
 *   - 有一个「全部应用」入口，但放在最后，而且只在真的需要时才用
 *     （比如搜索时想看全量）
 */

import type { Category, HomeConfig } from '../types'

export function sortedCategories(cfg: HomeConfig | null): Category[] {
  if (!cfg) return []
  return [...cfg.categories].sort((a, b) => a.order - b.order)
}

/** 首次进入该选哪个分组：第一个。一个分组都没有时返回 '' */
export function defaultCategoryId(cfg: HomeConfig | null): string {
  const first = sortedCategories(cfg)[0]
  return first?.id ?? ''
}

/**
 * 选中态校正。
 *
 * 选中的分组可能被删了、或者被新加的排序挤走了 —— 那就落回第一个。
 * **不要**落回空串（那会变成"全部"），否则删掉一个分组会突然跳成全量视图。
 */
export function resolveActive(cfg: HomeConfig | null, active: string): string {
  const cats = sortedCategories(cfg)
  if (cats.some((c) => c.id === active)) return active
  return defaultCategoryId(cfg)
}
