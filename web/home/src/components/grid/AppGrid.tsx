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
import { DragGrid, SortableCard } from './DragGrid'

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

  const idList = apps.map((a) => a.id)
  const grid = (
    <div
      // gap-2 而不是 gap-3：8 列时列间距要乘 7 遍，gap-3 加起来就吃掉半张卡片
      className="relative grid gap-2"
      style={{
        gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
        /**
         * 行高用 auto，**不要**给 minmax(6.5rem, ...)。
         *
         * 给一个较大的最小行高时，1×1 的卡片（图标 + 名字）撑不满，
         * 剩下的空白全落在行里 —— 视觉上就是卡片之间隔着一大片。
         * 1×1 那行由内容定高，2×2 那行自然就是它的两倍（grid-row: span 2）。
         */
        gridAutoRows: 'auto',
        // 用 'row' 而不是 'row dense'：dense 会把后面的小卡片回填到前面的空洞里，
        // 结果是"我以为排在第一个，实际被挤到了第二列"。顺序就是顺序，
        // 不要为了填满而自作聪明地重排。
        gridAutoFlow: 'row',
      }}
    >
      {apps.map((app, idx) => {
        const sz = sizeById.get(app.id) ?? { w: 1, h: 1 }
        const span = {
          gridColumn: `span ${Math.min(sz.w, cols)}`,
          gridRow: `span ${sz.h}`,
        }
        const card = (
          <AppCard app={app} prefer={prefer} placement={{ col: 0, row: 0, w: sz.w, h: sz.h }} onOpenMenu={onOpenMenu} />
        )

        return (
          <SortableCard key={app.id} id={app.id} index={idx} ids={idList}>
            <div style={span}>{card}</div>
          </SortableCard>
        )
      })}
    </div>
  )

  // 总是包 SortableContext：能不能拖由外层 HomeDnd 决定（访客没有 DndContext）
  return <DragGrid ids={idList}>{grid}</DragGrid>
}
