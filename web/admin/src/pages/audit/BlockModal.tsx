import { useState } from 'react'
import { durations } from './shared'

export function BlockModal({
  ips,
  whitelist,
  onCancel,
  onSubmit,
}: {
  ips: string[]
  whitelist: string[]
  onCancel: () => void
  onSubmit: (ips: string[], reason: string, minutes: number) => void
}) {
  const [reason, setReason] = useState(ips.length > 1 ? '批量拉黑：密集异常访问' : '')
  const [minutes, setMinutes] = useState(60)
  const [busy, setBusy] = useState(false)

  // 白名单里的地址封不了 —— 提前告诉用户，别等他点了才报错
  const inWhitelist = ips.filter((ip) => whitelist.includes(ip))

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 dark:bg-black/70">
      <div className="w-full max-w-md rounded-2xl bg-surface p-5 shadow-2xl">
        <div className="mb-3 text-base font-bold text-content">
          封禁 {ips.length === 1 ? ips[0] : `${ips.length} 个地址`}
        </div>

        {ips.length > 1 && (
          <div className="mb-3 max-h-24 overflow-y-auto rounded-xl bg-surface-2 p-2 font-mono text-[11px] text-content">
            {ips.join('、')}
          </div>
        )}

        {inWhitelist.length > 0 && (
          <div className="mb-3 rounded-xl bg-amber-50 px-3 py-2 text-[11px] text-amber-700">
            有 {inWhitelist.length} 个在白名单里，它们不会被封禁。
          </div>
        )}

        <label className="mb-3 block">
          <div className="mb-1 text-xs font-semibold text-muted">原因（写清楚，以后自己看得懂）</div>
          <input
            autoFocus
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="如：大量 404 扫描"
            className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
          />
        </label>

        <div className="mb-4">
          <div className="mb-1 text-xs font-semibold text-muted">封禁时长</div>
          <div className="flex flex-wrap gap-2">
            {durations.map((d) => (
              <button
                key={d.label}
                type="button"
                onClick={() => setMinutes(d.minutes)}
                className={`rounded-lg px-3 py-1.5 text-xs font-semibold transition ${
                  minutes === d.minutes ? 'bg-blue-500 text-white' : 'bg-surface-2 text-content hover:bg-fill'
                }`}
              >
                {d.label}
              </button>
            ))}
          </div>
          {minutes === 0 && (
            <div className="mt-1.5 text-[11px] text-rose-500">
              永久封禁只能手动解除。不确定对方是不是误伤时，建议先用临时封禁。
            </div>
          )}
        </div>

        <div className="flex justify-end gap-2">
          <button onClick={onCancel} className="rounded-xl px-4 py-2 text-sm text-muted hover:bg-surface-2">
            取消
          </button>
          <button
            onClick={() => {
              setBusy(true)
              onSubmit(ips, reason, minutes)
            }}
            disabled={busy}
            className="rounded-xl bg-rose-500 px-4 py-2 text-sm font-semibold text-white hover:bg-rose-600 disabled:opacity-50"
          >
            {busy ? '处理中…' : '确认封禁'}
          </button>
        </div>
      </div>
    </div>
  )
}

