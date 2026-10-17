/**
 * AccessLogModal 一个服务的访问日志 —— **反代站点和内网穿透共用同一个弹窗**。
 *
 * 数据全部来自访问审计（audit）：
 *   - 反代站点：按 siteId 取
 *   - 穿透服务：按 ref 取（"s<设备ID>-<服务ID>"，和流量统计同一个键）
 *
 * 为什么要统一：穿透以前根本没有访问记录（出了事不知道谁来过），后来在洞口
 * Accept 处补了记录，但如果展示各写一套，两边又会不一样。现在同一个弹窗、
 * 同一张表：时间 / 来源 IP / 请求 / 状态 / 耗时 —— 看哪个服务都是同一副样子。
 */
import { useEffect, useState } from 'react'
import { X } from 'lucide-react'
import * as api from '../lib/api'
import { modalBackdrop } from './modal'

const iconBtn = 'grid h-6 w-6 shrink-0 place-items-center rounded-md transition'

export function AccessLogModal({
  title,
  siteId,
  ref,
  onClose,
}: {
  /** 弹窗标题里显示的服务名 */
  title: string
  /** 反代站点 ID（二选一） */
  siteId?: number
  /** 穿透服务引用键（二选一），形如 "s<设备ID>-<服务ID>" */
  ref?: string
  onClose: () => void
}) {
  const [list, setList] = useState<api.AuditRecordLike[]>([])
  const [err, setErr] = useState('')
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    let alive = true
    setLoading(true)
    const p = siteId ? { siteId, limit: 50 } : { ref: ref ?? '', limit: 50 }
    api
      .getAuditRecords(p)
      .then((r) => alive && setList(r.list ?? []))
      .catch((e) => alive && setErr(e instanceof Error ? e.message : '加载失败'))
      .finally(() => alive && setLoading(false))
    return () => {
      alive = false
    }
  }, [siteId, ref])

  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="w-full max-w-2xl rounded-2xl bg-surface p-5 shadow-xl ring-1 ring-line">
        <div className="mb-3 flex items-center justify-between gap-2">
          <h3 className="truncate text-sm font-bold text-content">访问日志 · {title}</h3>
          <button
            onClick={onClose}
            className={`${iconBtn} bg-surface-2 text-muted hover:bg-fill`}
            title="关闭"
          >
            <X className="h-3.5 w-3.5" />
          </button>
        </div>

        {loading && <div className="py-8 text-center text-xs text-muted">加载中…</div>}
        {err && <div className="rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>}
        {!loading && !err && list.length === 0 && (
          <div className="py-8 text-center text-xs leading-relaxed text-muted">
            还没有访问记录。
            <br />
            有人访问过这个服务之后，来源 IP、状态、耗时会在这里出现。
          </div>
        )}

        {list.length > 0 && (
          <div className="max-h-[60vh] overflow-y-auto">
            <table className="w-full text-left text-xs">
              <thead className="sticky top-0 bg-surface text-muted">
                <tr>
                  <th className="pb-1.5 font-medium">时间</th>
                  <th className="pb-1.5 font-medium">来源 IP</th>
                  <th className="pb-1.5 font-medium">请求</th>
                  <th className="pb-1.5 font-medium">状态</th>
                  <th className="pb-1.5 font-medium">耗时</th>
                </tr>
              </thead>
              <tbody>
                {list.map((r, i) => (
                  <tr key={i} className="border-t border-line">
                    <td className="py-1.5 font-mono text-muted">
                      {new Date(r.time).toLocaleString()}
                    </td>
                    <td className="py-1.5 font-mono font-semibold text-content">{r.ip}</td>
                    <td
                      className="max-w-[220px] truncate py-1.5 text-content"
                      title={`${r.method ?? ''} ${r.host ?? ''}${r.path ?? ''}`}
                    >
                      {/* 穿透没有 HTTP 方法/路径，显示个标记就好 */}
                      {r.method === 'TUNNEL' ? '穿透连接' : `${r.method ?? ''} ${r.path ?? ''}`}
                    </td>
                    <td className="py-1.5">
                      <span
                        className={`rounded px-1 py-px font-bold ${
                          r.status >= 500
                            ? 'bg-rose-50 text-rose-600'
                            : r.status >= 400
                              ? 'bg-amber-50 text-amber-600'
                              : 'bg-emerald-50 text-emerald-600'
                        }`}
                        title={r.status === 403 ? '被门禁（黑名单/全局封禁）挡下' : ''}
                      >
                        {r.status}
                      </span>
                    </td>
                    <td className="py-1.5 font-mono text-muted">{r.durationMs ?? 0} ms</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}
