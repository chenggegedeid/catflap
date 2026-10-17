import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {AlertCircle, ChevronRight, ClipboardCopy, Computer, CornerUpRight, ExternalLink, FileText, Gamepad2, Globe, Info, LoaderCircle, Pencil, Plus, Radar, Power, RotateCw, Server, Send, ShieldCheck, Trash2, TriangleAlert, Wifi, X, } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import { LanScanModal } from '../components/LanScanModal'
import { CardBadge, ServiceCard, iconBtn } from '../components/ServiceCard'
import { DomainInput } from '../components/DomainInput'
import { AccessControlEditor, type AccessMode } from '../components/AccessControlEditor'
import { AccessLogModal } from '../components/AccessLogModal'
import type { ServiceState } from '../components/ServiceCard'
import { modalBackdrop } from '../components/modal'
import * as api from '../lib/api'
import { hasCapability, loadProviderTypes } from '../lib/providerTypes'
import { ProviderCap } from '../types'
import type {
  NotifyChannel,
  Certificate,
  Device,
  DdnsProvider,
  DdnsProviderTypeSpec,
  LanHost,
  LanPort,
  LanSubnet,
  NatDetectResult,
  NatTypeInfo,
  StunConfig,
  StunDevice,
  RedirectConfig,
  RedirectInspection,
  MCEntryConfig,
  StunService,
  StunStatusEvent,
  WebhookConfig,
} from '../types'

const phaseLabel: Record<string, string> = {
  PROBING: '探测中',
  RUNNING: '穿透成功',
  RESTARTING: '重启中',
  FAILED: '探测失败',
  STOPPED: '已停止',
}

/** 后端给的是 time.Time，没同步过时是零值，别显示成 0001 年 */
function fmtTime(v?: string) {
  if (!v) return ''
  const d = new Date(v)
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return ''
  return d.toLocaleString()
}

interface Toast {
  id: number
  text: string
}

function useToast() {
  const [toasts, setToasts] = useState<Toast[]>([])
  const idRef = useRef(0)
  const show = useCallback((text: string) => {
    const id = ++idRef.current
    setToasts((p) => [...p, { id, text }])
    // 「已保存」两秒够了，但同步失败那种带着「接下来该干什么」的长句，
    // 两秒读不完就没了，等于没说。按字数给时间，最多十二秒
    const ms = Math.min(12000, Math.max(2000, text.length * 130))
    window.setTimeout(() => setToasts((p) => p.filter((t) => t.id !== id)), ms)
  }, [])
  return { toasts, show }
}

function getDeviceId(d?: StunDevice | null): number {
  // 空值保护：**必须**。设备的 deviceId 可能是悬空引用 ——
  // 服务/站点记着的 id 指向一台已被删除的设备时，devices.find() 返回 undefined，
  // 直接取 .DeviceID 会让整个穿透页崩掉（实测：删设备后打开穿透页必崩）。
  if (!d) return 0
  return d.DeviceID ?? d.deviceId ?? d.id
}

interface DeviceFormState {
  name: string
  ip: string
}
const emptyDevice: DeviceFormState = { name: '', ip: '' }

function DeviceModal({
  initial,
  prefill,
  onCancel,
  onSubmit,
}: {
  initial?: StunDevice
  /** 扫描结果点「添加」时带过来的 IP。只在新增时用，改的时候以 initial 为准 */
  prefill?: DeviceFormState
  onCancel: () => void
  onSubmit: (form: DeviceFormState) => Promise<void>
}) {
  const [form, setForm] = useState<DeviceFormState>(() =>
    initial ? { name: initial.name, ip: initial.ip } : (prefill ?? emptyDevice),
  )
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const submit = async () => {
    if (!form.name.trim() || !form.ip.trim()) {
      setErr('设备名称和 IP 不能为空')
      return
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

  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="max-h-full w-full max-w-md overflow-y-auto rounded-2xl bg-white p-6 text-slate-700 shadow-2xl ring-1 ring-slate-200">
        <div className="mb-5 flex items-center justify-between">
          <div className="text-base font-bold text-slate-800">{initial ? '编辑设备' : '添加设备'}</div>
          <button
            type="button"
            onClick={onCancel}
            className="grid h-7 w-7 place-items-center rounded-full text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="space-y-3">
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">设备名称</div>
            <input
              autoFocus
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              placeholder="如 群晖NAS / 树莓派"
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">设备内网 IP</div>
            <input
              value={form.ip}
              onChange={(e) => setForm((p) => ({ ...p, ip: e.target.value }))}
              placeholder="如 192.168.1.100"
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          {err && <div className="text-xs text-rose-500">{err}</div>}
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            className="rounded-xl bg-slate-100 px-4 py-2 text-sm font-semibold text-slate-600 transition hover:bg-slate-200"
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

/**
 * 局域网扫描。
 *
 * 「添加设备」原本要用户自己知道那台机器的内网 IP——得去路由器后台翻 DHCP 列表，
 * 或者跑去那台机器上敲命令。卡在这一步的人，后面打洞根本轮不到。
 * 这里直接把这段网里的机器摆出来选。
 *
 * 网段列表由外面先取好再传进来，弹窗一开下拉框就是满的。
 */

interface ServiceFormState {
  name: string
  internalPort: string
  protocol: 'TCP' | 'UDP'
  useUpnp: boolean
  // 访问控制（IP 门禁）：和反代站点同一个组件、同一套语义
  accessMode: string
  accessRules: string
  /** 打洞用的本机端口，'' 表示随机（和 0 一个意思） */
  punchLocalPort: string
  domain: string
  tlsTerminate: boolean
  /** 证书 ID，'0' 表示按域名自动匹配 */
  certId: string
  /**
   * 内网服务本身就是 HTTPS。界面上只有这一个勾，存的时候拆回后端那两个字段：
   * 加了证书时是 backendHttps（Catflap 解密后再用 HTTPS 连内网），
   * 没加证书时是 https（洞口不碰字节，只决定链接写成 https://）。见 formToPayloadTLS。
   */
  internalHttps: boolean
  enabled: boolean
  description: string
  webhookconfig: WebhookConfig
  /** 选用的全局通知渠道 ID（渠道本身在通知页配置） */
  channelIds: string[]
  /** 自定义通知正文（#{xxx} 占位符）。留空用默认文案 */
  notifyTemplate: string
  redirect: RedirectConfig
  mcEntry: MCEntryConfig
}

/** 界面上那一个「内网是 HTTPS」拆回后端的 https / backendHttps，语义和 stun.PublicScheme 一致 */
function formToPayloadTLS(form: ServiceFormState) {
  const tls = form.tlsTerminate && form.protocol === 'TCP'
  return {
    tlsTerminate: tls,
    certId: tls ? Number(form.certId) || 0 : 0,
    backendHttps: tls && form.internalHttps,
    https: !tls && form.protocol === 'TCP' && form.internalHttps,
  }
}

type ServiceModalTab = 'basic' | 'webhook' | 'redirect'

const fieldLabelCls = 'mb-1 text-xs font-semibold text-slate-500'
const fieldInputCls =
  'w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400'

const emptyRedirect: RedirectConfig = {
  enabled: false,
  providerId: 0,
  entryHost: '',
  zoneDomain: '',
}

const emptyMCEntry: MCEntryConfig = {
  enabled: false,
  providerId: 0,
  host: '',
  zoneDomain: '',
  target: '',
}

const emptyWebhook: WebhookConfig = {
  enabled: false,
  onlyWhenChanged: true,
  url: '',
  method: 'POST',
  headers: '',
  body: '{"service":"#{service_name}","address":"#{address}","phase":"#{phase}"}',
  disableSuccessCheck: true,
  successContains: '',
  proxy: '',
}


/**
 * 通知正文模板预设。
 *
 * 通知**怎么发**（PushPlus / 钉钉 / Bark 各自的格式和 token）在通知页统一配，
 * 服务里只写"我想说什么" —— 所以这里给的是**内容模板**而不是请求体，
 * 每个渠道的包装格式由后端按渠道类型处理，用户不用关心。
 * 占位符沿用 #{xxx}，和 webhook 那套一致，只有一套要记。
 */
const NOTIFY_TEMPLATES: { key: string; label: string; hint: string; body: string }[] = [
  {
    key: 'plain',
    label: '纯文本 · 通用',
    hint: '所有渠道都能用',
    body: '域名+端口：#{address}\nIP地址+端口：#{ip_addr}\n设备：#{device_name}\n时间：#{time}',
  },
  {
    key: 'pushplus',
    label: 'PushPlus',
    hint: 'markdown，信息全',
    body:
      '- **域名+端口**：#{address}\n- **IP地址+端口**：#{ip_addr}\n- **设备**：#{device_name}\n- **内网端口**：#{internal_port}\n- **时间**：#{time}',
  },
  {
    key: 'robot',
    label: '钉钉 / 企微 / 飞书',
    hint: '群机器人，引用块',
    body:
      '> 域名+端口：**#{address}**\n> IP地址+端口：#{ip_addr}\n\n设备：#{device_name}\n内网端口：#{internal_port}\n时间：#{time}',
  },
  {
    key: 'bark',
    label: 'Bark（iOS）',
    hint: '手机上越短越好',
    body: '#{address}',
  },
  {
    key: 'serverchan',
    label: 'Server 酱',
    hint: '一行地址 + 分隔线',
    body: '**#{address}**\n\n---\n\nIP地址+端口：#{ip_addr}\n设备：#{device_name}\n时间：#{time}',
  },
]

/** 可用占位符（发的时候自动换成真实值） */
const NOTIFY_PLACEHOLDERS = [
  '#{service_name}',
  '#{address}',
  '#{ip_addr}',
  '#{domain}',
  '#{external_ip}',
  '#{external_port}',
  '#{internal_port}',
  '#{device_name}',
  '#{target_ip}',
  '#{protocol}',
  '#{time}',
  '#{updated_at}',
]

// 通知模块的默认渠道：运行时由 effect 拉取并写进来。
// 模块顶层（emptyService）也引用它，所以必须是模块级的 mutable。
// 拿当前默认渠道（新服务用）。模块顶层：空服务常量也引用它。
function defaultChannels(): string[] { return [...defaultChannelIds] }

let defaultChannelIds: string[] = []
function setDefaultChannelIds(ids: string[]): void { defaultChannelIds = ids }

const emptyService: ServiceFormState = {
  name: '',
  internalPort: '',
  protocol: 'TCP',
  // UPnP 默认**关**：以前默认开，结果每条服务都去路由器上开映射，
  // 端口开一片自己都不知道。需要的人自己勾（打洞本身够用）。
  useUpnp: false,
  // 访问控制默认**黑名单**（规则空 = 谁都能连，但模式是 deny）——
  // 用户要求：避免每个服务都要去设一遍模式。审计里封 IP 同步过来直接能放。
  accessMode: 'deny',
  accessRules: '',
  punchLocalPort: '',
  domain: '',
  tlsTerminate: false,
  certId: '0',
  internalHttps: false,
  enabled: true,
  description: '',
  webhookconfig: emptyWebhook,
  channelIds: defaultChannels(),
  notifyTemplate: '',
  redirect: emptyRedirect,
  mcEntry: emptyMCEntry,
}

/** *.example.com 这种通配证书挑不出具体主机名——用哪个标签只能由用户决定 */
function isWildcardDomain(d: string) {
  return d.startsWith('*.')
}

/** 和后端 utils/domain.Match 同一套语义：通配只覆盖恰好多一层标签的名字 */
function matchCertDomain(pattern: string, name: string) {
  if (!name) return false
  if (!isWildcardDomain(pattern)) return pattern === name
  const suffix = pattern.slice(1) // ".example.com"
  if (!name.endsWith(suffix)) return false
  const label = name.slice(0, name.length - suffix.length)
  return label !== '' && !label.includes('.')
}

/** 选中的证书覆盖哪些域名。certId 为 0（按 SNI 自动匹配）时是所有启用证书的并集 */
function certDomainsOf(certs: Certificate[], certId: string): string[] {
  const id = Number(certId) || 0
  if (id !== 0) return certs.find((c) => c.id === id)?.domains ?? []
  return certs.filter((c) => c.enabled).flatMap((c) => c.domains)
}

/**
 * pickCertDomain 从证书上挑一个能直接当「对外域名」用的具体域名。
 *
 * 终结 TLS 时证书是按域名签的，而对外域名留空会回落公网 IP——IP 不发 SNI
 * （RFC 6066），证书也不可能覆盖 IP，首页链接和 /go 跳转就会带用户去一个
 * 必然报 ERR_CERT_COMMON_NAME_INVALID 的地址。证书上既然写着域名，
 * 就直接填上，不该让用户自己去证书页抄一遍。
 *
 * 挑不出来时返回空串（通配证书、或自签这种压根没有域名的证书）。
 */
function pickCertDomain(certs: Certificate[], certId: string): string {
  const concrete = (c?: Certificate) => c?.domains?.find((d) => !isWildcardDomain(d)) || ''

  const id = Number(certId) || 0
  if (id !== 0) return concrete(certs.find((c) => c.id === id))

  // 按 SNI 自动匹配：兜底证书是 SNI 落空时真正会用上的那张，优先看它；
  // 它没有具体域名（自签）时，只有唯一一张带域名的证书才好替用户做主
  const withDomain = certs.filter((c) => c.enabled && c.domains.some((d) => !isWildcardDomain(d)))
  const fallback = concrete(certs.find((c) => c.enabled && c.isDefault))
  if (fallback) return fallback
  return withDomain.length === 1 ? concrete(withDomain[0]) : ''
}

function normalizeWebhook(cfg?: Partial<WebhookConfig>): WebhookConfig {
  return {
    ...emptyWebhook,
    ...(cfg || {}),
    method: cfg?.method || emptyWebhook.method,
  }
}

/** 已存在的服务 → 表单。编辑和「复制」都从这儿取初值，省得两处各写一遍 */
function serviceToForm(svc: StunService): ServiceFormState {
  return {
    name: svc.name,
    internalPort: String(svc.internalPort || ''),
    protocol: (svc.protocol as 'TCP' | 'UDP') || 'TCP',
    useUpnp: !!svc.useUpnp,
    accessMode: svc.access?.mode ?? 'deny',
    accessRules: (svc.access?.rules ?? []).join('\n'),
    punchLocalPort: svc.punchLocalPort ? String(svc.punchLocalPort) : '',
    domain: svc.domain || '',
    tlsTerminate: !!svc.tlsTerminate,
    certId: String(svc.certId || 0),
    internalHttps: !!svc.backendHttps || !!svc.https,
    enabled: svc.enabled !== false,
    description: svc.description || '',
    webhookconfig: normalizeWebhook(svc.webhookconfig),
    channelIds: [...(svc.channelIds ?? [])],  // 后端可能是 null，见 notify 的同类修复
    notifyTemplate: svc.notifyTemplate ?? '',
    redirect: { ...emptyRedirect, ...(svc.redirect ?? {}) },
    mcEntry: { ...emptyMCEntry, ...(svc.mcEntry ?? {}) },
  }
}

/**
 * 已存在的服务 → 更新报文，原样一份。
 *
 * 列表上那个启停开关也得把整个服务发回去：后端的更新接口是整体覆盖，
 * 少带哪个字段，那个字段就被清成空的了。
 */
function serviceToPayload(deviceId: number, svc: StunService): api.StunServicePayload {
  return {
    deviceId,
    name: svc.name,
    internalPort: svc.internalPort,
    protocol: svc.protocol || 'TCP',
    useUpnp: !!svc.useUpnp,
    punchLocalPort: svc.punchLocalPort || 0,
    https: !!svc.https,
    domain: svc.domain || '',
    tlsTerminate: !!svc.tlsTerminate,
    certId: svc.certId || 0,
    backendHttps: !!svc.backendHttps,
    enabled: svc.enabled !== false,
    description: svc.description || '',
    webhookconfig: normalizeWebhook(svc.webhookconfig),
    channelIds: [...(svc.channelIds ?? [])],  // 后端可能是 null，见 notify 的同类修复
    notifyTemplate: svc.notifyTemplate ?? '',
    // access 必须带上：更新接口是整体覆盖，后端现在会读这个字段，
    // 少带就会被写成默认（等于用户配的门禁被抹掉）。
    access: { mode: svc.access?.mode ?? 'deny', rules: svc.access?.rules ?? [] },
    redirect: { ...emptyRedirect, ...(svc.redirect ?? {}) },
    mcEntry: { ...emptyMCEntry, ...(svc.mcEntry ?? {}) },
  }
}

function getWebhookStatus(status?: StunStatusEvent) {
  const logs = status?.logs ?? []
  for (let i = logs.length - 1; i >= 0; i--) {
    const msg = logs[i]?.message || ''
    if (msg.includes('Webhook 发送成功')) {
      return { state: 'success' as const, text: '最近发送成功', at: logs[i].createdAt }
    }
    if (msg.includes('Webhook 发送失败')) {
      return { state: 'failed' as const, text: '最近发送失败', at: logs[i].createdAt }
    }
  }
  return { state: 'pending' as const, text: '等待后台发送', at: '' }
}

function ServiceModal({
  deviceId,
  onDeviceChange,
  initial,
  prefill,
  status,
  onCancel,
  onSubmit,
  onToast,
}: {
  deviceId: number
  /** 换设备：服务会挪到另一台机器上，由父组件改 deviceId */
  onDeviceChange: (id: number) => void
  initial?: StunService
  /** 新建时的初值：扫描里点端口、或者复制现有服务带过来的。改的时候以 initial 为准 */
  prefill?: Partial<ServiceFormState>
  status?: StunStatusEvent
  onCancel: () => void
  onSubmit: (form: ServiceFormState) => Promise<void>
  onToast: (text: string) => void
}) {
  const [form, setForm] = useState<ServiceFormState>(() =>
    initial ? serviceToForm(initial) : { ...emptyService, ...prefill },
  )
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [activeTab, setActiveTab] = useState<ServiceModalTab>('basic')

  // 设备列表：服务要挂在某台设备上，和反代的服务表单同一套做法 ——
  // 选设备而不是先去加设备。deviceId 为 0 时用户必须选一台。
  const [devices, setDevices] = useState<Device[]>([])
  // 主域候选（来自 DDNS 解析记录，去重）—— 域名输入的下拉用
  const [ddnsZones, setDdnsZones] = useState<string[]>([])
  useEffect(() => {
    let alive = true
    api
      .getDdnsZones()
      .then((z) => alive && setDdnsZones(z))
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [])
  // 扫这台设备得到的端口（只探这一台，一两秒）
  const [hostPorts, setHostPorts] = useState<LanPort[]>([])
  const [scanning, setScanning] = useState(false)

  /** 扫**这台设备**上开了哪些端口，点结果就填进「内网端口」 */
  const scanThisDevice = async () => {
    const d = devices.find((x) => x.id === deviceId)
    if (!d) return
    setScanning(true)
    try {
      const ports = await api.scanHostPorts(d.ip)
      setHostPorts(ports)
      if (ports.length === 0) {
        onToast(`没探到 ${d.name}（${d.ip}）上开放的端口`)
      }
    } catch (e) {
      onToast('扫描失败: ' + (e instanceof Error ? e.message : ''))
    } finally {
      setScanning(false)
    }
  }
  useEffect(() => {
    let alive = true
    api
      .getDevices()
      .then((l) => alive && setDevices(l))
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [])
  const [certs, setCerts] = useState<Certificate[]>([])
  // 通知渠道列表：给「通知渠道」那个页签做多选。拉不到不该拦住表单。
  const [channels, setChannels] = useState<NotifyChannel[]>([])
  const [channelsLoading, setChannelsLoading] = useState(false)
  // 「发一条测试通知」的进行中与结果。结果按渠道逐条列出，
  // 哪个渠道没通一眼能看到，不用去猜。
  const [notifyTesting, setNotifyTesting] = useState(false)
  const [notifyTestResult, setNotifyTestResult] = useState<string>('')
  const [redirectProviders, setRedirectProviders] = useState<DdnsProvider[]>([])
  const [providerTypes, setProviderTypes] = useState<DdnsProviderTypeSpec[]>([])
  const [ddnsHosts, setDdnsHosts] = useState<string[]>([])
  const [redirectBusy, setRedirectBusy] = useState(false)
  const [inspect, setInspect] = useState<RedirectInspection | null>(null)
  const [inspecting, setInspecting] = useState(false)
  const [inspectErr, setInspectErr] = useState('')

  // 全局默认渠道：通知模块"启用"了哪些，新服务默认就用哪些
  const defaultChannelIdsRef = useRef<string[]>([])
  // 模块顶层常量（emptyService 引用）取不到 ref 当前值 ——
  // 这里用 ref 的 .current（取不到时是空数组）——

  // 通知渠道同理：拉不到就显示"还没有渠道"，用户去通知页建一个
  useEffect(() => {
    let alive = true
    setChannelsLoading(true)
    api
      .getNotifyConfig()
      .then((cfg) => {
        if (!alive) return
        setChannels(cfg.channels ?? [])
        setDefaultChannelIds(cfg.defaultChannelIds ?? [])
        // 全局默认渠道：新建服务时用它（用户从通知页"启用"了哪些渠道，
        // 新服务就默认用这些 —— 不用每次都重新选）
        defaultChannelIdsRef.current = cfg.defaultChannelIds ?? []
      })
      .catch(() => undefined)
      .finally(() => {
        if (alive) setChannelsLoading(false)
      })
    return () => {
      alive = false
    }
  }, [])

  /**
   * 发一条测试通知。
   *
   * 用户选好渠道、写完模板，最想知道的就是"这样配能不能真的收到"。
   * 单靠"保存成功"他没法验证 —— 真正的通知要等下一次穿透成功才发，
   * 而穿透成功什么时候来是不确定的。所以这里给一个立刻能按的按钮，
   * 走和真实事件完全相同的渲染 + 发送路径。
   */
  const testNotify = async () => {
    const ids = form.channelIds ?? []
    if (ids.length === 0) {
      onToast('先勾一个通知渠道再测试')
      return
    }
    setNotifyTesting(true)
    setNotifyTestResult('')
    try {
      const res = await api.testNotifyService({
        channelIds: ids,
        template: form.notifyTemplate,
        fields: {
          service_name: form.name || '测试服务',
          internal_port: form.internalPort || '',
          protocol: form.protocol,
          // 域名和当前外部端口一起带上，试发出来的地址才和真实通知一样
          // （不然模板里的 #{address} 是空的，看着像模板写错了）。
          // 公网 IP 不传 —— 后端会用当前的，前端拿不到这个服务的实时公网 IP。
          domain: form.domain || '',
          external_port: status?.externalPort ? String(status.externalPort) : '',
        },
      })
      const failed = (res.results ?? []).filter((r) => !r.ok)
      if (failed.length === 0) {
        setNotifyTestResult(`已发往 ${res.sent} 个渠道，去看手机有没有收到`)
      } else {
        setNotifyTestResult(
          `${res.sent}/${res.total} 成功；失败：` +
            failed.map((f) => `${f.channelName}（${f.error}）`).join('、'),
        )
      }
    } catch (e) {
      setNotifyTestResult('测试发送失败：' + (e instanceof Error ? e.message : ''))
    } finally {
      setNotifyTesting(false)
    }
  }

  // 证书列表拿不到不该拦住整个服务表单，失败就当没有证书
  useEffect(() => {
    let alive = true
    api
      .getCertList()
      .then((list) => alive && setCerts(list))
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [])

  // 重定向规则不是每家服务商都能写（Cloudflare 能，其余目前不能），
  // 只有具备 dns-redirect 能力的才列出来 —— 别的列出来只会让人白填。
  // 「谁能写」这件事由后端注册表说，这里不再写死服务商名字。
  //
  // DDNS 里现成的 A 记录也一起拿：MC 入口的 SRV 可以直接指过去，不用再建一条
  useEffect(() => {
    let alive = true
    loadProviderTypes().then((list) => {
      if (alive) setProviderTypes(list)
    })
    api
      .getDdnsConfig()
      .then((cfg) => {
        if (!alive) return
        setRedirectProviders(cfg.providers ?? [])
        setDdnsHosts(
          (cfg.records ?? [])
            .filter((r) => r.recordType === 'A')
            .map((r) => (r.subDomain && r.subDomain !== '@' ? `${r.subDomain}.${r.domain}` : r.domain).toLowerCase()),
        )
      })
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [])

  // 能力表还没拿到（首次加载中，或该接口失败）时**不筛**：
  // 宁可让用户试一次并看到后端那句明确的报错，也好过把一个能用的服务商藏起来，
  // 让人对着空下拉框找不到原因。
  const redirectCapableProviders = useMemo(() => {
    if (providerTypes.length === 0) return redirectProviders
    return redirectProviders.filter((p) => hasCapability(providerTypes, p.type, ProviderCap.Redirect))
  }, [redirectProviders, providerTypes])

  const updateRedirect = (patch: Partial<RedirectConfig>) =>
    setForm((p) => ({ ...p, redirect: { ...p.redirect, ...patch } }))
  const updateMCEntry = (patch: Partial<MCEntryConfig>) =>
    setForm((p) => ({ ...p, mcEntry: { ...p.mcEntry, ...patch } }))

  // 两个都开着（老配置）时先显示网页那块：它是先有的，用户多半就是为它开的
  const entryKind: 'none' | 'web' | 'mc' = form.redirect.enabled ? 'web' : form.mcEntry.enabled ? 'mc' : 'none'

  // 立即同步打的是已保存的配置，表单改了没保存就按不动（和入口重定向同一个道理）
  const mcEntryDirty = useMemo(() => {
    const saved = { ...emptyMCEntry, ...(initial?.mcEntry ?? {}) }
    const norm = (v?: string) => (v ?? '').trim().toLowerCase()
    return (
      !!saved.enabled !== form.mcEntry.enabled ||
      (saved.providerId || 0) !== form.mcEntry.providerId ||
      norm(saved.host) !== norm(form.mcEntry.host) ||
      norm(saved.zoneDomain) !== norm(form.mcEntry.zoneDomain) ||
      norm(saved.target) !== norm(form.mcEntry.target)
    )
  }, [initial, form.mcEntry])
  const [mcEntryBusy, setMCEntryBusy] = useState(false)
  const syncMCEntry = async () => {
    if (!initial) return
    setMCEntryBusy(true)
    try {
      const r = await api.syncStunMCEntry(deviceId, initial.id)
      onToast(r.msg || '已同步')
    } catch (e) {
      onToast('同步失败: ' + (e instanceof Error ? e.message : ''))
    } finally {
      setMCEntryBusy(false)
    }
  }
  /** 入口域名后两段，和后端 redirectZone 留空时的算法一致 */
  const redirectGuessedZone = useMemo(() => {
    const parts = form.redirect.entryHost.trim().toLowerCase().split('.').filter(Boolean)
    return parts.length >= 2 ? parts.slice(-2).join('.') : ''
  }, [form.redirect.entryHost])

  // 只是给人看这条规则长什么样；真正写过去的地址由后端按当时的端口算
  const redirectPreviewTarget = useMemo(() => {
    const port = status?.externalPort ?? 0
    if (!port) return status?.redirectTarget ?? ''
    // 与 stun.PublicScheme 同一套判断：加了证书，或者内网自己就是 HTTPS，外面都是 https
    const scheme = form.tlsTerminate || form.internalHttps ? 'https' : 'http'
    return `${scheme}://${form.domain.trim() || '公网IP'}:${port}`
  }, [status?.externalPort, status?.redirectTarget, form.tlsTerminate, form.internalHttps, form.domain])

  // 下面两个按钮打的是后端「已保存」的配置，不是眼前这张表单。
  // 刚勾上启用还没保存就点同步，后端读到的 enabled 还是 false，
  // 只会回一句「这个服务没有开启入口重定向」——界面上明明写着已启用，看着像见鬼。
  // 干脆表单一改就按不动，旁边直接说清楚该点哪个按钮。
  const redirectDirty = useMemo(() => {
    const saved = { ...emptyRedirect, ...(initial?.redirect ?? {}) }
    const norm = (v?: string) => (v ?? '').trim().toLowerCase()
    return (
      !!saved.enabled !== form.redirect.enabled ||
      (saved.providerId || 0) !== form.redirect.providerId ||
      norm(saved.entryHost) !== norm(form.redirect.entryHost) ||
      norm(saved.zoneDomain) !== norm(form.redirect.zoneDomain)
    )
  }, [initial, form.redirect])

  // 这两条记录归 Cloudflare 那边管，本地配置里看不出来，只能现场去查。
  // 查一次可能几百毫秒，所以不跟着服务列表一起刷，只在打开这个页签时拉一次。
  const savedRedirectOn = !!initial?.redirect?.enabled
  const loadInspect = useCallback(async () => {
    if (!initial || !savedRedirectOn) return
    setInspecting(true)
    setInspectErr('')
    try {
      setInspect(await api.inspectStunRedirect(deviceId, initial.id))
    } catch (e) {
      setInspect(null)
      setInspectErr(e instanceof Error ? e.message : '查不到')
    } finally {
      setInspecting(false)
    }
  }, [deviceId, initial, savedRedirectOn])

  // 切到「CF 重定向」那页才去查那两条记录：查一次要打 Cloudflare 的接口，
  // 打开弹窗就顺手跑一趟的话，只想改个端口的人也得白等
  const openTab = (key: ServiceModalTab) => {
    setActiveTab(key)
    if (key === 'redirect' && !inspect && !inspecting) void loadInspect()
  }

  // 同步/删除打的是已保存的配置，所以新建服务时按不动
  const runRedirect = async (action: 'sync' | 'remove') => {
    if (!initial) return
    if (action === 'remove' && !window.confirm('确认删掉 Cloudflare 上那条规则？')) return
    setRedirectBusy(true)
    try {
      if (action === 'sync') {
        // 直接用后端那句话：这一步可能只做成一半（规则写好了但解析记录没建上），
        // 在这儿按 target 自己拼一句「已同步」，就会把那半截失败盖掉
        const r = await api.syncStunRedirect(deviceId, initial.id)
        onToast(r.msg || `已同步：${r.data?.target ?? ''}`)
      } else {
        await api.removeStunRedirect(deviceId, initial.id)
        onToast('已删除 Cloudflare 上的规则')
      }
      // 同步那一步会顺手建入口记录，下面那块得跟着变，不然还摆着同步前的样子
      void loadInspect()
    } catch (e) {
      onToast((action === 'sync' ? '同步失败: ' : '删除失败: ') + (e instanceof Error ? e.message : ''))
    } finally {
      setRedirectBusy(false)
    }
  }

  // 老服务可能是「已经终结 TLS、对外域名却空着」的状态（那时候还没这条约束）。
  // 证书列表是异步来的，等它到位再补，且只补空的——用户自己填过的不动。
  useEffect(() => {
    if (certs.length === 0) return
    setForm((p) => {
      if (!p.tlsTerminate || p.protocol === 'UDP' || p.domain.trim()) return p
      const filled = pickCertDomain(certs, p.certId)
      return filled ? { ...p, domain: filled } : p
    })
  }, [certs])

  const tlsOn = form.tlsTerminate && form.protocol !== 'UDP'
  const certDomains = useMemo(() => certDomainsOf(certs, form.certId), [certs, form.certId])
  const domainValue = form.domain.trim().toLowerCase()
  // 绑定了一张带域名的证书却不填对外域名，生成出来的地址必然是证书盖不住的公网 IP。
  // 正常情况下 pickCertDomain 已经自动填好了，走到这里只剩通配证书——
  // *.example.com 用哪个标签只有用户知道，替他猜不如让他填。
  const domainRequired = tlsOn && Number(form.certId) !== 0 && certDomains.length > 0
  const domainCovered = certDomains.some((d) => matchCertDomain(d, domainValue))






  const submit = async () => {
    if (!form.name.trim()) {
      setErr('请填写服务名称')
      return
    }
    // 端口必填：留空的话后端地址会拼成 "ip:"，解析不出端口，转发直接失败
    const port = Number(form.internalPort)
    if (!port || port < 1 || port > 65535) {
      setErr('请填写内网端口(1-65535)。不知道填什么就填 80 —— 内网服务大多数跑在 80 上')
      return
    }
    if (domainRequired && !domainValue) {
      setErr(
        `请填写对外域名：绑定的证书覆盖 ${certDomains.join('、')}，留空会回落到公网 IP，证书盖不住 IP，浏览器会报证书错误`,
      )
      return
    }
    // 只留选择条上选中的那一种入口：老配置两个都开着时，没显示的那个就是要关掉的
    const entry = {
      redirect: { ...form.redirect, enabled: entryKind === 'web' },
      mcEntry: { ...form.mcEntry, enabled: entryKind === 'mc' },
    }
    if (form.protocol === 'TCP' && entry.mcEntry.enabled) {
      if (!form.mcEntry.providerId) {
        setErr('MC 入口要选一个 Cloudflare 账号')
        return
      }
      if (!form.mcEntry.host.trim().includes('.')) {
        setErr('MC 入口要填联机域名，如 mc.example.com')
        return
      }
    }
    setErr('')
    setBusy(true)
    try {
      // UDP 没有证书、HTTPS、入口重定向、SRV 一说，先勾选过再改协议的情况在这里抹掉
      await onSubmit(
        form.protocol === 'UDP'
          ? {
              ...form,
              tlsTerminate: false,
              certId: '0',
              internalHttps: false,
              redirect: { ...form.redirect, enabled: false },
              mcEntry: { ...form.mcEntry, enabled: false },
            }
          : { ...form, ...entry },
      )
    } catch (e) {
      setErr(e instanceof Error ? e.message : '操作失败')
      setBusy(false)
    }
  }

  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="flex max-h-[92vh] w-full max-w-3xl flex-col rounded-2xl bg-white text-slate-700 shadow-2xl ring-1 ring-slate-200">
        <div className="flex items-center justify-between px-6 py-5">
          <div className="text-base font-bold text-slate-800">{initial ? '编辑服务' : '添加服务'}</div>
          <button
            type="button"
            onClick={onCancel}
            className="grid h-7 w-7 place-items-center rounded-full text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="flex gap-8 border-b border-slate-100 px-6">
          {[
            { key: 'basic' as const, label: '基础配置' },
            // 入口重定向是 HTTP 307，只有浏览器听得懂；UDP 根本没有这回事
            ...(form.protocol === 'TCP' ? [{ key: 'redirect' as const, label: '对外入口' }] : []),
            { key: 'webhook' as const, label: 'Webhook' },
          ].map((tab) => (
            <button
              key={tab.key}
              type="button"
              onClick={() => openTab(tab.key)}
              className={`border-b-2 px-0 pb-3 text-sm font-bold transition ${
                activeTab === tab.key
                  ? 'border-blue-500 text-blue-600'
                  : 'border-transparent text-slate-500 hover:text-slate-800'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
          {activeTab === 'basic' && (
            <div className="space-y-4">
              {/*
                设备：先选跑在哪台机器上，再填服务本身 ——
                和反代的服务表单同一套顺序。**不是**先去加设备再来加服务：
                设备列表是共用的，选一个就行。
              */}
              <label className="block">
                <div className={fieldLabelCls}>设备</div>
                <div className="flex gap-2">
                  <select
                    value={deviceId || ''}
                    onChange={(e) => onDeviceChange(Number(e.target.value) || 0)}
                    className={`${fieldInputCls} min-w-0 flex-1`}
                  >
                    <option value="">选择设备…</option>
                    {devices.map((d) => (
                      <option key={d.id} value={d.id}>
                        {d.name}（{d.ip}）
                      </option>
                    ))}
                  </select>

                  {/* 扫**这台设备**上的端口：不用记端口号，点一下就填进「内网端口」。
                      和反代 / 端口转发的服务表单是同一个交互（后端同为 core/lanscan 的 ScanHostPorts）。 */}
                  <button
                    type="button"
                    onClick={scanThisDevice}
                    disabled={!deviceId || scanning}
                    className="flex shrink-0 items-center gap-1 rounded-xl px-2.5 py-2 text-sm font-semibold text-blue-600 ring-1 ring-blue-200 transition hover:bg-blue-50 disabled:opacity-40"
                    title="扫这台机器开了哪些端口"
                  >
                    <Radar className={`h-3.5 w-3.5 ${scanning ? 'animate-pulse' : ''}`} />
                    {scanning ? '扫描中' : '扫这台'}
                  </button>
                </div>

                {hostPorts.length > 0 && (
                  <div className="mt-2 flex flex-wrap gap-1 rounded-xl bg-slate-50 p-2">
                    {hostPorts.map((p) => (
                      <button
                        key={p.port}
                        type="button"
                        onClick={() => setForm((prev) => ({ ...prev, internalPort: String(p.port) }))}
                        className="rounded bg-white px-2 py-1 text-[11px] font-semibold text-slate-600 ring-1 ring-slate-200 transition hover:bg-blue-50 hover:text-blue-600 hover:ring-blue-300"
                      >
                        {p.port}
                        {p.name && <span className="ml-1 opacity-70">{p.name}</span>}
                      </button>
                    ))}
                  </div>
                )}

                <div className="mt-1 text-[11px] text-slate-400">
                  服务会加在这台设备下。机器没在列表里就去「设备」页加一台。
                </div>
              </label>

              <div className="grid grid-cols-2 gap-3">
                <label className="col-span-2 block sm:col-span-1">
                  <div className={fieldLabelCls}>服务名称</div>
                  <input
                    autoFocus
                    value={form.name}
                    onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
                    placeholder="如 我的世界 / NAS 后台"
                    className={fieldInputCls}
                  />
                </label>
                <label className="col-span-2 block sm:col-span-1">
                  <div className={fieldLabelCls}>内网端口</div>
                  <input
                    type="number"
                    value={form.internalPort}
                    onChange={(e) => setForm((p) => ({ ...p, internalPort: e.target.value }))}
                    placeholder="如 25565"
                    min={1}
                    max={65535}
                    className={fieldInputCls}
                  />
                </label>
              </div>

              {/* 打洞本机端口：钉住它外部端口才稳。不填就随机（兼容老用法） */}
              <label className="block">
                <div className={fieldLabelCls}>
                  穿透通道本机端口
                  <span className="ml-1 font-normal text-slate-400">（可选，建议填）</span>
                </div>
                <input
                  type="number"
                  value={form.punchLocalPort}
                  onChange={(e) => setForm((p) => ({ ...p, punchLocalPort: e.target.value }))}
                  placeholder="留空 = 每次随机（外部端口会一直变）"
                  min={1}
                  max={65535}
                  className={fieldInputCls}
                />
                <p className="mt-1 text-xs leading-5 text-slate-400">
                  外部端口是路由器照着这个本机端口分配的。不填的话本机端口每次随机，
                  外部端口也跟着一直变，配好的「域名+端口」过一会儿就失效；
                  填一个没被占用的端口（如 30000-60000），外部端口就稳定下来了，
                  不开 UPnP 也能长期使用。
                </p>
              </label>

              {/* 协议决定了下面能配什么：证书、HTTPS、入口重定向都只有 TCP 才有 */}
              <div>
                <div className={fieldLabelCls}>协议</div>
                <div className="grid grid-cols-2 gap-2">
                  {(
                    [
                      { value: 'TCP', hint: '网页、MC Java 版、SSH、远程桌面…' },
                      { value: 'UDP', hint: 'MC 基岩版、WireGuard、部分游戏…' },
                    ] as const
                  ).map((o) => (
                    <button
                      key={o.value}
                      type="button"
                      onClick={() => setForm((p) => ({ ...p, protocol: o.value }))}
                      className={`rounded-xl border px-3 py-2 text-left transition ${
                        form.protocol === o.value
                          ? 'border-blue-300 bg-blue-50/60 ring-1 ring-blue-200'
                          : 'border-slate-200 hover:border-slate-300'
                      }`}
                    >
                      <div
                        className={`text-sm font-bold ${form.protocol === o.value ? 'text-blue-600' : 'text-slate-700'}`}
                      >
                        {o.value}
                      </div>
                      <div className="mt-0.5 text-[11px] text-slate-400">{o.hint}</div>
                    </button>
                  ))}
                </div>
              </div>

              <div className="space-y-3 rounded-xl border border-slate-200/80 bg-slate-50/50 p-3">
                <div className="flex items-center gap-1.5 text-xs font-bold text-slate-700">
                  <ShieldCheck className="h-3.5 w-3.5 text-blue-500" />
                  对外访问
                </div>

                <label className="block">
                  <div className={fieldLabelCls}>对外域名</div>
                  {/* 子域手填 + 主域从 DDNS 选：打错主域整条规则解析不到，
                      而你能用哪些主域 DDNS 里本来就有 —— 选比填可靠。
                      没配 DDNS 时这个控件退化成普通完整域名输入。 */}
                  <DomainInput
                    value={form.domain}
                    zones={ddnsZones}
                    onChange={(v) => setForm((p) => ({ ...p, domain: v }))}
                    placeholder={tlsOn ? '必须是证书覆盖的域名' : '留空用公网 IP'}
                    className={fieldInputCls}
                  />
                  <div className="mt-1 text-[11px] text-slate-400">
                    {ddnsZones.length > 0
                      ? '主域来自 DDNS 解析记录；删掉那条解析后这里的主域就没了，服务会回落到公网 IP'
                      : '要在 DDNS 页解析到你的公网 IP'}
                  </div>
                </label>

                {form.protocol === 'TCP' && (
                  <>
                    {/* 洞口一终结 TLS，非 HTTPS 的连接全会被当成敲错地址的浏览器回 302，
                        MC、SSH 这类直接连不上——这是最容易配错的一项，旁边必须写明 */}
                    <label className="flex cursor-pointer items-start gap-2 rounded-xl bg-white px-3 py-2 ring-1 ring-slate-200">
                      <input
                        type="checkbox"
                        checked={form.tlsTerminate}
                        onChange={(e) =>
                          setForm((p) => ({
                            ...p,
                            tlsTerminate: e.target.checked,
                            // 证书按域名签，对外域名不能再空着回落 IP；证书上有现成的具体域名就直接填上
                            domain:
                              e.target.checked && !p.domain.trim() ? pickCertDomain(certs, p.certId) : p.domain,
                          }))
                        }
                        className="mt-0.5 h-3.5 w-3.5"
                      />
                      <span>
                        <span className="block text-xs font-semibold text-slate-700">加 HTTPS 证书</span>
                        <span className="block text-[11px] text-slate-400">
                          只有网页才勾。MC、SSH、远程桌面勾了会连不上
                        </span>
                      </span>
                    </label>

                    {tlsOn && (
                      <label className="ml-6 block">
                        <select
                          value={form.certId}
                          onChange={(e) =>
                            setForm((p) => ({
                              ...p,
                              certId: e.target.value,
                              domain: p.domain.trim() ? p.domain : pickCertDomain(certs, e.target.value),
                            }))
                          }
                          className={fieldInputCls}
                        >
                          <option value="0">自动（按域名匹配）</option>
                          {certs.map((c) => (
                            <option key={c.id} value={String(c.id)}>
                              {c.name}
                              {c.domains.length > 0 ? ` — ${c.domains.join('、')}` : ''}
                            </option>
                          ))}
                        </select>
                        {certs.length === 0 && (
                          <div className="mt-1 text-[11px] text-amber-600">还没有证书，先去「证书」页加一张</div>
                        )}
                      </label>
                    )}

                    {/* 证书按域名签：域名空着或不在证书范围内，浏览器必报证书错误，只在这时才说 */}
                    {tlsOn && certDomains.length > 0 && (!domainValue || !domainCovered) && (
                      <div className="ml-6 flex gap-2 rounded-xl bg-amber-50 px-3 py-2 text-[11px] leading-relaxed text-amber-700 ring-1 ring-amber-200">
                        <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                        <span>
                          对外域名要填证书覆盖的：
                          <span className="font-mono font-semibold">{certDomains.join('、')}</span>
                        </span>
                      </div>
                    )}

                    {/* 一个勾两种含义，后端拆成 backendHttps / https，见 formToPayloadTLS */}
                    <label className="flex cursor-pointer items-start gap-2 rounded-xl bg-white px-3 py-2 ring-1 ring-slate-200">
                      <input
                        type="checkbox"
                        checked={form.internalHttps}
                        onChange={(e) => setForm((p) => ({ ...p, internalHttps: e.target.checked }))}
                        className="mt-0.5 h-3.5 w-3.5"
                      />
                      <span>
                        <span className="block text-xs font-semibold text-slate-700">内网服务本身就是 HTTPS</span>
                        <span className="block text-[11px] text-slate-400">
                          {tlsOn
                            ? '勾了 Catflap 会用 HTTPS 去连它；它是明文就别勾，不然连不上'
                            : '它自带证书就勾，外面的链接会写成 https://'}
                        </span>
                      </span>
                    </label>
                  </>
                )}
              </div>

              <div className="grid grid-cols-1 gap-2 sm:grid-cols-3">
                {[
                  { key: 'enabled' as const, label: '启用服务', hint: '' },
                  {
                    key: 'useUpnp' as const,
                    label: '启用 UPnP',
                    hint: form.protocol === 'UDP' ? 'UDP 目前不做 UPnP 映射，勾了不起作用' : '',
                  },
                ].map((opt) => (
                  <label
                    key={opt.key}
                    title={opt.hint || undefined}
                    className="flex cursor-pointer items-center gap-2 rounded-xl bg-slate-50 px-3 py-2 text-xs font-medium text-slate-600"
                  >
                    <input
                      type="checkbox"
                      checked={form[opt.key]}
                      onChange={(e) => setForm((p) => ({ ...p, [opt.key]: e.target.checked }))}
                      className="h-3.5 w-3.5"
                    />
                    {opt.label}
                    {opt.hint && <span className="text-[10px] text-slate-400">（UDP 无效）</span>}
                  </label>
                ))}
              </div>

              <label className="block">
                <div className={fieldLabelCls}>描述（可选）</div>
                <input
                  value={form.description}
                  onChange={(e) => setForm((p) => ({ ...p, description: e.target.value }))}
                  placeholder="给自己看的备注"
                  className={fieldInputCls}
                />
              </label>
            </div>
          )}

          {activeTab === 'webhook' && (
            <div className="mx-auto max-w-2xl">
              <div className="mb-2 flex flex-wrap items-center gap-2 text-sm font-bold text-slate-800">
                <Send className="h-4 w-4 text-blue-500" />
                通知渠道
              </div>
              <div className="mb-3 rounded-xl bg-slate-50 px-3 py-2.5 text-[11px] leading-relaxed text-slate-500">
                渠道在「通知」页里统一配置（推送地址、token 都存那儿），这里只选用 ——
                同一个群机器人被多个服务用的时候，改一次全都跟着变。
                <a href="#/notify" className="ml-1 text-blue-600 underline-offset-2 hover:underline">
                  去配置渠道 ↗
                </a>
              </div>

              {channelsLoading ? (
                <div className="rounded-xl border border-slate-200 px-3 py-6 text-center text-xs text-slate-400">
                  正在加载渠道…
                </div>
              ) : channels.length === 0 ? (
                <div className="rounded-xl border border-dashed border-slate-300 px-3 py-6 text-center text-xs text-slate-400">
                  还没有任何通知渠道。
                  <a href="#/notify" className="ml-1 text-blue-600 underline-offset-2 hover:underline">
                    去添加一个
                  </a>
                  ，之后这个服务就能选它来发通知了。
                </div>
              ) : (
                <div className="space-y-1.5 rounded-xl border border-slate-200 p-3">
                  {channels.map((ch) => (
                    <label key={ch.id} className="flex cursor-pointer items-center gap-2 text-xs">
                      <input
                        type="checkbox"
                        checked={(form.channelIds ?? []).includes(ch.id)}
                        onChange={(e) =>
                          setForm((p) => ({
                            ...p,
                            channelIds: e.target.checked
                              ? [...(p.channelIds ?? []), ch.id]
                              : (p.channelIds ?? []).filter((id) => id !== ch.id),
                          }))
                        }
                      />
                      <span className={ch.enabled ? 'text-slate-700' : 'text-slate-400'}>
                        {ch.name}
                        {!ch.enabled && '（已停用，勾了也不会发）'}
                      </span>
                    </label>
                  ))}
                  {(form.channelIds ?? []).length === 0 && (
                    <div className="pt-1 text-[11px] text-slate-400">
                      一个都不选 = 这个服务不发通知。
                    </div>
                  )}
                </div>
              )}

              {/* 自定义通知正文：怎么发在通知页配好了，这里只写"说什么" */}
              <div className="mt-4">
                <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
                  <div className="text-sm font-bold text-slate-800">通知内容（可选）</div>
                  <select
                    value=""
                    onChange={(e) => {
                      const tpl = NOTIFY_TEMPLATES.find((t) => t.key === e.target.value)
                      if (tpl) setForm((p) => ({ ...p, notifyTemplate: tpl.body }))
                      e.target.value = ''
                    }}
                    className="rounded-lg border border-slate-200 bg-white px-2 py-1 text-[11px] outline-none focus:border-blue-400"
                    title="按渠道挑一条模板，填进下面"
                  >
                    <option value="">用一条模板…</option>
                    {NOTIFY_TEMPLATES.map((t) => (
                      <option key={t.key} value={t.key}>
                        {t.label} · {t.hint}
                      </option>
                    ))}
                  </select>
                </div>
                <textarea
                  value={form.notifyTemplate}
                  onChange={(e) => setForm((p) => ({ ...p, notifyTemplate: e.target.value }))}
                  rows={5}
                  placeholder="留空就用默认文案：服务名 + 外部地址 + 内网端口"
                  className="w-full resize-y rounded-xl border border-slate-200 bg-white px-3 py-2 font-mono text-xs outline-none focus:border-blue-400"
                />
                <div className="mt-1.5 text-[11px] leading-relaxed text-slate-400">
                  可用占位符（发的时候会自动换成真实值）：
                  <div className="mt-1 flex flex-wrap gap-1">
                    {NOTIFY_PLACEHOLDERS.map((p) => (
                      <span
                        key={p}
                        className="rounded bg-slate-50 px-1.5 py-0.5 font-mono text-[10px] text-slate-500 ring-1 ring-slate-200"
                      >
                        {p}
                      </span>
                    ))}
                  </div>
                </div>
              </div>

              {/* 立刻验证：保存成功不等于能收到，得真发一条才知道 */}
              <div className="mt-3 flex flex-wrap items-center gap-2">
                <button
                  type="button"
                  onClick={testNotify}
                  disabled={notifyTesting || (form.channelIds ?? []).length === 0}
                  className="rounded-lg bg-blue-500 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-blue-600 disabled:cursor-not-allowed disabled:opacity-40"
                >
                  {notifyTesting ? '正在发送…' : '发一条测试通知'}
                </button>
                <span className="text-[11px] text-slate-400">
                  用上面选的渠道和这条模板真发一次，验证能不能收到
                </span>
              </div>
              {notifyTestResult && (
                <div
                  className={`mt-2 rounded-xl px-3 py-2 text-[11px] leading-relaxed ${
                    notifyTestResult.includes('失败')
                      ? 'bg-rose-50 text-rose-600 ring-1 ring-rose-200'
                      : 'bg-emerald-50 text-emerald-700 ring-1 ring-emerald-200'
                  }`}
                >
                  {notifyTestResult}
                </div>
              )}

              {form.webhookconfig.url !== '' && (
                <div className="mt-3 rounded-xl bg-amber-50 px-3 py-2.5 text-[11px] leading-relaxed text-amber-700 ring-1 ring-amber-200">
                  这个服务原来配过内联 Webhook（{form.webhookconfig.url}）。
                  它已被迁移成一个全局渠道，原来的配置保留着没动 ——
                  如果迁移结果不对，可以在通知页打开「回退」退回旧行为。
                </div>
              )}
            </div>
          )}


          {/* 访问控制：和反代站点用同一个组件、同一套语义。
              穿透直接暴露在公网上，门禁比反代更需要 —— 默认就是黑名单。 */}
          {activeTab === 'basic' && (
            <div className="mt-4">
              <AccessControlEditor
                mode={form.accessMode as AccessMode}
                rules={form.accessRules}
                onChange={(next) =>
                  setForm((p) => ({ ...p, accessMode: next.mode, accessRules: next.rules }))
                }
              />
            </div>
          )}

          {activeTab === 'redirect' && form.protocol === 'TCP' && (
            <div className="mx-auto max-w-2xl">
              {/* 一个服务只用一种入口：网页走 307，MC 走 SRV。选哪个就只显示哪块，另一个自动关掉 */}
              <div className="mb-4 inline-flex w-full flex-wrap gap-1 rounded-xl bg-slate-100 p-1">
                {(
                  [
                    { key: 'none', label: '不用', icon: null },
                    { key: 'web', label: '网页 · Cloudflare 重定向', icon: CornerUpRight },
                    { key: 'mc', label: 'MC Java 版 · SRV', icon: Gamepad2 },
                  ] as const
                ).map((o) => (
                  <button
                    key={o.key}
                    type="button"
                    onClick={() =>
                      setForm((p) => ({
                        ...p,
                        redirect: { ...p.redirect, enabled: o.key === 'web' },
                        mcEntry: { ...p.mcEntry, enabled: o.key === 'mc' },
                      }))
                    }
                    className={`flex flex-1 items-center justify-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-semibold transition ${
                      entryKind === o.key ? 'bg-white text-blue-600 shadow-sm' : 'text-slate-500 hover:text-slate-700'
                    }`}
                  >
                    {o.icon && <o.icon className="h-3.5 w-3.5" />}
                    {o.label}
                  </button>
                ))}
              </div>

              {/* 老配置可能两个都开着（那时候没这个选择条），说清楚保存后会怎样 */}
              {form.redirect.enabled && form.mcEntry.enabled && (
                <div className="mb-4 flex gap-2 rounded-xl bg-amber-50 px-3 py-2 text-[11px] leading-relaxed text-amber-700 ring-1 ring-amber-200">
                  <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  两种入口现在都开着，上面选一个，另一个保存后会关掉、Cloudflare 上对应的记录也会清掉。
                </div>
              )}

              {entryKind === 'none' && (
                <div className="rounded-xl bg-slate-50 p-4 text-xs leading-6 text-slate-500">
                  外部端口会变。想让别人用一个固定的名字找到这个服务，选一种入口：
                  <div className="mt-1">
                    · <span className="font-semibold text-slate-600">网页</span>：浏览器访问入口域名，自动跳到当前端口
                  </div>
                  <div>
                    · <span className="font-semibold text-slate-600">MC Java 版</span>：朋友在 MC 里只填域名，不用填端口
                  </div>
                  <div className="text-slate-400">SSH、远程桌面这类两种都用不了，只能用「域名 + 当前端口」。</div>
                </div>
              )}

              {entryKind === 'web' && (
                <>
                  <div className="mb-4 rounded-xl bg-slate-50 p-3 text-xs leading-5 text-slate-500">
                    端口会变，入口域名不变：浏览器访问入口域名，Cloudflare 自动跳到当前端口。
                    <span className="text-slate-400">只对网页有用，MC、SSH 这类不认跳转。</span>
                    <div className="mt-2 flex flex-wrap items-center gap-2 font-mono text-[11px] text-slate-600">
                      <span className="rounded-md bg-white px-2 py-1 ring-1 ring-slate-200">
                        {form.redirect.entryHost.trim() || '入口域名'}
                      </span>
                      <span className="text-slate-400">→</span>
                      <span className="rounded-md bg-white px-2 py-1 ring-1 ring-slate-200">
                        {redirectPreviewTarget || '等服务跑起来才知道端口'}
                      </span>
                    </div>
                  </div>

                  {/*
                    规则写对了也可能访问不了，差的是这两条解析记录。
                    它们的状态只有 Cloudflare 那边知道，本地配置里一点都看不出来，
                    出了问题也不报错——只表现为「打不开」。所以现场查出来摆在这儿。
                  */}
                  {initial && savedRedirectOn && (
                    <div className="mb-4 rounded-xl border border-slate-200 p-3">
                      <div className="mb-2 flex items-center justify-between gap-2">
                        <div className="text-xs font-semibold text-slate-500">Cloudflare 上的解析记录</div>
                        <button
                          type="button"
                          disabled={inspecting}
                          onClick={() => void loadInspect()}
                          className="inline-flex items-center gap-1 rounded-lg px-1.5 py-0.5 text-[11px] font-semibold text-slate-400 transition hover:bg-slate-100 hover:text-slate-600 disabled:opacity-50"
                        >
                          <RotateCw className={`h-3 w-3 ${inspecting ? 'animate-spin' : ''}`} />
                          {inspecting ? '查询中' : '刷新'}
                        </button>
                      </div>

                      {inspectErr && <div className="text-[11px] leading-5 text-rose-500">查不了：{inspectErr}</div>}
                      {!inspectErr && !inspect && (
                        <div className="text-[11px] text-slate-400">
                          {inspecting ? '正在去 Cloudflare 查…' : '点右上角刷新看现在什么样'}
                        </div>
                      )}

                      {inspect && (
                        <div className="space-y-2.5">
                          <div>
                            <div className="flex flex-wrap items-center gap-1.5 text-[11px]">
                              <span className="rounded bg-slate-100 px-1.5 py-0.5 font-semibold text-slate-500">
                                入口
                              </span>
                              <span className="font-mono text-slate-700">{inspect.entry.host}</span>
                              {inspect.entry.found && (
                                <span className="font-mono text-slate-400">
                                  {inspect.entry.type} {inspect.entry.content}
                                </span>
                              )}
                              {inspect.entry.byCatflap && (
                                <span className="text-slate-400">· Catflap 建的</span>
                              )}
                            </div>
                            <div className="mt-1 text-[11px] leading-5">
                              {inspect.entry.warn ? (
                                <span className="text-amber-600">{inspect.entry.warn}</span>
                              ) : !inspect.entry.found ? (
                                <span className="text-rose-500">
                                  还没有这条记录，现在访问它会提示域名不存在。点下面「立即同步」，Catflap
                                  会建好
                                </span>
                              ) : inspect.entry.proxied ? (
                                <span className="text-emerald-600">小黄云开着，重定向能生效</span>
                              ) : (
                                <span className="text-amber-600">
                                  小黄云是关的 ——
                                  请求根本到不了 Cloudflare，重定向不会执行，访问只会停在{' '}
                                  <span className="font-mono">{inspect.entry.wantIP}</span>{' '}
                                  这个谁都不在的地址上。去 Cloudflare 的 DNS
                                  里把这条记录的云朵点成橙色
                                </span>
                              )}
                            </div>
                          </div>

                          <div>
                            <div className="flex flex-wrap items-center gap-1.5 text-[11px]">
                              <span className="rounded bg-slate-100 px-1.5 py-0.5 font-semibold text-slate-500">
                                落地
                              </span>
                              <span className="font-mono text-slate-700">
                                {inspect.landingHost || '还没定'}
                              </span>
                              {inspect.landing.managed && inspect.landing.lastIP && (
                                <span className="font-mono text-slate-400">A {inspect.landing.lastIP}</span>
                              )}
                            </div>
                            <div className="mt-1 text-[11px] leading-5">
                              {!inspect.landingHost ? (
                                <span className="text-slate-400">
                                  这个服务没填对外域名，307 直接跳到公网 IP，不需要解析记录
                                </span>
                              ) : inspect.landing.managed && inspect.landing.status === 'failed' ? (
                                // 「在改」和「没改成」凑一句话会自相矛盾，失败就只说失败
                                <span className="text-amber-600">
                                  DDNS 记录「{inspect.landing.name}」上次没改成 ——{' '}
                                  {inspect.landing.message || '未知原因'}
                                </span>
                              ) : inspect.landing.managed ? (
                                <span className="text-emerald-600">
                                  DDNS 记录「{inspect.landing.name}」跟着公网 IP 在改
                                  {fmtTime(inspect.landing.at) ? `（${fmtTime(inspect.landing.at)}）` : ''}
                                </span>
                              ) : (
                                <span className="text-amber-600">
                                  DDNS 里没有记录管它，公网 IP
                                  一变这个域名就指错地方了。点一次下面的「立即同步」，Catflap 会加上
                                </span>
                              )}
                            </div>
                          </div>
                        </div>
                      )}

                      <div className="mt-2.5 border-t border-slate-100 pt-2 text-[11px] leading-5 text-slate-400">
                        两条的要求正好相反：入口那条要开小黄云，落地那条要关 —— 开了的话 Cloudflare
                        不转发打洞出来的高位端口，一样访问不了。
                      </div>
                    </div>
                  )}

                  <div className="grid grid-cols-2 gap-3">
                    <label className="col-span-2 block sm:col-span-1">
                      <div className="mb-1 text-xs font-semibold text-slate-500">Cloudflare 账号</div>
                      <select
                        value={String(form.redirect.providerId || 0)}
                        onChange={(e) => updateRedirect({ providerId: Number(e.target.value) || 0 })}
                        className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400"
                      >
                        <option value="0">请选择</option>
                        {redirectCapableProviders.map((p) => (
                          <option key={p.id} value={String(p.id)}>
                            {p.name}
                          </option>
                        ))}
                      </select>
                      <div className="mt-1 text-[11px] text-slate-400">
                        {redirectCapableProviders.length === 0
                          ? '"DDNS" 页面里还没有 Cloudflare 账号，先去那边加一个'
                          : '令牌要有 DNS 编辑 + 单一重定向编辑两项权限'}
                      </div>
                    </label>

                    <label className="col-span-2 block sm:col-span-1">
                      <div className="mb-1 text-xs font-semibold text-slate-500">入口域名</div>
                      <input
                        value={form.redirect.entryHost}
                        onChange={(e) => updateRedirect({ entryHost: e.target.value })}
                        placeholder="catflap.example.com"
                        className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400"
                      />
                      <div className="mt-1 text-[11px] text-slate-400">
                        外面记的就是这个，Catflap 会自己建好；别和基础配置里的对外域名填成同一个
                      </div>
                    </label>

                    <label className="col-span-2 block sm:col-span-1">
                      <div className="mb-1 text-xs font-semibold text-slate-500">主域名</div>
                      <input
                        value={form.redirect.zoneDomain}
                        onChange={(e) => updateRedirect({ zoneDomain: e.target.value })}
                        placeholder={redirectGuessedZone || 'example.com'}
                        className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400"
                      />
                      <div className="mt-1 text-[11px] text-slate-400">
                        {redirectGuessedZone
                          ? `留空就按 ${redirectGuessedZone} 算；域名多一层（如 a.b.co.uk）才需要填`
                          : '留空按入口域名的后两段算'}
                      </div>
                    </label>
                  </div>

                  {initial ? (
                    <div className="mt-4 rounded-xl border border-slate-200 p-3">
                      <div className="mb-2 text-xs font-semibold text-slate-500">最近一次同步</div>
                      {status?.redirectStatus === 'ok' && (
                        <div className="text-xs text-emerald-600">
                          成功 → <span className="font-mono">{status.redirectTarget}</span>
                          {status.redirectAt ? `（${fmtTime(status.redirectAt)}）` : ''}
                          {status.redirectKeepPath === false && (
                            <div className="mt-1 text-amber-600">
                              服务商没接受保留路径的写法，从子路径进来会落到根路径
                            </div>
                          )}
                        </div>
                      )}
                      {status?.redirectStatus === 'stale' && (
                        <div className="text-xs text-amber-600">
                          外网端口已经换了，现在从入口域名进来打不开 —— 那边还指着上一个地址{' '}
                          <span className="font-mono">{status.redirectTarget}</span>
                          {status.redirectAt ? `（${fmtTime(status.redirectAt)}）` : ''}
                          <div className="mt-1 text-slate-400">
                            洞确认通了会自己改过去，一般几十秒。等不及就点下面的「立即同步」
                          </div>
                        </div>
                      )}
                      {status?.redirectStatus === 'failed' && (
                        <div className="text-xs text-rose-500">
                          失败：{status.redirectError || '未知原因'}
                          {status.redirectAt ? `（${fmtTime(status.redirectAt)}）` : ''}
                        </div>
                      )}
                      {!status?.redirectStatus && (
                        <div className="text-xs text-slate-400">还没同步过</div>
                      )}

                      <div className="mt-3 flex flex-wrap items-center gap-2">
                        <button
                          type="button"
                          disabled={redirectBusy || redirectDirty}
                          onClick={() => runRedirect('sync')}
                          className="inline-flex items-center gap-1.5 rounded-xl bg-blue-500 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-blue-600 disabled:opacity-50"
                        >
                          <RotateCw className="h-3.5 w-3.5" />
                          立即同步
                        </button>
                        <button
                          type="button"
                          disabled={redirectBusy || redirectDirty}
                          onClick={() => runRedirect('remove')}
                          className="inline-flex items-center gap-1.5 rounded-xl bg-slate-100 px-3 py-1.5 text-xs font-semibold text-slate-600 transition hover:bg-rose-50 hover:text-rose-500 disabled:opacity-50"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                          删掉 Cloudflare 上的规则
                        </button>
                        {redirectDirty ? (
                          <span className="text-[11px] font-semibold text-amber-600">
                            上面改的还没保存 —— 点右下角「保存」，几秒后自己就同步了，不用回来点这两个
                          </span>
                        ) : (
                          <span className="text-[11px] text-slate-400">
                            端口一变会自动同步，这里是手动再跑一遍
                          </span>
                        )}
                      </div>
                    </div>
                  ) : (
                    <div className="mt-4 text-[11px] text-slate-400">先把服务保存了，再回来点同步</div>
                  )}
                </>
              )}

              {entryKind === 'mc' && (
                <div>
                  <div className="mb-4 rounded-xl bg-slate-50 p-3 text-xs leading-5 text-slate-500">
                    朋友在 MC 里只填域名，不用填端口；端口变了 Catflap 自动改 SRV 记录。
                    <span className="text-slate-400">基岩版不认 SRV，用不上。</span>
                    <div className="mt-2 flex flex-wrap items-center gap-2 font-mono text-[11px] text-slate-600">
                      <span className="text-slate-400">朋友填</span>
                      <span className="rounded-md bg-white px-2 py-1 font-semibold ring-1 ring-slate-200">
                        {form.mcEntry.host.trim() || 'mc.example.com'}
                      </span>
                    </div>
                  </div>

                  <div className="grid grid-cols-2 gap-3">
                    <label className="col-span-2 block sm:col-span-1">
                      <div className={fieldLabelCls}>Cloudflare 账号</div>
                      <select
                        value={String(form.mcEntry.providerId || 0)}
                        onChange={(e) => updateMCEntry({ providerId: Number(e.target.value) || 0 })}
                        className={`${fieldInputCls} disabled:bg-slate-50 disabled:text-slate-400`}
                      >
                        <option value="0">请选择</option>
                        {redirectCapableProviders.map((p) => (
                          <option key={p.id} value={String(p.id)}>
                            {p.name}
                          </option>
                        ))}
                      </select>
                      <div className="mt-1 text-[11px] text-slate-400">
                        {redirectCapableProviders.length === 0
                          ? '"DDNS" 页面里还没有 Cloudflare 账号，先去那边加一个'
                          : '「编辑区域 DNS」模板建的令牌就够'}
                      </div>
                    </label>

                    <label className="col-span-2 block sm:col-span-1">
                      <div className={fieldLabelCls}>联机域名</div>
                      <input
                        value={form.mcEntry.host}
                        onChange={(e) => updateMCEntry({ host: e.target.value })}
                        placeholder="mc.example.com"
                        className={`${fieldInputCls} disabled:bg-slate-50 disabled:text-slate-400`}
                      />
                      <div className="mt-1 text-[11px] text-slate-400">朋友在 MC 里填的就是它</div>
                    </label>

                    {/* SRV 只能指向域名不能指向 IP，那个域名自己得解析到公网 IP。
                        要么让 Catflap 替联机域名新建一条，要么直接用 DDNS 里现成的 */}
                    <div className="col-span-2">
                      <div className={fieldLabelCls}>SRV 指向</div>
                      <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                        {(
                          [
                            {
                              existing: false,
                              title: '新建一条',
                              hint: `在 DDNS 里给 ${form.mcEntry.host.trim() || '联机域名'} 加一条 A 记录`,
                            },
                            { existing: true, title: '用 DDNS 里已有的', hint: '指向已经解析好的域名，不多建记录' },
                          ] as const
                        ).map((o) => {
                          const on = !!form.mcEntry.target === o.existing
                          return (
                            <button
                              key={o.title}
                              type="button"
                              onClick={() =>
                                updateMCEntry({
                                  target: o.existing ? form.mcEntry.target || form.domain.trim() || ddnsHosts[0] || '' : '',
                                })
                              }
                              className={`rounded-xl border px-3 py-2 text-left transition disabled:opacity-50 ${
                                on ? 'border-blue-300 bg-blue-50/60 ring-1 ring-blue-200' : 'border-slate-200 hover:border-slate-300'
                              }`}
                            >
                              <div className={`text-xs font-bold ${on ? 'text-blue-600' : 'text-slate-700'}`}>{o.title}</div>
                              <div className="mt-0.5 text-[11px] text-slate-400">{o.hint}</div>
                            </button>
                          )
                        })}
                      </div>

                      {!!form.mcEntry.target && (
                        <select
                          value={form.mcEntry.target}
                          onChange={(e) => updateMCEntry({ target: e.target.value })}
                          className={`${fieldInputCls} mt-2 disabled:bg-slate-50 disabled:text-slate-400`}
                        >
                          {/* 存着的那个不在 DDNS 里了（被删了）也得让人看见选的是谁 */}
                          {!ddnsHosts.includes(form.mcEntry.target) && (
                            <option value={form.mcEntry.target}>{form.mcEntry.target}（DDNS 里没有这条）</option>
                          )}
                          {ddnsHosts.map((h) => (
                            <option key={h} value={h}>
                              {h}
                            </option>
                          ))}
                        </select>
                      )}

                      <div className="mt-2 font-mono text-[11px] text-slate-500">
                        _minecraft._tcp.{form.mcEntry.host.trim() || 'mc.example.com'} →{' '}
                        {form.mcEntry.target || form.mcEntry.host.trim() || 'mc.example.com'}:端口
                      </div>
                    </div>
                  </div>

                  {initial ? (
                    <div className="mt-4 rounded-xl border border-slate-200 p-3">
                      <div className="mb-2 text-xs font-semibold text-slate-500">最近一次同步</div>
                      {status?.mcEntryStatus === 'ok' && (
                        <div className="text-xs text-emerald-600">
                          成功 →{' '}
                          <span className="font-mono">
                            {initial.mcEntry?.target || initial.mcEntry?.host}:{status.mcEntryPort}
                          </span>
                          {status.mcEntryAt ? `（${fmtTime(status.mcEntryAt)}）` : ''}
                          {!!status.externalPort && status.externalPort !== status.mcEntryPort && (
                            <div className="mt-1 text-amber-600">
                              外部端口已经换成 {status.externalPort}，洞确认通了会自己改过去
                            </div>
                          )}
                        </div>
                      )}
                      {status?.mcEntryStatus === 'failed' && (
                        <div className="text-xs text-rose-500">
                          失败：{status.mcEntryError || '未知原因'}
                          {status.mcEntryAt ? `（${fmtTime(status.mcEntryAt)}）` : ''}
                        </div>
                      )}
                      {!status?.mcEntryStatus && <div className="text-xs text-slate-400">还没同步过</div>}

                      <div className="mt-3 flex flex-wrap items-center gap-2">
                        <button
                          type="button"
                          disabled={mcEntryBusy || mcEntryDirty || !initial.mcEntry?.enabled}
                          onClick={syncMCEntry}
                          className="inline-flex items-center gap-1.5 rounded-xl bg-blue-500 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-blue-600 disabled:opacity-50"
                        >
                          <RotateCw className="h-3.5 w-3.5" />
                          立即同步
                        </button>
                        <span
                          className={`text-[11px] ${mcEntryDirty ? 'font-semibold text-amber-600' : 'text-slate-400'}`}
                        >
                          {mcEntryDirty ? '上面改的还没保存 —— 点右下角「保存」，几秒后自己就同步了' : '端口一变会自动同步'}
                        </span>
                      </div>
                    </div>
                  ) : (
                    <div className="mt-4 text-[11px] text-slate-400">先把服务保存了，再回来点同步</div>
                  )}
                </div>
              )}
            </div>
          )}
        </div>

        {err && <div className="px-6 pb-2 text-xs text-rose-500">{err}</div>}
        <div className="flex justify-end gap-2 border-t border-slate-100 px-6 py-4">
          <button
            type="button"
            onClick={onCancel}
            className="rounded-xl bg-slate-100 px-4 py-2 text-sm font-semibold text-slate-600 transition hover:bg-slate-200"
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

function LogModal({
  status,
  onClose,
}: {
  status: StunStatusEvent | null
  onClose: () => void
}) {
  const bodyRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!bodyRef.current) return
    const el = bodyRef.current
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40
    if (nearBottom) el.scrollTop = el.scrollHeight
  }, [status?.logs?.length])

  if (!status) return null
  const logs = status.logs ?? []
  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="flex w-full max-w-2xl flex-col rounded-2xl bg-white shadow-2xl ring-1 ring-slate-200">
        <div className="flex items-center justify-between border-b border-slate-100 px-6 py-4">
          <div className="text-base font-bold text-slate-800">
            {status.deviceName || ''} / {status.serviceName || ''} - 服务日志
          </div>
          <button
            type="button"
            onClick={onClose}
            className="grid h-7 w-7 place-items-center rounded-full text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="px-6 py-3 text-xs text-slate-500">
          当前阶段：<span className="font-semibold text-slate-700">{phaseLabel[status.phaseStr] || status.phaseStr}</span>
          {' · '}重启次数：<span className="font-semibold text-slate-700">{status.restartCount ?? 0}</span>
          {' · '}共 {logs.length} 条日志
        </div>
        <div ref={bodyRef} className="max-h-[60vh] overflow-y-auto bg-slate-50 px-6 py-3">
          {logs.length === 0 ? (
            <div className="py-8 text-center text-sm text-slate-400">暂无日志</div>
          ) : (
            logs.map((l, i) => (
              <div
                key={i}
                className="flex items-start gap-3 border-b border-slate-100 py-1.5 text-xs last:border-0"
              >
                <span className="font-mono text-[11px] text-slate-400 whitespace-nowrap">
                  {new Date(l.createdAt).toLocaleString()}
                </span>
                <span className="shrink-0 rounded-md bg-blue-50 px-1.5 py-0.5 text-[10px] font-bold text-blue-600">
                  {phaseLabel[l.phaseStr] || l.phaseStr}
                </span>
                <span className="break-all text-slate-700">{l.message}</span>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}

const mappingLabels: Record<string, string> = {
  EIM: '端点无关映射',
  ADM: '地址相关映射',
  APDM: '地址和端口相关映射',
  unknown: '未知',
}

const filteringLabels: Record<string, string> = {
  EIF: '端点无关过滤',
  ADF: '地址相关过滤',
  APDF: '地址和端口相关过滤',
  unknown: '未知',
}

function behaviorText(value: string, labels: Record<string, string>) {
  if (!value) return '--'
  const label = labels[value]
  return label ? `${label} (${value})` : value
}

function natTypeTone(natType: string) {
  if (natType.includes('NAT1')) return { panel: 'border-emerald-200 bg-emerald-50/40', badge: 'bg-emerald-100 text-emerald-700' }
  if (natType.includes('NAT2')) return { panel: 'border-sky-200 bg-sky-50/40', badge: 'bg-sky-100 text-sky-700' }
  if (natType.includes('NAT3')) return { panel: 'border-amber-200 bg-amber-50/40', badge: 'bg-amber-100 text-amber-700' }
  if (natType.includes('NAT4') || natType.includes('阻断')) return { panel: 'border-rose-200 bg-rose-50/40', badge: 'bg-rose-100 text-rose-700' }
  if (natType.includes('公网')) return { panel: 'border-teal-200 bg-teal-50/40', badge: 'bg-teal-100 text-teal-700' }
  return { panel: 'border-slate-200 bg-slate-50/70', badge: 'bg-slate-100 text-slate-600' }
}

function NatProtocolResult({ protocol, result }: { protocol: 'UDP' | 'TCP'; result: NatDetectResult | null }) {
  const tone = natTypeTone(result?.natType ?? '')
  const ProtocolIcon = protocol === 'UDP' ? Wifi : Server

  if (!result) {
    return (
      <section className="rounded-lg border border-dashed border-slate-200 bg-slate-50/70 p-4">
        <div className="flex items-center gap-2 text-sm font-bold text-slate-700">
          <ProtocolIcon className="h-4 w-4 text-slate-400" /> {protocol}
        </div>
        <div className="mt-5 text-center text-sm text-slate-400">尚无检测结果</div>
      </section>
    )
  }

  return (
    <section className={`rounded-lg border p-4 ${tone.panel}`}>
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <span className="grid h-8 w-8 shrink-0 place-items-center rounded-md bg-white text-slate-600 shadow-sm ring-1 ring-slate-200/80">
            <ProtocolIcon className="h-4 w-4" />
          </span>
          <div>
            <div className="text-sm font-bold text-slate-800">{protocol}</div>
            <div className="text-[11px] text-slate-400">{protocol === 'UDP' ? 'Mapping + Filtering' : 'Mapping'}</div>
          </div>
        </div>
        <span className={`shrink-0 rounded-md px-2 py-1 text-[11px] font-bold ${tone.badge}`}>
          {result.mappingErr ? '部分失败' : '检测完成'}
        </span>
      </div>

      <div className="mt-4">
        <div className="text-[11px] font-semibold text-slate-400">NAT 类型</div>
        <div className="mt-1 break-words text-sm font-bold leading-5 text-slate-800">{result.natType || '无法确定'}</div>
      </div>

      <dl className="mt-4 grid gap-2 text-xs sm:grid-cols-2">
        <div className="min-w-0 rounded-md bg-white/80 px-3 py-2 ring-1 ring-slate-200/70">
          <dt className="text-[11px] font-semibold text-slate-400">Mapping</dt>
          <dd className="mt-1 break-words font-medium leading-5 text-slate-700">{behaviorText(result.mapping, mappingLabels)}</dd>
          {result.mapping && result.mapping !== 'unknown' && (
            <dd className="mt-1.5 text-[10px] leading-relaxed text-slate-500">
              {result.mapping === 'EIM' && '✓ 最佳：同一端口对所有目标使用相同映射，打洞成功率最高'}
              {result.mapping === 'ADM' && '△ 中等：不同目标 IP 使用不同映射，需要端口预测'}
              {result.mapping === 'APDM' && '✗ 困难：每个目标 IP+端口都不同，打洞成功率低'}
            </dd>
          )}
        </div>
        <div className="min-w-0 rounded-md bg-white/80 px-3 py-2 ring-1 ring-slate-200/70">
          <dt className="text-[11px] font-semibold text-slate-400">Filtering</dt>
          <dd className="mt-1 break-words font-medium leading-5 text-slate-700">
            {protocol === 'TCP' ? 'TCP 不适用' : behaviorText(result.filtering, filteringLabels)}
          </dd>
          {protocol === 'TCP' ? (
            <dd className="mt-1.5 text-[10px] leading-relaxed text-slate-500">
              TCP 面向连接特性导致 Filtering 无法准确测试
            </dd>
          ) : result.filtering && result.filtering !== 'unknown' ? (
            <dd className="mt-1.5 text-[10px] leading-relaxed text-slate-500">
              {result.filtering === 'EIF' && '✓ 宽松：任何外部地址都能连入'}
              {result.filtering === 'ADF' && '△ 中等：需要先向目标发包建立连接'}
              {result.filtering === 'APDF' && '○ 严格：需要精确匹配端口，但 EIM 下仍可打洞'}
            </dd>
          ) : null}
        </div>
      </dl>
      {(result.mappingErr || result.filteringErr) && (
        <div className="mt-3 space-y-1 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">
          {result.mappingErr && <div className="break-words">Mapping: {result.mappingErr}</div>}
          {result.filteringErr && <div className="break-words">Filtering: {result.filteringErr}</div>}
        </div>
      )}
    </section>
  )
}

function NatTypeModal({ onClose }: { onClose: () => void }) {
  const [data, setData] = useState<NatTypeInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [detecting, setDetecting] = useState(false)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      setData(await api.getNatType())
    } catch (e) {
      setError(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  const detect = async () => {
    setDetecting(true)
    setError('')
    try {
      setData(await api.detectNatType())
    } catch (e) {
      setError(e instanceof Error ? e.message : '重新检测失败')
    } finally {
      setDetecting(false)
    }
  }

  useEffect(() => {
    let active = true
    api.getNatType()
      .then((result) => {
        if (active) setData(result)
      })
      .catch((e) => {
        if (active) setError(e instanceof Error ? e.message : '加载失败')
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [])

  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="flex max-h-[88vh] w-full max-w-2xl flex-col rounded-lg bg-white shadow-2xl ring-1 ring-slate-200">
        <div className="flex items-center justify-between gap-4 border-b border-slate-100 px-5 py-4">
          <div className="flex min-w-0 items-center gap-3">
            <span className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-blue-50 text-blue-600 ring-1 ring-blue-100">
              <Info className="h-4.5 w-4.5" />
            </span>
            <div className="min-w-0">
              <div className="text-base font-bold text-slate-800">NAT 类型详情</div>
              <div className="mt-0.5 text-xs text-slate-400">RFC 5780 行为检测</div>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            title="关闭"
            aria-label="关闭"
            className="grid h-8 w-8 place-items-center rounded-md text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="overflow-y-auto p-5">
          {loading ? (
            <div className="flex min-h-48 items-center justify-center gap-2 text-sm text-slate-400">
              <LoaderCircle className="h-4 w-4 animate-spin" /> 正在加载
            </div>
          ) : error ? (
            <div className="flex min-h-48 flex-col items-center justify-center gap-3 text-sm text-rose-600">
              <AlertCircle className="h-6 w-6" />
              <span>{error}</span>
              <button
                type="button"
                onClick={load}
                className="flex items-center gap-1 rounded-md border border-slate-200 px-3 py-1.5 text-xs font-semibold text-slate-600 hover:bg-slate-50"
              >
                <RotateCw className="h-3.5 w-3.5" /> 重试
              </button>
            </div>
          ) : (
            <div className="space-y-3">
              {data?.error && (
                <div className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800">
                  <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
                  <span className="break-words">部分探测未完成：{data.error}</span>
                </div>
              )}
              <div className="grid gap-3 md:grid-cols-2">
                <NatProtocolResult protocol="UDP" result={data?.udp ?? null} />
                <NatProtocolResult protocol="TCP" result={data?.tcp ?? null} />
              </div>
            </div>
          )}
        </div>
        {!loading && !error && (
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-slate-100 px-5 py-3">
            <span className="text-xs text-slate-400">重新检测通常需要数秒</span>
            <button
              type="button"
              onClick={detect}
              disabled={detecting}
              className="flex min-w-[104px] items-center justify-center gap-1.5 rounded-md bg-blue-600 px-3 py-2 text-xs font-semibold text-white shadow-sm transition hover:bg-blue-700 disabled:cursor-wait disabled:opacity-60"
            >
              <RotateCw className={`h-3.5 w-3.5 ${detecting ? 'animate-spin' : ''}`} />
              {detecting ? '检测中...' : '重新检测'}
            </button>
          </div>
        )}
      </div>
    </div>
  )
}

// StunServersModal 编辑 STUN 服务器列表（每行一个），保存后后端立即重测选优。
// 实测有些服务商的 STUN 会很快掐掉 TCP 连接（比如 sonetel），导致 TCP 打洞
// 在 25s 保活检查时必然失败。把这类服务器从列表里去掉，换成长连接稳定的。
function StunServersModal({ initial, onClose, onSaved }: { initial: string[]; onClose: () => void; onSaved: (best: string) => void }) {
  const [text, setText] = useState(initial.join('\n'))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const save = async () => {
    setSaving(true)
    setError('')
    try {
      const servers = text
        .split('\n')
        .map((s) => s.trim())
        .filter(Boolean)
      const result = await api.updateStunServers(servers)
      onSaved(result.data?.best ?? '')
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  // 恢复内置列表：把后端返回的默认列表填回输入框，不直接保存。
  // 让用户先看一眼再决定 —— 一键覆盖掉他刚敲的内容会很烦人。
  const resetDefault = async () => {
    setSaving(true)
    setError('')
    try {
      const result = await api.resetStunServers()
      setText((result.data?.servers ?? []).join('\n'))
    } catch (e) {
      setError(e instanceof Error ? e.message : '恢复失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="flex max-h-[88vh] w-full max-w-2xl flex-col rounded-lg bg-white shadow-2xl ring-1 ring-slate-200">
        <div className="flex items-center justify-between gap-4 border-b border-slate-100 px-5 py-4">
          <div className="flex min-w-0 items-center gap-3">
            <span className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-blue-50 text-blue-600 ring-1 ring-blue-100">
              <Server className="h-4.5 w-4.5" />
            </span>
            <div className="min-w-0">
              <div className="text-base font-bold text-slate-800">STUN 服务器</div>
              <div className="mt-0.5 text-xs text-slate-400">每行一个，host 或 host:port（默认 3478）</div>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            title="关闭"
            aria-label="关闭"
            className="grid h-8 w-8 place-items-center rounded-md text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="overflow-y-auto p-5">
          <textarea
            value={text}
            onChange={(e) => setText(e.target.value)}
            spellCheck={false}
            className="h-72 w-full resize-y rounded-md border border-slate-200 bg-slate-50 p-3 font-mono text-xs leading-5 text-slate-700 outline-none transition focus:border-blue-300 focus:bg-white focus:ring-2 focus:ring-blue-100"
            placeholder={'stun.moonlight-stream.org:3478\nstun.threema.ch:3478\n...'}
          />
          <div className="mt-2 space-y-1 text-xs leading-5 text-slate-400">
            <p>保存后会立即重测一遍延迟并选出「最优」。正在跑的打洞服务不受影响，下一次打洞或保活重连起用新列表。</p>
            <p>提示：部分服务商（如 sonetel / nextcloud）会在几秒内掐掉 TCP 连接，TCP 打洞会反复重启，建议从列表里去掉它们。</p>
          </div>
          {error && (
            <div className="mt-3 flex items-start gap-2 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-xs leading-5 text-rose-700">
              <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
              <span className="break-words">{error}</span>
            </div>
          )}
        </div>
        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-slate-100 px-5 py-3">
          <span className="text-xs text-slate-400">保存前会逐行校验并去重</span>
          <div className="flex items-center gap-2">
            {/* 恢复内置列表放在「取消」左边：它是个不常用但关键的回头路，
                排在次要位置，避免和「保存」抢注意力 */}
            <button
              type="button"
              onClick={resetDefault}
              disabled={saving}
              title="丢弃当前编辑，恢复成程序内置的默认列表并立即重测"
              className="rounded-md border border-slate-200 px-3 py-2 text-xs font-semibold text-slate-500 transition hover:bg-slate-50 disabled:opacity-50"
            >
              恢复内置列表
            </button>
            <button
              type="button"
              onClick={onClose}
              className="rounded-md border border-slate-200 px-3 py-2 text-xs font-semibold text-slate-600 transition hover:bg-slate-50"
            >
              取消
            </button>
            <button
              type="button"
              onClick={save}
              disabled={saving}
              className="flex min-w-[88px] items-center justify-center gap-1.5 rounded-md bg-blue-600 px-3 py-2 text-xs font-semibold text-white shadow-sm transition hover:bg-blue-700 disabled:cursor-wait disabled:opacity-60"
            >
              <RotateCw className={`h-3.5 w-3.5 ${saving ? 'animate-spin' : ''}`} />
              {saving ? '重测中...' : '保存并重测'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

export function Stun() {
  const [config, setConfig] = useState<StunConfig | null>(null)
  const [loadErr, setLoadErr] = useState('')
  const [statusMap, setStatusMap] = useState<Record<string, StunStatusEvent>>({})
  const { toasts, show: toast } = useToast()

  const [deviceModal, setDeviceModal] = useState<{
    open: boolean
    initial?: StunDevice
    prefill?: DeviceFormState
  }>({ open: false })
  const [scanModal, setScanModal] = useState<{ open: boolean; subnets: LanSubnet[] }>({
    open: false,
    subnets: [],
  })
  // 正在切启停的服务，键是「设备ID-服务ID」
  const [toggling, setToggling] = useState<Record<string, boolean>>({})
  const [serviceModal, setServiceModal] = useState<{
    open: boolean
    deviceId: number
    initial?: StunService
    prefill?: Partial<ServiceFormState>
  }>({ open: false, deviceId: 0 })
  const [logKey, setLogKey] = useState<string | null>(null)
  // 访问日志弹窗（从审计取，和反代同一个组件）：ref 形如 "s<设备ID>-<服务ID>"
  const [accessLog, setAccessLog] = useState<{ ref: string; name: string } | null>(null)
  const [natTypeOpen, setNatTypeOpen] = useState(false)
  const [stunServersOpen, setStunServersOpen] = useState(false)

  const refresh = useCallback(async () => {
    try {
      const cfg = await api.getStunConfig()
      setConfig(cfg)
    } catch (e) {
      setLoadErr(e instanceof Error ? e.message : String(e))
    }
  }, [])

  useEffect(() => {
    refresh()
    const t = window.setInterval(refresh, 30000)
    return () => window.clearInterval(t)
  }, [refresh])

  useEffect(() => {
    const es = api.subscribeStunStatus((data) => {
      try {
        const evt = JSON.parse(data) as StunStatusEvent
        setStatusMap((p) => ({ ...p, [evt.key]: evt }))
      } catch {
        // ignore
      }
    })
    return () => es.close()
  }, [])


  const submitDevice = async (form: DeviceFormState) => {
    if (deviceModal.initial) {
      await api.updateStunDevice({
        deviceId: getDeviceId(deviceModal.initial),
        name: form.name.trim(),
        ip: form.ip.trim(),
      })
      toast('设备保存成功')
    } else {
      await api.addStunDevice({ name: form.name.trim(), ip: form.ip.trim() })
      toast('设备添加成功')
    }
    setDeviceModal({ open: false })
    await refresh()
  }

  // 网段列表先取好再开弹窗——这一步只读本机网卡，不发包，几毫秒的事，
  // 放进弹窗里反而要先闪一个空下拉框


  // 不关扫描弹窗：一次扫描常常要加好几台，关掉就得再等一轮。
  // 添加窗盖在上面，存完退回来，刚加的那台会变成「已加过」
  // 这个 IP 的这个端口上已经建过服务了。
  // 不标的话同一个端口很容易建出两份，两份各自去打一个洞，
  // 对外两个地址指向同一个服务。
  const scanTakenPorts = useMemo(() => {
    const set = new Set<string>()
    for (const d of config?.devices ?? []) {
      for (const svc of d.services ?? []) set.add(`${d.ip}:${svc.internalPort}`)
    }
    return set
  }, [config])

  // 已经在设备列表里的机器：标出来，免得用户照着扫描结果又加一遍
  // （会出现两个同 IP 的设备，而这个错要等服务加错地方才发现）
  const scanAddedHosts = useMemo(() => {
    const m = new Map<string, string>()
    for (const d of config?.devices ?? []) {
      if (d.ip) m.set(d.ip, d.name || d.ip)
    }
    return m
  }, [config])

  const pickScanned = (h: LanHost) => {
    setDeviceModal({ open: true, prefill: { name: h.name, ip: h.ip } })
  }

  // 扫描结果里点一个端口：名字和内网端口都填好，剩下的按默认来，直接能存
  /**
   * 点了扫描结果里的端口。
   *
   * 共用组件给的是 LanHost（它不认识"设备"这个概念），这里按 IP 反查设备
   * 拿 ID —— 服务必须挂在某台设备底下。查不到说明那台还没加成设备，
   * 提示一下而不是静默失败。
   */
  const pickScannedPort = (host: LanHost, p: LanPort) => {
    const dev = (config?.devices ?? []).find((d) => d.ip === host.ip)
    if (!dev) {
      toast(`请先把 ${host.ip} 添加为设备，服务要挂在这台设备下面`)
      return
    }
    setServiceModal({
      open: true,
      deviceId: getDeviceId(dev),
      prefill: { name: p.name || `端口 ${p.port}`, internalPort: String(p.port) },
    })
  }

  const submitService = async (form: ServiceFormState) => {
    // 成功才走「关闭弹窗 + refresh」；失败要重抛给 ServiceModal ，
    // 让它内部的 busy 闸门重置（不抛就卡死了）。
    const payload: api.StunServicePayload = {
      deviceId: serviceModal.deviceId,
      name: form.name.trim(),
      internalPort: Number(form.internalPort),
      protocol: form.protocol,
      useUpnp: form.useUpnp,
      access: {
        mode: form.accessMode,
        rules: form.accessRules.split('\n').map((x) => x.trim()).filter(Boolean),
      },
      punchLocalPort: Number(form.punchLocalPort) || 0,
      domain: form.domain.trim().toLowerCase(),
      ...formToPayloadTLS(form),
      enabled: form.enabled,
      description: form.description.trim(),
      // channelIds 是服务自己的字段（渠道 ID 列表），不属于 webhookconfig
      channelIds: form.channelIds,
      // 自定义通知正文。这两个字段以前没进报文：界面上写好了、点保存，
      // 落盘里根本没有，于是"保存没生效"。改保存接口时务必同步这里。
      notifyTemplate: form.notifyTemplate,
      webhookconfig: {
        ...form.webhookconfig,
        url: form.webhookconfig.url.trim(),
        proxy: form.webhookconfig.proxy.trim(),
      },
      redirect: {
        ...form.redirect,
        entryHost: form.redirect.entryHost.trim().toLowerCase(),
        zoneDomain: form.redirect.zoneDomain.trim().toLowerCase(),
      },
      mcEntry: {
        ...form.mcEntry,
        host: form.mcEntry.host.trim().toLowerCase(),
        zoneDomain: form.mcEntry.zoneDomain.trim().toLowerCase(),
        target: form.mcEntry.target.trim().toLowerCase(),
      },
    }
    try {
      if (serviceModal.initial) {
        await api.updateStunService({ ...payload, serviceId: serviceModal.initial.id })
        toast('服务保存成功')
      } else {
        await api.addStunService(payload)
        toast('服务添加成功')
      }
    } catch (e) {
      toast('保存失败: ' + (e instanceof Error ? e.message : ''))
      throw e  // 抛给 ServiceModal 内部，触发 busy=false 和显示错误
    }
    setServiceModal({ open: false, deviceId: 0 })
    await refresh()
  }

  /**
   * 照着现有服务再开一个，只改要改的那几项。
   *
   * 有四样不能照抄：
   * 入口域名两个服务共用一个，后同步的那个会把先前那条 Cloudflare 规则顶掉，
   * 而且两边都不报错；webhook 的 URL 通常指着一条具体的记录或规则（CF 那份模板
   * 的 URL 里就写死了 rule ID），抄过来就是两个服务轮流往同一条上写，
   * 谁后跑谁说了算，两边还都显示「发送成功」——配置留着，开关默认关掉，
   * 等用户把 URL 改成新的那条再自己打开；UPnP 固定端口抄过来就是两个服务抢
   * 同一个外部端口；名字一样的话列表上根本分不出谁是谁。
   */

  /**
   * 列表上直接启停，不用为了勾一个框把整个服务表单打开一遍。
   *
   * 后端要停掉再重开打洞，这一下要好几秒。这几秒里按钮上什么都不变的话，
   * 用户以为没点着，再点一下就又切回去了——所以切换中先把按钮锁上。
   */
  const toggleServiceEnabled = async (deviceId: number, svc: StunService) => {
    const next = svc.enabled === false
    setToggling((m) => ({ ...m, [`${deviceId}-${svc.id}`]: true }))
    try {
      await api.updateStunService({
        ...serviceToPayload(deviceId, svc),
        enabled: next,
        serviceId: svc.id,
      })
      toast(next ? `已启用「${svc.name}」` : `已停用「${svc.name}」`)
      await refresh()
    } catch (e) {
      toast('切换失败: ' + (e instanceof Error ? e.message : ''))
    } finally {
      setToggling((m) => {
        const n = { ...m }
        delete n[`${deviceId}-${svc.id}`]
        return n
      })
    }
  }

  const removeService = async (deviceId: number, svc: StunService) => {
    if (!window.confirm(`确认删除服务「${svc.name}」？`)) return
    try {
      await api.deleteStunService(deviceId, svc.id)
      toast('服务删除成功')
      await refresh()
    } catch (e) {
      toast('删除失败: ' + (e instanceof Error ? e.message : ''))
    }
  }

  const copy = (text: string) => {
    navigator.clipboard
      .writeText(text)
      .then(() => toast(`已复制: ${text}`))
      .catch(() => toast('复制失败'))
  }

  const openAddress = (svc: StunService, addr: string) => {
    if (!addr) return
    if ((svc.protocol || '').toLowerCase() === 'ssh') {
      const [host, port] = addr.split(':')
      const cmd = `ssh root@${host} -p ${port || '22'}`
      copy(cmd)
      window.setTimeout(() => alert(`SSH 连接命令已复制：\n\n${cmd}`), 100)
      return
    }
    // 和后端 PublicScheme 保持一致：洞口终结 TLS 时外部就是 https；
    // 不终结却勾了「转发给内网时也用 HTTPS」时洞口在替内网加密，外面那一段是明文，
    // 写成 https:// 会直接给出一个 ERR_SSL_PROTOCOL_ERROR 的链接
    const scheme =
      svc.tlsTerminate || (!svc.backendHttps && svc.https) ? 'https://' : 'http://'
    window.open(`${scheme}${addr}`, '_blank', 'noopener,noreferrer')
  }

  const logStatus = useMemo(() => (logKey ? statusMap[logKey] ?? null : null), [logKey, statusMap])

  if (loadErr && !config) {
    return (
      <Card>
        <div className="py-12 text-center text-rose-500">
          <AlertCircle className="mx-auto h-8 w-8" />
          <div className="mt-2 text-sm font-semibold">数据加载失败</div>
          <div className="mt-1 text-xs text-slate-500">{loadErr}</div>
          <button
            onClick={refresh}
            className="mt-4 rounded-xl bg-blue-500 px-4 py-2 text-xs font-semibold text-white"
          >
            重试
          </button>
        </div>
      </Card>
    )
  }

  // 拖动排序：服务之间换顺序
  const [dragSvc, setDragSvc] = useState('')
  const [overSvc, setOverSvc] = useState('')

  if (!config) {
    return <Card><div className="py-12 text-center text-sm text-slate-400">正在加载...</div></Card>
  }


  /** 同一台设备下的服务换顺序（保留设备维度，因为不同设备的 SortOrder 是独立的） */
  const commitServiceOrder = async (deviceId: number, from: number, to: number) => {
    // 数据源用**顶层 services**（和列表渲染一致），按 deviceId 过滤出这一组 ——
    // 不再读设备树的 services：那样 deviceId=0（"未归到设备"那组）的服务根本不在树里，
    // 拖它时会静默失效。
    const group = (config?.services ?? []).filter((s2) => (s2.deviceId ?? 0) === deviceId)
    const ids = group.map((s2) => s2.id)
    const i = ids.indexOf(from)
    const j = ids.indexOf(to)
    if (i < 0 || j < 0 || i === j) return
    const [moved] = ids.splice(i, 1)
    ids.splice(j, 0, moved)
    try {
      await api.reorderStunServices(deviceId, ids)
      await refresh()
    } catch (e) {
      toast('排序保存失败: ' + (e instanceof Error ? e.message : ''))
    }
  }


  /**
   * 一张穿透服务卡片。
   *
   * 抽成函数是因为设备改成分组排版后，每个设备下都要渲染自己的服务 ——
   * 和反代页的 SiteCard 是同一个套路：卡片逻辑不变，外层换容器。
   */
  const renderServiceCard = (dev: StunDevice, svc: StunService) => {
    // **和反代/转发共用同一张卡**（components/ServiceCard）—— 骨架、拖动、状态点
    // 都是同一套；穿透特有的两样通过 props 传进去：图标行里多一个"日志"，
    // 第三行带 HTTPS / UPnP / Webhook 小标签。
    const key = `${getDeviceId(dev)}-${svc.id}`
    const status = statusMap[key]
    const phase = status?.phaseStr || 'STOPPED'
    const running = phase === 'RUNNING'
    const externalPort = status?.externalPort ?? 0
    // 配了对外域名就用域名——终结 TLS 时证书是按域名签的，用 IP 访问必然报证书错
    const externalHost = svc.domain || config.publicIP
    const fullAddr = externalPort > 0 && externalHost ? `${externalHost}:${externalPort}` : ''
    const lastError = status?.lastError || ''
    const restartCount = status?.restartCount ?? 0
    const off = svc.enabled === false
    const busy = !!toggling[`${getDeviceId(dev)}-${svc.id}`]
    const webhookEnabled = !!svc.webhookconfig?.enabled
    const webhookStatus = webhookEnabled ? getWebhookStatus(status) : null

    const state: ServiceState = off ? 'off' : running ? 'running' : 'failed'
    const hint = off
      ? '已停用'
      : running
        ? '穿透成功'
        : lastError
          ? `${lastError}${restartCount > 0 ? `（重启 ${restartCount} 次）` : ''}`
          : phaseLabel[phase] || phase

    return (
      <ServiceCard
        // "通知已开启"小徽标：用户从服务列表上一眼能看出哪些配了通知、哪些没
        notifyActive={(svc.channelIds ?? []).length > 0}
        notifyCount={(svc.channelIds ?? []).length}
        name={svc.name}
        state={state}
        stateHint={hint}
        dragging={dragSvc === String(svc.id)}
        dropTarget={overSvc === String(svc.id) && dragSvc !== String(svc.id)}
        onDragStart={() => setDragSvc(String(svc.id))}
        onDragEnd={() => {
          setDragSvc('')
          setOverSvc('')
        }}
        onDragOver={(e) => {
          e.preventDefault()
          const k = String(svc.id)
          if (k !== overSvc) setOverSvc(k)
        }}
        onDrop={(e) => {
          e.preventDefault()
          if (dragSvc) void commitServiceOrder(svc.deviceId, Number(dragSvc), svc.id)
        }}
        icons={
          <>
            <button
              onClick={() => toggleServiceEnabled(getDeviceId(dev), svc)}
              disabled={busy}
              title={busy ? '正在切换，打洞要几秒' : off ? '点一下启用' : '点一下停用'}
              className={`${iconBtn} disabled:cursor-wait ${
                off ? 'bg-slate-200 text-slate-500 hover:bg-slate-300' : 'bg-emerald-50 text-emerald-600 hover:bg-emerald-100'
              }`}
            >
              {busy ? <LoaderCircle className="h-3 w-3 animate-spin" /> : <Power className="h-3 w-3" />}
            </button>
            <button
              onClick={() => setServiceModal({ open: true, deviceId: getDeviceId(dev), initial: svc })}
              className={`${iconBtn} bg-slate-100 text-slate-600 hover:bg-slate-200`}
              title="编辑"
            >
              <Pencil className="h-3 w-3" />
            </button>
            {/* 访问日志：和反代一样从审计取（按 ref = s<设备>-<服务>）。
                以前穿透外面谁来过完全没记录，现在洞口 Accept 时会记一笔。 */}
            <button
              onClick={() =>
                setAccessLog({
                  // ref 必须带 s 前缀：后端记的是 "s<设备ID>-<服务ID>"
                  //（见 modules/stun/tunnel_access.go）。少了这个 s 就一条都查不到。
                  ref: 's' + key,
                  name: svc.name || '未命名服务',
                })
              }
              className={`${iconBtn} bg-slate-100 text-slate-600 hover:bg-slate-200`}
              title="访问日志（谁访问过它）"
            >
              <FileText className="h-3 w-3" />
            </button>
            <button
              onClick={() => removeService(getDeviceId(dev), svc)}
              className={`${iconBtn} bg-rose-50 text-rose-500 hover:bg-rose-100`}
              title="删除"
            >
              <Trash2 className="h-3 w-3" />
            </button>
          </>
        }
        subtitle={
          off ? (
            <span className="text-slate-400">已停用，外部地址不通</span>
          ) : fullAddr ? (
            <div className="flex min-w-0 items-center gap-1">
              <button
                onClick={() => copy(fullAddr)}
                title={`点击复制：${fullAddr}`}
                className="flex min-w-0 flex-1 items-center gap-1 text-left"
              >
                <ClipboardCopy className="h-3 w-3 shrink-0 text-slate-400" />
                <span className="truncate font-mono text-xs font-semibold text-blue-600">{fullAddr}</span>
              </button>
              <button
                onClick={() => openAddress(svc, fullAddr)}
                title="一键直达（新标签打开）"
                className="shrink-0 rounded p-0.5 text-blue-500 transition hover:bg-blue-50"
              >
                <ExternalLink className="h-3.5 w-3.5" />
              </button>
            </div>
          ) : (
            <span className="text-slate-400">{phaseLabel[phase] || '等待外部地址…'}</span>
          )
        }
        meta={
          <>
            <span className="font-mono text-[11px] text-slate-400">: {svc.internalPort}</span>
            <CardBadge tone="slate" title="协议">
              {(svc.protocol || 'TCP').toUpperCase()}
            </CardBadge>
            {svc.tlsTerminate && (
              <CardBadge tone="violet" title="Catflap 用证书给外面加 HTTPS">
                HTTPS
              </CardBadge>
            )}
            {(svc.backendHttps || svc.https) && !svc.tlsTerminate && (
              <CardBadge tone="teal" title="内网服务本身就是 HTTPS">
                内网 HTTPS
              </CardBadge>
            )}
            {svc.useUpnp && (
              <CardBadge tone="amber" title="UPnP 自动端口映射">
                UPnP
              </CardBadge>
            )}
            {webhookEnabled && webhookStatus && (
              <CardBadge
                tone={
                  webhookStatus.state === 'success'
                    ? 'emerald'
                    : webhookStatus.state === 'failed'
                      ? 'rose'
                      : 'amber'
                }
                title={`Webhook：${webhookStatus.text}${webhookStatus.at ? `（${new Date(webhookStatus.at).toLocaleString()}）` : ''}`}
              >
                Webhook
              </CardBadge>
            )}
          </>
        }
      />
    )
  }

  return (
    <div className="space-y-4">
      {/* 网络拓扑 */}
      <Card>
        <CardHeader
          title="基础网络与路由拓扑"
          className="flex-wrap gap-2"
          action={
            <div className="flex max-w-full flex-wrap items-center justify-end gap-2 text-xs">
              <span className="text-slate-400">最优 STUN:</span>
              <span className="max-w-[220px] break-all text-right font-mono font-semibold text-blue-500 sm:max-w-none">
                {config.bestStun || '--'}
              </span>
              <button
                type="button"
                onClick={() => setStunServersOpen(true)}
                title="编辑 STUN 服务器列表"
                className="flex items-center gap-1 rounded-md border border-slate-200 px-2 py-1 text-slate-600 transition hover:border-blue-200 hover:bg-blue-50 hover:text-blue-600"
              >
                <Server className="h-3.5 w-3.5" /> 服务器
              </button>
              <button
                type="button"
                onClick={() => setNatTypeOpen(true)}
                title="查看 NAT 类型详情"
                className="flex items-center gap-1 rounded-md border border-slate-200 px-2 py-1 text-slate-600 transition hover:border-blue-200 hover:bg-blue-50 hover:text-blue-600"
              >
                <Info className="h-3.5 w-3.5" /> NAT 类型
              </button>
              <button
                onClick={refresh}
                className="ml-2 flex items-center gap-1 rounded-md text-slate-500 transition hover:text-blue-500"
              >
                <RotateCw className="h-3.5 w-3.5" /> 刷新
              </button>
            </div>
          }
        />
        <div className="flex flex-wrap items-center gap-2 rounded-xl bg-slate-50 p-4">
          <div className="flex min-w-[140px] flex-col items-center gap-1 rounded-xl bg-sky-50 px-4 py-3 ring-1 ring-sky-200">
            <div className="flex items-center gap-1.5">
              <Computer className="h-4 w-4 text-sky-500" />
              <span className="text-xs font-semibold text-sky-600">本机 IP</span>
            </div>
            <span className="font-mono text-xs text-slate-600">{config.localIP || '--'}</span>
          </div>
          {[...(config.natRouterList || [])]
            .sort((a, b) => a.natLevel - b.natLevel)
            .map((r) => (
              <div key={r.natLevel} className="flex items-center gap-2">
                <ChevronRight className="h-4 w-4 text-slate-300" />
                <div
                  className={`flex min-w-[140px] flex-col items-center gap-1 rounded-xl px-4 py-3 ring-1 ${
                    r.ipType === 'cgn' ? 'bg-violet-50 ring-violet-200' : 'bg-amber-50 ring-amber-200'
                  }`}
                >
                  <div className="flex items-center gap-1.5">
                    <Wifi className={`h-4 w-4 ${r.ipType === 'cgn' ? 'text-violet-500' : 'text-amber-500'}`} />
                    <span
                      className={`text-xs font-semibold ${
                        r.ipType === 'cgn' ? 'text-violet-600' : 'text-amber-600'
                      }`}
                    >
                      {r.ipType === 'cgn' ? 'CGN 网关' : `路由 NAT ${r.natLevel}`}
                    </span>
                  </div>
                  <span className="font-mono text-xs text-slate-600">{r.lanIP}</span>
                </div>
              </div>
            ))}
          <ChevronRight className="h-4 w-4 text-slate-300" />
          <div className="flex min-w-[140px] flex-col items-center gap-1 rounded-xl bg-emerald-50 px-4 py-3 ring-1 ring-emerald-200">
            <div className="flex items-center gap-1.5">
              <Globe className="h-4 w-4 text-emerald-500" />
              <span className="text-xs font-semibold text-emerald-600">公网 IP</span>
            </div>
            <span className="font-mono text-xs text-slate-600">{config.publicIP || '--'}</span>
          </div>
        </div>
      </Card>

      {/*
        设备 + 服务：**和反代页同一套排版** —— 设备卡片下面挂它的服务卡片。
        以前是左边一列设备、右边显示选中设备的服务，两个页面长得不一样。
        统一管理成设备分组后，穿透和反代的操作方式是一致的。
      */}
      {/* 统一的「添加服务」入口：弹出表单里选设备 —— 和反代同一套做法，
          不是先加设备再加服务。设备列表是共用的，选一台就行。 */}
      <div className="flex items-center justify-between">
        <div className="text-[11px] leading-relaxed text-slate-400">
          穿透服务按设备分组。加服务时选好设备，它会归到那台设备下面。
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          {/* 扫描不在这里：集中在「设备」页，穿透页只管服务。
              设备的增删是设备页的事，两个入口容易让人分不清该去哪加。 */}
          <button
            onClick={() => setServiceModal({ open: true, deviceId: 0 })}
            className="flex items-center gap-1 rounded-md bg-blue-500 px-2.5 py-1 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 hover:bg-blue-600"
          >
            <Plus className="h-3 w-3" /> 添加服务
          </button>
        </div>
      </div>

      <div className="space-y-3">
        {(config?.services ?? []).length === 0 ? (
          // 真正的「服务列表」空态：没建过任何穿透服务。
          // 设备的增删属于「设备」页，穿透页不再铺开所有设备。
          <Card className="!p-0">
            <div className="px-5 py-12 text-center">
              <Server className="mx-auto h-8 w-8 text-slate-300" />
              <p className="mt-2 text-sm text-slate-500">还没有内网穿透服务</p>
              <p className="mt-1 text-[11px] text-slate-400">
                点右上角「添加服务」，选设备、填端口就行
              </p>
            </div>
          </Card>
        ) : (
          // **和反代页同一套设计：以设备为主项，服务挂在设备下。**
          // 数据源是顶层 services（不是设备树）—— 按 svc.deviceId 归到设备，
          // 改设备 IP 时 deviceId 不变，归属不会掉；设备只是展示分组。
          (() => {
            const byDev = new Map<number, StunService[]>()
            const rest: StunService[] = []
            for (const svc of config?.services ?? []) {
              if (svc.deviceId) {
                const arr = byDev.get(svc.deviceId) ?? []
                arr.push(svc)
                byDev.set(svc.deviceId, arr)
              } else {
                rest.push(svc)
              }
            }
            const devList = config?.devices ?? []
            const out: { dev?: (typeof devList)[number]; svcs: StunService[] }[] = []
            for (const [id, svcs] of byDev) {
              out.push({ dev: devList.find((d) => getDeviceId(d) === id), svcs })
            }
            if (rest.length > 0) out.push({ svcs: rest })

            return out.map(({ dev, svcs }, gi) => (
              <Card key={dev ? getDeviceId(dev) : `rest-${gi}`} className="!p-0">
                {/* 设备头：和反代页同一套样式 */}
                <div className="flex items-center gap-2 border-b border-slate-100 px-3 py-2">
                  <span className="grid h-7 w-7 shrink-0 place-items-center rounded-lg bg-emerald-50 text-emerald-600">
                    <Server className="h-3.5 w-3.5" />
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-[13px] font-bold text-slate-800">
                      {dev ? dev.name : '未归到设备'}
                    </p>
                    {dev && <p className="font-mono text-[10px] text-slate-400">{dev.ip}</p>}
                  </div>
                  <span className="shrink-0 text-[11px] text-slate-400">{svcs.length} 个服务</span>
                  {dev && (
                    <button
                      onClick={() => setServiceModal({ open: true, deviceId: getDeviceId(dev) })}
                      className="flex shrink-0 items-center gap-1 rounded-md px-2 py-1 text-[11px] font-semibold text-blue-600 ring-1 ring-blue-200 transition hover:bg-blue-50"
                      title="给这台设备添加服务"
                    >
                      <Plus className="h-3 w-3" /> 加服务
                    </button>
                  )}
                </div>

                <div className="grid grid-cols-[repeat(auto-fill,minmax(min(16rem,100%),1fr))] gap-2.5 p-2.5">
                  {svcs.map((svc) => (
                    <div
                      key={svc.id}
                      draggable
                      onDragStart={() => setDragSvc(String(svc.id))}
                      onDragEnd={() => {
                        setDragSvc('')
                        setOverSvc('')
                      }}
                      onDragOver={(e) => {
                        e.preventDefault()
                        const k = String(svc.id)
                        if (k !== overSvc) setOverSvc(k)
                      }}
                      onDrop={(e) => {
                        e.preventDefault()
                        if (dragSvc) void commitServiceOrder(svc.deviceId, Number(dragSvc), svc.id)
                      }}
                      className={`cursor-grab transition ${
                        overSvc === String(svc.id) && dragSvc !== String(svc.id)
                          ? 'ring-2 ring-blue-300'
                          : ''
                      } ${dragSvc === String(svc.id) ? 'opacity-40' : ''}`}
                    >
                      {renderServiceCard(dev as StunDevice, svc)}
                    </div>
                  ))}
                </div>
              </Card>
            ))
          })()
        )}
      </div>


      {/* 弹窗 */}
      {/* 扫描窗排在前面，添加窗才能盖在它上面 */}
      {scanModal.open && (
        <LanScanModal
          subnets={scanModal.subnets}
          // 这两组数据是 stun 的业务知识（什么叫"已建过服务"、什么叫"已添加"），
          // 由这里算好传给共用组件 —— 组件那边不认识 devices 是什么
          takenPorts={scanTakenPorts}
          addedHosts={scanAddedHosts}
          onCancel={() => setScanModal({ open: false, subnets: [] })}
          onPick={pickScanned}
          onPickPort={pickScannedPort}
        />
      )}
      {deviceModal.open && (
        <DeviceModal
          initial={deviceModal.initial}
          prefill={deviceModal.prefill}
          onCancel={() => setDeviceModal({ open: false })}
          onSubmit={submitDevice}
        />
      )}
      {serviceModal.open && (
        <ServiceModal
          deviceId={serviceModal.deviceId}
          onDeviceChange={(id) =>
            setServiceModal((p) => ({ ...p, deviceId: id }))
          }
          initial={serviceModal.initial}
          prefill={serviceModal.prefill}
          status={
            serviceModal.initial
              ? statusMap[`${serviceModal.deviceId}-${serviceModal.initial.id}`]
              : undefined
          }
          onCancel={() => setServiceModal({ open: false, deviceId: 0 })}
          onSubmit={submitService}
          onToast={toast}
        />
      )}
      {logKey && <LogModal status={logStatus} onClose={() => setLogKey(null)} />}
      {accessLog && (
        <AccessLogModal
          title={accessLog.name}
          ref={accessLog.ref}
          onClose={() => setAccessLog(null)}
        />
      )}
      {natTypeOpen && <NatTypeModal onClose={() => setNatTypeOpen(false)} />}
      {stunServersOpen && (
        <StunServersModal
          initial={config.stunServerList ?? []}
          onClose={() => setStunServersOpen(false)}
          onSaved={(best) => {
            setConfig((prev) => (prev ? { ...prev, bestStun: best } : prev))
            toast('STUN 服务器已保存并重测')
            refresh()
          }}
        />
      )}

      {/* Toasts */}
      <div className="pointer-events-none fixed bottom-6 left-1/2 z-[60] flex -translate-x-1/2 flex-col items-center gap-2">
        {toasts.map((t) => (
          <div
            key={t.id}
            // 限宽换行：长的那几句不限宽会拉成一条横穿屏幕的线，反而看不下去
            className="max-w-[min(90vw,34rem)] break-words rounded-lg bg-slate-900/85 px-4 py-2 text-sm leading-relaxed text-white shadow-lg"
          >
            {t.text}
          </div>
        ))}
      </div>
    </div>
  )
}
