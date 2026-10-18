import { useState } from 'react'
import { X } from 'lucide-react'
import { modalBackdrop } from '../../components/modal'
import { intervalOptions } from './shared'

export function SettingsModal({
  intervalSec,
  onCancel,
  onSubmit,
}: {
  intervalSec: number
  onCancel: () => void
  onSubmit: (intervalSec: number) => Promise<void>
}) {
  const [sec, setSec] = useState(intervalSec)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const submit = async () => {
    setErr('')
    setBusy(true)
    try {
      await onSubmit(sec)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '操作失败')
      setBusy(false)
    }
  }

  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="max-h-full w-full max-w-md overflow-y-auto rounded-2xl bg-surface p-6 text-content shadow-2xl ring-1 ring-line">
        <div className="mb-5 flex items-center justify-between">
          <div className="text-base font-bold text-content">DDNS 设置</div>
          <button
            type="button"
            onClick={onCancel}
            className="grid h-7 w-7 place-items-center rounded-full text-muted transition hover:bg-surface-2 hover:text-content"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <label className="block">
          <div className="mb-1 text-xs font-semibold text-muted">全局同步间隔</div>
          <select
            value={sec}
            onChange={(e) => setSec(Number(e.target.value))}
            className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
          >
            {intervalOptions.map((o) => (
              <option key={o.value} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
          <div className="mt-1.5 text-[11px] text-muted">每条记录按此间隔检查 IP 是否变化并自动同步</div>
        </label>
        {err && <div className="mt-3 text-xs text-rose-500">{err}</div>}
        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            className="rounded-xl bg-surface-2 px-4 py-2 text-sm font-semibold text-content transition hover:bg-fill"
          >
            取消
          </button>
          <button
            type="button"
            onClick={submit}
            disabled={busy}
            className="rounded-xl bg-blue-500 px-4 py-2 text-sm font-semibold text-white shadow-md shadow-blue-500/20 transition hover:bg-blue-600 disabled:opacity-50"
          >
            {busy ? '保存中...' : '保存'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ===================== 主页面 =====================

