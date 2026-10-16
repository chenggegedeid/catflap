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

export function Audit() {
  const [cfg, setCfg] = useState<AuditConfigView | null>(null)
  const [ips, setIps] = useState<AuditIPStat[]>([])
  const [sort, setSort] = useState<'errors' | 'requests' | 'recent'>('errors')
  const [onlySuspicious, setOnlySuspicious] = useState(false)
  const [keyword, setKeyword] = useState('')
  const [loading, setLoading] = useState(false)
  const [toast, setToast] = useState('')
  const [err, setErr] = useState('')

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
        api.getAuditIPs({ sort, suspicious: onlySuspicious, limit: 300 }),
      ])
      setCfg(c)
      setIps(list.list ?? [])
      setErr('')
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [sort, onlySuspicious])

  useEffect(() => {
    void refresh()
  }, [refresh])

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

  // 前端按关键字过滤（后端没做 IP 模糊搜索：列表最多几百条，
  // 传回来前端筛更简单，也避免多一个接口参数）
  const visible = useMemo(() => {
    const kw = keyword.trim()
    if (!kw) return ips
    return ips.filter((x) => x.ip.includes(kw) || x.blockReason.includes(kw))
  }, [ips, keyword])

  const suspiciousList = useMemo(() => visible.filter((x) => isSuspicious(x)), [visible])

  if (err && !cfg) {
    return (
      <Card>
        <div className="text-sm text-rose-600">{err}</div>
      </Card>
    )
  }

  return (
    <div className="space-y-5">
      {/* 概览 */}
      <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Metric label="已记录 IP" value={String(cfg?.totalIPs ?? 0)} />
        <Metric
          label="可疑 IP"
          value={String(cfg?.suspiciousCount ?? 0)}
          tone={(cfg?.suspiciousCount ?? 0) > 0 ? 'danger' : undefined}
        />
        <Metric label="当前封禁" value={String(cfg?.blocks.length ?? 0)} />
        <Metric label="明细保留" value={`${cfg?.keepDays ?? 0} 天`} />
      </div>

      {/* 可疑提示条：有事的时候要在最上面说清 */}
      {suspiciousList.length > 0 && (
        <Card className="!py-3">
          <div className="flex flex-wrap items-center gap-3">
            <TriangleAlert className="h-4 w-4 shrink-0 text-rose-500" />
            <span className="text-sm text-slate-700">
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
                <select
                  value={sort}
                  onChange={(e) => setSort(e.target.value as typeof sort)}
                  className="rounded-lg border border-slate-200 bg-white px-2 py-1 text-xs outline-none focus:border-blue-400"
                >
                  <option value="errors">按异常数</option>
                  <option value="requests">按请求数</option>
                  <option value="recent">按最近访问</option>
                </select>
                <button
                  onClick={() => setOnlySuspicious((v) => !v)}
                  className={`rounded-lg px-2 py-1 text-xs font-semibold transition ${
                    onlySuspicious ? 'bg-rose-50 text-rose-600' : 'text-slate-500 hover:bg-slate-100'
                  }`}
                >
                  只看可疑
                </button>
                <div className="relative">
                  <Search className="pointer-events-none absolute left-2 top-1/2 h-3 w-3 -translate-y-1/2 text-slate-400" />
                  <input
                    value={keyword}
                    onChange={(e) => setKeyword(e.target.value)}
                    placeholder="筛 IP"
                    className="w-32 rounded-lg border border-slate-200 bg-white py-1 pl-7 pr-2 text-xs outline-none focus:border-blue-400"
                  />
                </div>
                <button
                  onClick={refresh}
                  className="flex items-center gap-1 rounded-lg px-2 py-1 text-xs font-semibold text-slate-500 hover:bg-slate-100"
                >
                  <RotateCw className={`h-3 w-3 ${loading ? 'animate-spin' : ''}`} />
                </button>
              </div>
            }
          />
        </div>

        <div className="px-5 pb-5">
          {visible.length === 0 ? (
            <div className="rounded-xl bg-slate-50 px-4 py-8 text-center text-xs text-slate-400">
              {onlySuspicious ? '没有可疑 IP —— 挺好' : '还没有访问记录'}
            </div>
          ) : (
            <div className="space-y-1.5">
              {visible.map((x) => (
                <IPRow
                  key={x.ip}
                  stat={x}
                  open={detailIP === x.ip}
                  records={records}
                  onToggle={() => toggleDetail(x.ip)}
                  onBlock={() => setBlockModal({ open: true, ips: [x.ip] })}
                  onUnblock={() => doUnblock(x.ip)}
                />
              ))}
            </div>
          )}
        </div>
      </Card>

      {/* 登录尝试：和"谁访问了服务"分开 —— 这是"谁在试着进后台" */}
      <LoginPanel />

      {/* 封禁表 + 设置 */}
      <div className="grid gap-4 lg:grid-cols-2">
        <Card className="!p-0">
          <div className="px-5 pt-4">
            <CardHeader title="封禁列表" />
          </div>
          <div className="px-5 pb-5">
            {(cfg?.blocks ?? []).length === 0 ? (
              <div className="rounded-xl bg-slate-50 px-4 py-6 text-center text-xs text-slate-400">
                当前没有封禁任何地址
              </div>
            ) : (
              <div className="space-y-1.5">
                {(cfg?.blocks ?? []).map((b) => (
                  <div
                    key={b.ip}
                    className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-xl border border-slate-200/70 px-3 py-2"
                  >
                    <span className="font-mono text-xs text-slate-700">{b.ip}</span>
                    {b.auto && (
                      <span className="rounded bg-amber-50 px-1.5 py-0.5 text-[10px] font-semibold text-amber-600">
                        自动
                      </span>
                    )}
                    <span className="truncate text-[11px] text-slate-400">{b.reason}</span>
                    <span className="text-[11px] text-slate-400">
                      {b.expiresAt ? `至 ${new Date(b.expiresAt).toLocaleString('zh-CN')}` : '永久'}
                    </span>
                    <button
                      onClick={() => doUnblock(b.ip)}
                      className="ml-auto flex items-center gap-1 rounded-lg px-2 py-0.5 text-xs font-semibold text-slate-500 hover:bg-slate-100"
                    >
                      <ShieldOff className="h-3 w-3" /> 解除
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        </Card>

        <SettingsCard cfg={cfg} onSaved={refresh} onToast={say} />
      </div>

      {blockModal.open && (
        <BlockModal
          ips={blockModal.ips}
          whitelist={cfg?.whitelist ?? []}
          onCancel={() => setBlockModal({ open: false, ips: [] })}
          onSubmit={doBlock}
        />
      )}

      {toast && (
        <div className="fixed bottom-6 left-1/2 z-50 -translate-x-1/2 rounded-xl bg-slate-800 px-4 py-2 text-xs font-medium text-white shadow-lg">
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
  open,
  records,
  onToggle,
  onBlock,
  onUnblock,
}: {
  stat: AuditIPStat
  open: boolean
  records: AuditRecord[]
  onToggle: () => void
  onBlock: () => void
  onUnblock: () => void
}) {
  const suspicious = isSuspicious(stat)

  return (
    <div className={`rounded-xl border ${suspicious ? 'border-rose-200 bg-rose-50/40' : 'border-slate-200/70'}`}>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5 px-3 py-2">
        <button onClick={onToggle} className="flex min-w-0 flex-1 items-center gap-3 text-left">
          <span className="font-mono text-xs text-slate-700">{stat.ip}</span>
          {suspicious && (
            <span className="rounded bg-rose-100 px-1.5 py-0.5 text-[10px] font-semibold text-rose-600">可疑</span>
          )}
          {stat.blocked && (
            <span className="rounded bg-slate-200 px-1.5 py-0.5 text-[10px] font-semibold text-slate-600">已封禁</span>
          )}
          <span className="text-[11px] text-slate-500">{fmtCount(stat.requests)} 次</span>
          {stat.errors > 0 && (
            <span className={`text-[11px] ${suspicious ? 'font-semibold text-rose-600' : 'text-amber-600'}`}>
              异常 {fmtCount(stat.errors)}
            </span>
          )}
          <span className="text-[11px] text-slate-400">{fmtBytes(stat.bytesIn + stat.bytesOut)}</span>
          <span className="ml-auto hidden text-[11px] text-slate-400 sm:inline">
            {new Date(stat.lastSeen).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' })}
          </span>
        </button>

        {stat.blocked ? (
          <button
            onClick={onUnblock}
            className="flex items-center gap-1 rounded-lg px-2 py-0.5 text-xs font-semibold text-slate-500 hover:bg-white"
          >
            <ShieldOff className="h-3 w-3" /> 解除
          </button>
        ) : (
          <button
            onClick={onBlock}
            className="flex items-center gap-1 rounded-lg bg-slate-100 px-2 py-0.5 text-xs font-semibold text-slate-600 hover:bg-rose-100 hover:text-rose-600"
          >
            <Ban className="h-3 w-3" /> 拉黑
          </button>
        )}
      </div>

      {open && (
        <div className="border-t border-slate-200/70 px-3 py-2.5 text-[11px]">
          <div className="mb-2 grid gap-2 sm:grid-cols-2">
            <div>
              <div className="mb-1 font-semibold text-slate-500">访问过的路径（看它在找什么）</div>
              <div className="flex flex-wrap gap-1">
                {(stat.paths ?? []).length === 0 ? (
                  <span className="text-slate-400">—</span>
                ) : (
                  (stat.paths ?? []).map((p) => (
                    <span key={p} className="rounded bg-white px-1.5 py-0.5 font-mono text-[10px] text-slate-600 ring-1 ring-slate-200">
                      {p}
                    </span>
                  ))
                )}
              </div>
            </div>
            <div>
              <div className="mb-1 font-semibold text-slate-500">User-Agent</div>
              <div className="space-y-0.5">
                {(stat.userAgents ?? []).length === 0 ? (
                  <span className="text-slate-400">—</span>
                ) : (
                  (stat.userAgents ?? []).map((u) => (
                    <div key={u} className="truncate font-mono text-[10px] text-slate-500">
                      {u}
                    </div>
                  ))
                )}
              </div>
            </div>
          </div>

          <div className="mb-1 font-semibold text-slate-500">最近的访问</div>
          <div className="max-h-48 space-y-0.5 overflow-y-auto rounded-lg bg-white p-1.5 ring-1 ring-slate-200">
            {records.length === 0 ? (
              <div className="py-3 text-center text-slate-400">读取中…</div>
            ) : (
              records.slice(0, 50).map((r, i) => (
                <div key={i} className="flex items-center gap-2 px-1 py-0.5 font-mono text-[10px]">
                  <span className="shrink-0 text-slate-400">{r.time.slice(11, 19)}</span>
                  <span className="shrink-0 text-slate-500">{r.method}</span>
                  <span className="truncate text-slate-700">{r.path}</span>
                  <span className={`ml-auto shrink-0 ${r.status >= 400 ? 'text-rose-500' : 'text-emerald-600'}`}>
                    {r.status}
                  </span>
                </div>
              ))
            )}
          </div>
        </div>
      )}
    </div>
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
  const [whitelist, setWhitelist] = useState((cfg?.whitelist ?? []).join('\n'))
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
    setWhitelist((cfg.whitelist ?? []).join('\n'))
  }, [cfg])

  const save = async () => {
    setBusy(true)
    try {
      await api.saveAuditSettings({
        enabled,
        keepDays: Number(keepDays) || 30,
        maxFileMB: Number(maxFileMB) || 32,
        autoBlock,
        autoBlockThreshold: Number(threshold) || 500,
        autoBlockMinutes: Number(minutes) || 60,
        whitelist: whitelist.split('\n').map((s) => s.trim()).filter(Boolean),
        trustedProxies: cfg?.trustedProxies ?? [],
      })
      onToast('已保存')
      onSaved()
    } catch (e) {
      onToast('保存失败：' + (e instanceof Error ? e.message : ''))
    } finally {
      setBusy(false)
    }
  }

  const cls = 'w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400'

  return (
    <Card className="!p-0">
      <div className="px-5 pt-4">
        <CardHeader title="设置" />
      </div>
      <div className="space-y-3 px-5 pb-5">
        <label className="flex cursor-pointer items-center gap-2 text-xs text-slate-600">
          <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
          记录访问（关掉之后不再产生新记录，已有数据保留）
        </label>

        <div className="grid grid-cols-2 gap-3">
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">明细保留（天）</div>
            <input value={keepDays} onChange={(e) => setKeepDays(e.target.value)} className={cls} />
          </label>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">单天文件上限（MB）</div>
            <input value={maxFileMB} onChange={(e) => setMaxFileMB(e.target.value)} className={cls} />
            <div className="mt-1 text-[11px] text-slate-400">超了停写，防磁盘被刷满</div>
          </label>
        </div>

        <div className="rounded-xl bg-slate-50 p-3">
          <label className="flex cursor-pointer items-center gap-2 text-xs text-slate-600">
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
                <div className="mb-1 text-xs font-semibold text-slate-500">异常数门槛</div>
                <input value={threshold} onChange={(e) => setThreshold(e.target.value)} className={cls} />
              </label>
              <label className="block">
                <div className="mb-1 text-xs font-semibold text-slate-500">封禁时长（分钟，0=永久）</div>
                <input value={minutes} onChange={(e) => setMinutes(e.target.value)} className={cls} />
              </label>
            </div>
          )}
        </div>

        <label className="block">
          <div className="mb-1 text-xs font-semibold text-slate-500">白名单（永不封禁，一行一个）</div>
          <textarea
            value={whitelist}
            onChange={(e) => setWhitelist(e.target.value)}
            rows={3}
            placeholder={'203.0.113.0/24\n198.51.100.7'}
            className={`${cls} font-mono resize-y`}
          />
          <div className="mt-1 text-[11px] text-slate-400">
            支持单个 IP 或 CIDR 网段。开了自动拉黑时这是防误伤的兜底。
          </div>
        </label>

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
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-slate-900/40 p-4">
      <div className="w-full max-w-md rounded-2xl bg-white p-5 shadow-2xl">
        <div className="mb-3 text-base font-bold text-slate-800">
          封禁 {ips.length === 1 ? ips[0] : `${ips.length} 个地址`}
        </div>

        {ips.length > 1 && (
          <div className="mb-3 max-h-24 overflow-y-auto rounded-xl bg-slate-50 p-2 font-mono text-[11px] text-slate-600">
            {ips.join('、')}
          </div>
        )}

        {inWhitelist.length > 0 && (
          <div className="mb-3 rounded-xl bg-amber-50 px-3 py-2 text-[11px] text-amber-700">
            有 {inWhitelist.length} 个在白名单里，它们不会被封禁。
          </div>
        )}

        <label className="mb-3 block">
          <div className="mb-1 text-xs font-semibold text-slate-500">原因（写清楚，以后自己看得懂）</div>
          <input
            autoFocus
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="如：大量 404 扫描"
            className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
          />
        </label>

        <div className="mb-4">
          <div className="mb-1 text-xs font-semibold text-slate-500">封禁时长</div>
          <div className="flex flex-wrap gap-2">
            {durations.map((d) => (
              <button
                key={d.label}
                type="button"
                onClick={() => setMinutes(d.minutes)}
                className={`rounded-lg px-3 py-1.5 text-xs font-semibold transition ${
                  minutes === d.minutes ? 'bg-blue-500 text-white' : 'bg-slate-100 text-slate-600 hover:bg-slate-200'
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
          <button onClick={onCancel} className="rounded-xl px-4 py-2 text-sm text-slate-500 hover:bg-slate-100">
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
    <div className="rounded-xl bg-slate-50 px-4 py-3">
      <div className="text-xs text-slate-500">{label}</div>
      <div className={`mt-0.5 text-xl font-semibold tabular-nums ${tone === 'danger' ? 'text-rose-600' : 'text-slate-700'}`}>
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
  const [data, setData] = useState<{ list: LoginEvent[]; day: string; summary: LoginSummary } | null>(null)

  const load = useCallback(async () => {
    try {
      setData(await api.getAuditLogins({ limit: 50 }))
    } catch {
      // 没有记录 / 接口不可用都不该影响这一页其它块
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const s = data?.summary
  const failed = s?.failed ?? 0
  const total = s?.total ?? 0

  return (
    <Card className="!p-0">
      <div className="px-5 pt-4">
        <CardHeader
          title={
            <span>
              登录尝试
              {data?.day && (
                <span className="ml-1 font-normal text-slate-400">{data.day}，最多 50 条</span>
              )}
            </span>
          }
        />
      </div>
      <div className="px-5 pb-5">
        {!s || total === 0 ? (
          <div className="rounded-xl bg-slate-50 px-4 py-6 text-center text-xs text-slate-400">
            还没有登录记录
          </div>
        ) : (
          <>
            <div className="mb-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
              <span className="text-slate-500">
                共 <span className="font-semibold text-slate-700">{total}</span> 次
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
                <span className="text-[11px] text-slate-400">
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

            <div className="max-h-64 space-y-0.5 overflow-y-auto rounded-lg bg-slate-50/60 p-2">
              {(data?.list ?? []).map((e, i) => (
                <div key={i} className="flex flex-wrap items-center gap-x-3 gap-y-0.5 px-1 py-1 text-[11px]">
                  <span className="font-mono text-slate-400">{e.time.slice(5, 19).replace('T', ' ')}</span>
                  <span className={`shrink-0 rounded px-1 py-0.5 text-[10px] font-bold ${
                    e.success ? 'bg-emerald-50 text-emerald-600' : 'bg-rose-50 text-rose-600'
                  }`}>
                    {e.success ? '成功' : '失败'}
                  </span>
                  <span className="font-mono text-slate-700">{e.ip}</span>
                  <span className="text-slate-500">
                    用户名 <span className="font-mono text-slate-700">{e.username || '（空）'}</span>
                  </span>
                  {!e.success && e.reason && (
                    <span className="text-[10px] text-slate-400">{e.reason}</span>
                  )}
                </div>
              ))}
            </div>
          </>
        )}
      </div>
    </Card>
  )
}
