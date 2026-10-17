/**
 * 左侧分组栏。
 *
 * 没有「全部」：首页是每天要点的东西，不是所有东西的清单。
 * 第一组就是最常用的那组 —— 用户已经用顺序表达过优先级了，
 * 再在最前面塞一个全量列表，等于每次都先给他看一遍不用的东西。
 *
 * 纯图标 + 悬停出名字，宽 44px，把宽度让给内容。
 * 键盘 ↑↓ 可切换（整栏是一个 listbox）。
 */

import { useRef, type ReactNode } from 'react'
import { Lock, Plus } from 'lucide-react'
import type { Category, Visibility } from '../../types'
import { DragList, SortableListItem } from '../grid/DragGrid'
import { AppIcon } from '../card/AppIcon'
import { useDroppable } from '@dnd-kit/core'

/**
 * 分组按钮作为"放入目标"。
 *
 * 拖一张卡到某个分组按钮上 = 把应用移到那个分组。
 *
 * 和排序落点的区分：over.id 会是分组 id，而 kind 是 'app' ——
 * HomeDnd 里就是靠这两条判断"跨类型 = 移动分组"而不是"排序"。
 * 卡片拖到自己所在的分组按钮上 → 不做任何事（id 相同会先被拦）。
 */
export function CategoryDropTarget({ id, children }: { id: string; children: ReactNode }) {
  const { setNodeRef, isOver } = useDroppable({ id, data: { kind: 'category', drop: true } })
  return (
    <div
      ref={setNodeRef}
      className={`rounded-xl transition ${isOver ? 'ring-2 ring-emerald-300' : ''}`}
    >
      {children}
    </div>
  )
}

function visibilityOf(c: Category): Visibility {
  if (c.visibility) return c.visibility
  return c.requireAuth ? 'login' : 'public'
}

export interface RailEntry {
  id: string
  name: string
  vis: Visibility
  /** lock 分组在本次会话里是否已解锁（后端按解锁 token 算好下发） */
  unlocked?: boolean
  icon?: string
}

export function CategoryRail({
  categories,
  active,
  onSelect,
  onAdd,
  canManage,
  hiddenCount,
  onReorder,
}: {
  categories: Category[]
  active: string
  onSelect: (id: string) => void
  onAdd?: () => void
  canManage: boolean
  /** 有值才可拖动排序（只给登录的人） */
  onReorder?: (ids: string[]) => void
  /** 被后端藏起来的分组数量：只有数量，没有名单 */
  hiddenCount?: number
}) {
  const railRef = useRef<HTMLDivElement>(null)

  const catIdList = categories.map((c) => c.id)
  const entries: RailEntry[] = categories.map((c) => ({
    id: c.id,
    name: c.name,
    vis: visibilityOf(c),
    unlocked: c.unlocked,
    icon: c.icon,
  }))

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
    e.preventDefault()
    const i = entries.findIndex((x) => x.id === active)
    const next = e.key === 'ArrowDown' ? Math.min(entries.length - 1, i + 1) : Math.max(0, i - 1)
    if (next < 0) return
    onSelect(entries[next].id)
    const btns = railRef.current?.querySelectorAll<HTMLButtonElement>('button[data-cat]')
    btns?.[next]?.focus()
  }

  const rail = (
    <div
      ref={railRef}
      role="listbox"
      aria-label="分组"
      aria-orientation="vertical"
      onKeyDown={onKeyDown}
      className="flex shrink-0 flex-row gap-2 overflow-x-auto pb-1 sm:flex-col sm:overflow-visible sm:pb-0"
    >
      {entries.map((e, idx) => {
        const selected = e.id === active
        /**
         * "显示锁"= 需要口令**且还没解锁**。
         *
         * 之前写成 `vis === 'login' || vis === 'lock'`，于是已经解锁的分组
         * 图标还是一把锁 —— 配上后端没下发 unlocked，就一直显示锁。
         */
        const locked = e.vis === 'lock' && !e.unlocked
        const btn = (
          <button
            key={e.id}
            data-cat={e.id}
            type="button"
            role="option"
            aria-selected={selected}
            title={e.name}
            onClick={() => onSelect(e.id)}
            className={`group relative flex h-11 w-11 shrink-0 touch-none items-center justify-center rounded-xl text-[11px] font-semibold leading-tight transition ${
              selected ? 'bg-white/95 text-slate-800 shadow-md' : 'bg-white/20 text-white/85 hover:bg-white/35'
            }`}
          >
            {/**
             * 有图标就显示图标，没有才退回"名字前两字"。
             * 之前这里不管有没有图标都只画文字，分组一多整列就是一排字，
             * 看着像"图标不见了"。
             */}
            {locked && !selected ? (
              <Lock className="h-4 w-4" />
            ) : e.icon ? (
              <span className="h-7 w-7 overflow-hidden rounded-[22%]">
                <AppIcon name={e.name} icon={e.icon} />
              </span>
            ) : (
              <span className="px-1 text-center">{firstChars(e.name)}</span>
            )}

            {/* 悬停/聚焦才出现名字（键盘用户靠 focus 也能看到） */}
            <span className="pointer-events-none absolute left-full z-20 ml-2 hidden whitespace-nowrap rounded-lg bg-slate-900/90 px-2 py-1 text-xs font-medium text-white group-hover:block group-focus-visible:block">
              {e.name}
              {locked && <span className="ml-1 text-white/60">（需解锁）</span>}
            </span>
          </button>
        )
        // 只在可管理时包成"可拖 + 可放入"：访客拖着没地方存，徒增困惑
        if (!onReorder) return <div key={e.id}>{btn}</div>
        return (
          <SortableListItem key={e.id} id={e.id} index={idx} ids={catIdList}>
            <CategoryDropTarget id={e.id}>{btn}</CategoryDropTarget>
          </SortableListItem>
        )
      })}

      {canManage && onAdd && (
        <button
          type="button"
          onClick={onAdd}
          title="管理分组"
          aria-label="管理分组"
          className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-dashed border-white/35 text-white/70 transition hover:bg-white/20 hover:text-white"
        >
          <Plus className="h-4 w-4" />
        </button>
      )}

      {/* 只在列表里没有 lock 分组时才画（否则和上面的分组重复） */}
      {hiddenCount && hiddenCount > 0 && !categories.some((c) => visibilityOf(c) === 'lock') ? (
        <div
          title="这些分组需要登录或口令才能查看"
          className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-dashed border-white/25 text-white/50"
        >
          <Lock className="h-3.5 w-3.5" />
        </div>
      ) : null}
    </div>
  )

  if (!onReorder) return rail
  return <DragList ids={catIdList}>{rail}</DragList>
}

/** 名字取前两个字做图标占位（图标是可选的，很多分组没图标） */
function firstChars(name: string): string {
  const t = (name || '').trim()
  if (!t) return '?'
  return t.slice(0, 2)
}
