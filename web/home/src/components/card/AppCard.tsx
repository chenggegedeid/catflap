/**
 * 卡片。
 *
 * 尺寸决定信息量 —— 这是这次改版的核心取舍：
 *
 *   1×1  只有图标 + 名字（默认，极简）
 *   2×2  把三个入口（公网 v4 / v6 / 内网）常驻显示出来
 *
 * 小卡片不是"没有地址"，是**悬停或键盘聚焦时才浮出来**。
 * 手机上没有悬停 —— onContextMenu 在触摸端就是长按，所以长按同样能唤出。
 *
 * 点卡片本体 = 按网络偏好走默认入口；点浮出来的某一项 = 指定走那个入口。
 */

import { useState } from 'react'
import { entriesOf, openEntry, preferredEntry } from '../../lib/addresses'
import type { AppView, NetworkPrefer } from '../../types'
import { AppIcon } from './AppIcon'

export interface Placement {
  col: number
  row: number
  w: number
  h: number
}

export function AppCard({
  app,
  prefer,
  placement,
  onOpenMenu,
  selectable = false,
  selected = false,
  onToggleSelect,
}: {
  app: AppView
  prefer: NetworkPrefer
  placement: Placement
  onOpenMenu?: (app: AppView, x: number, y: number) => void
  selectable?: boolean
  selected?: boolean
  onToggleSelect?: (id: string) => void
}) {
  const [open, setOpen] = useState(false)
  const entries = entriesOf(app.addresses)
  const url = preferredEntry(app.addresses, prefer)

  // 只有 1×1 且不止一个入口时才用浮层：地方够的卡片直接显示，只有一个可选也没什么好挑的
  const useFlyout = placement.w === 1 && placement.h === 1 && entries.length > 1
  const showFlyout = useFlyout && open

  return (
    <div
      className={`relative rounded-2xl ${selected ? 'ring-2 ring-blue-400' : ''}`}
      aria-selected={selectable ? selected : undefined}
      style={{
        gridColumnStart: placement.col + 1,
        gridColumnEnd: `span ${placement.w}`,
        gridRowStart: placement.row + 1,
        gridRowEnd: `span ${placement.h}`,
      }}
      onClickCapture={(e) => {
        // 批量删除模式下，点卡片 = 选中/取消，不打开链接
        if (selectable) {
          e.preventDefault()
          e.stopPropagation()
          onToggleSelect?.(app.id)
        }
      }}
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
      onContextMenu={(e) => {
        if (!onOpenMenu) return
        e.preventDefault()
        onOpenMenu(app, e.clientX, e.clientY)
      }}
      // 键盘用户的菜单入口：Shift+F10（Windows 惯例）或 Menu 键。
      // 没有这条的话，不用鼠标的人连删除/改大小都够不着。
      onKeyDown={(e) => {
        if (!onOpenMenu) return
        // Shift+F10 / Menu 键是 Windows 惯例；m 是好记的补充（卡片上没有输入框，不冲突）
        const isMenuKey =
          e.key === 'ContextMenu' || (e.key === 'F10' && e.shiftKey) || e.key === 'm'
        if (!isMenuKey) return
        e.preventDefault()
        const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
        onOpenMenu(app, r.left + r.width / 2, r.bottom)
      }}
    >
      <button
        type="button"
        aria-disabled={!url}
        onFocus={() => setOpen(true)}
        onBlur={() => setOpen(false)}
        onClick={() => {
          // 没有 url 就什么都不做，但按钮必须保持"活着" ——
          // disabled 会让整个元素收不到事件（包括右键），那样连菜单都出不来
          if (url) openEntry(url)
        }}
        title={
          url
            ? `${app.name}（${titleOf(prefer)}）`
            : `${app.name}：还没配地址`
        }
        className={`flex h-full w-full flex-col items-center justify-center gap-2 rounded-2xl p-2 text-center transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/70 ${
          url ? 'hover:-translate-y-0.5 hover:bg-white/15' : 'cursor-not-allowed opacity-60'
        }`}
      >
        {selectable && selected && (
          <span
            aria-hidden
            className="absolute right-1.5 top-1.5 grid h-5 w-5 place-items-center rounded-full bg-blue-500 text-[10px] font-bold text-white"
          >
            ✓
          </span>
        )}
        <span className="aspect-square w-full max-w-[4.5rem] overflow-hidden rounded-[22%] shadow-sm ring-1 ring-black/5">
          <AppIcon name={app.name} icon={app.icon} />
        </span>

        <span className="line-clamp-2 w-full text-xs font-medium leading-tight text-white drop-shadow">
          {app.name}
        </span>

        {/* 地方够的卡片：入口常驻，不用藏 */}
        {!useFlyout && entries.length > 0 && (
          <span className="flex flex-wrap justify-center gap-1">
            {entries.map((e) => (
              <span key={e.kind} className="rounded bg-white/25 px-1.5 py-0.5 text-[10px] font-semibold text-white/90">
                {shortLabel(e.kind)}
              </span>
            ))}
          </span>
        )}
      </button>

      {showFlyout && (
        <div className="absolute inset-x-0 top-full z-30 mt-1 flex justify-center gap-1 rounded-xl bg-slate-900/90 px-2 py-1.5 text-[11px] shadow-lg backdrop-blur">
          {entries.map((e) => (
            <button
              key={e.kind}
              type="button"
              onClick={() => openEntry(e.url)}
              className="rounded-lg px-2 py-0.5 font-semibold text-white/90 transition hover:bg-white/20 hover:text-white"
            >
              {shortLabel(e.kind)}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

function titleOf(prefer: NetworkPrefer): string {
  if (prefer === 'lan') return '内网'
  if (prefer === 'wanV6') return '公网 v6'
  return '公网 v4'
}

function shortLabel(kind: string): string {
  if (kind === 'wanV4') return 'v4'
  if (kind === 'wanV6') return 'v6'
  return '内网'
}
