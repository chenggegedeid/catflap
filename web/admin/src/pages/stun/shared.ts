/**
 * 穿透页的公共层：表单结构、常量、以及「界面表单 ↔ 后端载荷」的转换。
 *
 * 从 Stun.tsx 抽出来的原因：那一个文件 3010 行，其中这段是**所有弹窗都要用**
 * 的东西（服务弹窗、设备弹窗、日志弹窗都依赖它）。放在一处，改字段时不用在
 * 一千多行里翻，也不会因为各自复制一份而慢慢不一致。
 *
 * 这里只放**纯数据/纯函数**，不放 JSX —— 组件各自在自己的文件里。
 */
import * as api from '../../lib/api'
import type {
  Certificate,
  MCEntryConfig,
  RedirectConfig,
  StunService,
  StunStatusEvent,
  WebhookConfig,
} from '../../types'

export interface ServiceFormState {
  name: string
  internalPort: string
  protocol: 'TCP' | 'UDP'
  useUpnp: boolean
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
export function formToPayloadTLS(form: ServiceFormState) {
  const tls = form.tlsTerminate && form.protocol === 'TCP'
  return {
    tlsTerminate: tls,
    certId: tls ? Number(form.certId) || 0 : 0,
    backendHttps: tls && form.internalHttps,
    https: !tls && form.protocol === 'TCP' && form.internalHttps,
  }
}

export type ServiceModalTab = 'basic' | 'webhook' | 'redirect'

export const fieldLabelCls = 'mb-1 text-xs font-semibold text-muted'
export const fieldInputCls =
  'w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400'

export const emptyRedirect: RedirectConfig = {
  enabled: false,
  providerId: 0,
  entryHost: '',
  zoneDomain: '',
}

export const emptyMCEntry: MCEntryConfig = {
  enabled: false,
  providerId: 0,
  host: '',
  zoneDomain: '',
  target: '',
}

export const emptyWebhook: WebhookConfig = {
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
export const NOTIFY_TEMPLATES: { key: string; label: string; hint: string; body: string }[] = [
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
export const NOTIFY_PLACEHOLDERS = [
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
export function defaultChannels(): string[] { return [...defaultChannelIds] }

let defaultChannelIds: string[] = []
export function setDefaultChannelIds(ids: string[]): void { defaultChannelIds = ids }

export const emptyService: ServiceFormState = {
  name: '',
  internalPort: '',
  protocol: 'TCP',
  // UPnP 默认**关**：以前默认开，结果每条服务都去路由器上开映射，
  // 端口开一片自己都不知道。需要的人自己勾（打洞本身够用）。
  useUpnp: false,
  // 访问控制默认**关**：站点级 IP 名单已废弃，过滤统一走全局封禁表 / 白名单。
  // （老数据里可能还存着 deny 规则，读进来会被忽略，见 formToPayload。）
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
export function isWildcardDomain(d: string) {
  return d.startsWith('*.')
}

/** 和后端 utils/domain.Match 同一套语义：通配只覆盖恰好多一层标签的名字 */
export function matchCertDomain(pattern: string, name: string) {
  if (!name) return false
  if (!isWildcardDomain(pattern)) return pattern === name
  const suffix = pattern.slice(1) // ".example.com"
  if (!name.endsWith(suffix)) return false
  const label = name.slice(0, name.length - suffix.length)
  return label !== '' && !label.includes('.')
}

/** 选中的证书覆盖哪些域名。certId 为 0（按 SNI 自动匹配）时是所有启用证书的并集 */
export function certDomainsOf(certs: Certificate[], certId: string): string[] {
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
export function pickCertDomain(certs: Certificate[], certId: string): string {
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

export function normalizeWebhook(cfg?: Partial<WebhookConfig>): WebhookConfig {
  return {
    ...emptyWebhook,
    ...(cfg || {}),
    method: cfg?.method || emptyWebhook.method,
  }
}

/** 已存在的服务 → 表单。编辑和「复制」都从这儿取初值，省得两处各写一遍 */
export function serviceToForm(svc: StunService): ServiceFormState {
  return {
    name: svc.name,
    internalPort: String(svc.internalPort || ''),
    protocol: (svc.protocol as 'TCP' | 'UDP') || 'TCP',
    useUpnp: !!svc.useUpnp,
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
export function serviceToPayload(deviceId: number, svc: StunService): api.StunServicePayload {
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

export function getWebhookStatus(status?: StunStatusEvent) {
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


/** 穿透阶段 → 中文。日志弹窗与主列表都要用，所以放在公共层。 */
export const phaseLabel: Record<string, string> = {
  PROBING: '探测中',
  RUNNING: '穿透成功',
  RESTARTING: '重启中',
  FAILED: '探测失败',
  STOPPED: '已停止',
}

/**
 * 后端给的是 time.Time，没同步过时是零值 —— 别显示成 0001 年。
 * 服务弹窗与日志弹窗都要用，所以放公共层。
 */
export function fmtTime(v?: string) {
  if (!v) return ''
  const d = new Date(v)
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return ''
  return d.toLocaleString()
}
