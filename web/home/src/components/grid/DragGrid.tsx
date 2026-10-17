/**
 * 拖拽的构件。
 *
 * **这里只有 SortableContext，没有 DndContext。**
 *
 * 全页只有一个 DndContext（见 components/dnd/HomeDnd.tsx）—— 因为卡片网格、
 * 左侧分组栏、底部 Dock 之间要能互拖。每个区域各建一个 DndContext 的话，
 * 跨区域永远拖不过去，表现就是"拖到 Dock 上没反应"。
 *
 * 每个可拖元素通过 `data` 带上三样东西，HomeDnd 松手时靠它们算新顺序：
 *   kind   是应用还是分组（决定谁处理）
 *   index  它在自己那一组里的位置
 *   ids    它那一组的完整顺序
 *
 * 把 index/ids 放在元素上而不是让 HomeDnd 去查，是因为"哪一组"这个概念
 * 只有渲染方最清楚（卡片只知道自己在当前分组里）。
 */

import { SortableContext, rectSortingStrategy, useSortable, verticalListSortingStrategy } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import type { ReactNode } from 'react'

/**
 * 一张可拖的卡片。
 *
 * listeners 铺在整个 div 上 —— 用户习惯是按住卡片直接挪，不需要找把手。
 * 点击不会被误判成拖动，因为传感器设了 8px 的移动门槛。
 */
export function SortableCard({
  id,
  index,
  ids,
  children,
}: {
  id: string
  index: number
  ids: string[]
  children: ReactNode
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
    data: { kind: 'app', index, ids },
  })

  return (
    <div
      ref={setNodeRef}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.35 : 1,
        // 拖动中的卡片要浮在别的上面，不然会被后面那格的背景盖住
        zIndex: isDragging ? 20 : undefined,
        // 手机上按住卡片拖动，不能变成滚页面
        touchAction: 'none',
      }}
      className="relative cursor-grab active:cursor-grabbing"
      {...attributes}
      {...listeners}
    >
      {children}
    </div>
  )
}

/** 竖排列表里的一项（左侧分组栏用） */
export function SortableListItem({
  id,
  index,
  ids,
  disabled,
  children,
}: {
  id: string
  index: number
  ids: string[]
  disabled?: boolean
  children: ReactNode
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({
    id,
    disabled,
    data: { kind: 'category', index, ids },
  })

  return (
    <div
      ref={setNodeRef}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.4 : 1,
        zIndex: isDragging ? 30 : undefined,
        touchAction: 'none',
      }}
      {...attributes}
      {...listeners}
    >
      {children}
    </div>
  )
}

/** 卡片网格的排序上下文（横竖混排，用 rect 策略） */
export function DragGrid({ ids, children }: { ids: string[]; children: ReactNode }) {
  return (
    <SortableContext items={ids} strategy={rectSortingStrategy}>
      {children}
    </SortableContext>
  )
}

/** 竖排列表的排序上下文 */
export function DragList({ ids, children }: { ids: string[]; children: ReactNode }) {
  return (
    <SortableContext items={ids} strategy={verticalListSortingStrategy}>
      {children}
    </SortableContext>
  )
}
