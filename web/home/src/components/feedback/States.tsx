/**
 * 反馈组件。
 *
 * 这一组东西存在的理由只有一条：**任何状态都要让人知道发生了什么**。
 * 转圈不知道在等什么、空白不知道是没数据还是坏了、失败不知道该重试还是放弃 ——
 * 这三种是最伤人的。
 */

/** 首屏骨架。用占位块而不是转圈：转圈看不出页面长什么样，骨架能 */
export function GridSkeleton({ count = 12 }: { count?: number }) {
  return (
    <div className="grid grid-cols-4 gap-3 sm:grid-cols-6 lg:grid-cols-8" aria-hidden>
      {Array.from({ length: count }, (_, i) => (
        <div key={i} className="flex flex-col items-center gap-2">
          <div className="aspect-square w-full animate-pulse rounded-[22%] bg-white/25" />
          <div className="h-2.5 w-2/3 animate-pulse rounded bg-white/20" />
        </div>
      ))}
      <span className="sr-only">正在加载导航页</span>
    </div>
  )
}

export function EmptyState({
  title,
  hint,
  action,
}: {
  title: string
  hint?: string
  action?: { label: string; onClick: () => void }
}) {
  return (
    <div className="flex flex-col items-center gap-3 rounded-2xl border border-dashed border-white/30 px-6 py-12 text-center">
      <p className="text-base font-semibold text-white">{title}</p>
      {hint && <p className="max-w-sm text-sm leading-relaxed text-white/70">{hint}</p>}
      {action && (
        <button
          type="button"
          onClick={action.onClick}
          className="mt-1 rounded-xl bg-white/90 px-4 py-2 text-sm font-semibold text-slate-800 transition hover:bg-white"
        >
          {action.label}
        </button>
      )}
    </div>
  )
}

export function ErrorState({
  message,
  onRetry,
  retrying = false,
}: {
  message: string
  onRetry?: () => void
  retrying?: boolean
}) {
  return (
    <div
      role="alert"
      className="flex flex-col items-center gap-3 rounded-2xl bg-rose-950/40 px-6 py-10 text-center ring-1 ring-rose-300/40"
    >
      <p className="text-base font-semibold text-white">没加载起来</p>
      <p className="max-w-md text-sm leading-relaxed text-white/80">{message}</p>
      {onRetry && (
        <button
          type="button"
          onClick={onRetry}
          disabled={retrying}
          className="mt-1 rounded-xl bg-white/90 px-4 py-2 text-sm font-semibold text-slate-800 transition hover:bg-white disabled:opacity-60"
        >
          {retrying ? '重试中…' : '重试'}
        </button>
      )}
    </div>
  )
}
