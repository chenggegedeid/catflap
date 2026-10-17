/**
 * 访问审计与 IP 管理。
 *
 * 意图：把"谁在访问我的服务、谁在扫我"变成能查、能处置的东西。
 *
 * 页面分两块：
 *   - IP 列表：按错误数排序（扫描器的错误数会高得离谱），可疑的标红，
 *     每行能一键拉黑
 *   - 封禁与设置：当前封禁表、白名单、自动拉黑开关
 *
 * 一个刻意的取舍：**列表默认按错误数排，不是按请求数**。
 * 按请求数排最上面永远是自己家的常用服务；按错误数排最上面就是扫描器。
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Ban, Check, RotateCw, Search, ShieldOff, TriangleAlert } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import { AuditOverview } from './AuditOverview'
import { Pager } from '../components/Pager'
import { getPageLimit, setPageLimit } from '../lib/prefs'
import * as api from '../lib/api'
import type { AuditConfigView, AuditIPStat, AuditRecord, LoginEvent, LoginSummary } from '../types'

const fmtBytes = (n: number): string => {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024
    i++
  }
  return `${v < 10 && i > 0 ? v.toFixed(1) : v.toFixed(0)} ${u[i]}`
}

const fmtCount = (n: number): string => {
  if (n >= 1e8) return `${(n / 1e8).toFixed(1)} 亿`
  if (n >= 1e4) return `${(n / 1e4).toFixed(1)} 万`
  return String(n)
}

/** 封禁时长选项。0 = 永久 —— 放在最后，因为它最需要想清楚 */
const durations = [
  { label: '1 小时', minutes: 60 },
  { label: '1 天', minutes: 60 * 24 },
  { label: '7 天', minutes: 60 * 24 * 7 },
  { label: '永久', minutes: 0 },
]

/**
 * 改每页条数：先记进 localStorage，再更新组件状态。
 *
 * 顺序不能反 —— 也正因为以前压根没存，才会出现「调到 10、刷新又变 50」。
 */
function applyLimit(setter: (n: number) => void) {
  return (n: number) => {
    setPageLimit(n)
    setter(n)
  }
}

export function Audit() {
  const [cfg, setCfg] = useState<AuditConfigView | null>(null)
  const [ips, setIps] = useState<AuditIPStat[]>([])
  const [sort, setSort] = useState<'errors' | 'requests' | 'recent'>('errors')
  const [onlySuspicious, setOnlySuspicious] = useState(false)
  const [keyword, setKeyword] = useState('')
  const [loading, setLoading] = useState(false)
  const [toast, setToast] = useState('')
  const [err, setErr] = useState('')

  // 分页。被扫的时候 IP 数能到几千，全塞进一屏既卡又没法看
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(getPageLimit)
  const [total, setTotal] = useState(0)
  // 归属地库没加载成功时把这一列藏起来，而不是摆一列「未知」
  const [geoEnabled, setGeoEnabled] = useState(true)
  // 趋势图与指标卡的时间范围
  const [hours, setHours] = useState(24)
  // 访问来源按哪天看。空 = 累计（跨天累加）；有值 = 只算那一天
  const [ipDay, setIpDay] = useState('')
  const [ipDays, setIpDays] = useState<string[]>([])

  // 拉黑弹窗
  const [blockModal, setBlockModal] = useState<{ open: boolean; ips: string[] }>({ open: false, ips: [] })
  // 展开某个 IP 的访问记录
  const [detailIP, setDetailIP] = useState('')
  const [records, setRecords] = useState<AuditRecord[]>([])

  const say = (m: string) => {
    setToast(m)
    window.setTimeout(() => setToast(''), 2800)
  }

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const [c, list] = await Promise.all([
        api.getAuditConfig(),
        // 关键词交给后端：归属地和攻击类型不在前端拿到的原始数据里，
        // 前端自己筛只能筛 IP，「搜归属地」这种用法就没了
        api.getAuditIPs({
          sort,
          suspicious: onlySuspicious,
          limit,
          page,
          q: keyword.trim(),
          day: ipDay,
        }),
      ])
      setCfg(c)
      setIps(list.list ?? [])
      setTotal(list.total ?? 0)
      setIpDays(list.days ?? [])
      setGeoEnabled(list.geoEnabled !== false)
      setErr('')
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [sort, onlySuspicious, limit, page, keyword, ipDay])

  useEffect(() => {
    void refresh()
  }, [refresh])

  // 改了筛选条件就回到第一页：否则会停在一个已经不存在的页码上，
  // 看到的是空白，而数据其实有
  useEffect(() => {
    setPage(1)
  }, [sort, onlySuspicious, keyword, limit, ipDay])

  const loadRecords = async (ip: string) => {
    try {
      const r = await api.getAuditRecords({ ip, limit: 200 })
      setRecords(r.list ?? [])
    } catch {
      setRecords([])
    }
  }

  const toggleDetail = (ip: string) => {
    if (detailIP === ip) {
      setDetailIP('')
      return
    }
    setDetailIP(ip)
    setRecords([])
    void loadRecords(ip)
  }

  const doBlock = async (ipsToBlock: string[], reason: string, minutes: number) => {
    try {
      if (ipsToBlock.length === 1) {
        await api.blockIP({ ip: ipsToBlock[0], reason, minutes })
        say(`已封禁 ${ipsToBlock[0]}`)
      } else {
        const r = await api.blockIPBatch({ ips: ipsToBlock, reason, minutes })
        const failed = Object.keys(r.failed ?? {})
        say(failed.length > 0 ? `已封禁 ${r.ok} 个，${failed.length} 个失败` : `已封禁 ${r.ok} 个地址`)
      }
      setBlockModal({ open: false, ips: [] })
      await refresh()
    } catch (e) {
      say('封禁失败：' + (e instanceof Error ? e.message : ''))
    }
  }

  const doUnblock = async (ip: string) => {
    try {
      await api.unblockIP(ip)
      say(`已解除 ${ip}`)
      await refresh()
    } catch (e) {
      say('解除失败：' + (e instanceof Error ? e.message : ''))
    }
  }

  // 关键词过滤已经在后端做了（要能按归属地、扫过的路径搜，那些字段前端拿不到）。
  // 这里只留「可疑」的判断，它是纯前端可算的（阈值与后端一致）
  const suspiciousList = useMemo(() => ips.filter((x) => isSuspicious(x)), [ips])

  if (err && !cfg) {
    return (
      <Card>
        <div className="text-sm text-rose-600">{err}</div>
      </Card>
    )
  }

  return (
    <div className="space-y-5">
      {/* 时间范围：趋势图和指标卡跟着它走 */}
      <div className="flex items-center justify-end">
        <select
          value={hours}
          onChange={(e) => setHours(Number(e.target.value))}
          className="rounded-lg border border-line bg-surface px-2 py-1 text-xs outline-none focus:border-blue-400"
        >
          <option value={1}>最近 1 小时</option>
          <option value={6}>最近 6 小时</option>
          <option value={24}>最近 24 小时</option>
          <option value={168}>最近 7 天</option>
        </select>
      </div>

      {/* 指标卡 + 趋势 + 攻击类型 + 地区 + 站点 */}
      <AuditOverview hours={hours} onPickCountry={(name) => setKeyword(name)} />

      {/* 记录概况（累计口径，和上面「最近 N 小时」是两回事） */}
      <div className="grid grid-cols-3 gap-3">
        <Metric label="已记录 IP（累计）" value={String(cfg?.totalIPs ?? 0)} />
        <Metric
          label="可疑 IP"
          value={String(cfg?.suspiciousCount ?? 0)}
          tone={(cfg?.suspiciousCount ?? 0) > 0 ? 'danger' : undefined}
        />
        <Metric label="当前封禁" value={String(cfg?.blocks.length ?? 0)} />
      </div>

      {/* 可疑提示条：有事的时候要在最上面说清 */}
      {suspiciousList.length > 0 && (
        <Card className="!py-3">
          <div className="flex flex-wrap items-center gap-3">
            <TriangleAlert className="h-4 w-4 shrink-0 text-rose-500" />
            <span className="text-sm text-content">
              有 <span className="font-semibold text-rose-600">{suspiciousList.length}</span> 个 IP
              在密集试错（大量 4xx），像是在扫端口或猜路径
            </span>
            <button
              onClick={() => setBlockModal({ open: true, ips: suspiciousList.map((x) => x.ip) })}
              className="ml-auto rounded-lg bg-rose-500 px-3 py-1 text-xs font-semibold text-white hover:bg-rose-600"
            >
              一键全部拉黑
            </button>
          </div>
        </Card>
      )}

      {/* IP 列表 */}
      <Card className="!p-0">
        <div className="px-5 pt-4">
          <CardHeader
            title="访问来源"
            action={
              <div className="flex flex-wrap items-center gap-2">
                {/*
                  按日期看：默认看的是**累计**值（跨天累加），
                  选了某天就只算那天的明细 —— 和登录尝试那个下拉一个意思。
                */}
                <select
                  value={ipDay}
                  onChange={(e) => setIpDay(e.target.value)}
                  className="rounded-lg border border-line bg-surface px-2 py-1 text-xs outline-none focus:border-blue-400"
                  title="按日期查看（默认累计）"
                >
                  <option value="">累计</option>
                  {ipDays.map((d) => (
                    <option key={d} value={d}>
                      {d}
                    </option>
                  ))}
                </select>
                <select
                  value={sort}
                  onChange={(e) => setSort(e.target.value as typeof sort)}
                  className="rounded-lg border border-line bg-surface px-2 py-1 text-xs outline-none focus:border-blue-400"
                >
                  <option value="errors">按异常数</option>
                  <option value="requests">按请求数</option>
                  <option value="recent">按最近访问</option>
                </select>
                <button
                  onClick={() => setOnlySuspicious((v) => !v)}
                  className={`rounded-lg px-2 py-1 text-xs font-semibold transition ${
                    onlySuspicious ? 'bg-rose-50 text-rose-600' : 'text-muted hover:bg-surface-2'
                  }`}
                >
                  只看可疑
                </button>
                <div className="relative">
                  <Search className="pointer-events-none absolute left-2 top-1/2 h-3 w-3 -translate-y-1/2 text-muted" />
                  <input
                    value={keyword}
                    onChange={(e) => setKeyword(e.target.value)}
                    placeholder="搜 IP / 地区 / 路径"
                    className="w-44 rounded-lg border border-line bg-surface py-1 pl-7 pr-2 text-xs outline-none focus:border-blue-400"
                  />
                </div>
                <button
                  onClick={refresh}
                  className="flex items-center gap-1 rounded-lg px-2 py-1 text-xs font-semibold text-muted hover:bg-surface-2"
                >
                  <RotateCw className={`h-3 w-3 ${loading ? 'animate-spin' : ''}`} />
                </button>
              </div>
            }
          />
        </div>

        <div className="px-5 pb-5">
          {ips.length === 0 ? (
            <div className="rounded-xl bg-surface-2 px-4 py-8 text-center text-xs text-muted">
              {keyword
                ? `没有匹配「${keyword}」的记录`
                : onlySuspicious
                  ? '没有可疑 IP —— 挺好'
                  : '还没有访问记录'}
            </div>
          ) : (
            <div className="space-y-1.5">
              {ips.map((x) => (
                <IPRow
                  key={x.ip}
                  stat={x}
                  showGeo={geoEnabled}
                  open={detailIP === x.ip}
                  records={records}
                  onToggle={() => toggleDetail(x.ip)}
                  onBlock={() => setBlockModal({ open: true, ips: [x.ip] })}
                  onUnblock={() => doUnblock(x.ip)}
                />
              ))}
            </div>
          )}

          {/* 分页（三处列表共用同一个组件） */}
          <Pager
            total={total}
            page={page}
            limit={limit}
            onPage={setPage}
            onLimit={applyLimit(setLimit)}
            unit="个 IP"
          />
        </div>
      </Card>

      {/* 登录尝试：和"谁访问了服务"分开 —— 这是"谁在试着进后台" */}
      <LoginPanel />

      {/*
        顺序：设置 → IP 名单（黑名单 | 白名单）。

        - 设置里有「记录访问」总开关，关掉等于关掉整个审计功能，
          用户第一眼要找的是这个，不是先看一个空列表。
        - 黑名单和白名单并排：它们是一对（拦谁 / 不拦谁）。

        **不再单独放一张「封禁列表」** —— 它和黑名单读的是同一份数据
        （config/audit.json 的 Blocks）。分两块展示只会让人以为有两套名单，
        实际手动封禁和自动拉黑都写进同一张表。现在黑名单里全都有，
        并标出每条的原因、来源（手动/自动）和到期时间。
      */}
      <SettingsCard cfg={cfg} onSaved={refresh} onToast={say} />

      <IPLists cfg={cfg} onSaved={refresh} onToast={say} />

      {blockModal.open && (
        <BlockModal
          ips={blockModal.ips}
          whitelist={cfg?.whitelist ?? []}
          onCancel={() => setBlockModal({ open: false, ips: [] })}
          onSubmit={doBlock}
        />
      )}

      {toast && (
        <div className="fixed bottom-6 left-1/2 z-50 -translate-x-1/2 rounded-xl bg-inverse px-4 py-2 text-xs font-medium text-on-inverse shadow-lg">
          {toast}
        </div>
      )}
    </div>
  )
}

/**
 * 可疑判定（与后端 Suspicious() 同一套阈值）。
 *
 * 两边都判是因为后端要用来做自动拉黑与统计，前端要用来标红 ——
 * 前端不重新请求就能标出来，响应更快。阈值不一致的话会出现
 * "界面标红但没被自动拉黑"这种让人困惑的情况，所以保持一致。
 */
function isSuspicious(x: AuditIPStat): boolean {
  if (x.errors < 50 || x.requests === 0) return false
  return x.errors / x.requests >= 0.8
}

function IPRow({
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
function IPLists({
  cfg,
  onSaved,
  onToast,
}: {
  cfg: AuditConfigView | null
  onSaved: () => void
  onToast: (m: string) => void
}) {
  /*
    两个名单的交互刻意做成**一模一样**：填进去回车就存，没有「保存」按钮。
    用户分不出（也不该分）哪个需要手动保存 —— 加一条就生效一条。

    区别只在后端怎么存：
      - 黑名单每条是一次独立封禁（有自己的原因与到期时间），增删走
        Block / Unblock 单条接口；不做整份覆盖，否则会把自动拉黑的条目一起抹掉。
      - 白名单是一份配置数组，只能整份覆盖 —— 所以每次增删都用**新数组**整体保存。
  */

  // 黑名单就是**整张封禁表**：手动加的、自动拉黑的，都在这里。
  // 以前还单独摆一张「封禁列表」，其实读的是同一份 Blocks ——
  // 两块并列会让人以为有两套名单。现在合并，并标出每条的来源与原因。
  const blocks = cfg?.blocks ?? []

  // 黑名单分页：自动拉黑开着的时候条目会一直涨，
  // 不分页的话这一块会把整页撑得老长（和访问来源一个道理）
  const [blackPage, setBlackPage] = useState(1)
  const [blackLimit, setBlackLimit] = useState(getPageLimit)
  useEffect(() => {
    setBlackPage(1)
  }, [blackLimit, blocks.length])
  const blackStart = (blackPage - 1) * blackLimit
  const blackSlice = blocks.slice(blackStart, blackStart + blackLimit)

  const addBlack = async (line: string): Promise<boolean> => {
    try {
      const r = await api.blockIP({ ip: line, reason: '手动添加黑名单', minutes: 0 })
      onToast(r.msg || `已封禁 ${line}`)
      await onSaved()
      return true
    } catch (e) {
      onToast('添加失败：' + (e instanceof Error ? e.message : ''))
      return false
    }
  }

  const removeBlack = async (ip: string): Promise<void> => {
    try {
      const r = await api.unblockIP(ip)
      onToast(r.msg || `已解除 ${ip}`)
      await onSaved()
    } catch (e) {
      onToast('移除失败：' + (e instanceof Error ? e.message : ''))
    }
  }

  // 白名单：拿当前名单算出新数组，整份保存
  const commitWhite = async (next: string[]): Promise<boolean> => {
    try {
      const res = await api.saveAuditWhitelist({
        whitelist: next,
        autoBlock: cfg?.autoBlock ?? false,
        autoBlockThreshold: cfg?.autoBlockThreshold ?? 500,
        autoBlockMinutes: cfg?.autoBlockMinutes ?? 60,
        trustedProxies: cfg?.trustedProxies ?? [],
      })
      onToast(res.msg || '白名单已保存')
      await onSaved()
      return true
    } catch (e) {
      onToast('保存失败：' + (e instanceof Error ? e.message : ''))
      return false
    }
  }

  const addWhite = async (line: string): Promise<boolean> => {
    const cur = cfg?.whitelist ?? []
    if (cur.includes(line)) {
      onToast('已经在名单里了')
      return false
    }
    return commitWhite([...cur, line])
  }

  const removeWhite = async (ip: string): Promise<void> => {
    await commitWhite((cfg?.whitelist ?? []).filter((x) => x !== ip))
  }

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <IPCard
        title="黑名单（全局拦截）"
        hint="这些来源会被直接拦下，对全部站点生效。支持网段，例如 10.0.0.0/8、2409:8a60::/32。
              手动加的和自动拉黑的都在这里 —— 它就是整张封禁表，没有第二份。"
        entries={blackSlice.map((b) => ({
          value: b.ip,
          // 老数据里可能没写原因，给个兜底 —— 用户要靠这一行判断"为什么封的"
          reason: b.reason || (b.auto ? '自动拉黑' : '手动添加'),
          auto: b.auto,
          until: blockUntil(b.expiresAt),
        }))}
        onAdd={addBlack}
        onRemove={removeBlack}
        tone="danger"
        footer={
          <Pager
            total={blocks.length}
            page={blackPage}
            limit={blackLimit}
            onPage={setBlackPage}
            onLimit={applyLimit(setBlackLimit)}
            unit="条"
          />
        }
      />
      <IPCard
        title="白名单（永不封禁）"
        hint="这些来源永不被封，自动拉黑也会绕开。一般填你自己的出口 IP，别把自己关在门外。"
        entries={(cfg?.whitelist ?? []).map((w) => ({ value: w }))}
        onAdd={addWhite}
        onRemove={removeWhite}
        tone="ok"
      />
    </div>
  )
}

/**
 * 封禁的到期时间怎么显示。
 *
 * Go 的零值时间是 0001-01-01，序列化过来是 "0001-01-01T00:00:00Z" ——
 * 字符串**非空**，所以只判空是不够的，界面上会出现「至 0001/1/1」。
 * 零值表示"没设到期"，也就是永久。
 */
function blockUntil(expiresAt?: string): string {
  if (!expiresAt) return '永久'
  const d = new Date(expiresAt)
  if (Number.isNaN(d.getTime()) || d.getUTCFullYear() <= 1) return '永久'
  return `至 ${d.toLocaleString('zh-CN')}`
}

/** IPCard 一份 IP / 网段名单的编辑器（黑白名单共用） */
function IPCard({
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

function SettingsCard({
  cfg,
  onSaved,
  onToast,
}: {
  cfg: AuditConfigView | null
  onSaved: () => void
  onToast: (m: string) => void
}) {
  const [enabled, setEnabled] = useState(cfg?.enabled ?? true)
  const [keepDays, setKeepDays] = useState(String(cfg?.keepDays ?? 30))
  const [maxFileMB, setMaxFileMB] = useState(String(cfg?.maxFileMB ?? 32))
  const [autoBlock, setAutoBlock] = useState(cfg?.autoBlock ?? false)
  const [threshold, setThreshold] = useState(String(cfg?.autoBlockThreshold ?? 500))
  const [minutes, setMinutes] = useState(String(cfg?.autoBlockMinutes ?? 60))
  const [busy, setBusy] = useState(false)

  // cfg 从后端回来时同步进表单（首屏是 null，拿到后要填上）
  useEffect(() => {
    if (!cfg) return
    setEnabled(cfg.enabled)
    setKeepDays(String(cfg.keepDays))
    setMaxFileMB(String(cfg.maxFileMB))
    setAutoBlock(cfg.autoBlock)
    setThreshold(String(cfg.autoBlockThreshold))
    setMinutes(String(cfg.autoBlockMinutes))
  }, [cfg])

  const save = async () => {
    setBusy(true)
    try {
      const res = await api.saveAuditSettings({
        enabled,
        keepDays: Number(keepDays) || 30,
        maxFileMB: Number(maxFileMB) || 32,
        autoBlock,
        autoBlockThreshold: Number(threshold) || 500,
        autoBlockMinutes: Number(minutes) || 60,
        // 白名单已移到下面的「IP 名单」区块，这里原样带回，别把它清空
        whitelist: cfg?.whitelist ?? [],
        trustedProxies: cfg?.trustedProxies ?? [],
      })
      onToast(res.msg || '已保存')
      onSaved()
    } catch (e) {
      onToast('保存失败：' + (e instanceof Error ? e.message : ''))
    } finally {
      setBusy(false)
    }
  }

  const cls = 'w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400'

  return (
    <Card className="!p-0">
      <div className="px-5 pt-4">
        <CardHeader title="审计设置" />
      </div>
      <div className="space-y-3 px-5 pb-5">
        {/*
          审计总开关。

          为什么把它从「一个不起眼的小复选框」提成开关、放在最上面：
          它不是细节选项，而是**整个访问审计功能的开关** —— 关掉之后不再记录
          任何访问，统计、趋势、IP 列表、攻击类型全都停在原地。
          用户要一眼找到它，也要一眼看出它的分量。
        */}
        <div
          className={`flex items-start gap-3 rounded-xl px-3 py-3 ring-1 transition ${
            enabled ? 'bg-emerald-50/60 ring-emerald-200/70' : 'bg-surface-2 ring-line'
          }`}
        >
          <button
            type="button"
            role="switch"
            aria-checked={enabled}
            aria-label="记录访问（访问审计总开关）"
            onClick={() => setEnabled((v) => !v)}
            className={`mt-0.5 flex h-6 w-11 shrink-0 items-center rounded-full px-0.5 transition ${
              enabled ? 'bg-emerald-500' : 'bg-fill'
            }`}
          >
            <span
              className={`h-5 w-5 rounded-full bg-surface shadow transition-transform ${
                enabled ? 'translate-x-5' : 'translate-x-0'
              }`}
            />
          </button>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm font-semibold text-content">记录访问</span>
              <span
                className={`rounded-md px-1.5 py-0.5 text-[10px] font-semibold ${
                  enabled ? 'bg-emerald-100 text-emerald-700' : 'bg-fill text-content'
                }`}
              >
                {enabled ? '审计已开启' : '审计已关闭'}
              </span>
            </div>
            <div className="mt-1 text-[11px] leading-relaxed text-muted">
              {enabled
                ? '正在记录每一次访问：统计、趋势、IP 列表都基于它。关掉之后不再产生新记录。'
                : '已停止记录新访问。已有的记录仍然保留、仍然可以查看和封禁，但统计不再更新。'}
              <br />
              <span className="text-muted">
                关掉它等于关掉整个访问审计功能 —— 这是这一页的总开关。
              </span>
            </div>
          </div>
        </div>

        <div className="grid grid-cols-2 gap-3">
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-muted">明细保留（天）</div>
            <input value={keepDays} onChange={(e) => setKeepDays(e.target.value)} className={cls} />
          </label>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-muted">单天文件上限（MB）</div>
            <input value={maxFileMB} onChange={(e) => setMaxFileMB(e.target.value)} className={cls} />
            <div className="mt-1 text-[11px] text-muted">超了停写，防磁盘被刷满</div>
          </label>
        </div>

        <div className="rounded-xl bg-surface-2 p-3">
          <label className="flex cursor-pointer items-center gap-2 text-xs text-content">
            <input type="checkbox" checked={autoBlock} onChange={(e) => setAutoBlock(e.target.checked)} />
            <span className="font-semibold">自动拉黑可疑 IP</span>
          </label>
          <div className="mt-1 text-[11px] leading-relaxed text-amber-600">
            容易误伤：你自己调试时刷出几百个 404 也会被算成可疑。开启前请把常用地址
            填进白名单（白名单里的地址永不封禁）。
          </div>
          {autoBlock && (
            <div className="mt-2 grid grid-cols-2 gap-3">
              <label className="block">
                <div className="mb-1 text-xs font-semibold text-muted">异常数门槛</div>
                <input value={threshold} onChange={(e) => setThreshold(e.target.value)} className={cls} />
              </label>
              <label className="block">
                <div className="mb-1 text-xs font-semibold text-muted">封禁时长（分钟，0=永久）</div>
                <input value={minutes} onChange={(e) => setMinutes(e.target.value)} className={cls} />
              </label>
            </div>
          )}
        </div>

        <div className="flex justify-end">
          <button
            onClick={save}
            disabled={busy}
            className="flex items-center gap-1 rounded-xl bg-blue-500 px-4 py-2 text-xs font-semibold text-white hover:bg-blue-600 disabled:opacity-50"
          >
            <Check className="h-3 w-3" /> {busy ? '保存中…' : '保存设置'}
          </button>
        </div>
      </div>
    </Card>
  )
}

/** 拉黑弹窗：选时长 + 填原因 */
function BlockModal({
  ips,
  whitelist,
  onCancel,
  onSubmit,
}: {
  ips: string[]
  whitelist: string[]
  onCancel: () => void
  onSubmit: (ips: string[], reason: string, minutes: number) => void
}) {
  const [reason, setReason] = useState(ips.length > 1 ? '批量拉黑：密集异常访问' : '')
  const [minutes, setMinutes] = useState(60)
  const [busy, setBusy] = useState(false)

  // 白名单里的地址封不了 —— 提前告诉用户，别等他点了才报错
  const inWhitelist = ips.filter((ip) => whitelist.includes(ip))

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 dark:bg-black/70">
      <div className="w-full max-w-md rounded-2xl bg-surface p-5 shadow-2xl">
        <div className="mb-3 text-base font-bold text-content">
          封禁 {ips.length === 1 ? ips[0] : `${ips.length} 个地址`}
        </div>

        {ips.length > 1 && (
          <div className="mb-3 max-h-24 overflow-y-auto rounded-xl bg-surface-2 p-2 font-mono text-[11px] text-content">
            {ips.join('、')}
          </div>
        )}

        {inWhitelist.length > 0 && (
          <div className="mb-3 rounded-xl bg-amber-50 px-3 py-2 text-[11px] text-amber-700">
            有 {inWhitelist.length} 个在白名单里，它们不会被封禁。
          </div>
        )}

        <label className="mb-3 block">
          <div className="mb-1 text-xs font-semibold text-muted">原因（写清楚，以后自己看得懂）</div>
          <input
            autoFocus
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="如：大量 404 扫描"
            className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
          />
        </label>

        <div className="mb-4">
          <div className="mb-1 text-xs font-semibold text-muted">封禁时长</div>
          <div className="flex flex-wrap gap-2">
            {durations.map((d) => (
              <button
                key={d.label}
                type="button"
                onClick={() => setMinutes(d.minutes)}
                className={`rounded-lg px-3 py-1.5 text-xs font-semibold transition ${
                  minutes === d.minutes ? 'bg-blue-500 text-white' : 'bg-surface-2 text-content hover:bg-fill'
                }`}
              >
                {d.label}
              </button>
            ))}
          </div>
          {minutes === 0 && (
            <div className="mt-1.5 text-[11px] text-rose-500">
              永久封禁只能手动解除。不确定对方是不是误伤时，建议先用临时封禁。
            </div>
          )}
        </div>

        <div className="flex justify-end gap-2">
          <button onClick={onCancel} className="rounded-xl px-4 py-2 text-sm text-muted hover:bg-surface-2">
            取消
          </button>
          <button
            onClick={() => {
              setBusy(true)
              onSubmit(ips, reason, minutes)
            }}
            disabled={busy}
            className="rounded-xl bg-rose-500 px-4 py-2 text-sm font-semibold text-white hover:bg-rose-600 disabled:opacity-50"
          >
            {busy ? '处理中…' : '确认封禁'}
          </button>
        </div>
      </div>
    </div>
  )
}

function Metric({ label, value, tone }: { label: string; value: string; tone?: 'danger' }) {
  return (
    <div className="rounded-xl bg-surface-2 px-4 py-3">
      <div className="text-xs text-muted">{label}</div>
      <div className={`mt-0.5 text-xl font-semibold tabular-nums ${tone === 'danger' ? 'text-rose-600' : 'text-content'}`}>
        {value}
      </div>
    </div>
  )
}

/**
 * 登录尝试。
 *
 * 单独一块而不是混进上面的访问来源：那是"谁访问了我的服务"，这是
 * "谁在试着进我的后台"。两者的处置方式也不同 —— 后者要的是"看有没有人在
 * 试密码、试的是哪些用户名"，而不是按 IP 统计流量。
 *
 * 界面上刻意显示**用户名**：看到一堆 admin / root / test 就知道有人在字典
 * 攻击，这比只给一个 IP 有用得多。
 */
function LoginPanel() {
  const [data, setData] = useState<{
    list: LoginEvent[]
    day: string
    days: string[]
    total: number
    summary: LoginSummary
  } | null>(null)

  // 按哪天看。空 = 后端给最近有记录的那天
  const [day, setDay] = useState('')
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(getPageLimit)

  const load = useCallback(async () => {
    try {
      setData(await api.getAuditLogins({ day, limit, page }))
    } catch {
      // 没有记录 / 接口不可用都不该影响这一页其它块
    }
  }, [day, page, limit])

  useEffect(() => {
    void load()
  }, [load])

  // 换日期就回到第一页
  useEffect(() => {
    setPage(1)
  }, [day, limit])

  const s = data?.summary
  const failed = s?.failed ?? 0
  const total = s?.total ?? 0
  const days = data?.days ?? []

  return (
    <Card className="!p-0">
      <div className="px-5 pt-4">
        <CardHeader
          title={
            <span>
              登录尝试
              {data?.day && <span className="ml-1 font-normal text-muted">{data.day}</span>}
            </span>
          }
          action={
            /* 按日期看：登录事件本来就按天存文件（login-<日期>.jsonl），
               所以这里是挑文件，不是过滤 —— 选哪天就只看哪天 */
            <select
              value={day}
              onChange={(e) => setDay(e.target.value)}
              className="rounded-lg border border-line bg-surface px-2 py-1 text-xs outline-none focus:border-blue-400"
              title="选择查看哪一天的登录记录"
            >
              <option value="">最近一天</option>
              {days.map((d) => (
                <option key={d} value={d}>
                  {d}
                </option>
              ))}
            </select>
          }
        />
      </div>
      <div className="px-5 pb-5">
        {!s || total === 0 ? (
          <div className="rounded-xl bg-surface-2 px-4 py-6 text-center text-xs text-muted">
            还没有登录记录
          </div>
        ) : (
          <>
            <div className="mb-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
              <span className="text-muted">
                共 <span className="font-semibold text-content">{total}</span> 次
              </span>
              {failed > 0 && (
                <span className="text-rose-600">
                  失败 <span className="font-semibold">{failed}</span>
                </span>
              )}
              {s.succeeded > 0 && (
                <span className="text-emerald-600">
                  成功 <span className="font-semibold">{s.succeeded}</span>
                </span>
              )}
              {failed > 0 && (
                <span className="text-[11px] text-muted">
                  来自 {s.ips.length} 个地址
                  {s.usernames.length > 1 && ` · 试过 ${s.usernames.length} 个用户名`}
                </span>
              )}
            </div>

            {/* 失败明显偏多时提示一句 —— 这是用户真正需要被提醒的信号 */}
            {failed >= 10 && (
              <div className="mb-3 rounded-xl bg-rose-50 px-3 py-2 text-[11px] leading-relaxed text-rose-700 ring-1 ring-rose-200">
                有 {failed} 次登录失败。如果是你不小心输错，忽略即可；
                如果来自不认识的地址，说明有人在试你的密码 —— 检查上面的来源，必要时拉黑。
              </div>
            )}

            <div className="max-h-64 space-y-0.5 overflow-y-auto rounded-lg bg-surface-2/60 p-2">
              {(data?.list ?? []).map((e, i) => (
                <div key={i} className="flex flex-wrap items-center gap-x-3 gap-y-0.5 px-1 py-1 text-[11px]">
                  <span className="font-mono text-muted">{e.time.slice(5, 19).replace('T', ' ')}</span>
                  <span className={`shrink-0 rounded px-1 py-0.5 text-[10px] font-bold ${
                    e.success ? 'bg-emerald-50 text-emerald-600' : 'bg-rose-50 text-rose-600'
                  }`}>
                    {e.success ? '成功' : '失败'}
                  </span>
                  <span className="font-mono text-content">{e.ip}</span>
                  <span className="text-muted">
                    用户名 <span className="font-mono text-content">{e.username || '（空）'}</span>
                  </span>
                  {!e.success && e.reason && (
                    <span className="text-[10px] text-muted">{e.reason}</span>
                  )}
                </div>
              ))}

              {/* 分页：和访问来源、黑名单同一个组件 */}
              <Pager
                total={data?.total ?? 0}
                page={page}
                limit={limit}
                onPage={setPage}
                onLimit={applyLimit(setLimit)}
                unit="次"
              />
            </div>
          </>
        )}
      </div>
    </Card>
  )
}