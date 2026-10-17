/**
 * 全页唯一的拖拽上下文。
 *
 * **为什么必须只有一个**：卡片网格、左侧分组栏、底部 Dock 是三块独立区域，
 * 但它们之间要能互拖（卡片 ↔ Dock）。dnd-kit 的 DndContext 是"拖拽会话"的边界，
 * 每个区域各建一个的话，跨区域永远拖不过去 —— 表现是"拖到 Dock 上没反应"。
 *
 * 分流的规则（也是唯一容易出错的地方）写在这里，而不是散在各组件：
 *
 *   over.id === DOCK_ID            → 加进 Dock，**不参与排序**
 *   over.id 是应用 id              → 卡片排序
 *   over.id 是分组 id              → 分组排序
 *   一个都没命中                    → 什么都不做（卡片回原位）
 *
 * 判"是什么类型"靠一个显式的 type 标记（`data.current.type`），
 * 而不是靠"id 在不在这个数组里" —— 后者在 id 有重叠时会猜错，
 * 而且每加一种区域就要多改一处判断。
 */

import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  pointerWithin,
  useSensor,
  useSensors,
  type CollisionDetection,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import { sortableKeyboardCoordinates } from '@dnd-kit/sortable'
import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from 'react'

import { DOCK_ID } from '../dock/Dock'

/**
 * Dock 上方再宽限几像素：手指/鼠标停在托盘边缘上方一点点也算命中。
 * 太小会让"明明拖到托盘了却没反应"，太大会误吞排序操作。
 */
const DOCK_SLACK_PX = 12

export type DragKind = 'app' | 'category'

export interface DragInfo {
  id: string
  kind: DragKind
}

/**
 * 落点检测：先按指针位置，命中不了再按最近元素。
 *
 * 纯 pointerWithin 在"指针落在元素缝隙里"时返回空 —— 松手什么都不发生，
 * 用户会以为拖失败了。所以兜一层 closestCenter：至少按最近的算，
 * 更接近用户的预期（他想插在两张卡之间，落到旁边那张也算数）。
 */
const collision: CollisionDetection = (args) => {
  const hits = pointerWithin(args)
  if (hits.length > 0) {
    // Dock 优先：指针同时压住 Dock 和某张卡时，用户显然是想放进 Dock
    const dock = hits.find((h) => h.id === DOCK_ID)
    if (dock) return [dock]
    return hits
  }
  return closestCenter(args)
}

/** 当前拖拽状态。Dock 用它决定要不要把落区显示出来 */
const DragStateContext = createContext<DragInfo | null>(null)

export function useDragState(): DragInfo | null {
  return useContext(DragStateContext)
}

export function HomeDnd({
  enabled,
  onReorderApps,
  onReorderCategories,
  onDropToDock,
  onMoveToCategory,
  children,
}: {
  enabled: boolean
  onReorderApps: (ids: string[]) => void
  onReorderCategories: (ids: string[]) => void
  onDropToDock: (appId: string) => void
  /** 把应用移到某个分组。不传就不支持"拖到分组按钮上" */
  onMoveToCategory?: (appId: string, categoryId: string) => void
  children: ReactNode
}) {
  const [dragging, setDragging] = useState<DragInfo | null>(null)

  /**
   * 松手时的指针位置。
   *
   * 为什么要自己记：判断"拖到底部托盘"最可靠的依据是**指针落在哪**，
   * 而不是 dnd-kit 的 over —— over 依赖 Dock 那个 droppable 的矩形，
   * 而矩形是拖拽开始那一刻测的，Dock 若当时不可见就永远是 0 尺寸，命中不了。
   * 自己记指针位置就没有这个时序问题。
   */
  const pointer = useRef({ x: 0, y: 0 })
  useEffect(() => {
    const onMove = (e: PointerEvent) => {
      pointer.current = { x: e.clientX, y: e.clientY }
    }
    window.addEventListener('pointermove', onMove, { passive: true })
    return () => window.removeEventListener('pointermove', onMove)
  }, [])

  const sensors = useSensors(
    // 移动超过 8px 才算拖 —— 点击不会被误判成拖动
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const onDragEnd = useCallback(
    (e: DragEndEvent) => {
      setDragging(null)
      const { active, over } = e
      if (!over) return

      const kind = (active.data.current?.kind as DragKind | undefined) ?? 'app'
      const activeId = String(active.id)
      const overKind = (over.data.current?.kind as DragKind | undefined) ?? null

      // ① 放进 Dock：只对应用有意义（分组不该进 Dock）
      //
      // 两个判据取其一：
      //   a) dnd-kit 认出了 Dock 落点（over.id === DOCK_ID）
      //   b) 松手时指针在屏幕底部区域（几何兜底）
      // (b) 不能省 —— Dock 那个 droppable 的矩形是拖拽开始那一刻测的，
      // 若当时它不可见（空 Dock 平时是隐形的），矩形就是 0，永远命中不了。
      // 松手时**现量**一次 Dock 的矩形（不是用 dnd-kit 缓存的那份 ——
      // 缓存是拖拽开始那一刻测的，Dock 当时可能是隐形的，矩形为 0）。
      // 用实时矩形判断，就既精确又不会把"底部区域里本来就有的卡片"
      // 误判成要进托盘。
      const p = pointer.current
      const dockEl = document.querySelector<HTMLElement>('[data-dock-drop]')
      const inDock = (() => {
        if (!dockEl) return false
        const r = dockEl.getBoundingClientRect()
        if (r.width === 0 || r.height === 0) return false
        return p.x >= r.left && p.x <= r.right && p.y >= r.top - DOCK_SLACK_PX && p.y <= r.bottom
      })()

      if (kind === 'app' && (over.id === DOCK_ID || inDock)) {
        onDropToDock(activeId)
        return
      }

      // ② **跨类型 = 移动分组**：拖一张卡到某个分组按钮上。
      //    这条必须排在排序之前 —— 否则"卡片落在分组按钮上"会被当成
      //    一次排序，而分组和卡片不在同一个 SortableContext 里，
      //    算出来的 index 是错的，结果就是顺序乱跳。
      if (kind === 'app' && overKind === 'category' && onMoveToCategory) {
        onMoveToCategory(activeId, String(over.id))
        return
      }

      // ③ 同类型之间排序。分组拖到卡片上不做处理（静默回原位）——
      //    那种组合没有明确含义，硬猜一个只会让落点语义不可预测。
      if (active.id === over.id) return

      if (kind === 'app') {
        const from = active.data.current?.index as number | undefined
        const to = over.data.current?.index as number | undefined
        const siblings = active.data.current?.ids as string[] | undefined
        if (siblings && typeof from === 'number' && typeof to === 'number' && from !== to) {
          const next = [...siblings]
          next.splice(to, 0, next.splice(from, 1)[0])
          onReorderApps(next)
        }
        return
      }

      if (kind === 'category') {
        const from = active.data.current?.index as number | undefined
        const to = over.data.current?.index as number | undefined
        const siblings = active.data.current?.ids as string[] | undefined
        if (siblings && typeof from === 'number' && typeof to === 'number' && from !== to) {
          const next = [...siblings]
          next.splice(to, 0, next.splice(from, 1)[0])
          onReorderCategories(next)
        }
      }
    },
    [onReorderApps, onReorderCategories, onDropToDock, onMoveToCategory],
  )

  const onDragStart = useCallback((e: DragStartEvent) => {
    setDragging({
      id: String(e.active.id),
      kind: (e.active.data.current?.kind as DragKind | undefined) ?? 'app',
    })
  }, [])

  if (!enabled) return <>{children}</>

  return (
    <DndContext
      sensors={sensors}
      collisionDetection={collision}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      onDragCancel={() => setDragging(null)}
    >
      <DragStateContext.Provider value={dragging}>{children}</DragStateContext.Provider>
    </DndContext>
  )
}

/** 是否正在拖应用 —— Dock 据此决定要不要露出来 */
export function useIsDraggingApp(d: DragInfo | null): boolean {
  return d?.kind === 'app'
}
