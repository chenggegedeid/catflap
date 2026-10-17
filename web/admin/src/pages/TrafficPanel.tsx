/**
 * 流量大屏：仪表盘顶部那块。
 *
 * 图表全部**自绘 SVG**，没有引图表库。原因很直接：整个后台现在 460 KB 左右，
 * 引 ECharts 要再加 ~300 KB，而这里要的只是折线 / 柱状 / 横条三种形态，
 * 几十行 SVG 就够。等真有复杂图表需求（缩放、联动、多轴）再引不迟。
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { ArrowDown, ArrowUp, RotateCw } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import * as api from '../lib/api'
import type { AuditIPStat, TrafficRealtime, TrafficTrend } from '../types'

/** 刷新间隔（毫秒）。后端只读内存，2 秒一次没有压力。 */
const REFRESH_MS = 2000

/** 字节数格式化：1.2 TB / 340 GB / 8.2 MB */
export function fmtBytes(n: number): string {
  if (!n || n < 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  // 小于 10 时保留一位小数，读起来更有信息量（8.2 MB 比 8 MB 有用）
  const digits = v < 10 && i > 0 ? 1 : 0
  return `${v.toFixed(digits)} ${units[i]}`
}

/** 速率格式化：带 /s */
const fmtRate = (n: number) => `${fmtBytes(n)}/s`

export function TrafficPanel() {
  const [rt, setRt] = useState<TrafficRealtime | null>(null)
  const [trend, setTrend] = useState<TrafficTrend | null>(null)
  const [err, setErr] = useState('')
  const [paused, setPaused] = useState(false)
  const timerRef = useRef<number | null>(null)

  const loadRealtime = useCallback(async () => {
    try {
      setRt(await api.getTrafficRealtime())
      setErr('')
    } catch (e) {
      setErr(e instanceof Error ? e.message : '读取失败')
    }
  }, [])

  // 趋势范围（小时）：近1小时/近24小时/近7天/近30天 —— 参考飞牛的快捷档。
  // 聚合粒度后端自动定（≤48h 按小时，更大按天），前端只管把速率画出来。
  const [trendHours, setTrendHours] = useState(168)
  const loadTrend = useCallback(async () => {
    try {
      setTrend(await api.getTrafficTrend({ hours: trendHours }))
    } catch {
      // 趋势拿不到不该影响实时区，静默
    }
  }, [trendHours])

  // 实时：定时刷。页面藏到后台时停掉 —— 定时器在不可见标签页里
  // 本来就该少干活，而且用户看不到，刷新没有意义
  useEffect(() => {
    void loadRealtime()
    void loadTrend()

    const start = () => {
      if (timerRef.current != null) return
      timerRef.current = window.setInterval(() => void loadRealtime(), REFRESH_MS)
    }
    const stop = () => {
      if (timerRef.current != null) {
        window.clearInterval(timerRef.current)
        timerRef.current = null
      }
    }

    const onVis = () => (document.hidden ? stop() : start())

    if (!document.hidden) start()
    document.addEventListener('visibilitychange', onVis)
    return () => {
      stop()
      document.removeEventListener('visibilitychange', onVis)
    }
  }, [loadRealtime, loadTrend, trendHours])

  // 暂停：想安静看一个瞬时值时不希望它一直在跳
  useEffect(() => {
    if (timerRef.current == null) return
    if (paused) {
      window.clearInterval(timerRef.current)
      timerRef.current = null
    } else {
      timerRef.current = window.setInterval(() => void loadRealtime(), REFRESH_MS)
    }
  }, [paused, loadRealtime])

  if (err && !rt) {
    return (
      <Card>
        <div className="text-sm text-rose-600">{err}</div>
      </Card>
    )
  }

  return (
    <div className="space-y-4">
      {/* 实时速率 */}
      <Card className="!p-0">
        <div className="px-5 pt-4">
          <CardHeader
            title="实时速率"
            action={
              <div className="flex items-center gap-2">
                <span className="text-[11px] text-muted">近 1 分钟</span>
                <button
                  onClick={() => setPaused((p) => !p)}
                  title={paused ? '继续刷新' : '暂停刷新（看瞬时值时用）'}
                  className={`flex items-center gap-1 rounded-lg px-2 py-1 text-xs font-semibold transition ${
                    paused ? 'bg-amber-50 text-amber-600' : 'text-muted hover:bg-surface-2'
                  }`}
                >
                  <RotateCw className={`h-3 w-3 ${paused ? '' : 'animate-spin'}`} />
                  {paused ? '已暂停' : '刷新中'}
                </button>
              </div>
            }
          />
        </div>

        <div className="px-5 pb-4">
          <div className="mb-3 flex flex-wrap items-baseline gap-x-8 gap-y-2">
            <RateNumber label="入站" value={rt?.inPerSec ?? 0} color="#185FA5" dir="in" />
            <RateNumber label="出站" value={rt?.outPerSec ?? 0} color="#3B6D11" dir="out" />
          </div>

          <RateChart series={rt?.series ?? []} />
        </div>
      </Card>

      {/* 累计与今日 */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Metric label="累计入站" value={fmtBytes(rt?.totals.totalIn ?? 0)} />
        <Metric label="累计出站" value={fmtBytes(rt?.totals.totalOut ?? 0)} />
        <Metric label="今日流量" value={fmtBytes((rt?.today.in ?? 0) + (rt?.today.out ?? 0))} />
        <Metric label="今日请求" value={fmtCount(rt?.today.requests ?? 0)} />
      </div>

      {/* 趋势 + 站点 Top */}
      <div className="grid gap-4 lg:grid-cols-2">
        <Card className="!p-0">
          <div className="flex flex-wrap items-center justify-between gap-2 px-5 pt-4">
            <CardHeader
              title={
                trend?.bucket === 'hour'
                  ? `流量速率 · 近 ${trend.hours} 小时（按小时聚合）`
                  : `流量速率 · 近 ${Math.round((trend?.hours ?? 168) / 24)} 天（按天聚合）`
              }
            />
            {/* 快捷范围（和飞牛同一个四档）：聚合粒度自动，不用人选 */}
            <div className="flex gap-1">
              {(
                [
                  [1, '近1小时'],
                  [24, '近24小时'],
                  [168, '近7天'],
                  [720, '近30天'],
                ] as [number, string][]
              ).map(([h, label]) => (
                <button
                  key={h}
                  onClick={() => setTrendHours(h)}
                  className={`rounded-md px-2 py-1 text-[11px] font-semibold transition ${
                    trendHours === h
                      ? 'bg-blue-50 text-blue-600 ring-1 ring-blue-200'
                      : 'text-muted hover:bg-surface-2'
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>
          <div className="px-5 pb-4">
            <RateLineChart
              series={trend?.series ?? []}
              bucketSeconds={trend?.bucketSeconds ?? 3600}
            />
          </div>
        </Card>

        <Card className="!p-0">
          <div className="px-5 pt-4">
            <CardHeader title="各服务流量" />
          </div>
          <div className="px-5 pb-4">
            <TopList trend={trend} />
          </div>
        </Card>
      </div>

      {/* 可疑 IP：仪表盘上直接能看到"有没有人在扫我"，并能一键拉黑 */}
      <SuspiciousPanel />

      {rt?.totals.since ? (
        <div className="px-1 text-[11px] text-muted">
          统计始于 {new Date(rt.totals.since * 1000).toLocaleString('zh-CN')}
          {paused && ' · 已暂停刷新'}
        </div>
      ) : null}
    </div>
  )
}

/** 一个方向的大数字。数字用等宽字体，跳动时不会左右抖 */
function RateNumber({ label, value, color, dir }: { label: string; value: number; color: string; dir: 'in' | 'out' }) {
  const Icon = dir === 'in' ? ArrowDown : ArrowUp
  return (
    <div>
      <div className="flex items-center gap-1 text-xs text-muted">
        <Icon className="h-3 w-3" style={{ color }} />
        {label}
      </div>
      {/* tabular-nums：每次刷新数字位数变化时不会把后面的东西挤来挤去 */}
      <div className="text-3xl font-semibold tabular-nums" style={{ color }}>
        {fmtRate(value)}
      </div>
    </div>
  )
}

/**
 * 实时速率折线。
 *
 * 纵轴按窗口内的**峰值**自适应，而不是固定刻度 —— 家里带宽从 0 到
 * 100 MB/s 都可能，固定刻度要么全是平的、要么全顶到天花板。
 */
function RateChart({ series }: { series: { in: number; out: number }[] }) {
  const W = 600
  const H = 90
  const pad = 2

  if (series.length === 0) {
    return <div className="flex h-[90px] items-center justify-center text-xs text-muted">等待数据…</div>
  }

  let peak = 0
  for (const p of series) {
    peak = Math.max(peak, p.in, p.out)
  }
  // 全 0 时给个 1 避免除零；峰值加 10% 顶部余量，曲线不贴顶
  const max = peak > 0 ? peak * 1.1 : 1

  const toPoints = (pick: (p: { in: number; out: number }) => number) =>
    series
      .map((p, i) => {
        const x = (i / Math.max(series.length - 1, 1)) * (W - pad * 2) + pad
        const y = H - pad - (pick(p) / max) * (H - pad * 2)
        return `${x.toFixed(1)},${y.toFixed(1)}`
      })
      .join(' ')

  return (
    <div>
      <svg viewBox={`0 0 ${W} ${H}`} width="100%" height={H} preserveAspectRatio="none" role="img">
        <title>近 1 分钟实时速率</title>
        {/* 中线：给个参照，不然空图上不知道有没有数据 */}
        <line x1={0} y1={H / 2} x2={W} y2={H / 2} stroke="#e2e8f0" strokeWidth={0.5} />
        <polyline points={toPoints((p) => p.in)} fill="none" stroke="#378ADD" strokeWidth={1.5} />
        <polyline points={toPoints((p) => p.out)} fill="none" stroke="#639922" strokeWidth={1.5} />
      </svg>
      <div className="mt-1 flex items-center justify-between text-[11px] text-muted">
        <span className="flex items-center gap-3">
          <span className="flex items-center gap-1">
            <span className="inline-block h-0.5 w-2.5 bg-[#378ADD]" /> 入站
          </span>
          <span className="flex items-center gap-1">
            <span className="inline-block h-0.5 w-2.5 bg-[#639922]" /> 出站
          </span>
        </span>
        <span>峰值 {fmtRate(peak)}</span>
      </div>
    </div>
  )
}

/** 近 N 天柱状图。入站在左、出站在右，同一天并排 */
function TopList({ trend }: { trend: TrafficTrend | null }) {
  // 展开的是哪个服务（key = site-<id> / tun-<ref>）
  const [expanded, setExpanded] = useState('')
  // 名字由后端解析好（「设备名 · 服务名」），拿不到就回落成编号
  const names = trend?.names ?? {}
  const rows = [
    ...Object.entries(trend?.bySite ?? {}).map(([id, t]) => ({
      key: `site-${id}`,
      label: names[id] ?? (id === '0' ? '未匹配站点' : `站点 #${id}`),
      total: t.in + t.out,
      requests: t.requests,
      tunnel: false,
    })),
    ...Object.entries(trend?.byTunnel ?? {}).map(([key, t]) => ({
      key: `tun-${key}`,
      label: names[key] ?? `穿透 #${key.replace(/^s/, '')}`,
      total: t.in + t.out,
      requests: t.requests,
      tunnel: true,
    })),
  ]
    .sort((a, b) => b.total - a.total)
    .slice(0, 6)

  if (rows.length === 0) {
    return <div className="flex h-[110px] items-center justify-center text-xs text-muted">暂无流量</div>
  }

  const peak = rows[0].total || 1

  return (
    <div className="space-y-2.5">
      {rows.map((r) => (
        <div key={r.key}>
          <div className="mb-1 flex items-baseline justify-between text-xs">
            <span className="text-content">
              {r.tunnel && (
                <span
                  title="穿透服务（不经过反向代理，直接由洞口转发）"
                  className="mr-1 rounded bg-blue-50 px-1 py-0.5 text-[10px] font-bold text-blue-600"
                >
                  穿透
                </span>
              )}
              {r.label}
              <span className="ml-2 text-[11px] text-muted">
                {fmtCount(r.requests)} {r.tunnel ? '次连接' : '次'}
              </span>
            </span>
            <span className="tabular-nums text-muted">{fmtBytes(r.total)}</span>
          </div>
          <div className="h-1.5 overflow-hidden rounded-full bg-surface-2">
            <div
              className={`h-full rounded-full ${r.tunnel ? 'bg-blue-400' : 'bg-blue-500'}`}
              style={{ width: `${Math.max((r.total / peak) * 100, 2)}%` }}
            />
          </div>

          {/* 展开看这个服务**按天**的流量历史 —— 之前只有总量，看不出哪天用得多 */}
          {expanded === r.key && (
            <div className="mt-2 rounded-lg bg-surface-2 px-2.5 py-2">
              {(trend?.perServiceDay?.[r.key.replace(/^(site-|tun-)/, '')] ?? []).length === 0 ? (
                <div className="text-[11px] text-muted">这个范围内没有它的记录</div>
              ) : (
                <MiniDayBars
                  points={trend!.perServiceDay![r.key.replace(/^(site-|tun-)/, '')]}
                />
              )}
            </div>
          )}
          <button
            onClick={() => setExpanded(expanded === r.key ? '' : r.key)}
            className="mt-0.5 text-[10px] text-muted transition hover:text-blue-500"
          >
            {expanded === r.key ? '收起历史 ▲' : '按天历史 ▼'}
          </button>
        </div>
      ))}
    </div>
  )
}

/** 小型按天柱状图（服务的历史流量展开用） */
function MiniDayBars({ points }: { points: { date: string; in: number; out: number }[] }) {
  const peak = Math.max(...points.map((p) => p.in + p.out), 1)
  return (
    <div className="space-y-1">
      {points.map((p) => (
        <div key={p.date} className="flex items-center gap-2 text-[10px]">
          <span className="w-[68px] shrink-0 font-mono text-muted">{p.date.slice(5)}</span>
          <div className="h-2 flex-1 overflow-hidden rounded-full bg-surface ring-1 ring-line/70">
            <div
              className="h-full rounded-full bg-blue-400"
              style={{ width: `${Math.max(((p.in + p.out) / peak) * 100, 2)}%` }}
              title={`${p.date}：入 ${fmtBytes(p.in)} / 出 ${fmtBytes(p.out)}`}
            />
          </div>
          <span className="w-[64px] shrink-0 text-right tabular-nums text-muted">
            {fmtBytes(p.in + p.out)}
          </span>
        </div>
      ))}
    </div>
  )
}

/** 指标卡 */
function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl bg-surface-2 px-4 py-3">
      <div className="text-xs text-muted">{label}</div>
      <div className="mt-0.5 text-xl font-semibold tabular-nums text-content">{value}</div>
    </div>
  )
}

/** 大数字（万 / 亿），请求数用它比原始数字好读 */
function fmtCount(n: number): string {
  if (n >= 1e8) return `${(n / 1e8).toFixed(1)} 亿`
  if (n >= 1e4) return `${(n / 1e4).toFixed(1)} 万`
  return String(n)
}

/**
 * 仪表盘上的可疑 IP 块。
 *
 * 为什么要放仪表盘：用户进管理页时的两个问题是"流量怎么样"和
 * "有没有被扫"。第二个问题的答案不该藏到另一个页面里。
 *
 * 这里只展示 + 提供"去处理"的入口，完整的 IP 管理在「访问审计」页 ——
 * 仪表盘上堆太多操作会变成第二个审计页。
 */
function SuspiciousPanel() {
  const [list, setList] = useState<AuditIPStat[]>([])
  const [busy, setBusy] = useState('')
  const [msg, setMsg] = useState('')

  const load = useCallback(async () => {
    try {
      const r = await api.getAuditIPs({ sort: 'errors', suspicious: true, limit: 5 })
      setList(r.list ?? [])
    } catch {
      // 审计没开或没数据都不该影响仪表盘其它块
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const block = async (ip: string) => {
    setBusy(ip)
    try {
      await api.blockIP({ ip, reason: '仪表盘快捷拉黑', minutes: 60 * 24 })
      setMsg(`已封禁 ${ip}（1 天）`)
      window.setTimeout(() => setMsg(''), 2800)
      await load()
    } catch (e) {
      setMsg('封禁失败：' + (e instanceof Error ? e.message : ''))
    } finally {
      setBusy('')
    }
  }

  if (list.length === 0) return null

  return (
    <Card className="!p-0">
      <div className="px-5 pt-4">
        <CardHeader
          title="可疑来源"
          action={
            <a
              href="#/audit"
              className="rounded-lg px-2 py-1 text-xs font-semibold text-blue-600 hover:bg-blue-50"
            >
              查看全部 ↗
            </a>
          }
        />
      </div>
      <div className="px-5 pb-4">
        <div className="mb-2 text-[11px] text-muted">
          24 小时内异常访问（4xx）密集的来源。点「拉黑」临时封 1 天。
        </div>
        <div className="space-y-1.5">
          {list.map((x) => (
            <div key={x.ip} className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg bg-rose-50/50 px-3 py-2">
              <span className="font-mono text-xs text-content">{x.ip}</span>
              <span className="text-[11px] text-muted">{x.requests} 次</span>
              <span className="text-[11px] font-semibold text-rose-600">异常 {x.errors}</span>
              {x.paths.length > 0 && (
                <span className="truncate font-mono text-[10px] text-muted">
                  在扫 {x.paths.slice(0, 2).join('、')}
                </span>
              )}
              <button
                onClick={() => block(x.ip)}
                disabled={busy === x.ip}
                className="ml-auto rounded-lg bg-rose-500 px-2 py-0.5 text-xs font-semibold text-white hover:bg-rose-600 disabled:opacity-50"
              >
                {busy === x.ip ? '处理中…' : '拉黑'}
              </button>
            </div>
          ))}
        </div>
        {msg && <div className="mt-2 text-[11px] text-muted">{msg}</div>}
      </div>
    </Card>
  )
}

/**
 * RateLineChart 速率折线图（入站/出站两条线），参考飞牛的"网络速率历史趋势"。
 *
 * 速率 = 桶内流量 / 桶长 —— 后端给的是每个桶的**总量**和桶长（秒），
 * 换算成 Mbps 在这里做，图上直接就是速率而不是"这段时间用了多少"。
 *
 * 用手写 SVG 不引图表库：两条折线 + 一圈刻度，引个 chart 库比图本身还重。
 */
function RateLineChart({
  series,
  bucketSeconds,
}: {
  series: { time: number; in: number; out: number }[]
  bucketSeconds: number
}) {
  if (series.length === 0) {
    return <div className="flex h-[180px] items-center justify-center text-xs text-muted">这个范围内没有流量记录</div>
  }

  // 速率（bps），画图前先找到峰值用来定 y 轴刻度
  const pts = series.map((p) => ({
    t: p.time,
    inBps: (p.in * 8) / bucketSeconds,
    outBps: (p.out * 8) / bucketSeconds,
  }))
  const peak = Math.max(...pts.map((p) => Math.max(p.inBps, p.outBps)), 1)

  const W = 600
  const H = 170
  const PAD_L = 46
  const PAD_B = 18
  const t0 = pts[0].t
  const t1 = Math.max(pts[pts.length - 1].t, t0 + bucketSeconds)
  const x = (t: number) => PAD_L + ((t - t0) / (t1 - t0)) * (W - PAD_L - 8)
  const y = (v: number) => H - PAD_B - (v / peak) * (H - PAD_B - 8)

  const line = (get: (p: (typeof pts)[number]) => number) =>
    pts.map((p, i) => `${i === 0 ? 'M' : 'L'}${x(p.t).toFixed(1)},${y(get(p)).toFixed(1)}`).join(' ')

  // y 轴刻度：峰值取整到好看的档位（0.4 / 1.2 / 2.8 Mbps 这种）
  const nicePeak = (() => {
    const mag = Math.pow(10, Math.floor(Math.log10(peak)))
    return Math.ceil(peak / mag) * mag
  })()
  const fmtRate = (bps: number) =>
    bps >= 1e6 ? (bps / 1e6).toFixed(1) + 'M' : bps >= 1e3 ? (bps / 1e3).toFixed(0) + 'K' : bps.toFixed(0)

  // x 轴刻度：取 4 个均匀的时间点
  const ticks = [0, 1, 2, 3].map((i) => t0 + ((t1 - t0) * i) / 3)
  const fmtTick = (t: number) =>
    new Date(t * 1000).toLocaleString([], { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })

  return (
    <div>
      <div className="mb-1 flex items-center gap-4 text-[11px] text-muted">
        <span className="flex items-center gap-1">
          <span className="inline-block h-2 w-2 rounded-full bg-fill" /> 入站速率
        </span>
        <span className="flex items-center gap-1">
          <span className="inline-block h-2 w-2 rounded-full bg-violet-400" /> 出站速率
        </span>
        <span className="ml-auto text-muted">
          峰值 {fmtRate(peak)}bps
        </span>
      </div>
      <svg viewBox={`0 0 ${W} ${H}`} className="w-full" role="img" aria-label="流量速率趋势">
        {[0, 0.25, 0.5, 0.75, 1].map((f) => (
          <g key={f}>
            <line x1={PAD_L} x2={W - 8} y1={y(nicePeak * f)} y2={y(nicePeak * f)} stroke="#e2e8f0" strokeWidth="0.5" />
            <text x={PAD_L - 4} y={y(nicePeak * f)} textAnchor="end" dominantBaseline="central" fontSize="9" fill="#94a3b8">
              {fmtRate(nicePeak * f)}
            </text>
          </g>
        ))}
        {/* 出站画在下层、入站在上层；线宽 1.2 两个桶之间就有连续感 */}
        <path d={line((p) => p.outBps)} fill="none" stroke="#a78bfa" strokeWidth="1.2" strokeLinejoin="round" />
        <path d={line((p) => p.inBps)} fill="none" stroke="#94a3b8" strokeWidth="1.2" strokeLinejoin="round" />
        {ticks.map((t, i) => (
          <text key={i} x={x(t)} y={H - 4} textAnchor={i === 0 ? 'start' : i === 3 ? 'end' : 'middle'} fontSize="9" fill="#94a3b8">
            {fmtTick(t)}
          </text>
        ))}
      </svg>
    </div>
  )
}
