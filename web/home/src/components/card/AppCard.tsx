/**
 * 卡片。
 *
 * 设计取向：**卡片只负责"看"和"点"，地址明细交给右键菜单**。
 *
 * 以前的做法是在卡片下面标 `v4 · v6 · 内网`，再在悬停时浮出一排按钮 ——
 * 这有两个毛病：标注区把卡片撑得很高、每一格都要重复一遍同样的信息；
 * 而且浮层和右键菜单是两套交互，同一件事两条路。
 *
 * 现在：
 *   - 卡片 = 图标 + 名字（干净，1×1 就这么大）
 *   - 点 = 按网络偏好走默认入口（日常最常用的那一下）
 *   - 右键 = 默认地址 / 内网地址各自的「打开」「复制」「新窗口」，外加编辑、删除
 *
 * 也就是说：**信息量交给菜单，卡片回到最小**。尺寸（1×1 / 2×2）只影响占几格，
 * 不再影响显示多少东西 —— 用户不必为了"看到地址"而把卡片调大。
 */

import { useState } from 'react'
import { openEntry, resolvedEntry } from '../../lib/addresses'
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
  const [hover, setHover] = useState(false)
  // 用 resolvedEntry 而不是 preferredEntry：要知道**实际**走的是哪个入口，
  // 否则偏好 v4 但只有内网时，提示还写「公网 v4」—— 那是在骗人
  const target = resolvedEntry(app.addresses, prefer)
  const url = target?.url ?? null

  // 大卡片图标也大一点：2×2 里放一个 4.5rem 的图标会显得很小气
  const big = placement.w > 1 || placement.h > 1

  return (
    <div
      className={`relative rounded-2xl ${selected ? 'ring-2 ring-blue-400' : ''}`}
      // 只给 span，不给起点 —— 起点交给 CSS 的自动排布（见 AppGrid 里的说明）
      style={{ gridColumn: `span ${placement.w}`, gridRow: `span ${placement.h}` }}
      aria-selected={selectable ? selected : undefined}
      onClickCapture={(e) => {
        // 批量删除模式下，点卡片 = 选中/取消，不打开链接
        if (selectable) {
          e.preventDefault()
          e.stopPropagation()
          onToggleSelect?.(app.id)
        }
      }}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      onContextMenu={(e) => {
        // 必须挡住冒泡：网格容器上也接了右键（空白处 → 新建），
        // 不挡的话点卡片会同时弹出"卡片菜单"和"新建菜单"两层
        e.stopPropagation()
        if (!onOpenMenu) return
        e.preventDefault()
        onOpenMenu(app, e.clientX, e.clientY)
      }}
      // 键盘用户的菜单入口：Shift+F10 / Menu 键（Windows 惯例），m 是好记的补充。
      // 没有这条的话，不用鼠标的人连复制地址、改大小都够不着。
      onKeyDown={(e) => {
        if (!onOpenMenu) return
        const isMenuKey = e.key === 'ContextMenu' || (e.key === 'F10' && e.shiftKey) || e.key === 'm'
        if (!isMenuKey) return
        e.preventDefault()
        const r = (e.currentTarget as HTMLElement).getBoundingClientRect()
        onOpenMenu(app, r.left + r.width / 2, r.bottom)
      }}
    >
      <button
        type="button"
        aria-disabled={!url}
        onFocus={() => setHover(true)}
        onBlur={() => setHover(false)}
        onClick={() => {
          // 没有 url 就什么都不做，但按钮必须保持"活着" ——
          // disabled 会让整个元素收不到事件（包括右键），那样连菜单都出不来
          if (url) openEntry(url)
        }}
        title={
          url && target
            ? `${app.name}（${titleOf(target.kind)}${target.kind !== prefer ? ' · 按偏好本该走 v4/v6，但没配' : ''}）`
            : `${app.name}：还没配地址，右键可编辑`
        }
        className={`flex h-full w-full flex-col items-center justify-center gap-2 rounded-2xl p-2 text-center transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-white/70 ${
          url ? 'hover:-translate-y-0.5 hover:bg-white/15' : 'opacity-60'
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

        <span
          // 手机上列窄，图标要跟着收小 —— 否则 4 列时图标顶到格子边上
          className={`aspect-square w-full overflow-hidden rounded-[22%] shadow-sm ring-1 ring-black/5 ${
            big ? 'max-w-[7rem]' : 'max-w-[3.25rem] sm:max-w-[4.5rem]'
          }`}
        >
          <AppIcon name={app.name} icon={app.icon} />
        </span>

        <span className="line-clamp-2 w-full text-xs font-medium leading-tight text-white drop-shadow">
          {app.name}
        </span>
      </button>

      {/* 没配地址：给一句极短的提示，且只在悬停时出现（不占常驻空间） */}
      {!url && hover && (
        <span className="pointer-events-none absolute inset-x-0 bottom-1 text-center text-[10px] text-white/70">
          未配置地址
        </span>
      )}
    </div>
  )
}

function titleOf(kind: 'wanV4' | 'wanV6' | 'lan'): string {
  if (kind === 'lan') return '内网'
  if (kind === 'wanV6') return '公网 v6'
  return '公网 v4'
}
