/**
 * 日志弹窗：看某个穿透服务后台的日志。
 */
import { useEffect, useRef } from 'react'
import { X } from 'lucide-react'
import { modalBackdrop } from '../../components/modal'
import type { StunStatusEvent } from '../../types'
import { phaseLabel } from './shared'

export function LogModal({
  status,
  onClose,
}: {
  status: StunStatusEvent | null
  onClose: () => void
}) {
  const bodyRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!bodyRef.current) return
    const el = bodyRef.current
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40
    if (nearBottom) el.scrollTop = el.scrollHeight
  }, [status?.logs?.length])

  if (!status) return null
  const logs = status.logs ?? []
  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="flex w-full max-w-2xl flex-col rounded-2xl bg-surface shadow-2xl ring-1 ring-line">
        <div className="flex items-center justify-between border-b border-line px-6 py-4">
          <div className="text-base font-bold text-content">
            {status.deviceName || ''} / {status.serviceName || ''} - 服务日志
          </div>
          <button
            type="button"
            onClick={onClose}
            className="grid h-7 w-7 place-items-center rounded-full text-muted transition hover:bg-surface-2 hover:text-content"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="px-6 py-3 text-xs text-muted">
          当前阶段：<span className="font-semibold text-content">{phaseLabel[status.phaseStr] || status.phaseStr}</span>
          {' · '}重启次数：<span className="font-semibold text-content">{status.restartCount ?? 0}</span>
          {' · '}共 {logs.length} 条日志
        </div>
        <div ref={bodyRef} className="max-h-[60vh] overflow-y-auto bg-surface-2 px-6 py-3">
          {logs.length === 0 ? (
            <div className="py-8 text-center text-sm text-muted">暂无日志</div>
          ) : (
            logs.map((l, i) => (
              <div
                key={i}
                className="flex items-start gap-3 border-b border-line py-1.5 text-xs last:border-0"
              >
                <span className="font-mono text-[11px] text-muted whitespace-nowrap">
                  {new Date(l.createdAt).toLocaleString()}
                </span>
                <span className="shrink-0 rounded-md bg-blue-50 px-1.5 py-0.5 text-[10px] font-bold text-blue-600">
                  {phaseLabel[l.phaseStr] || l.phaseStr}
                </span>
                <span className="break-all text-content">{l.message}</span>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}

