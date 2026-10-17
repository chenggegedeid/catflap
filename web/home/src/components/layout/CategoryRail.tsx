/**
 * 左侧分类栏。
 *
 * 纯图标 + 悬停（或键盘聚焦）出名字，宽约 56px —— 这样内容区最宽。
 * 名字用 title 属性 + 一个自绘的气泡：title 是浏览器原生的（可靠、读屏也念），
 * 气泡只是为了好看和即时。
 *
 * 键盘可用性是硬要求：整栏是一个 listbox，↑↓ 切换、Enter 选中。
 * 只看得见鼠标的话，键盘用户就只能用 Tab 一个个挪过去。
 */

import { useRef } from 'react'
import { LayoutGrid, Lock, Plus } from 'lucide-react'
import type { Category, Visibility } from '../../types'

function visibilityOf(c: Category): Visibility {
  if (c.visibility) return c.visibility
  return c.requireAuth ? 'login' : 'public'
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

  const entries = [{ id: '', name: '全部', vis: 'public' as Visibility }, ...categories.map((c) => ({ id: c.id, name: c.name, vis: visibilityOf(c) }))]

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
    e.preventDefault()
    const i = entries.findIndex((x) => x.id === active)
    const next = e.key === 'ArrowDown' ? Math.min(entries.length - 1, i + 1) : Math.max(0, i - 1)
    onSelect(entries[next].id)
    // 焦点跟着走：否则键盘选了，视觉焦点还留在原来那颗按钮上
    const btns = railRef.current?.querySelectorAll<HTMLButtonElement>('button[data-cat]')
    btns?.[next]?.focus()
  }

  return (
    <div
      ref={railRef}
      role="listbox"
      aria-label="分类"
      aria-orientation="vertical"
      onKeyDown={onKeyDown}
      className="flex shrink-0 flex-row gap-2 overflow-x-auto sm:flex-col sm:overflow-visible"
    >
      {entries.map((e) => {
        const selected = e.id === active
        const locked = e.vis === 'login' || e.vis === 'lock'
        return (
          <button
            key={e.id || '__all__'}
            data-cat={e.id}
            type="button"
            role="option"
            aria-selected={selected}
            title={e.name}
            onClick={() => onSelect(e.id)}
            className={`group relative flex h-11 w-11 shrink-0 items-center justify-center rounded-xl text-sm font-semibold transition ${
              selected
                ? 'bg-white/95 text-slate-800 shadow'
                : 'bg-white/25 text-white/90 hover:bg-white/40'
            }`}
          >
            {e.id === '' ? (
              <LayoutGrid className="h-4.5 w-4.5" />
            ) : locked ? (
              <Lock className="h-4 w-4" />
            ) : (
              <span className="px-1 text-center text-[11px] leading-tight">{firstChars(e.name)}</span>
            )}

            {/* 悬停/聚焦才出现的名字（键盘用户靠 focus 也能看到） */}
            <span className="pointer-events-none absolute left-full z-20 ml-2 hidden whitespace-nowrap rounded-lg bg-slate-900/90 px-2 py-1 text-xs text-white group-hover:block group-focus-visible:block">
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
          title="新增分类"
          aria-label="新增分类"
          className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-dashed border-white/40 text-white/80 transition hover:bg-white/20 hover:text-white"
        >
          <Plus className="h-4 w-4" />
        </button>
      )}

      {/*
        隐藏计数占位。

        **登录后不要再画**：lock 分组本身已经在上面那一列里出现了（带锁图标），
        后端同时回一个 hiddenCategoryCount 表示"还有 N 个锁着的分组" ——
        那是同一件事的两种说法。两个都画出来，左栏就会竖两根锁。

        所以这条只在**列表里没有 lock 分组**时才画，代表"访客被挡在外面的那些"。
      */}
      {hiddenCount && hiddenCount > 0 && !categories.some((c) => visibilityOf(c) === 'lock') ? (
        <div
          title="这些分组需要登录或口令才能查看"
          className="flex h-11 w-11 shrink-0 items-center justify-center rounded-xl border border-dashed border-white/30 text-white/60"
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
