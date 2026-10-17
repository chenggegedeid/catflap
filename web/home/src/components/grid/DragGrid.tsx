/**
 * 拖拽排序。
 *
 * 用 dnd-kit 的 sortable。要点不在"能拖动"，在几个容易做错的地方：
 *
 * 1. **只让登录的人拖。** 访客拖了也没地方存 —— 拖完请求 401，
 *    卡片弹回原位，看着像"卡了"。
 * 2. **键盘可拖**（shift + 方向键 / space 提起），这是 dnd-kit 自带的，
 *    但要记得给 draggable 的把手加 `touch-none`，否则手机上是滚动页面而不是拖。
 * 3. **落点先算好，松手才发请求**。dnd-kit 的 onDragEnd 给出的是"从哪到哪"，
 *    我们把新的 id 顺序一次性 PUT 上去 —— 不用逐格算坐标，
 *    因为后端存的就是顺序。
 * 4. **拖拽过程中不要触发点击**。dnd-kit 默认会区分点击和拖动，
 *    但如果我们自己再加 onClick，得用 `isDragging` 挡一下。
 */

import {
  DndContext,
  KeyboardSensor,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import { SortableContext, arrayMove, rectSortingStrategy, sortableKeyboardCoordinates, useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import type { ReactNode } from 'react'

/**
 * 把一个卡片包成可拖的。
 *
 * attributes / listeners 直接铺在这个 div 上，不往 children 里传 ——
 * 传出去反而要 children 自己记得"必须挂到哪个元素上"，容易挂错。
 * 现在整张卡片都可拖（符合这类页面的直觉：按住卡片挪）。
 */
export function SortableCard({
  id,
  children,
}: {
  id: string
  children: ReactNode
}) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id })

  return (
    <div
      ref={setNodeRef}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.35 : 1,
        // 拖动中的卡片要浮在别的上面，不然会被后面那格的背景盖住
        zIndex: isDragging ? 20 : undefined,
      }}
      // 整张卡可拖（用户习惯了直接拖卡片），但激活门槛是移动 8px ——
      // 点击不会触发拖拽。把手只是给"不确定时"一个明确的抓取点。
      className="group/card relative cursor-grab active:cursor-grabbing"
      {...attributes}
      {...listeners}
    >
      {children}
    </div>
  )
}

export function DragGrid({
  ids,
  onReorder,
  children,
}: {
  ids: string[]
  onReorder: (ids: string[]) => void
  children: ReactNode
}) {
  const sensors = useSensors(
    // 手指/鼠标：移动超过 6px 才算拖 —— 不然轻轻一点就被当成拖动，
    // 点开链接会时灵时不灵
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const onDragEnd = (e: DragEndEvent) => {
    const { active, over } = e
    if (!over || active.id === over.id) return
    const from = ids.indexOf(String(active.id))
    const to = ids.indexOf(String(over.id))
    if (from < 0 || to < 0) return
    onReorder(arrayMove(ids, from, to))
  }

  return (
    <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={onDragEnd}>
      <SortableContext items={ids} strategy={rectSortingStrategy}>
        {children}
      </SortableContext>
    </DndContext>
  )
}
