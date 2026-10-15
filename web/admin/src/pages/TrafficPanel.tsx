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
import type { TrafficDailyTotals, TrafficRealtime, TrafficTrend } from '../types'

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

  const loadTrend = useCallback(async () => {
    try {
      setTrend(await api.getTrafficTrend(7))
    } catch {
      // 趋势拿不到不该影响实时区，静默
    }
  }, [])

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
  }, [loadRealtime, loadTrend])

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
                <span className="text-[11px] text-slate-400">近 1 分钟</span>
                <button
                  onClick={() => setPaused((p) => !p)}
                  title={paused ? '继续刷新' : '暂停刷新（看瞬时值时用）'}
                  className={`flex items-center gap-1 rounded-lg px-2 py-1 text-xs font-semibold transition ${
                    paused ? 'bg-amber-50 text-amber-600' : 'text-slate-500 hover:bg-slate-100'
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
          <div className="px-5 pt-4">
            <CardHeader title="近 7 天流量" />
          </div>
          <div className="px-5 pb-4">
            <DailyBars days={trend?.days ?? []} />
          </div>
        </Card>

        <Card className="!p-0">
          <div className="px-5 pt-4">
            <CardHeader title="各服务流量" />
          </div>
          <div className="px-5 pb-4">
            <SiteBars bySite={trend?.bySite ?? {}} />
          </div>
        </Card>
      </div>

      {rt?.totals.since ? (
        <div className="px-1 text-[11px] text-slate-400">
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
      <div className="flex items-center gap-1 text-xs text-slate-500">
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
    return <div className="flex h-[90px] items-center justify-center text-xs text-slate-400">等待数据…</div>
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
      <div className="mt-1 flex items-center justify-between text-[11px] text-slate-400">
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
function DailyBars({ days }: { days: TrafficDailyTotals[] }) {
  if (days.length === 0) {
    return <div className="flex h-[110px] items-center justify-center text-xs text-slate-400">暂无历史数据</div>
  }

  const W = 280
  const H = 100
  const slot = W / days.length
  const barW = Math.min(9, slot / 3)
  const gap = 3

  let peak = 1
  for (const d of days) peak = Math.max(peak, d.in, d.out)

  const h = (v: number) => (v / peak) * (H - 18)

  return (
    <div>
      <svg viewBox={`0 0 ${W} ${H}`} width="100%" height={H} role="img">
        <title>近 {days.length} 天入站与出站流量</title>
        {days.map((d, i) => {
          const cx = i * slot + slot / 2
          const hi = h(d.in)
          const ho = h(d.out)
          return (
            <g key={d.date}>
              <rect x={cx - barW - gap / 2} y={H - 16 - hi} width={barW} height={hi} fill="#378ADD" rx={1.5} />
              <rect x={cx + gap / 2} y={H - 16 - ho} width={barW} height={ho} fill="#639922" rx={1.5} />
            </g>
          )
        })}
      </svg>
      <div className="flex justify-between text-[11px] text-slate-400">
        <span>{days[0]?.date.slice(5)}</span>
        <span>峰值 {fmtBytes(peak)}</span>
        <span>{days[days.length - 1]?.date.slice(5)}</span>
      </div>
    </div>
  )
}

/**
 * 各服务流量横条。
 *
 * 站点名要从 proxy 配置里查（bySite 的键是站点 ID）—— 但这里不额外请求，
 * 拿不到名字就直接显示 `站点 #3`：这是**少一次请求**与"名字好看"的取舍，
 * 而流量数字本身才是这一块的重点。
 */
function SiteBars({ bySite }: { bySite: Record<string, { in: number; out: number; requests: number }> }) {
  const rows = Object.entries(bySite)
    .map(([id, t]) => ({ id, total: t.in + t.out, requests: t.requests }))
    .sort((a, b) => b.total - a.total)
    .slice(0, 6)

  if (rows.length === 0) {
    return <div className="flex h-[110px] items-center justify-center text-xs text-slate-400">暂无站点流量</div>
  }

  const peak = rows[0].total || 1

  return (
    <div className="space-y-2.5">
      {rows.map((r) => (
        <div key={r.id}>
          <div className="mb-1 flex items-baseline justify-between text-xs">
            <span className="text-slate-600">
              {r.id === '0' ? '未匹配站点' : `站点 #${r.id}`}
              <span className="ml-2 text-[11px] text-slate-400">{fmtCount(r.requests)} 次</span>
            </span>
            <span className="tabular-nums text-slate-500">{fmtBytes(r.total)}</span>
          </div>
          <div className="h-1.5 overflow-hidden rounded-full bg-slate-100">
            <div
              className="h-full rounded-full bg-blue-500"
              style={{ width: `${Math.max((r.total / peak) * 100, 2)}%` }}
            />
          </div>
        </div>
      ))}
    </div>
  )
}

/** 指标卡 */
function Metric({ label, value }: { label: string; value: string }) {
  return (
    <div className="rounded-xl bg-slate-50 px-4 py-3">
      <div className="text-xs text-slate-500">{label}</div>
      <div className="mt-0.5 text-xl font-semibold tabular-nums text-slate-700">{value}</div>
    </div>
  )
}

/** 大数字（万 / 亿），请求数用它比原始数字好读 */
function fmtCount(n: number): string {
  if (n >= 1e8) return `${(n / 1e8).toFixed(1)} 亿`
  if (n >= 1e4) return `${(n / 1e4).toFixed(1)} 万`
  return String(n)
}
