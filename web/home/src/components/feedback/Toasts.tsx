/**
 * 全局提示。
 *
 * 两条规矩：
 * - **成功的一闪而过**（2.2 秒），失败的不自动消失 —— 失败了要给时间读和重试
 * - 用 role="status" / role="alert"，读屏软件才念得出来
 */

import { Check, RotateCcw, X } from 'lucide-react'
import type { Toast } from '../../state/HomeProvider'

export function Toasts({
  toasts,
  onDismiss,
}: {
  toasts: Toast[]
  onDismiss: (id: number) => void
}) {
  if (toasts.length === 0) return null

  return (
    <div className="pointer-events-none fixed bottom-4 left-1/2 z-50 flex w-[min(92vw,26rem)] -translate-x-1/2 flex-col gap-2">
      {toasts.map((t) => (
        <div
          key={t.id}
          role={t.kind === 'error' ? 'alert' : 'status'}
          className={`pointer-events-auto flex items-start gap-2.5 rounded-xl px-3.5 py-2.5 text-sm shadow-lg backdrop-blur ${
            t.kind === 'error' ? 'bg-rose-950/85 text-white ring-1 ring-rose-300/40' : 'bg-slate-900/85 text-white'
          }`}
        >
          {t.kind === 'error' ? (
            <RotateCcw className="mt-0.5 h-4 w-4 shrink-0 opacity-80" />
          ) : (
            <Check className="mt-0.5 h-4 w-4 shrink-0 text-emerald-300" />
          )}
          <span className="min-w-0 flex-1 leading-relaxed">{t.text}</span>
          {t.retry && (
            <button
              type="button"
              onClick={() => {
                onDismiss(t.id)
                t.retry?.()
              }}
              className="shrink-0 rounded-lg bg-white/15 px-2 py-0.5 text-xs font-semibold transition hover:bg-white/25"
            >
              重试
            </button>
          )}
          <button
            type="button"
            onClick={() => onDismiss(t.id)}
            aria-label="关闭提示"
            className="shrink-0 rounded p-0.5 opacity-60 transition hover:opacity-100"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>
      ))}
    </div>
  )
}
