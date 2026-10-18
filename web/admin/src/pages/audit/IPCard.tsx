import { useState } from 'react'
import { Card, CardHeader } from '../../components/Card'

export function IPCard({
  title,
  hint,
  entries,
  onAdd,
  onRemove,
  tone,
  footer,
}: {
  title: string
  hint: string
  /** 名单条目。黑名单会带上原因/来源/到期，白名单只有 value */
  entries: { value: string; reason?: string; auto?: boolean; until?: string }[]
  /** 返回是否添加成功（决定要不要清空输入框） */
  onAdd: (entry: string) => Promise<boolean>
  onRemove: (entry: string) => Promise<void>
  tone: 'danger' | 'ok'
  /** 底部插槽：黑名单用它放分页控件 */
  footer?: React.ReactNode
}) {
  const [draft, setDraft] = useState('')
  const [busy, setBusy] = useState(false)

  const commit = async () => {
    const line = draft.trim()
    if (!line) return
    setBusy(true)
    const ok = await onAdd(line)
    setBusy(false)
    if (ok) setDraft('')
  }

  return (
    <Card className="!p-0">
      <div className="px-5 pt-4">
        <CardHeader
          title={title}
          action={<span className="text-[11px] text-muted">{entries.length} 条</span>}
        />
      </div>
      <div className="px-5 pb-5">
        <div className="mb-2 text-[11px] leading-relaxed text-muted">{hint}</div>

        <div className="flex gap-2">
          <input
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                e.preventDefault()
                void commit()
              }
            }}
            placeholder="如 10.0.0.0/8 或 203.0.113.5"
            className="min-w-0 flex-1 rounded-lg border border-line bg-surface px-2.5 py-1.5 font-mono text-xs outline-none transition focus:border-blue-400"
          />
          <button
            type="button"
            onClick={() => void commit()}
            disabled={busy || !draft.trim()}
            className={`shrink-0 rounded-lg px-2.5 py-1.5 text-xs font-semibold text-white transition disabled:opacity-40 ${
              tone === 'danger' ? 'bg-rose-500 hover:bg-rose-600' : 'bg-emerald-500 hover:bg-emerald-600'
            }`}
          >
            添加
          </button>
        </div>

        {footer}

        {entries.length > 0 && (
          <div className="mt-2 space-y-1">
            {entries.map((e) => (
              <div
                key={e.value}
                className="flex items-start gap-2 rounded-lg bg-surface-2 px-2 py-1.5 ring-1 ring-line/70"
              >
                <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
                    {/* IPv6 有 39 个字符，不折行会把「移除」顶出屏幕 */}
                    <span className="min-w-0 break-all font-mono text-[11px] text-content">
                      {e.value}
                    </span>
                    {e.auto && (
                      <span className="shrink-0 rounded bg-amber-50 px-1.5 py-0.5 text-[10px] font-semibold text-amber-600">
                        自动
                      </span>
                    )}
                    {e.until && <span className="shrink-0 text-[11px] text-muted">{e.until}</span>}
                  </div>
                  {/* 封禁原因：合并掉「封禁列表」之后，这里必须看得见它是为什么被封的 */}
                  {e.reason && (
                    <span className="text-[11px] leading-relaxed text-muted">{e.reason}</span>
                  )}
                </div>
                <button
                  type="button"
                  onClick={() => void onRemove(e.value)}
                  className="shrink-0 text-[11px] font-semibold text-muted transition hover:text-rose-500"
                >
                  移除
                </button>
              </div>
            ))}
          </div>
        )}

      </div>
    </Card>
  )
}

