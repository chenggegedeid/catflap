/**
 * 流式网格。
 *
 * 列数随窗口宽度变（手机 4 列 → 宽屏 8 列），卡片按 `order` 的顺序往下排。
 *
 * **不存坐标，只存顺序**：坐标由这里按当前列数现算（lib/grid.ts 的 normalize）。
 * 存死坐标的话，同一个配置在手机上就会跑到网格外面去。
 */

import { useEffect, useState } from 'react'
import { normalize, rowsOf } from '../../lib/grid'
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
  selection,
  onToggleSelect,
}: {
  apps: AppView[]
  prefer?: NetworkPrefer
  onOpenMenu?: (app: AppView, x: number, y: number) => void
  /** 批量删除模式的选中集合；非空时卡片右上角出现勾选框 */
  selection?: Set<string>
  onToggleSelect?: (id: string) => void
}) {
  const cols = useColumns()

  // 排布交给 normalize：越界的会被夹回来、重叠的会顺延、没坐标的按顺序补位。
  // 结果稳定可复现（同样的输入永远同样的输出），刷新不会跳。
  const placed = normalize(
    apps.map((a) => ({ id: a.id, size: a.size })),
    cols,
  )
  const byId = new Map(placed.map((p) => [p.id, p]))

  return (
    <div
      className="grid gap-3"
      style={{
        gridTemplateColumns: `repeat(${cols}, minmax(0, 1fr))`,
        gridAutoRows: 'minmax(5.5rem, auto)',
      }}
    >
      {apps.map((app) => {
        const p = byId.get(app.id) ?? { id: app.id, row: 0, col: 0, w: 1, h: 1 }
        return (
          <AppCard
            key={app.id}
            app={app}
            prefer={prefer}
            placement={{ col: p.col, row: p.row, w: p.w, h: p.h }}
            onOpenMenu={onOpenMenu}
            selectable={!!selection}
            selected={selection?.has(app.id) ?? false}
            onToggleSelect={onToggleSelect}
          />
        )
      })}

      {/* 用行数撑一下底部，免得最后一排贴着屏幕边缘 */}
      <div style={{ gridColumn: 1, gridRow: rowsOf(placed) + 1 }} aria-hidden />
    </div>
  )
}
