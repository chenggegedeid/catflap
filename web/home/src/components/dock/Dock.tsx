/**
 * 底部 Dock 栏（托盘）。
 *
 * 交互：把卡片拖到底部的条上，它就固定在这儿，不用滚回去找。
 * 托盘里的项**也能拖出去** —— 拖到左侧某个分组按钮上 = 把应用移到那个分组。
 *
 * ── 几个关键设计（都踩过坑）──
 *
 * 1. **空的托盘也必须常驻渲染出真实尺寸**。dnd-kit 是在**拖拽开始那一刻**
 *    测量各落点矩形的，那时托盘若是 `display:none`（0×0），之后显示也来不及，
 *    指针永远落不进去。所以用 opacity + pointer-events 隐藏，不用 display:none
 *    —— dnd-kit 的命中判定是纯几何计算（指针坐标 vs 元素矩形），不看 pointer-events。
 *
 * 2. **右键弹菜单，不是直接删除**。之前右键=删除，误触一下托盘项就没了。
 *    现在和卡片用同一套菜单（打开/复制/编辑/从托盘移除）。
 *
 * 3. 托盘项可拖，id 加 `dock:` 前缀，避免和网格里同名应用的 sortable id 撞车
 *    （同一个 DndContext 内 id 必须唯一），真实 appId 放在 data 里带出去。
 */

import { useDraggable, useDroppable } from '@dnd-kit/core'
import { CSS } from '@dnd-kit/utilities'
import { openEntry, resolvedEntry } from '../../lib/addresses'
import type { AppView } from '../../types'
import { AppIcon } from '../card/AppIcon'
import { useDragState } from '../dnd/HomeDnd'

/** Dock 的 drop 目标 id。用不可能和应用 id 撞的值 */
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
  onOpenMenu,
  canManage,
}: {
  items: AppView[]
  prefer: Parameters<typeof resolvedEntry>[1]
  isOver: boolean
  dropRef: (el: HTMLElement | null) => void
  onOpenApp: (app: AppView) => void
  canManage: boolean
  /** 右键托盘项 → 弹出和卡片一样的菜单 */
  onOpenMenu?: (app: AppView, x: number, y: number) => void
}) {
  const drag = useDragState()
  const draggingApp = drag?.kind === 'app'
  const empty = items.length === 0
  // 空且没在拖 → 隐形但**保留矩形**（保证能被拖中，见文件头注释）
  const idle = empty && !draggingApp

  return (
    <div
      ref={dropRef}
      // HomeDnd 松手时靠这个标记现量一次矩形
      data-dock-drop=""
      style={{
        paddingBottom: 'calc(0.75rem + var(--safe-bottom))',
        opacity: idle ? 0 : 1,
        pointerEvents: idle ? 'none' : 'auto',
      }}
      className="fixed inset-x-0 bottom-0 z-30 flex justify-center px-3 transition-opacity"
      aria-label="常用（Dock）"
    >
      <div
        className={`flex max-w-[min(96vw,52rem)] items-center gap-1.5 overflow-x-auto rounded-2xl px-2.5 py-2 backdrop-blur transition-all ${
          isOver ? 'bg-blue-500/35 ring-2 ring-blue-300/80' : 'bg-slate-900/45 ring-1 ring-white/15'
        }`}
      >
        {empty ? (
          <span className="px-4 py-1 text-xs font-medium text-white/80">
            {isOver ? '松手添加' : '拖到这里，添加到托盘'}
          </span>
        ) : (
          items.map((app) => {
            const target = resolvedEntry(app.addresses, prefer)
            return (
              <DockItem
                key={app.id}
                app={app}
                onOpen={() => (target ? openEntry(target.url) : onOpenApp(app))}
                onContextMenu={canManage ? onOpenMenu : undefined}
              />
            )
          })
        )}
      </div>
    </div>
  )
}

/** 托盘里的一项：可点开、可右键弹菜单、可拖到分组 */
function DockItem({
  app,
  onOpen,
  onContextMenu,
}: {
  app: AppView
  onOpen: () => void
  onContextMenu?: (app: AppView, x: number, y: number) => void
}) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: `dock:${app.id}`,
    data: { kind: 'app', appId: app.id },
  })

  return (
    <div
      ref={setNodeRef}
      style={{
        transform: CSS.Translate.toString(transform),
        opacity: isDragging ? 0.4 : 1,
        touchAction: 'none',
      }}
      className="group relative shrink-0"
      {...attributes}
      {...listeners}
    >
      <button
        type="button"
        onClick={onOpen}
        onContextMenu={(e) => {
          if (!onContextMenu) return
          e.preventDefault()
          e.stopPropagation()
          onContextMenu(app, e.clientX, e.clientY)
        }}
        title={app.name}
        className="grid h-11 w-11 place-items-center overflow-hidden rounded-xl bg-white/10 transition hover:scale-105 hover:bg-white/25 active:scale-95"
      >
        <span className="h-7 w-7 overflow-hidden rounded-[22%]">
          <AppIcon name={app.name} icon={app.icon} />
        </span>
      </button>

      {/* 悬停显示名字 —— 托盘里全是图标，不给名字就得靠猜 */}
      <span className="pointer-events-none absolute bottom-full left-1/2 mb-1.5 hidden -translate-x-1/2 whitespace-nowrap rounded-md bg-slate-900/95 px-2 py-0.5 text-[11px] text-white group-hover:block">
        {app.name}
      </span>
    </div>
  )
}
