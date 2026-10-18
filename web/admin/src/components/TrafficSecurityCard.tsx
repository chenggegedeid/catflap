/**
 * 仪表盘的「流量与安全」联合卡。
 *
 * 它要回答一个问题：**现在的流量是正常访问，还是有人在扫我。**
 *
 * 这两件事原本是分开的：流量在流量面板里，异常在访问审计页里。分开看
 * 永远看不出关联 —— 20 点流量冲高一倍，你不知道那是家人在看视频，
 * 还是有人在爆破后台。所以这里把它们画在**同一条时间轴**上：
 * 流量是面积图，异常请求是叠在上面的红柱。两者一重合，答案就出来了。
 *
 * 能叠加是因为两边数据对齐：`/api/traffic/trend?hours=24` 与
 * `/api/audit/overview?hours=24` 都是按小时聚合，同一窗口点数一致（各 24 点）。
 *
 * 这一块只放信号。完整的趋势、各服务 Top 在「流量统计」页，
 * 完整的 IP 管理在「访问审计」页 —— 仪表盘上堆操作会变成第二个审计页。
 *
 * # 分两拨加载（首屏不再等重接口）
 *
 * 首屏只发两个轻的：实时速率（内存里直接取）和 IP 摘要计数（只数不加工）。
 * 这两样一到，四个指标卡和「可疑来源」就能画出来。
 *
 * 重的两样（24 小时趋势、审计 overview）延后一帧再发 —— 它们要读当天的
 * 明细/流量文件，是真正的慢源头。以前四路一起并发发出去，四张卡要等
 * 最慢的那个，整个仪表盘看起来就是在转圈。
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Activity, ShieldAlert } from 'lucide-react'
import { Card, CardHeader } from './Card'
import * as api from '../lib/api'
import type { AuditIPStat, AuditOverview, TrafficRealtime, TrafficTrend } from '../types'
import { fmtBytes } from '../pages/TrafficPanel'

/** 判定「疑似扫描」：异常占比过半，且该时段请求数明显高于平时 */
const ATTACK_RATIO = 0.5
const ATTACK_SURGE = 2
/** 可疑来源最多显示几条。仪表盘不是审计页，多了就喧宾夺主 */
const SUSPICIOUS_LIMIT = 3

interface IPSummary {
  total: number
  suspicious: number
  attackers: number
  blocked: number
}

export function TrafficSecurityCard() {
  const [rt, setRt] = useState<TrafficRealtime | null>(null)
  const [summary, setSummary] = useState<IPSummary | null>(null)
  const [trend, setTrend] = useState<TrafficTrend | null>(null)
  const [ov, setOv] = useState<AuditOverview | null>(null)
  const [suspicious, setSuspicious] = useState<AuditIPStat[]>([])
  const [busy, setBusy] = useState('')
  const [msg, setMsg] = useState('')

  /** 轻的：实时速率 + 计数。这两样不需要读任何明细文件 */
  const loadLight = useCallback(async () => {
    const [r, s] = await Promise.all([
      api.getTrafficRealtime().catch(() => null),
      api.getAuditIPSummary().catch(() => null),
    ])
    setRt(r)
    setSummary(s)
  }, [])

  /** 重的：24 小时趋势 + 审计概览 + 可疑来源。要读当天文件，慢 */
  const loadHeavy = useCallback(async () => {
    const [t, o, ips] = await Promise.all([
      api.getTrafficTrend({ hours: 24 }).catch(() => null),
      api.getAuditOverview(24).catch(() => null),
      api
        .getAuditIPs({ sort: 'errors', suspicious: true, limit: SUSPICIOUS_LIMIT })
        .then((x) => x.list ?? [])
        .catch(() => [] as AuditIPStat[]),
    ])
    setTrend(t)
    setOv(o)
    setSuspicious(ips)
  }, [])

  const reload = useCallback(async () => {
    await Promise.all([loadLight(), loadHeavy()])
  }, [loadLight, loadHeavy])

  useEffect(() => {
    void loadLight()
    // 让首屏先渲染出来再发重请求：两拨请求同时发的话，浏览器仍然要等
    // 最慢的那个才认为「这一屏稳定」，分帧发能让轻的那部分先落屏
    const t = window.setTimeout(() => void loadHeavy(), 60)
    return () => window.clearTimeout(t)
  }, [loadLight, loadHeavy])

  const block = async (ip: string) => {
    setBusy(ip)
    try {
      await api.blockIP({ ip, reason: '仪表盘快捷拉黑', minutes: 60 * 24 })
      setMsg(`已封禁 ${ip}（1 天）`)
      window.setTimeout(() => setMsg(''), 2800)
      await reload()
    } catch (e) {
      setMsg('封禁失败：' + (e instanceof Error ? e.message : ''))
    } finally {
      setBusy('')
    }
  }

  const surge = useMemo(() => findSurge(ov), [ov])
  const kinds = useMemo(() => (ov?.attackKinds ?? []).filter((k) => k.count > 0).slice(0, 3), [ov])
  const overview = ov?.overview
  const todayBytes = (rt?.today.in ?? 0) + (rt?.today.out ?? 0)
  const hasAudit = (overview?.requests ?? 0) > 0
  // 审计数据还在路上时，别急着说「没有可疑访问」—— 那是在说谎
  const auditPending = ov === null && summary === null

  return (
    <Card className="!p-0">
      <div className="px-5 pt-4">
        <CardHeader
          title={
            <span className="inline-flex items-center gap-1.5">
              <Activity className="h-4 w-4 text-muted" />
              流量与安全
            </span>
          }
          action={
            <div className="flex flex-wrap items-center gap-3 text-xs">
              <span className="text-muted">近 24 小时</span>
              <a href="#/traffic" className="rounded-lg px-2 py-1 font-semibold text-blue-600 hover:bg-blue-50">
                流量统计 ↗
              </a>
              <a href="#/audit" className="rounded-lg px-2 py-1 font-semibold text-blue-600 hover:bg-blue-50">
                访问审计 ↗
              </a>
            </div>
          }
        />
      </div>

      <div className="px-5 pb-4">
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Metric label="今日流量" value={fmtBytes(todayBytes)} hint={`实时 ${fmtBytes(rt?.inPerSec ?? 0)}/s 入站`} />
          <Metric
            label="今日请求"
            value={fmtCount(overview?.requests ?? 0)}
            hint={`${overview?.uniqueIPs ?? 0} 个来源 IP`}
          />
          <Metric
            label="异常请求"
            value={fmtCount(overview?.errors ?? 0)}
            hint={overview?.errorRatio ? `占 ${(overview.errorRatio * 100).toFixed(1)}%` : '—'}
            tone={(overview?.errors ?? 0) > 0 ? 'warn' : 'plain'}
          />
          <Metric
            label="已拦截"
            value={fmtCount(overview?.blocked ?? 0)}
            hint={
              summary
                ? `${summary.attackers} 个 IP 出过异常`
                : `${overview?.attackIPs ?? 0} 个可疑 IP`
            }
            tone={(overview?.blocked ?? 0) > 0 ? 'danger' : 'plain'}
          />
        </div>

        <div className="mt-4">
          <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
            <span className="text-[11px] text-muted">流量与异常 · 同一时间轴</span>
            <div className="flex gap-3 text-[11px] text-muted">
              <span className="flex items-center gap-1">
                <span className="h-2 w-3 rounded-sm bg-blue-500" />流量
              </span>
              <span className="flex items-center gap-1">
                <span className="h-2 w-3 rounded-sm bg-red-500" />异常请求
              </span>
            </div>
          </div>

          <OverlayChart trend={trend} audit={ov?.trends ?? []} />

          {surge ? (
            <div className="mt-2 rounded-lg bg-amber-50 px-3 py-2 text-[11px] leading-relaxed text-amber-700">
              {surge.label} 请求升到 {fmtCount(surge.requests)} 次，其中 {Math.round(surge.ratio * 100)}% 是异常 → 疑似扫描，不是正常访问
            </div>
          ) : auditPending ? (
            <div className="mt-2 h-7 animate-pulse rounded-lg bg-surface-2" />
          ) : hasAudit ? (
            <div className="mt-2 rounded-lg bg-surface-2 px-3 py-2 text-[11px] text-muted">
              近 24 小时没有可疑访问
            </div>
          ) : null}
        </div>

        {kinds.length > 0 || suspicious.length > 0 ? (
          <div className="mt-4 grid gap-4 border-t border-line pt-4 lg:grid-cols-2">
            {kinds.length > 0 ? (
              <div>
                <div className="mb-2 text-[11px] text-muted">攻击类型</div>
                <div className="space-y-1.5">
                  {kinds.map((k) => (
                    <BarRow key={k.kind} label={k.label} value={k.count} max={kinds[0]?.count || 1} />
                  ))}
                </div>
              </div>
            ) : null}

            {suspicious.length > 0 ? (
              <div>
                <div className="mb-2 flex items-center justify-between">
                  <span className="text-[11px] text-muted">可疑来源</span>
                  <ShieldAlert className="h-3 w-3 text-muted" />
                </div>
                <div className="space-y-1.5">
                  {suspicious.map((s) => (
                    <div
                      key={s.ip}
                      className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg bg-rose-50/50 px-3 py-1.5"
                    >
                      <span className="font-mono text-xs text-content">{s.ip}</span>
                      <span className="text-[11px] text-muted">{fmtCount(s.errors)} 次异常</span>
                      {s.paths?.[0] ? (
                        <span className="truncate font-mono text-[11px] text-muted" title={s.paths.join('\n')}>
                          {s.paths[0]}
                        </span>
                      ) : null}
                      <button
                        onClick={() => block(s.ip)}
                        disabled={busy === s.ip}
                        className="ml-auto rounded-md bg-white px-2 py-0.5 text-[11px] font-semibold text-rose-600 hover:bg-rose-100 disabled:opacity-50"
                      >
                        {busy === s.ip ? '…' : '拉黑'}
                      </button>
                    </div>
                  ))}
                </div>
                {msg ? <div className="mt-2 text-[11px] text-muted">{msg}</div> : null}
              </div>
            ) : null}
          </div>
        ) : null}
      </div>
    </Card>
  )
}

function Metric({
  label,
  value,
  hint,
  tone = 'plain',
}: {
  label: string
  value: string
  hint: string
  tone?: 'plain' | 'warn' | 'danger'
}) {
  const color = tone === 'danger' ? 'text-rose-600' : tone === 'warn' ? 'text-amber-600' : 'text-content'
  return (
    <div className="rounded-xl bg-surface-2 p-3">
      <div className="text-xs text-muted">{label}</div>
      <div className={`mt-1 text-xl font-semibold ${color}`}>{value}</div>
      <div className="mt-0.5 truncate text-[11px] text-muted">{hint}</div>
    </div>
  )
}

function BarRow({ label, value, max }: { label: string; value: number; max: number }) {
  const pct = max > 0 ? Math.max(4, Math.round((value / max) * 100)) : 0
  return (
    <div className="flex items-center gap-2 text-[11px]">
      <span className="w-20 shrink-0 truncate text-content">{label}</span>
      <span className="h-1.5 flex-1 overflow-hidden rounded-full bg-surface-2">
        <span className="block h-full rounded-full bg-red-500" style={{ width: `${pct}%` }} />
      </span>
      <span className="w-10 shrink-0 text-right font-mono text-muted">{fmtCount(value)}</span>
    </div>
  )
}

/**
 * 流量与异常的叠加图。手写 SVG —— 全站没有图表库，也不打算为一个卡片引入。
 *
 * 两条线**各按自己的峰值归一化**：流量单位是字节、异常单位是次数，绝对值
 * 没法共用纵轴。这里要的是**形状上的重合关系**（流量涨的同一时段异常是不是
 * 也涨），不是数值比较，所以各自归一化后画在一起是对的。
 *
 * 异常请求画成从基线往上的红柱，流量是叠在上面的面积 —— 红柱顶到流量线
 * 附近，就说明那一段流量几乎全是异常。
 */
function OverlayChart({
  trend,
  audit,
}: {
  trend: TrafficTrend | null
  audit: { label: string; requests: number; errors: number }[]
}) {
  const W = 600
  const H = 96
  const base = 88

  // 流量序列与审计序列都按小时，取较长的那个作为 x 轴刻度数
  const tSeries = trend?.series ?? []
  const n = Math.max(audit.length, tSeries.length)

  if (n === 0) {
    return <div className="flex h-24 items-center justify-center text-xs text-muted">等待数据…</div>
  }

  const step = W / n
  const barW = Math.max(3, Math.min(12, step * 0.45))

  /*
    流量序列是**稀疏**的：只有发生过流量的小时才有记录（刚开机可能只有 1 个点），
    而审计序列是**稠密**的：24 个小时桶全部预填，没数据的小时是 0。
    所以不能按下标一一对应 —— 那样流量的那 1 个点会被画到最左边（24 小时前），
    实际它属于「当前小时」。

    两边都覆盖同一个窗口、且都以「当前小时」结尾，所以**从右往左对齐**是正确的：
    流量的最后一个点对应审计的最后一个点，倒数第二个对应倒数第二个。
  */
  const tPeak = Math.max(1, ...tSeries.map((p) => p.in + p.out))
  const tPts = tSeries.map((p, idxFromStart) => {
    const i = n - (tSeries.length - idxFromStart) // 换算成整条轴的绝对下标（右对齐）
    const x = i * step + step / 2
    const y = base - ((p.in + p.out) / tPeak) * (base - 10)
    return `${x.toFixed(1)},${y.toFixed(1)}`
  })

  // 异常：按请求数峰值归一化
  const aPeak = Math.max(1, ...audit.map((p) => p.requests))
  const bars = audit.map((p, i) => {
    const ratio = p.errors / Math.max(p.requests, 1)
    const h = (p.errors / aPeak) * (base - 12)
    const x = i * step + (step - barW) / 2
    return { x, y: base - h, h: Math.max(h, p.errors > 0 ? 1.5 : 0), hot: ratio > ATTACK_RATIO }
  })

  return (
    <div>
      <svg viewBox={`0 0 ${W} ${H}`} className="h-24 w-full" role="img" aria-label="流量与异常请求在同一时间轴上的叠加图">
        <line x1="0" y1={base} x2={W} y2={base} stroke="currentColor" strokeOpacity="0.15" strokeWidth="1" />
        <line x1="0" y1={base - 30} x2={W} y2={base - 30} stroke="currentColor" strokeOpacity="0.08" strokeWidth="1" />

        {bars.map((b, i) => (
          <rect
            key={`e${i}`}
            x={b.x}
            y={b.y}
            width={barW}
            height={b.h}
            fill="#E24B4A"
            opacity={b.hot ? 0.85 : 0.45}
          />
        ))}

        {tPts.length > 1 ? (
          <>
            <polygon points={`0,${base} ${tPts.join(' ')} ${W},${base}`} fill="#378ADD" opacity="0.14" />
            <polyline
              points={tPts.join(' ')}
              fill="none"
              stroke="#378ADD"
              strokeWidth="1.6"
              strokeLinejoin="round"
            />
          </>
        ) : null}

        {/* 只有一个小时的流量（刚开机常见）：画个点，别让整条线凭空消失 */}
        {tPts.length === 1 ? (
          <circle
            cx={tPts[0].split(',')[0]}
            cy={tPts[0].split(',')[1]}
            r="3"
            fill="#378ADD"
          />
        ) : null}
      </svg>

      <div className="flex justify-between text-[10px] text-muted">
        <span>{audit[0]?.label || tSeries[0]?.time ? '24 小时前' : ''}</span>
        <span>{n > 2 ? (audit[Math.floor(n / 2)]?.label ?? '') : ''}</span>
        <span>现在</span>
      </div>
    </div>
  )
}

/** 找出最像攻击的时段：异常占比过半，且请求数高于均值 ATTACK_SURGE 倍 */
function findSurge(ov: AuditOverview | null) {
  const pts = ov?.trends ?? []
  if (pts.length === 0) return null

  const avg = pts.reduce((s, p) => s + p.requests, 0) / pts.length
  if (avg <= 0) return null

  let best: { label: string; requests: number; ratio: number } | null = null
  for (const p of pts) {
    if (p.requests < avg * ATTACK_SURGE) continue
    const ratio = p.errors / Math.max(p.requests, 1)
    if (ratio < ATTACK_RATIO) continue
    if (!best || p.requests > best.requests) best = { label: p.label, requests: p.requests, ratio }
  }
  return best
}

function fmtCount(n: number): string {
  if (n >= 10000) return `${(n / 10000).toFixed(1)} 万`
  return String(Math.round(n))
}
