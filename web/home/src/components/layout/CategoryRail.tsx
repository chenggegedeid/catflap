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

import { useRef } from 'react'
import { Lock, Plus } from 'lucide-react'
import type { Category, Visibility } from '../../types'

function visibilityOf(c: Category): Visibility {
  if (c.visibility) return c.visibility
  return c.requireAuth ? 'login' : 'public'
}

export interface RailEntry {
  id: string
  name: string
  vis: Visibility
}

export function CategoryRail({
  categories,
  active,
  onSelect,
  onAdd,
  canManage,
  hiddenCount,
}: {
  categories: Category[]
  active: string
  onSelect: (id: string) => void
  onAdd?: () => void
  canManage: boolean
  /** 被后端藏起来的分组数量：只有数量，没有名单 */
  hiddenCount?: number
}) {
  const railRef = useRef<HTMLDivElement>(null)

  const entries: RailEntry[] = categories.map((c) => ({
    id: c.id,
    name: c.name,
    vis: visibilityOf(c),
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

  return (
    <div
      ref={railRef}
      role="listbox"
      aria-label="分组"
      aria-orientation="vertical"
      onKeyDown={onKeyDown}
      className="flex shrink-0 flex-row gap-2 overflow-x-auto pb-1 sm:flex-col sm:overflow-visible sm:pb-0"
    >
      {entries.map((e) => {
        const selected = e.id === active
        const locked = e.vis === 'login' || e.vis === 'lock'
        return (
          <button
            key={e.id}
            data-cat={e.id}
            type="button"
            role="option"
            aria-selected={selected}
            title={e.name}
            onClick={() => onSelect(e.id)}
            className={`group relative flex h-11 w-11 shrink-0 items-center justify-center rounded-xl text-[11px] font-semibold leading-tight transition ${
              selected ? 'bg-white/95 text-slate-800 shadow-md' : 'bg-white/20 text-white/85 hover:bg-white/35'
            }`}
          >
            {locked && !selected ? (
              <Lock className="h-4 w-4" />
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
}

/** 名字取前两个字做图标占位（图标是可选的，很多分组没图标） */
function firstChars(name: string): string {
  const t = (name || '').trim()
  if (!t) return '?'
  return t.slice(0, 2)
}
