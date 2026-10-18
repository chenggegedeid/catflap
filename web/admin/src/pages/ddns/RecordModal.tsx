import { useEffect, useState } from 'react'
import { X } from 'lucide-react'
import { modalBackdrop } from '../../components/modal'
import * as api from '../../lib/api'
import { hasCapability } from '../../lib/providerTypes'
import { ProviderCap } from '../../types'
import type {
  DdnsProvider, DdnsRecord, DnsRecordType, NetInterface,
} from '../../types'
import {
  ifaceHasType, ifaceOptionText, ipSourceArgMeta,
  type ProviderTypeSpec, type RecordFormState,
} from './shared'

export function RecordModal({
  initial,
  providers,
  types,
  onCancel,
  onSubmit,
}: {
  initial?: DdnsRecord
  providers: DdnsProvider[]
  /** 服务商能力表，用于判断该服务商支不支持 AAAA / 小云朵 */
  types: ProviderTypeSpec[]
  onCancel: () => void
  onSubmit: (form: RecordFormState) => Promise<void>
}) {
  const [form, setForm] = useState<RecordFormState>(() => {
    if (!initial) {
      return {
        enabled: true,
        providerId: providers[0] ? String(providers[0].id) : '',
        name: '',
        domain: '',
        subDomain: '',
        recordType: 'A',
        ipSource: 'stun',
        ipSourceArg: '',
        ttl: '1',
        proxied: false,
      }
    }
    return {
      enabled: initial.enabled,
      providerId: String(initial.providerId),
      name: initial.name,
      domain: initial.domain,
      subDomain: initial.subDomain,
      recordType: initial.recordType,
      ipSource: initial.ipSource,
      ipSourceArg: initial.ipSourceArg,
      ttl: String(initial.ttl || 1),
      proxied: initial.proxied,
    }
  })
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  // 网卡列表只在选了「本地网卡」时才去拿：别的来源用不上，没必要每次开弹窗都请求一遍
  const [ifaces, setIfaces] = useState<NetInterface[] | null>(null)
  const [ifaceErr, setIfaceErr] = useState('')

  useEffect(() => {
    if (form.ipSource !== 'interface' || ifaces) return
    api
      .getDdnsInterfaces()
      .then((list) => setIfaces(list ?? []))
      .catch((e) => setIfaceErr(e instanceof Error ? e.message : '读取网卡列表失败'))
  }, [form.ipSource, ifaces])

  const submit = async () => {
    if (!form.providerId) {
      setErr('请先选择服务商')
      return
    }
    if (!form.name.trim()) {
      setErr('请填写记录名称')
      return
    }
    if (!form.domain.trim()) {
      setErr('请填写主域名')
      return
    }
    if (argMeta.required && !form.ipSourceArg.trim()) {
      setErr(`请填写${argMeta.label}`)
      return
    }
    // 选好网卡之后又把记录类型改了：这块网卡上没有对应的地址，
    // 存下去要等到下一轮同步失败才看得见
    if (form.ipSource === 'interface' && form.ipSourceArg) {
      const picked = (ifaces ?? []).find((n) => n.name === form.ipSourceArg)
      if (picked && !ifaceHasType(picked, form.recordType)) {
        setErr(`网卡「${picked.name}」上没有 ${form.recordType === 'AAAA' ? 'IPv6' : 'IPv4'} 地址，换一块或改记录类型`)
        return
      }
    }
    setErr('')
    setBusy(true)
    try {
      await onSubmit(form)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '操作失败')
      setBusy(false)
    }
  }

  const argMeta = ipSourceArgMeta[form.ipSource] ?? ipSourceArgMeta.stun
  const needArg = form.ipSource !== 'stun'

  const selectedProvider = providers.find((p) => String(p.id) === form.providerId)
  // 能力由后端注册表声明，这里只读 —— 不再判断「是不是 cloudflare」这类硬编码
  const ipv4Only = selectedProvider
    ? hasCapability(types, selectedProvider.type, ProviderCap.IPv4Only)
    : false
  const supportsProxied = selectedProvider
    ? hasCapability(types, selectedProvider.type, ProviderCap.Proxied)
    : false

  // NameCheap 等仅支持 IPv4：强制 A 记录
  useEffect(() => {
    if (ipv4Only && form.recordType !== 'A') {
      setForm((p) => ({ ...p, recordType: 'A' }))
    }
  }, [ipv4Only, form.recordType])

  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="max-h-full w-full max-w-lg overflow-y-auto rounded-2xl bg-surface p-6 text-content shadow-2xl ring-1 ring-line">
        <div className="mb-5 flex items-center justify-between">
          <div className="text-base font-bold text-content">{initial ? '编辑解析记录' : '添加解析记录'}</div>
          <button
            type="button"
            onClick={onCancel}
            className="grid h-7 w-7 place-items-center rounded-full text-muted transition hover:bg-surface-2 hover:text-content"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <label className="col-span-2 block sm:col-span-1">
            <div className="mb-1 text-xs font-semibold text-muted">记录名称</div>
            <input
              autoFocus
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              placeholder="如 家庭宽带"
              className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="col-span-2 block sm:col-span-1">
            <div className="mb-1 text-xs font-semibold text-muted">服务商</div>
            <select
              value={form.providerId}
              onChange={(e) => setForm((p) => ({ ...p, providerId: e.target.value }))}
              className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
            >
              {providers.length === 0 && <option value="">请先添加服务商</option>}
              {providers.map((p) => (
                <option key={p.id} value={String(p.id)}>
                  {p.name}
                </option>
              ))}
            </select>
          </label>
          <label className="col-span-2 block sm:col-span-1">
            <div className="mb-1 text-xs font-semibold text-muted">主域名</div>
            <input
              value={form.domain}
              onChange={(e) => setForm((p) => ({ ...p, domain: e.target.value }))}
              placeholder="如 example.com"
              className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="col-span-2 block sm:col-span-1">
            <div className="mb-1 text-xs font-semibold text-muted">子域名（@ 或留空表示主域名）</div>
            <input
              value={form.subDomain}
              onChange={(e) => setForm((p) => ({ ...p, subDomain: e.target.value }))}
              placeholder="如 home"
              className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
            <div className="mt-1 text-[11px] leading-relaxed text-muted">
              填 <span className="font-mono">*</span> 建一条通配记录，一次覆盖所有二级域名 ——
              之后加服务不用再来建解析
            </div>
          </label>
          <label className="col-span-2 block sm:col-span-1">
            <div className="mb-1 text-xs font-semibold text-muted">记录类型</div>
            <select
              value={form.recordType}
              onChange={(e) => setForm((p) => ({ ...p, recordType: e.target.value as DnsRecordType }))}
              className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
            >
              <option value="A">A (IPv4)</option>
              <option value="AAAA" disabled={ipv4Only}>
                AAAA (IPv6){ipv4Only ? '（该服务商不支持）' : ''}
              </option>
              <option value="A+AAAA" disabled={ipv4Only}>
                A + AAAA（双栈）{ipv4Only ? '（该服务商不支持）' : ''}
              </option>
            </select>
            {form.recordType === 'A+AAAA' && (
              <div className="mt-1 text-[11px] leading-relaxed text-muted">
                一条记录同时同步 A 和 AAAA 两条，不必建两次。
                IP 来源选「本地网卡」最省事——双栈会分别取网卡上的 IPv4 与 IPv6。
              </div>
            )}
          </label>
          <label className="col-span-2 block sm:col-span-1">
            <div className="mb-1 text-xs font-semibold text-muted">TTL（1 = 自动）</div>
            <input
              type="number"
              value={form.ttl}
              onChange={(e) => setForm((p) => ({ ...p, ttl: e.target.value }))}
              min={1}
              className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="col-span-2 block sm:col-span-1">
            <div className="mb-1 text-xs font-semibold text-muted">IP 来源</div>
            <select
              value={form.ipSource}
              onChange={(e) => setForm((p) => ({ ...p, ipSource: e.target.value as RecordFormState['ipSource'] }))}
              className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
            >
              <option value="stun">STUN 公网 IP</option>
              <option value="web">Web 查询</option>
              <option value="dns">DNS 解析</option>
              <option value="interface">本地网卡</option>
              <option value="custom">自定义</option>
            </select>
          </label>
          <label className="col-span-2 block sm:col-span-1">
            <div className="mb-1 text-xs font-semibold text-muted">{argMeta.label}</div>
            {form.ipSource === 'interface' ? (
              <select
                value={form.ipSourceArg}
                onChange={(e) => setForm((p) => ({ ...p, ipSourceArg: e.target.value }))}
                className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
              >
                <option value="">自动挑一块</option>
                {(ifaces ?? []).map((n) => (
                  <option key={n.name} value={n.name} disabled={!ifaceHasType(n, form.recordType)}>
                    {ifaceOptionText(n, form.recordType)}
                  </option>
                ))}
                {/* 存着的网卡现在不在了（换了机器、禁用了网卡）：留着这一项，
                    不然下拉会显示成「自动挑一块」，一保存就悄悄换了意思 */}
                {form.ipSourceArg !== '' &&
                  !(ifaces ?? []).some((n) => n.name === form.ipSourceArg) && (
                    <option value={form.ipSourceArg}>{form.ipSourceArg}（现在没有这块网卡）</option>
                  )}
              </select>
            ) : (
              <input
                value={form.ipSourceArg}
                onChange={(e) => setForm((p) => ({ ...p, ipSourceArg: e.target.value }))}
                disabled={!needArg}
                placeholder={argMeta.placeholder}
                className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-surface-2 disabled:text-muted"
              />
            )}
            {form.ipSource === 'interface' && (
              <div className="mt-1 text-[11px] text-muted">
                {ifaceErr ||
                  (ifaces === null
                    ? '正在读网卡...'
                    : ifaces.length === 0
                      ? '这台机器上没有地址能往公网写的网卡，换个 IP 来源'
                      : '选哪块，就把哪块上面的地址写进这条记录')}
              </div>
            )}
            {form.ipSource === 'custom' && (
              <div className="mt-1 text-[11px] text-muted">
                这条记录就固定指着你填的地址，不随公网 IP 变
              </div>
            )}
          </label>

          <div className="col-span-2 grid grid-cols-1 gap-2 sm:grid-cols-2">
            <label className="flex cursor-pointer items-center gap-2 rounded-xl bg-surface-2 px-3 py-2 text-xs font-medium text-content">
              <input
                type="checkbox"
                checked={form.enabled}
                onChange={(e) => setForm((p) => ({ ...p, enabled: e.target.checked }))}
                className="h-3.5 w-3.5"
              />
              启用自动同步
            </label>
            {supportsProxied && (
              <label className="flex cursor-pointer items-center gap-2 rounded-xl bg-surface-2 px-3 py-2 text-xs font-medium text-content">
                <input
                  type="checkbox"
                  checked={form.proxied}
                  onChange={(e) => setForm((p) => ({ ...p, proxied: e.target.checked }))}
                  className="h-3.5 w-3.5"
                />
                Cloudflare 小云朵代理
              </label>
            )}
          </div>
          {err && <div className="col-span-2 text-xs text-rose-500">{err}</div>}
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            className="rounded-xl bg-surface-2 px-4 py-2 text-sm font-semibold text-content transition hover:bg-fill"
          >
            取消
          </button>
          <button
            type="button"
            onClick={submit}
            disabled={busy}
            className="rounded-xl bg-blue-500 px-4 py-2 text-sm font-semibold text-white shadow-md shadow-blue-500/20 transition hover:bg-blue-600 disabled:opacity-50"
          >
            {busy ? '保存中...' : '保存'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ===================== 设置弹窗 =====================

