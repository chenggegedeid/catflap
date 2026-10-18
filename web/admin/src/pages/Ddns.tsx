/**
 * 动态域名（DDNS）页面。
 *
 * 原来 1174 行塞在一个文件里：服务商弹窗、解析记录弹窗、设置弹窗、
 * 一堆常量与工具。现在按职责拆到 `./ddns/` 下，这里只留**列表与编排**：
 *   - 服务商列表 / 记录列表怎么摆
 *   - 点哪个按钮弹哪个窗（弹窗 state 在这里，因为要互斥）
 *   - 与各弹窗之间的数据流转
 *
 * 加新弹窗时：在 ./ddns/ 下新建文件，然后在这里 import 并加一个 state 即可。
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  AlertCircle, CircleCheck, Cloud, Globe, Pencil, Plus, RefreshCw, Search,
  Server, Settings, Trash2, TriangleAlert,
} from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import * as api from '../lib/api'
import { providerTypeLabel as labelOfType, loadProviderTypes } from '../lib/providerTypes'
import type {
  DdnsConfig, DdnsProvider, DdnsRecord, DdnsRecordStatus,
} from '../types'
import { ProviderModal } from './ddns/ProviderModal'
import { RecordModal } from './ddns/RecordModal'
import { SettingsModal } from './ddns/SettingsModal'
import {
  type ProviderFormState, type ProviderTypeSpec, type RecordFormState,
  fmtTime, ipSourceLabel, statusInfo, useToast,
} from './ddns/shared'

export function Ddns() {
  const [config, setConfig] = useState<DdnsConfig | null>(null)
  const [types, setTypes] = useState<ProviderTypeSpec[]>([])
  const [loadErr, setLoadErr] = useState('')
  const [query, setQuery] = useState('')
  const [filterStatus, setFilterStatus] = useState<'all' | DdnsRecordStatus>('all')
  const [syncingId, setSyncingId] = useState<number | null>(null)
  const { toasts, show: toast } = useToast()

  const [providerModal, setProviderModal] = useState<{ open: boolean; initial?: DdnsProvider }>({ open: false })
  const [recordModal, setRecordModal] = useState<{ open: boolean; initial?: DdnsRecord }>({ open: false })
  const [settingsOpen, setSettingsOpen] = useState(false)

  const refresh = useCallback(async () => {
    try {
      const cfg = await api.getDdnsConfig()
      setConfig(cfg)
      setLoadErr('')
    } catch (e) {
      setLoadErr(e instanceof Error ? e.message : String(e))
    }
  }, [])

  useEffect(() => {
    refresh()
    const t = window.setInterval(refresh, 15000)
    return () => window.clearInterval(t)
  }, [refresh])

  // 服务商类型表：进程内缓存，只需拉一次（STUN 页也读同一份）
  useEffect(() => {
    let alive = true
    loadProviderTypes().then((list) => {
      if (alive) setTypes(list)
    })
    return () => {
      alive = false
    }
  }, [])

  const providers = config?.providers ?? []
  const records = config?.records ?? []

  const providerName = useMemo(() => {
    const m = new Map<number, string>()
    for (const p of providers) m.set(p.id, p.name)
    return m
  }, [providers])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return records.filter((r) => {
      if (filterStatus !== 'all' && r.lastStatus !== filterStatus) return false
      if (q) {
        const hay = `${r.name} ${r.subDomain}.${r.domain} ${r.lastIP}`.toLowerCase()
        if (!hay.includes(q)) return false
      }
      return true
    })
  }, [records, query, filterStatus])

  const total = records.length
  const success = records.filter((r) => r.lastStatus === 'success').length
  const failed = records.filter((r) => r.lastStatus === 'failed').length

  // ---- 全局设置 ----
  const changeInterval = async (sec: number) => {
    if (!config || sec < 30) return
    await api.updateDdnsSettings({ intervalSec: sec })
    setSettingsOpen(false)
    toast('同步间隔已更新')
    await refresh()
  }

  // ---- 服务商 ----
  const submitProvider = async (form: ProviderFormState) => {
    const credential: Record<string, string> = {}
    for (const [k, v] of Object.entries(form.credential)) {
      const t = v.trim()
      if (t) credential[k] = t
    }
    if (providerModal.initial) {
      await api.updateDdnsProvider({
        id: providerModal.initial.id,
        name: form.name.trim(),
        credential,
      })
      toast('服务商已更新')
    } else {
      await api.addDdnsProvider({ name: form.name.trim(), type: form.type, credential })
      toast('服务商已添加')
    }
    setProviderModal({ open: false })
    await refresh()
  }

  const removeProvider = async (p: DdnsProvider) => {
    if (!window.confirm(`确认删除服务商「${p.name}」？`)) return
    try {
      await api.deleteDdnsProvider(p.id)
      toast('服务商已删除')
      await refresh()
    } catch (e) {
      toast('删除失败: ' + (e instanceof Error ? e.message : ''))
    }
  }

  // ---- 记录 ----
  const submitRecord = async (form: RecordFormState) => {
    const payload: api.DdnsRecordPayload = {
      enabled: form.enabled,
      providerId: Number(form.providerId),
      name: form.name.trim(),
      domain: form.domain.trim(),
      subDomain: form.subDomain.trim(),
      recordType: form.recordType,
      ipSource: form.ipSource,
      ipSourceArg: form.ipSourceArg.trim(),
      ttl: Number(form.ttl) || 1,
      proxied: form.proxied,
    }
    if (recordModal.initial) {
      await api.updateDdnsRecord({ ...payload, id: recordModal.initial.id })
      toast('记录已更新')
    } else {
      await api.addDdnsRecord(payload)
      toast('记录已添加')
    }
    setRecordModal({ open: false })
    await refresh()
  }

  const removeRecord = async (r: DdnsRecord) => {
    if (!window.confirm(`确认删除记录「${r.name}」？`)) return
    try {
      await api.deleteDdnsRecord(r.id)
      toast('记录已删除')
      await refresh()
    } catch (e) {
      toast('删除失败: ' + (e instanceof Error ? e.message : ''))
    }
  }

  const syncRecord = async (r: DdnsRecord) => {
    setSyncingId(r.id)
    try {
      await api.syncDdnsRecord(r.id)
      toast('同步成功')
      await refresh()
    } catch (e) {
      toast('同步失败: ' + (e instanceof Error ? e.message : ''))
    } finally {
      setSyncingId(null)
    }
  }

  const toggleRecordEnabled = async (r: DdnsRecord) => {
    try {
      await api.updateDdnsRecord({
        id: r.id,
        enabled: !r.enabled,
        providerId: r.providerId,
        name: r.name,
        domain: r.domain,
        subDomain: r.subDomain,
        recordType: r.recordType,
        ipSource: r.ipSource,
        ipSourceArg: r.ipSourceArg,
        ttl: r.ttl,
        proxied: r.proxied,
      })
      toast(r.enabled ? '已关闭自动同步' : '已开启自动同步')
      await refresh()
    } catch (e) {
      toast('操作失败: ' + (e instanceof Error ? e.message : ''))
    }
  }

  if (loadErr && !config) {
    return (
      <Card>
        <div className="py-12 text-center text-rose-500">
          <AlertCircle className="mx-auto h-8 w-8" />
          <div className="mt-2 text-sm font-semibold">数据加载失败</div>
          <div className="mt-1 text-xs text-muted">{loadErr}</div>
          <button onClick={refresh} className="mt-4 rounded-xl bg-blue-500 px-4 py-2 text-xs font-semibold text-white">
            重试
          </button>
        </div>
      </Card>
    )
  }

  if (!config) {
    return <Card><div className="py-12 text-center text-sm text-muted">正在加载...</div></Card>
  }

  return (
    <div className="space-y-4">
      {/* 顶部标题 + 设置入口 */}
      <div className="flex items-center justify-between">
        <div className="text-base font-bold text-content">动态域名解析</div>
        <button
          onClick={() => setSettingsOpen(true)}
          className="flex items-center gap-1.5 rounded-xl bg-surface px-3 py-2 text-xs font-semibold text-content ring-1 ring-line transition hover:bg-surface-2"
          title="DDNS 设置"
        >
          <Settings className="h-4 w-4" />
          设置
        </button>
      </div>

      {/* 顶部概览 */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-blue-50 text-blue-500">
              <Globe className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-muted">解析记录</div>
              <div className="text-2xl font-bold text-content">{total}</div>
            </div>
          </div>
        </Card>
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-emerald-50 text-emerald-500">
              <CircleCheck className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-muted">正常解析</div>
              <div className="text-2xl font-bold text-content">{success}</div>
            </div>
          </div>
        </Card>
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-rose-50 text-rose-500">
              <TriangleAlert className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-muted">异常记录</div>
              <div className="text-2xl font-bold text-content">{failed}</div>
            </div>
          </div>
        </Card>
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-violet-50 text-violet-500">
              <Cloud className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-muted">已接入服务商</div>
              <div className="text-2xl font-bold text-content">{providers.length}</div>
            </div>
          </div>
        </Card>
      </div>

      {/* 服务商 */}
      <Card>
        <CardHeader
          title="DNS 服务商"
          action={
            <button
              onClick={() => setProviderModal({ open: true })}
              className="flex items-center gap-1 rounded-md bg-blue-500 px-2.5 py-1 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 hover:bg-blue-600"
            >
              <Plus className="h-3 w-3" /> 添加凭证
            </button>
          }
        />
        {providers.length === 0 ? (
          <div className="py-6 text-center text-xs text-muted">尚未配置服务商，先添加一个 DNS 服务商凭证</div>
        ) : (
          <div className="flex flex-wrap gap-2">
            {providers.map((p) => (
              <div
                key={p.id}
                className="group flex items-center gap-2 rounded-xl bg-orange-50 px-3 py-2 text-xs font-semibold text-orange-600 ring-1 ring-orange-200"
              >
                <Server className="h-3.5 w-3.5" />
                {p.name}
                <span className="text-[10px] font-normal text-orange-400">({labelOfType(types, p.type)})</span>
                <button
                  onClick={() => setProviderModal({ open: true, initial: p })}
                  className="ml-1 grid h-5 w-5 place-items-center rounded-md text-orange-400 transition hover:bg-surface hover:text-emerald-600"
                  title="编辑"
                >
                  <Pencil className="h-3 w-3" />
                </button>
                <button
                  onClick={() => removeProvider(p)}
                  className="grid h-5 w-5 place-items-center rounded-md text-orange-400 transition hover:bg-surface hover:text-rose-500"
                  title="删除"
                >
                  <Trash2 className="h-3 w-3" />
                </button>
              </div>
            ))}
          </div>
        )}
      </Card>

      {/* 记录列表 */}
      <Card className="!p-0">
        <div className="flex flex-wrap items-center gap-3 border-b border-line px-5 py-4">
          <div className="text-sm font-bold text-content">域名解析列表</div>
          <div className="relative ml-auto w-64">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-muted" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="搜索域名或 IP"
              className="h-8 w-full rounded-full border border-line bg-surface pl-8 pr-3 text-xs outline-none focus:border-blue-400"
            />
          </div>
          <button
            type="button"
            className="flex items-center gap-1 rounded-md bg-surface px-2 py-1 text-xs font-semibold text-content ring-1 ring-line hover:bg-surface-2"
            onClick={() =>
              setFilterStatus((p) =>
                p === 'all' ? 'success' : p === 'success' ? 'pending' : p === 'pending' ? 'failed' : 'all',
              )
            }
          >
            状态: {filterStatus === 'all' ? '全部' : statusInfo[filterStatus].label}
          </button>
          <button
            onClick={() => {
              if (providers.length === 0) {
                toast('请先添加 DNS 服务商')
                return
              }
              setRecordModal({ open: true })
            }}
            className="flex items-center gap-1 rounded-md bg-blue-500 px-3 py-1.5 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 hover:bg-blue-600"
          >
            <Plus className="h-3 w-3" /> 添加域名
          </button>
        </div>

        {/* 手机上列太窄会把中文压成一列一个字，给张表一个下限，窄屏改成横向滚动 */}
        <div className="overflow-x-auto">
          <table className="w-full min-w-[960px] text-sm">
            <thead>
              <tr className="bg-surface-2/60 text-xs text-muted">
                <th className="px-5 py-2.5 text-left font-medium">域名</th>
                <th className="px-3 py-2.5 text-left font-medium">类型</th>
                <th className="px-3 py-2.5 text-left font-medium">当前 IP</th>
                <th className="px-3 py-2.5 text-left font-medium">IP 来源</th>
                <th className="px-3 py-2.5 text-left font-medium">服务商</th>
                <th className="px-3 py-2.5 text-left font-medium">状态</th>
                <th className="px-3 py-2.5 text-left font-medium">最近同步</th>
                <th className="px-3 py-2.5 text-left font-medium">自动同步</th>
                <th className="px-5 py-2.5 text-right font-medium">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line">
              {filtered.length === 0 && (
                <tr>
                  <td colSpan={9} className="px-5 py-8 text-center text-sm text-muted">
                    {records.length === 0 ? '暂无解析记录，点击右上角"添加域名"开始' : '没有匹配的记录'}
                  </td>
                </tr>
              )}
              {filtered.map((r) => {
                const s = statusInfo[r.lastStatus] ?? statusInfo.pending
                const Icon = s.icon
                const fqdn = !r.subDomain || r.subDomain === '@' ? r.domain : `${r.subDomain}.${r.domain}`
                const busy = syncingId === r.id
                return (
                  <tr key={r.id} className="text-content transition hover:bg-surface-2/60">
                    <td className="px-5 py-3">
                      <div className="font-semibold">{fqdn}</div>
                      <div className="text-[11px] text-muted">{r.name}</div>
                    </td>
                    <td className="px-3 py-3">
                      <span className="rounded-md bg-blue-50 px-1.5 py-0.5 text-[11px] font-bold text-blue-600">
                        {r.recordType}
                      </span>
                    </td>
                    <td className="px-3 py-3 font-mono text-xs">{r.lastIP || '-'}</td>
                    <td className="px-3 py-3 text-xs text-muted">{ipSourceLabel[r.ipSource] ?? r.ipSource}</td>
                    <td className="px-3 py-3 text-xs">{providerName.get(r.providerId) ?? '已删除'}</td>
                    <td className="px-3 py-3">
                      <span
                        className={`inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] font-semibold ${s.tone}`}
                        title={r.lastMessage}
                      >
                        <Icon className={`h-3 w-3 ${r.lastStatus === 'pending' ? 'animate-spin' : ''}`} />
                        {s.label}
                      </span>
                    </td>
                    <td className="px-3 py-3 text-xs text-muted">{fmtTime(r.lastSyncAt)}</td>
                    <td className="px-3 py-3">
                      <button onClick={() => toggleRecordEnabled(r)} className="flex items-center gap-1.5" title="切换自动同步">
                        <span className={`inline-block h-2 w-2 rounded-full ${r.enabled ? 'bg-emerald-500' : 'bg-fill'}`} />
                        <span className="text-xs text-content">{r.enabled ? '已开启' : '已关闭'}</span>
                      </button>
                    </td>
                    <td className="px-5 py-3 text-right">
                      <div className="inline-flex items-center gap-1">
                        <button
                          onClick={() => syncRecord(r)}
                          disabled={busy}
                          className="grid h-6 w-6 place-items-center rounded-md text-muted hover:bg-blue-50 hover:text-blue-600 disabled:opacity-40"
                          title="立即同步"
                        >
                          <RefreshCw className={`h-3 w-3 ${busy ? 'animate-spin' : ''}`} />
                        </button>
                        <button
                          onClick={() => setRecordModal({ open: true, initial: r })}
                          className="grid h-6 w-6 place-items-center rounded-md text-muted hover:bg-emerald-50 hover:text-emerald-600"
                          title="编辑"
                        >
                          <Pencil className="h-3 w-3" />
                        </button>
                        <button
                          onClick={() => removeRecord(r)}
                          className="grid h-6 w-6 place-items-center rounded-md text-muted hover:bg-rose-50 hover:text-rose-500"
                          title="删除"
                        >
                          <Trash2 className="h-3 w-3" />
                        </button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>

        <div className="border-t border-line px-5 py-3 text-xs text-muted">
          共 {filtered.length} 条记录
        </div>
      </Card>

      {/* 弹窗 */}
      {providerModal.open && (
        <ProviderModal
          types={types}
          initial={providerModal.initial}
          onCancel={() => setProviderModal({ open: false })}
          onSubmit={submitProvider}
        />
      )}
      {recordModal.open && (
        <RecordModal
          initial={recordModal.initial}
          providers={providers}
          types={types}
          onCancel={() => setRecordModal({ open: false })}
          onSubmit={submitRecord}
        />
      )}
      {settingsOpen && (
        <SettingsModal
          intervalSec={config.intervalSec}
          onCancel={() => setSettingsOpen(false)}
          onSubmit={changeInterval}
        />
      )}

      {/* Toasts */}
      <div className="pointer-events-none fixed bottom-6 left-1/2 z-[60] flex -translate-x-1/2 flex-col items-center gap-2">
        {toasts.map((t) => (
          <div key={t.id} className="rounded-lg bg-inverse/90 px-4 py-2 text-sm text-on-inverse shadow-lg">
            {t.text}
          </div>
        ))}
      </div>
    </div>
  )
}
