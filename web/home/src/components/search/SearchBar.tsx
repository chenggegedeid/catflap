/**
 * 搜索栏 + 智能直达。
 *
 * 交互设计：
 * - 输入即出结果，**↑↓ 选择、Enter 直达、Esc 关闭** —— 手不离键盘
 * - 无结果时兜底交给默认引擎（敲了字总是想去个地方，别让人对着空列表发呆）
 * - 结果为空且没输入时不渲染浮层，别让一个空盒子杵在那
 *
 * 这个搜索框和我们相对同类产品的差异点：**应用是后端 hydrate 出来的真数据**，
 * 名字、域名、端口全能搜 —— 不需要用户先存一遍书签。
 */

import { useEffect, useMemo, useRef, useState } from 'react'
import { Search } from 'lucide-react'
import { searchAll, type SearchHit } from '../../lib/search'
import { openEntry } from '../../lib/addresses'
import { addSearchHistory } from '../../api/home'
import type { HomeConfig } from '../../types'
import { AppIcon } from '../card/AppIcon'

export function SearchBar({ config, onLoggedAction }: { config: HomeConfig; onLoggedAction?: () => void }) {
  const [q, setQ] = useState('')
  const [active, setActive] = useState(0)
  const [focused, setFocused] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  const hits = useMemo<SearchHit[]>(
    () => searchAll(q, config.apps, config.searchEngines, config.searchEngines.find((e) => e.id === config.defaultSearchEngineId)),
    [q, config],
  )
  const open = focused && q.trim() !== ''

  // 选择位置跟着结果数量收着点，免得 ↑ 走到不存在的项上
  useEffect(() => setActive(0), [q])

  const go = (hit: SearchHit) => {
    openEntry(hit.url)
    // 应用命中才记搜索历史（引擎跳转记下来没有意义）
    if (hit.kind === 'app') void addSearchHistory(hit.label).catch(() => {})
    setQ('')
    setFocused(false)
    inputRef.current?.blur()
    onLoggedAction?.()
  }

  const onKeyDown = (e: React.KeyboardEvent) => {
    if (!open) return
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActive((i) => Math.min(hits.length - 1, i + 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActive((i) => Math.max(0, i - 1))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const hit = hits[active]
      if (hit) go(hit)
    } else if (e.key === 'Escape') {
      setQ('')
      inputRef.current?.blur()
    }
  }

  const engine = config.searchEngines.find((e) => e.id === config.defaultSearchEngineId)

  return (
    <div className="relative w-full max-w-xl">
      <div className="flex items-center gap-2 rounded-full bg-white/85 px-4 py-2.5 shadow-lg ring-1 ring-black/5 backdrop-blur transition focus-within:ring-2 focus-within:ring-blue-400">
        {engine?.icon ? (
          <img src={engine.icon} alt="" className="h-4 w-4 rounded" />
        ) : (
          <span className="grid h-4 w-4 place-items-center rounded bg-slate-700 text-[10px] font-bold text-white">
            {engine?.shortName ?? 'S'}
          </span>
        )}
        <input
          ref={inputRef}
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onFocus={() => setFocused(true)}
          onBlur={() => setTimeout(() => setFocused(false), 150)}
          onKeyDown={onKeyDown}
          placeholder="搜索应用，或输入网址"
          aria-label="搜索应用或输入网址"
          className="min-w-0 flex-1 bg-transparent text-sm text-slate-800 outline-none placeholder:text-slate-400"
        />
        <Search className="h-4 w-4 shrink-0 text-slate-400" />
      </div>

      {open && hits.length > 0 && (
        <ul
          role="listbox"
          aria-label="搜索结果"
          className="absolute inset-x-0 top-full z-40 mt-2 overflow-hidden rounded-xl bg-slate-900/92 py-1 text-sm text-white shadow-xl ring-1 ring-white/10 backdrop-blur"
        >
          {hits.map((hit, i) => (
            <li key={`${hit.kind}-${hit.refId}`} role="option" aria-selected={i === active}>
              <button
                type="button"
                // onMouseDown 抢在输入框 blur 之前处理，否则点下去浮层先没了
                onMouseDown={(e) => {
                  e.preventDefault()
                  go(hit)
                }}
                onMouseEnter={() => setActive(i)}
                className={`flex w-full items-center gap-2.5 px-3 py-2 text-left transition ${
                  i === active ? 'bg-white/15' : 'hover:bg-white/10'
                }`}
              >
                {hit.kind === 'app' ? (
                  <span className="h-6 w-6 shrink-0 overflow-hidden rounded-lg">
                    <AppIcon name={hit.label} icon={hit.icon ?? ''} />
                  </span>
                ) : hit.shortName ? (
                  <span
                    className="grid h-6 w-6 shrink-0 place-items-center rounded-lg text-[10px] font-bold text-white"
                    style={{ background: hit.color || '#334155' }}
                  >
                    {hit.shortName}
                  </span>
                ) : (
                  <Search className="h-4 w-4 shrink-0 opacity-70" />
                )}
                <span className="min-w-0 flex-1 truncate">{hit.label}</span>
                {hit.sub && <span className="max-w-[40%] shrink-0 truncate text-[11px] text-white/50">{hit.sub}</span>}
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
