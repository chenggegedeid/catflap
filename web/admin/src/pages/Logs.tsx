/**
 * 日志页。
 *
 * 形态：**分类折叠，折叠态只给数字**。
 *
 * 为什么这样设计：日志页最常见的用法不是"从头读到尾"，而是"哪儿出问题了"。
 * 一屏摊开几千行日志谁也读不完，而按分类折叠之后，一眼就能看出哪个模块在闹
 * （异常数用颜色标出来），再点开那个分类看详情。
 *
 * 两个接口分工：
 *   - log/summary：只回各模块的条数与异常数，**不读内容** —— 折叠态用它，
 *     否则为了显示"穿透 1204 条"要先下载 2MB 日志，打开页面就卡一下
 *   - log：展开某个分类时才拉
 */
import { useCallback, useEffect, useState } from 'react'
import { ChevronDown, ChevronRight, RotateCw, Search, TriangleAlert } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import * as api from '../lib/api'
import type { LogEntry, LogModuleStat, LogSummary } from '../types'

const levelStyle: Record<string, string> = {
  error: 'bg-rose-50 text-rose-600',
  fatal: 'bg-rose-50 text-rose-600',
  panic: 'bg-rose-50 text-rose-600',
  warning: 'bg-amber-50 text-amber-600',
  warn: 'bg-amber-50 text-amber-600',
  info: 'bg-blue-50 text-blue-600',
  debug: 'bg-surface-2 text-muted',
  trace: 'bg-surface-2 text-muted',
}

const levelFilters = [
  { value: '', label: '全部' },
  { value: 'error', label: '错误' },
  { value: 'warning', label: '警告' },
  { value: 'info', label: '信息' },
]

/** 模块名 → 界面上的中文分类名 */
const moduleLabels: Record<string, string> = {
  stun: '穿透',
  proxy: '反向代理',
  tunnel: 'Tunnel',
  ddns: '域名解析',
  cert: '证书',
  notify: '通知',
  traffic: '流量',
  home: '导航页',
  auth: '安全',
  task: '计划任务',
  system: '系统',
  update: '更新',
  wol: '网络唤醒',
  core: '系统与框架',
  router: '系统与框架',
  other: '其它',
}

const moduleLabel = (m: string) => moduleLabels[m] ?? m

export function Logs() {
  const [days, setDays] = useState<string[]>([])
  const [day, setDay] = useState('')
  const [file, setFile] = useState<'info' | 'err'>('info')
  const [summary, setSummary] = useState<LogSummary | null>(null)

  // 展开的分类（同时只开一个：看日志通常是顺着一条线看）
  const [expanded, setExpanded] = useState('')
  const [rows, setRows] = useState<LogEntry[]>([])
  const [level, setLevel] = useState('')
  const [keyword, setKeyword] = useState('')
  // 输入框和真正发出去的关键字分开，不然每敲一个字都打一次接口
  const [search, setSearch] = useState('')
  const [truncated, setTruncated] = useState(false)
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState('')

  useEffect(() => {
    api
      .getLogDays()
      .then((d) => {
        setDays(d)
        setDay((cur) => cur || d[0] || '')
      })
      .catch((e) => setErr(e instanceof Error ? e.message : String(e)))
  }, [])

  // 摘要：折叠态的数字
  const loadSummary = useCallback(async () => {
    if (!day) return
    try {
      setSummary(await api.getLogSummary({ day, file }))
      setErr('')
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
      setSummary(null)
    }
  }, [day, file])

  useEffect(() => {
    void loadSummary()
  }, [loadSummary])

  // 详情：展开某个分类时才拉
  const loadRows = useCallback(async () => {
    if (!day) return
    setLoading(true)
    try {
      const r = await api.getLogs({ day, file, level, keyword: search, limit: 500 })
      setRows(r.list || [])
      setTruncated(r.truncated)
      setErr('')
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
      setRows([])
    } finally {
      setLoading(false)
    }
  }, [day, file, level, search])

  useEffect(() => {
    if (!expanded) {
      setRows([])
      return
    }
    void loadRows()
  }, [expanded, loadRows])

  const refresh = () => {
    void loadSummary()
    if (expanded) void loadRows()
  }

  const toggle = (m: string) => setExpanded((cur) => (cur === m ? '' : m))

  // 展开的是哪个分类，就只显示那个分类的（后端若已按 module 过滤则这里不会筛掉东西）
  const visibleRows = expanded ? rows.filter((e) => (e.module || 'other') === expanded) : []

  return (
    <div className="space-y-5">
      <Card className="!p-0">
        <div className="px-5 pt-4">
          <div className="mb-1 text-xs text-muted">按分类折叠，只显示异常数量；点开看详情</div>
          <CardHeader
            title="日志"
            action={
              <div className="flex items-center gap-2">
                <select
                  value={day}
                  onChange={(e) => {
                    setDay(e.target.value)
                    setExpanded('')
                  }}
                  className="rounded-lg border border-line bg-surface px-2 py-1 text-xs outline-none focus:border-blue-400"
                >
                  {days.length === 0 && <option value="">暂无日志</option>}
                  {days.map((d) => (
                    <option key={d} value={d}>
                      {d}
                    </option>
                  ))}
                </select>
                <select
                  value={file}
                  onChange={(e) => {
                    setFile(e.target.value as 'info' | 'err')
                    setExpanded('')
                  }}
                  title="「仅错误」是错误及以上级别的副本"
                  className="rounded-lg border border-line bg-surface px-2 py-1 text-xs outline-none focus:border-blue-400"
                >
                  <option value="info">全部</option>
                  <option value="err">仅错误</option>
                </select>
                <button
                  onClick={refresh}
                  className="flex items-center gap-1 rounded-lg px-2 py-1 text-xs font-semibold text-muted hover:bg-surface-2"
                >
                  <RotateCw className={`h-3 w-3 ${loading ? 'animate-spin' : ''}`} /> 刷新
                </button>
              </div>
            }
          />
        </div>

        {/* 总计一行 */}
        {summary && summary.total > 0 && (
          <div className="flex flex-wrap items-center gap-x-4 gap-y-1 px-5 pb-2 text-xs text-muted">
            <span>
              共 <span className="font-semibold text-content">{summary.total}</span> 条
            </span>
            {summary.warn > 0 && <span className="text-amber-600">警告 {summary.warn}</span>}
            {summary.error > 0 && <span className="text-rose-600">错误 {summary.error}</span>}
            {summary.truncated && <span className="text-muted">（只统计了最近一段）</span>}
          </div>
        )}

        {/* 分类列表 */}
        <div className="divide-y divide-line border-t border-line">
          {!summary || summary.modules.length === 0 ? (
            <div className="px-5 py-8 text-center text-xs text-muted">{err || '这一天没有日志'}</div>
          ) : (
            summary.modules.map((m) => (
              <ModuleRow
                key={m.module}
                stat={m}
                open={expanded === m.module}
                onToggle={() => toggle(m.module)}
                rows={visibleRows}
                loading={loading && expanded === m.module}
                truncated={truncated}
                level={level}
                onLevelChange={setLevel}
                search={search}
                onSearchChange={setSearch}
                keyword={keyword}
                onSearchSubmit={() => setKeyword(search)}
              />
            ))
          )}
        </div>
      </Card>
    </div>
  )
}

/**
 * 一个分类的行。折叠时只有一行摘要，展开后才显示日志内容与筛选。
 *
 * 筛选控件放在展开区里而不是页面顶部：它们是"看某个分类详情"时才需要的，
 * 摊在顶上会让人以为筛选是全局的。
 */
function ModuleRow({
  stat,
  open,
  onToggle,
  rows,
  loading,
  truncated,
  level,
  onLevelChange,
  search,
  onSearchChange,
  keyword,
  onSearchSubmit,
}: {
  stat: LogModuleStat
  open: boolean
  onToggle: () => void
  rows: LogEntry[]
  loading: boolean
  truncated: boolean
  level: string
  onLevelChange: (v: string) => void
  search: string
  onSearchChange: (v: string) => void
  keyword: string
  onSearchSubmit: () => void
}) {
  const hasIssue = stat.warn + stat.error > 0

  return (
    <div>
      <button
        type="button"
        onClick={onToggle}
        className="flex w-full items-center gap-3 px-5 py-3 text-left transition hover:bg-surface-2"
      >
        {open ? (
          <ChevronDown className="h-3.5 w-3.5 shrink-0 text-muted" />
        ) : (
          <ChevronRight className="h-3.5 w-3.5 shrink-0 text-muted" />
        )}
        <span className="w-28 shrink-0 text-sm font-semibold text-content">{moduleLabel(stat.module)}</span>
        <span className="text-xs text-muted">{stat.total} 条</span>

        <span className="ml-auto flex items-center gap-3 text-xs">
          {stat.warn > 0 && <span className="text-amber-600">⚠ {stat.warn}</span>}
          {stat.error > 0 && <span className="font-semibold text-rose-600">✕ {stat.error}</span>}
          {!hasIssue && <span className="text-muted">正常</span>}
        </span>
      </button>

      {open && (
        <div className="bg-surface-2/60 px-5 pb-4 pt-1">
          <div className="mb-2 flex flex-wrap items-center gap-2">
            <div className="flex gap-1">
              {levelFilters.map((f) => (
                <button
                  key={f.value}
                  type="button"
                  onClick={() => onLevelChange(f.value)}
                  className={`rounded-md px-2 py-0.5 text-[11px] font-semibold transition ${
                    level === f.value ? 'bg-blue-500 text-white' : 'bg-surface text-content hover:bg-surface-2'
                  }`}
                >
                  {f.label}
                </button>
              ))}
            </div>
            <div className="relative ml-auto">
              <Search className="pointer-events-none absolute left-2 top-1/2 h-3 w-3 -translate-y-1/2 text-muted" />
              <input
                value={search}
                onChange={(e) => onSearchChange(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') onSearchSubmit()
                }}
                placeholder="搜索内容，回车"
                className="w-44 rounded-lg border border-line bg-surface py-1 pl-7 pr-2 text-xs outline-none focus:border-blue-400"
              />
            </div>
          </div>

          {loading ? (
            <div className="py-6 text-center text-xs text-muted">读取中…</div>
          ) : rows.length === 0 ? (
            <div className="py-6 text-center text-xs text-muted">
              没有匹配的日志{keyword && `（关键字：${keyword}）`}
            </div>
          ) : (
            <>
              <div className="max-h-[26rem] space-y-0.5 overflow-y-auto rounded-lg bg-surface p-2 ring-1 ring-line/70">
                {rows.map((e, i) => (
                  <div
                    key={i}
                    className="flex items-start gap-2 rounded px-1.5 py-1 font-mono text-[11px] hover:bg-surface-2"
                  >
                    <span className="shrink-0 text-muted">{e.time.slice(11)}</span>
                    <span
                      className={`shrink-0 rounded px-1 text-[10px] font-semibold uppercase ${
                        levelStyle[e.level] ?? 'bg-surface-2 text-muted'
                      }`}
                    >
                      {e.level}
                    </span>
                    <span className="whitespace-pre-wrap break-all text-content">{e.message}</span>
                  </div>
                ))}
              </div>
              {truncated && (
                <div className="mt-1.5 flex items-center gap-1 text-[11px] text-amber-600">
                  <TriangleAlert className="h-3 w-3" />
                  日志较大，只显示最近一段
                </div>
              )}
            </>
          )}
        </div>
      )}
    </div>
  )
}
