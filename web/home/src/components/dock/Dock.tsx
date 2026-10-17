/**
 * 底部 Dock 栏。
 *
 * 交互（照极光）：把卡片拖到屏幕底部的条上，它就固定在 Dock 里，
 * 不用滚回去找。Dock 里的项也能拖出去移除。
 *
 * ── 三个真会出问题的地方，都处理了 ──
 *
 * 1. **落点检测**。Dock 是 `position: fixed` 的浮层，而 dnd-kit 默认的
 *    closestCenter 算的是元素矩形距离 —— 浮层在滚动容器外面，算出来的
 *    "最近"会跳来跳去。这里用 `pointerWithin`：**先看指针在不在 Dock 上**，
 *    在就归 Dock，不在再走普通排序。指针位置是最可靠的判据。
 *
 * 2. **两个拖拽区打架**。同一个 DndContext 里既有卡片排序又有 Dock，
 *    松手时必须只有一个赢家。规则：**指针落在 Dock 区域 → 进 Dock，不排序**；
 *    否则正常排序。用 `over.id === DOCK_ID` 判断，然后提前 return。
 *
 * 3. **加进去就删不掉**。Dock 项也要能拖出去。这里不做"拖出去移除"
 *    （目标区是整个屏幕，判据模糊），改成 **右键/长按 Dock 项 → 移除**，
 *    和卡片的右键菜单一致。少了这一步，用户加错了就再也拿不掉。
 */

import { useDroppable } from '@dnd-kit/core'
import { useDragState } from '../dnd/HomeDnd'
import { X } from 'lucide-react'
import type { AppView } from '../../types'
import { openEntry, resolvedEntry } from '../../lib/addresses'
import { AppIcon } from '../card/AppIcon'

/** Dock 的 drop 目标 id。用不可能和 app id 撞的值 */
export const DOCK_ID = '__dock__'

export function useDockDroppable() {
  const { setNodeRef, isOver } = useDroppable({ id: DOCK_ID })
  return { setNodeRef, isOver }
}

export function Dock({
  items,
  prefer,
  isOver,
  dropRef,
  onOpenApp,
  onRemove,
  canManage,
}: {
  items: AppView[]
  prefer: Parameters<typeof resolvedEntry>[1]
  isOver: boolean
  dropRef: (el: HTMLElement | null) => void
  onOpenApp: (app: AppView) => void
  onRemove: (id: string) => void
  canManage: boolean
}) {
  /**
   * 关键：**空的 Dock 也必须在拖拽期间渲染出来**，否则拖不进去。
   *
   * 这是个"鸡生蛋"问题：dnd-kit 的 droppable 要渲染了才注册得上，
   * 而不渲染的判断又依赖 useDroppable 的 isOver —— 不渲染就永远没有 isOver，
   * 于是空的 Dock 是个永远命中不了的落区。用户表现为"拖到下面没反应"。
   *
   * 解法：拖拽期间（无论空不空）都渲染，只靠 CSS 让它平时不可见。
   */
  const drag = useDragState()
  const showEmpty = items.length === 0 && drag?.kind === 'app'

  return (
    <div
      ref={dropRef}
      // 空且没在拖 → 彻底移出交互（visibility:hidden 仍会挡住点击）
      style={{
        paddingBottom: 'calc(0.75rem + var(--safe-bottom))',
        ...(items.length === 0 && !showEmpty ? { display: 'none' } : null),
      }}
      // 拖拽落区要能收到指针事件，但空 Dock 不该挡住下面的卡片：
      // pointer-events 由外层容器控制，这里保持 auto 让 useDroppable 生效
      className="fixed inset-x-0 bottom-0 z-30 flex justify-center px-3 transition-all"
      aria-label="常用（Dock）"
    >
      <div
        className={`flex max-w-[min(96vw,52rem)] items-center gap-1.5 overflow-x-auto rounded-2xl px-2.5 py-2 backdrop-blur transition-all ${
          isOver
            ? 'bg-blue-500/35 ring-2 ring-blue-300/80'
            : 'bg-slate-900/45 ring-1 ring-white/15'
        }`}
      >
        {items.length === 0 ? (
          <span className="px-4 py-1 text-xs font-medium text-white/80">
            {isOver ? '松手添加' : '拖到这里，添加到托盘'}
          </span>
        ) : (
          items.map((app) => {
            const target = resolvedEntry(app.addresses, prefer)
            return (
              <div key={app.id} className="group relative shrink-0">
                <button
                  type="button"
                  onClick={() => (target ? openEntry(target.url) : onOpenApp(app))}
                  onContextMenu={(e) => {
                    // 右键移除：没有这条，加错了就再也拿不掉
                    if (!canManage) return
                    e.preventDefault()
                    e.stopPropagation()
                    onRemove(app.id)
                  }}
                  title={app.name}
                  className="grid h-11 w-11 place-items-center overflow-hidden rounded-xl bg-white/10 transition hover:scale-105 hover:bg-white/25 active:scale-95"
                >
                  <span className="h-7 w-7 overflow-hidden rounded-[22%]">
                    <AppIcon name={app.name} icon={app.icon} />
                  </span>
                </button>

                {/* 悬停显示名字 —— Dock 里全是图标，不给名字就得靠猜 */}
                <span className="pointer-events-none absolute bottom-full left-1/2 mb-1.5 hidden -translate-x-1/2 whitespace-nowrap rounded-md bg-slate-900/95 px-2 py-0.5 text-[11px] text-white group-hover:block">
                  {app.name}
                </span>

                {canManage && (
                  <button
                    type="button"
                    onClick={() => onRemove(app.id)}
                    aria-label={`从托盘移除 ${app.name}`}
                    className="absolute -right-1 -top-1 hidden h-4 w-4 place-items-center rounded-full bg-rose-500 text-[9px] text-white group-hover:grid"
                  >
                    <X className="h-2.5 w-2.5" />
                  </button>
                )}
              </div>
            )
          })
        )}
      </div>
    </div>
  )
}
