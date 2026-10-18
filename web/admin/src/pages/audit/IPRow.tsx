import { Ban, ShieldOff } from 'lucide-react'
import type { AuditIPStat, AuditRecord } from '../../types'
import { fmtBytes, fmtCount, isSuspicious } from './shared'

export function IPRow({
  stat,
  showGeo,
  open,
  records,
  onToggle,
  onBlock,
  onUnblock,
}: {
  stat: AuditIPStat
  /** 归属地库是否可用。不可用时不要显示一列「未知」 */
  showGeo: boolean
  open: boolean
  records: AuditRecord[]
  onToggle: () => void
  onBlock: () => void
  onUnblock: () => void
}) {
  const suspicious = isSuspicious(stat)

  return (
    <div className={`rounded-xl border ${suspicious ? 'border-rose-200 bg-rose-50/40' : 'border-line/70'}`}>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 px-3 py-2">
        <button onClick={onToggle} className="flex min-w-0 flex-1 flex-wrap items-center gap-x-3 gap-y-1.5 text-left">
          {/*
            IPv6 地址有 39 个字符。以前这里是单行 shrink-0，长地址会把
            归属地、次数一路挤到屏幕外。改成 break-all 让它自己折行。
          */}
          <span className="max-w-full break-all font-mono text-xs leading-relaxed text-content">
            {stat.ip}
          </span>
          {showGeo && stat.geo && (
            <span
              className="max-w-32 shrink-0 truncate text-[11px] text-muted"
              title={[stat.geo.country, stat.geo.province, stat.geo.city, stat.geo.isp]
                .filter(Boolean)
                .join(' · ')}
            >
              {stat.geo.label}
            </span>
          )}
          {suspicious && (
            <span className="shrink-0 rounded bg-rose-100 px-1.5 py-0.5 text-[10px] font-semibold text-rose-600">可疑</span>
          )}
          {stat.attack && !stat.blocked && (
            <span className="shrink-0 rounded bg-amber-50 px-1.5 py-0.5 text-[10px] font-semibold text-amber-700">
              {stat.attack}
            </span>
          )}
          {stat.blocked && (
            <span className="shrink-0 rounded bg-fill px-1.5 py-0.5 text-[10px] font-semibold text-content">已封禁</span>
          )}
          <span className="shrink-0 text-[11px] text-muted">{fmtCount(stat.requests)} 次</span>
          {stat.errors > 0 && (
            <span className={`shrink-0 text-[11px] ${suspicious ? 'font-semibold text-rose-600' : 'text-amber-600'}`}>
              异常 {fmtCount(stat.errors)}
            </span>
          )}
          <span className="text-[11px] text-muted">{fmtBytes(stat.bytesIn + stat.bytesOut)}</span>
          <span className="ml-auto hidden text-[11px] text-muted sm:inline">
            {new Date(stat.lastSeen).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}
          </span>
        </button>

        {stat.blocked ? (
          <button
            onClick={onUnblock}
            className="flex items-center gap-1 rounded-lg px-2 py-0.5 text-xs font-semibold text-muted hover:bg-surface"
          >
            <ShieldOff className="h-3 w-3" /> 解除
          </button>
        ) : (
          <button
            onClick={onBlock}
            className="flex items-center gap-1 rounded-lg bg-surface-2 px-2 py-0.5 text-xs font-semibold text-content hover:bg-rose-100 hover:text-rose-600"
          >
            <Ban className="h-3 w-3" /> 拉黑
          </button>
        )}
      </div>

      {open && (
        <div className="border-t border-line/70 px-3 py-2.5 text-[11px]">
          <div className="mb-2 grid gap-2 sm:grid-cols-2">
            <div>
              <div className="mb-1 font-semibold text-muted">访问过的路径（看它在找什么）</div>
              <div className="flex flex-wrap gap-1">
                {(stat.paths ?? []).length === 0 ? (
                  <span className="text-muted">—</span>
                ) : (
                  (stat.paths ?? []).map((p) => (
                    <span key={p} className="rounded bg-surface px-1.5 py-0.5 font-mono text-[10px] text-content ring-1 ring-line">
                      {p}
                    </span>
                  ))
                )}
              </div>
            </div>
            <div>
              <div className="mb-1 font-semibold text-muted">User-Agent</div>
              <div className="space-y-0.5">
                {(stat.userAgents ?? []).length === 0 ? (
                  <span className="text-muted">—</span>
                ) : (
                  (stat.userAgents ?? []).map((u) => (
                    <div key={u} className="truncate font-mono text-[10px] text-muted">
                      {u}
                    </div>
                  ))
                )}
              </div>
            </div>
          </div>

          <div className="mb-1 flex items-center gap-2">
            <span className="font-semibold text-muted">最近的访问</span>
            <span className="text-muted">最近 {records.slice(0, 50).length} 条</span>
          </div>
          <div className="max-h-56 overflow-y-auto rounded-lg bg-surface ring-1 ring-line">
            {records.length === 0 ? (
              <div className="py-3 text-center text-muted">读取中…</div>
            ) : (
              <>
                {/* 列多了没表头就看不出哪一列是什么 */}
                <div className="sticky top-0 grid grid-cols-[3.5rem_3rem_1fr_2.5rem_3.5rem] gap-2 border-b border-line bg-surface-2 px-2 py-1 text-[10px] font-semibold text-muted">
                  <span>时间</span>
                  <span>方法</span>
                  <span>主机 / 路径</span>
                  <span className="text-right">状态</span>
                  <span className="text-right">耗时</span>
                </div>
                {records.slice(0, 50).map((r, i) => (
                  <div
                    key={i}
                    className="grid grid-cols-[3.5rem_3rem_1fr_2.5rem_3.5rem] items-center gap-2 border-b border-line px-2 py-1 font-mono text-[10px] last:border-0 hover:bg-surface-2"
                    title={`${r.method} ${r.host}${r.path} · ${r.status} · ${r.durationMs}ms · 入 ${fmtBytes(r.bytesIn)} / 出 ${fmtBytes(r.bytesOut)}${r.userAgent ? `\n${r.userAgent}` : ''}`}
                  >
                    <span className="text-muted">{r.time.slice(11, 19)}</span>
                    <span className="text-muted">{r.method}</span>
                    <span className="min-w-0 truncate text-content">
                      {r.host && <span className="text-muted">{r.host}</span>}
                      {r.path || (r.kind === 'tunnel' ? `（穿透 ${r.ref || ''}）` : '—')}
                    </span>
                    <span className={`text-right ${r.status >= 400 ? 'text-rose-500' : 'text-emerald-600'}`}>
                      {r.status}
                    </span>
                    <span className="text-right text-muted">{r.durationMs}ms</span>
                  </div>
                ))}
              </>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

// IPLists 全局的两份 IP 名单：黑名单与白名单。
//
// 它们是**同一件事的两面**，所以并排放：
//   - 黑名单：被列进去的来源直接拦下，对所有站点生效
//   - 白名单：列进去的来源永不封禁，自动拉黑也会绕开
//
// 站点上不再各配一份 —— 面向互联网的服务没法提前知道谁会来，
// 预先写网段没有意义（见 CHANGELOG 2.5.0「移除」）。
