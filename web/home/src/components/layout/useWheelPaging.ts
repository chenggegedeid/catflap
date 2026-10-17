/**
 * 滚轮翻分组（"滚一下翻一组"，极光那种手感）。
 *
 * ── 为什么需要它 ──
 * 分组内容有长有短。短的组，一屏能放下好几组，用户滚一下其实想的是
 * "看下一组"，而不是"往下挪 100 像素"。所以滚一下 → 平滑跳到下一组。
 *
 * ── 判据（这里是关键，写错就完全不生效）──
 * 要看的是**当前分组这一节**是否滚到头，不是整页。
 * 用整页判据（scrollY 到底）的话，一屏能放好几组时页面永远没到底 → 永远不翻，
 * 表现成"滚轮没反应"。这个 bug 上一版就犯过。
 *
 * ── 三个必须处理的细节 ──
 * 1. **节流**：触控板一次手势会发几十个 wheel 事件，不冷却的话一次手势翻到底。
 *    冷却 700ms，一次手势最多翻一组。
 * 2. **只认纵向**：触控板横扫（|deltaX| 更大）不该翻页。
 * 3. **尊重边界**：已经在第一组还往上滚、最后一组还往下滚 → 不拦截，
 *    交给浏览器自己的回弹，别让用户觉得"卡住了"。
 */

import { useEffect, useRef } from 'react'

const COOLDOWN_MS = 700
const MIN_DELTA = 20
/** 距离"看到头"还差多少像素内就算到头了 */
const EDGE_TOLERANCE = 40

export function useWheelPaging(
  enabled: boolean,
  /** 返回当前分组这一节的 DOM 节点；拿不到就退成整页判据 */
  getCurrentSection: () => HTMLElement | null,
  onPage: (dir: 1 | -1) => void,
  /**
   * paged 模式 = true：滚轮直接翻，不做"滚到边界"判断。
   * 因为 paged 每次只渲染一组，页面通常根本不可滚 ——
   * 判边界的话永远不满足，滚轮就永远没反应（stream 模式犯过的错）。
   */
  alwaysPage = false,
) {
  const last = useRef(0)
  const cb = useRef(onPage)
  const getSection = useRef(getCurrentSection)
  cb.current = onPage
  getSection.current = getCurrentSection

  useEffect(() => {
    if (!enabled) return

    const onWheel = (e: WheelEvent) => {
      // 横向滑动（触控板横扫）不翻页
      if (Math.abs(e.deltaY) <= Math.abs(e.deltaX)) return
      if (Math.abs(e.deltaY) < MIN_DELTA) return

      const now = Date.now()
      if (now - last.current < COOLDOWN_MS) {
        // 冷却期内：还在同一个手势里，拦掉默认滚动，
        // 否则"已经跳了一组"之后页面又被滚一段，看起来像跳过头
        if (Math.abs(e.deltaY) > MIN_DELTA * 2) e.preventDefault()
        return
      }

      // 当前分组这一节是否滚到头
      const section = getSection.current()
      const down = e.deltaY > 0
      let atEdge: boolean
      if (alwaysPage) {
        atEdge = true
      } else if (section) {
        const r = section.getBoundingClientRect()
        // 往下滚：这一节的底边已经进到视口上沿附近 → 看完了
        atEdge = down
          ? r.bottom <= window.innerHeight * 0.5 + EDGE_TOLERANCE
          : r.top >= -EDGE_TOLERANCE
      } else {
        // 拿不到那一节就退成整页判据（不至于完全失效）
        const atBottom = window.innerHeight + window.scrollY >= document.body.scrollHeight - 2
        const atTop = window.scrollY <= 2
        atEdge = down ? atBottom : atTop
      }
      if (!atEdge) return

      last.current = now
      e.preventDefault()
      cb.current(down ? 1 : -1)
    }

    // passive: false —— 要 preventDefault，passive 下浏览器会忽略
    window.addEventListener('wheel', onWheel, { passive: false })
    return () => window.removeEventListener('wheel', onWheel)
  }, [enabled])
}
