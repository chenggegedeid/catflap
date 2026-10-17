/**
 * 流式网格。
 *
 * 列数随窗口宽度变（手机 4 列 → 宽屏 8 列），卡片按 `order` 的顺序往下排。
 *
 * **不存坐标，只存顺序**：坐标由这里按当前列数现算（lib/grid.ts 的 normalize）。
 * 存死坐标的话，同一个配置在手机上就会跑到网格外面去。
 */

import { useEffect, useState } from 'react'
import { normalize } from '../../lib/grid'
import type { AppView, NetworkPrefer } from '../../types'
import { AppCard } from '../card/AppCard'

/** 列数跟着宽度走，和 lib/grid 里 narrowCols=4 的判断保持一致 */
function useColumns(): number {
  const read = () => {
    const w = window.innerWidth
    if (w < 640) return 4
    if (w < 1024) return 6
    return 8
  }
  const [cols, setCols] = useState(read)

  useEffect(() => {
    const onResize = () => setCols(read())
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  }, [])

  return cols
}

export function AppGrid({
  apps,
  prefer = 'wanV4',
  onOpenMenu,
}: {
  apps: AppView[]
  prefer?: NetworkPrefer
  onOpenMenu?: (app: AppView, x: number, y: number) => void
}) {
  const cols = useColumns()

  /**
   * 这里**只取尺寸，不用坐标**。
   *
   * 曾经按 normalize 算出的 row/col 显式定位（grid-row-start / grid-column-start），
   * 结果是每张卡片各占一行、中间留一大片空白 —— 因为 CSS 网格的隐式行高是 auto，
   * 一格里内容小，那一行就被别的卡片撑高，视觉上全散了。
   *
   * 流式布局本来就是"按顺序往下排"，顺序由 order 决定，坐标是**渲染结果**
   * 而不是输入。所以交给 auto-placement：浏览器自己会贴着排满。
   * normalize 仍然用来把 size 收敛成合法尺寸（越界/写错的尺寸会退成 1×1）。
   */
  const placed = normalize(
    apps.map((a) => ({ id: a.id, size: a.size })),
    cols,
  )
  const sizeById = new Map(placed.map((p) => [p.id, { w: p.w, h: p.h }]))

  return (
    <div
      className="grid gap-3"
      style={{
        gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
        // auto-rows 给一个"能装下一张 1×1"的最小高度：
        // 不给的话行高由内容决定，1×1 和 2×2 混排时行高会忽大忽小
        gridAutoRows: 'minmax(6.5rem, max-content)',
        gridAutoFlow: 'row dense',
      }}
    >
      {apps.map((app) => {
        const sz = sizeById.get(app.id) ?? { w: 1, h: 1 }
        return (
          <AppCard
            key={app.id}
            app={app}
            prefer={prefer}
            placement={{ col: 0, row: 0, w: sz.w, h: sz.h }}
            onOpenMenu={onOpenMenu}
          />
        )
      })}
    </div>
  )
}
