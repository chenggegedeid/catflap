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
import { RotateCw, Search, TriangleAlert } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import { Collapsible } from '../components/Collapsible'
import { BlockModal } from './audit/BlockModal'
import { IPLists } from './audit/IPLists'
import { IPRow } from './audit/IPRow'
import { LoginPanel } from './audit/LoginPanel'
import { Metric } from './audit/Metric'
import { SettingsCard } from './audit/SettingsCard'
import { applyLimit, isSuspicious } from './audit/shared'
import { AuditOverview } from './AuditOverview'
import { Pager } from '../components/Pager'
import { getPageLimit } from '../lib/prefs'
import * as api from '../lib/api'
import type { AuditConfigView, AuditIPStat, AuditRecord } from '../types'

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
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
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

      {/* 登录尝试：和"谁访问了服务"分开 —— 这是"谁在试着进后台"。
          默认收起：日常只看访问来源，登录记录是出事才翻的。
          收起时子组件整块不渲染，那一路请求也就不发了。 */}
      <Collapsible
        title="登录尝试"
        storageKey="audit.loginPanel.open"
        hint={
          cfg && cfg.blocks.length > 0
            ? `当前封禁 ${cfg.blocks.length} 个地址`
            : '谁在试着进后台'
        }
      >
        <LoginPanel />
      </Collapsible>

      {/*
        顺序：设置 → IP 名单（黑名单 | 白名单）。

        - 设置里有「记录访问」总开关，关掉等于关掉整个审计功能，
          用户第一眼要找的是这个，不是先看一个空列表。
        - 黑名单和白名单并排：它们是一对（拦谁 / 不拦谁）。

        **不再单独放一张「封禁列表」** —— 它和黑名单读的是同一份数据
        （config/audit.json 的 Blocks）。分两块展示只会让人以为有两套名单，
        实际手动封禁和自动拉黑都写进同一张表。现在黑名单里全都有，
        并标出每条的原因、来源（手动/自动）和到期时间。

        **设置与名单默认收起**：它们是一年配一次的东西，占着首屏只会把
        真正每天要看的「访问来源」推下去。
      */}
      <Collapsible
        title="审计设置"
        storageKey="audit.settings.open"
        hint={`记录保留 ${cfg?.keepDays ?? 30} 天 · 单文件上限 ${cfg?.maxFileMB ?? 32} MB`}
      >
        <div className="space-y-4">
          <SettingsCard cfg={cfg} onSaved={refresh} onToast={say} />
          <IPLists cfg={cfg} onSaved={refresh} onToast={say} />
        </div>
      </Collapsible>

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
