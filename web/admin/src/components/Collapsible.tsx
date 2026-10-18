/**
 * 可折叠的区块。
 *
 * 用途：把「平时不看、出事才看」的内容收起来，让一屏能装下真正要看的东西。
 *
 * # 两个刻意的设计
 *
 * 1. **收起时不渲染子节点**（不是用 CSS 隐藏）。
 *    这一点是关键：区块里的内容常常自己发请求（登录记录、设置项、
 *    日志列表……）。用 CSS 隐藏的话请求照发不误，页面还是一样慢；
 *    不渲染就真的不请求。展开时才挂载 = 展开时才加载。
 *
 * 2. **记住展开状态**（按 storageKey 写进 localStorage）。
 *    用户一旦展开「登录尝试」，下次进这个页面应该还是展开的 ——
 *    否则每次都要重复一次「找 → 点」。
 */
import { useEffect, useState, type ReactNode } from 'react'
import { ChevronDown } from 'lucide-react'

export function Collapsible({
  title,
  /** 收起时右侧的一句话摘要，比如「今天 12 次，3 次失败」 */
  hint,
  /** 右侧固定操作（展开收起的箭头会加在它后面） */
  action,
  /** 记住展开状态用的键。留空则不记忆，默认收起 */
  storageKey,
  /** 默认是否展开（没有存过状态时生效） */
  defaultOpen = false,
  /** 收起时标题左侧的警示色小圆点（有事要提醒时才用） */
  alert = false,
  className = '',
  children,
}: {
  title: ReactNode
  hint?: ReactNode
  action?: ReactNode
  storageKey?: string
  defaultOpen?: boolean
  alert?: boolean
  className?: string
  children: ReactNode
}) {
  const [open, setOpen] = useState(() => {
    if (!storageKey) return defaultOpen
    try {
      const v = window.localStorage.getItem(storageKey)
      return v === null ? defaultOpen : v === '1'
    } catch {
      // 隐私模式下 localStorage 会抛，别让这件事把整个页面带崩
      return defaultOpen
    }
  })

  useEffect(() => {
    if (!storageKey) return
    try {
      window.localStorage.setItem(storageKey, open ? '1' : '0')
    } catch {
      /* 同上：存不进去也不影响使用 */
    }
  }, [open, storageKey])

  return (
    <div className={`rounded-2xl border border-line bg-surface shadow-sm shadow-black/5 ${className}`}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
        className="flex w-full items-center gap-2 rounded-2xl px-5 py-3 text-left transition hover:bg-surface-2/60"
      >
        {alert && <span className="h-2 w-2 shrink-0 rounded-full bg-rose-500" />}
        <span className="min-w-0 truncate text-sm font-bold text-content">{title}</span>
        {hint && !open && (
          <span className="hidden min-w-0 flex-1 truncate text-[11px] text-muted sm:block">{hint}</span>
        )}
        <span className="ml-auto flex shrink-0 items-center gap-2">
          {action}
          <ChevronDown
            className={`h-4 w-4 text-muted transition-transform ${open ? 'rotate-180' : ''}`}
          />
        </span>
      </button>

      {/* 收起时**整块不渲染** —— 子组件里那些 useEffect 发的请求也就不会发 */}
      {open && <div className="px-5 pb-5">{children}</div>}
    </div>
  )
}
