import { useEffect, useState } from 'react'
import { Activity, Ban, Globe2, ShieldAlert, Timer, Users } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import * as api from '../lib/api'
import type { AuditOverview as OverviewData, AuditTrendPoint } from '../types'

/**
 * 审计页顶部那一整块：指标卡 + 趋势图 + 攻击类型 + 地区分布 + 站点。
 *
 * 为什么把五个东西塞在一个文件里：它们的数据来自同一个接口
 * （/api/audit/overview），也总是一起刷新。拆成五个组件各自拉数据，
 * 会出现「指标卡是这一分钟的、趋势图是上一分钟的」这种对不上的画面。
 *
 * 图表全是手写 SVG，不引图表库：
 * 项目里仪表盘的流量曲线也是这么画的，多引一个 100KB+ 的库只为两条折线不划算，
 * 而且图表库的默认样式跟面板不搭，调起来比自己画还费事。
 */

interface Props {
  /** 时间范围（小时）。1=最近 24 小时，7=最近 7 天 */
  hours: number
  /** 点某个地区时把关键词带进 IP 列表 */
  onPickCountry: (name: string) => void
}

export function AuditOverview({ hours, onPickCountry }: Props) {
  const [data, setData] = useState<OverviewData | null>(null)
  const [err, setErr] = useState('')

  useEffect(() => {
    let alive = true
    api
      .getAuditOverview(hours)
      .then((d) => {
        if (alive) {
          setData(d)
          setErr('')
        }
      })
      .catch((e) => {
        if (alive) setErr(e instanceof Error ? e.message : '加载失败')
      })
    return () => {
      alive = false
    }
  }, [hours])

  if (err) {
    return (
      <Card>
        <div className="text-sm text-rose-600">{err}</div>
      </Card>
    )
  }

  if (!data) {
    return (
      <Card>
        <div className="py-8 text-center text-sm text-muted">正在统计…</div>
      </Card>
    )
  }

  const ov = data.overview
  const maxKind = Math.max(1, ...data.attackKinds.map((k) => k.count))
  const maxCountry = Math.max(1, ...data.countries.map((c) => c.requests))

  return (
    <div className="space-y-4">
      {/* 指标卡 */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 sm:grid-cols-3 xl:grid-cols-6">
        <Stat icon={<Activity className="h-3.5 w-3.5" />} label="请求次数" value={fmt(ov.requests)} />
        <Stat icon={<Users className="h-3.5 w-3.5" />} label="独立 IP" value={fmt(ov.uniqueIPs)} />
        <Stat
          icon={<Ban className="h-3.5 w-3.5" />}
          label="拦截次数"
          value={fmt(ov.blocked)}
          tone={ov.blocked > 0 ? 'warn' : undefined}
        />
        <Stat
          icon={<ShieldAlert className="h-3.5 w-3.5" />}
          label="错误率"
          value={`${(ov.errorRatio * 100).toFixed(1)}%`}
          tone={ov.errorRatio >= 0.5 ? 'bad' : undefined}
        />
        <Stat
          icon={<ShieldAlert className="h-3.5 w-3.5" />}
          label="攻击 IP"
          value={fmt(ov.attackIPs)}
          tone={ov.attackIPs > 0 ? 'bad' : undefined}
        />
        <Stat icon={<Timer className="h-3.5 w-3.5" />} label="平均耗时" value={`${ov.avgDurMs}ms`} />
      </div>

      <div className="grid gap-4 lg:grid-cols-[1.5fr_1fr]">
        {/* 趋势 */}
        <Card className="!p-0">
          <div className="px-5 pt-4">
            <CardHeader
              title="请求与拦截趋势"
              action={
                <div className="flex items-center gap-3 text-[11px] text-muted">
                  <span className="flex items-center gap-1">
                    <span className="h-2 w-2 rounded-sm bg-blue-400" />
                    请求
                  </span>
                  <span className="flex items-center gap-1">
                    <span className="h-2 w-2 rounded-sm bg-rose-500" />
                    错误
                  </span>
                </div>
              }
            />
          </div>
          <div className="px-3 pb-3">
            <TrendChart points={data.trends} />
          </div>
        </Card>

        {/* 地区 */}
        <Card className="!p-0">
          <div className="px-5 pt-4">
            <CardHeader
              title="访问来源地区"
              action={
                <span className="text-[11px] text-muted">
                  {data.geoEnabled ? '按累计请求排序' : '归属地库未加载'}
                </span>
              }
            />
          </div>
          <div className="px-5 pb-4">
            {data.countries.length === 0 ? (
              <div className="rounded-xl bg-surface-2 px-3 py-6 text-center text-xs text-muted">
                还没有访问记录
              </div>
            ) : (
              <div className="space-y-2">
                {data.countries.map((c) => (
                  <button
                    key={c.name}
                    onClick={() => onPickCountry(c.name === '内网/未知' ? '' : c.name)}
                    className="group flex w-full items-center gap-2 text-left"
                    title={`${c.name}：${c.requests} 次请求，${c.ips} 个 IP`}
                  >
                    <span className="w-20 shrink-0 truncate text-xs text-content group-hover:text-blue-600">
                      {c.name}
                    </span>
                    <span className="h-1.5 flex-1 overflow-hidden rounded-full bg-blue-50">
                      <span
                        className="block h-full rounded-full bg-blue-400 transition-all"
                        style={{ width: `${Math.max(2, (c.requests / maxCountry) * 100)}%` }}
                      />
                    </span>
                    <span className="w-12 shrink-0 text-right font-mono text-[11px] text-muted">
                      {fmt(c.requests)}
                    </span>
                  </button>
                ))}
              </div>
            )}
          </div>
        </Card>
      </div>

      <div className="grid gap-4 lg:grid-cols-[1fr_1.5fr]">
        {/* 攻击类型 */}
        <Card className="!p-0">
          <div className="px-5 pt-4">
            <CardHeader title="攻击类型" />
          </div>
          <div className="px-5 pb-4">
            {data.attackKinds.length === 0 ? (
              <div className="rounded-xl bg-surface-2 px-3 py-6 text-center text-xs text-muted">
                这段时间没有异常请求
              </div>
            ) : (
              <div className="space-y-2">
                {data.attackKinds.map((k) => (
                  <div key={k.kind} className="flex items-center gap-2">
                    <span className="w-24 shrink-0 text-xs text-content">{k.label}</span>
                    <span className="h-1.5 flex-1 overflow-hidden rounded-full bg-surface-2">
                      <span
                        className={`block h-full rounded-full ${
                          k.kind === 'other' || k.kind === 'blocked' ? 'bg-fill' : 'bg-rose-400'
                        }`}
                        style={{ width: `${Math.max(2, (k.count / maxKind) * 100)}%` }}
                      />
                    </span>
                    <span className="w-12 shrink-0 text-right font-mono text-[11px] text-muted">
                      {fmt(k.count)}
                    </span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </Card>

        {/* 站点 */}
        <Card className="!p-0">
          <div className="px-5 pt-4">
            <CardHeader title="被访问最多的站点" />
          </div>
          <div className="px-5 pb-4">
            {data.sites.length === 0 ? (
              <div className="rounded-xl bg-surface-2 px-3 py-6 text-center text-xs text-muted">
                还没有访问记录
              </div>
            ) : (
              <div className="space-y-1.5">
                {data.sites.map((s) => (
                  <div
                    key={s.site}
                    className="flex items-center gap-2 rounded-lg px-2 py-1.5 hover:bg-surface-2"
                  >
                    <Globe2 className="h-3 w-3 shrink-0 text-muted" />
                    <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-content">
                      {s.site}
                    </span>
                    <span className="shrink-0 text-xs font-semibold text-content">{fmt(s.count)}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </Card>
      </div>
    </div>
  )
}

const toneCls = {
  warn: 'text-amber-600',
  bad: 'text-rose-600',
} as const

function Stat({
  icon,
  label,
  value,
  tone,
}: {
  icon: React.ReactNode
  label: string
  value: string
  tone?: keyof typeof toneCls
}) {
  return (
    <div className="rounded-xl bg-surface-2 px-3 py-2.5 ring-1 ring-line/60">
      <div className="flex items-center gap-1 text-[11px] text-muted">
        {icon}
        {label}
      </div>
      <div className={`mt-0.5 text-xl font-bold ${tone ? toneCls[tone] : 'text-content'}`}>
        {value}
      </div>
    </div>
  )
}

/**
 * 趋势折线。
 *
 * 两条线共用一套纵轴（按两者的最大值归一）。分轴看起来更"专业"，
 * 但读者会比错高度 —— 一眼看到"红线在蓝线上面"就以为错误比请求多。
 */
function TrendChart({ points }: { points: AuditTrendPoint[] }) {
  const W = 640
  const H = 140
  const PAD = 6

  if (points.length === 0) {
    return <div className="py-10 text-center text-xs text-muted">暂无数据</div>
  }

  const max = Math.max(1, ...points.map((p) => Math.max(p.requests, p.errors)))
  const stepX = points.length > 1 ? (W - PAD * 2) / (points.length - 1) : 0
  const y = (v: number) => H - PAD - (v / max) * (H - PAD * 2)
  const line = (pick: (p: AuditTrendPoint) => number) =>
    points.map((p, i) => `${i === 0 ? 'M' : 'L'}${PAD + i * stepX},${y(pick(p))}`).join(' ')

  // 只标首、中、尾三个时间点：24 个标签全标会糊成一片
  const labels = [points[0], points[Math.floor(points.length / 2)], points[points.length - 1]]

  return (
    <div>
      <svg viewBox={`0 0 ${W} ${H}`} className="w-full" role="img" aria-label="请求量与错误量的小时趋势">
        {/* 三条水平参考线，给个高度感 */}
        {[0.25, 0.5, 0.75].map((r) => (
          <line
            key={r}
            x1={PAD}
            x2={W - PAD}
            y1={PAD + r * (H - PAD * 2)}
            y2={PAD + r * (H - PAD * 2)}
            stroke="#e2e8f0"
            strokeWidth="0.5"
            strokeDasharray="2 3"
          />
        ))}
        <path d={line((p) => p.requests)} fill="none" stroke="#60a5fa" strokeWidth="1.6" strokeLinejoin="round" />
        <path d={line((p) => p.errors)} fill="none" stroke="#f43f5e" strokeWidth="1.6" strokeLinejoin="round" />
      </svg>
      <div className="flex justify-between px-1 text-[10px] text-muted">
        {labels.map((p, i) => (
          <span key={i}>{p.label}</span>
        ))}
      </div>
    </div>
  )
}

/** 大数字缩写：72.5k 比 72531 好读，也不占宽度 */
function fmt(n: number): string {
  if (n >= 1e8) return `${(n / 1e8).toFixed(1)}亿`
  if (n >= 1e4) return `${(n / 1e4).toFixed(1)}万`
  if (n >= 1000) return `${(n / 1000).toFixed(1)}k`
  return String(n)
}
