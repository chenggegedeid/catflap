import type {
  AcmeProvider,
  Certificate,
  AuditConfigView,
  AuditIPStat,
  AuditOverview,
  AuditRecord,
  CertSource,
  ComponentsView,
  DdnsConfig,
  DdnsProvider,
  DdnsProviderTypeSpec,
  DdnsRecord,
  LanHost,
  LanSubnet,
  LogEntry,
  LogSummary,
  LoginEvent,
  LoginSummary,
  NatTypeInfo,
  NetInterface,
  NotifyChannelType,
  NotifyConfigView,
  ProxyConfig,
  ProxyEntry,
  ProxyAccess,
  ProxyProbeResult,
  TunnelConfigView,
  TunnelSavePayload,
  TunnelRuntimeState,
  ProxySite,

  ProxySiteOptions,
  RedirectConfig,
  MCEntryConfig,
  RedirectInspection,
  StunConfig,
  StunNetwork,
  StunNetworkConfig,
  TrafficRealtime,
  TrafficTrend,
  UpdateConfig,
  UpdateStatus,
  WebhookConfig,
  WebhookTemplate,
  Device,
  LanPort,
  ProxyForward,
  NotifyServiceRef,
} from '../types'

interface ApiResponse<T> {
  code: number
  msg?: string
  data?: T
}

const TOKEN_KEY = 'catflap_token'
const USERNAME_KEY = 'catflap_username'
const DESKTOP_SECRET_KEY = 'catflap_desktop_secret'

// captureDesktopSecret 桌面版首屏：从 URL 读 desktop_secret 存入 sessionStorage 并抹掉地址栏
function captureDesktopSecret() {
  const params = new URLSearchParams(window.location.search)
  const secret = params.get('desktop_secret')
  if (secret) {
    sessionStorage.setItem(DESKTOP_SECRET_KEY, secret)
    params.delete('desktop_secret')
    const q = params.toString()
    const url = window.location.pathname + (q ? `?${q}` : '') + window.location.hash
    window.history.replaceState(null, '', url)
  }
}
captureDesktopSecret()

export const getToken = () => localStorage.getItem(TOKEN_KEY)
export const setToken = (t: string) => localStorage.setItem(TOKEN_KEY, t)
export const clearToken = () => localStorage.removeItem(TOKEN_KEY)

/**
 * 当前登录的用户名。
 *
 * 存起来而不是每次问后端：顶栏头像要显示它，而顶栏在每个页面都在，
 * 为一个字母多发一次请求不值得。**登录/初始化/改用户名**这三个会改它的
 * 地方必须跟着 setUsername，否则界面会停在上一个名字上（这就是
 * 「改了用户名、右上角还是 admin」的由来 —— 那边以前直接写死了字面量）。
 */
export const getUsername = () => localStorage.getItem(USERNAME_KEY) || ''
export const setUsername = (u: string) => {
  if (u) localStorage.setItem(USERNAME_KEY, u)
  else localStorage.removeItem(USERNAME_KEY)
  // 顶栏在另一棵子树里，改完得让它知道 —— 否则要等下次整页刷新才看得见
  window.dispatchEvent(new Event(USERNAME_EVENT))
}

/** 用户名变化事件名。顶栏订阅它来重读名字 */
export const USERNAME_EVENT = 'catflap:username'

export const isDesktop = () => !!sessionStorage.getItem(DESKTOP_SECRET_KEY)

// requestWithMsg 连后端那句话一起拿回来。
//
// 大部分接口只要 data，成功与否界面自己有话说；但有些接口做成了一半——
// 规则写进去了、记录没建上——那句「还差什么」只有后端知道，丢掉就等于报了个假的成功。
async function requestWithMsg<T>(
  path: string,
  init?: RequestInit,
): Promise<{ data: T; msg: string }> {
  const headers: Record<string, string> = {
    // FormData 要让浏览器自己带上 multipart 的 boundary，不能写死成 JSON
    ...(init?.body instanceof FormData ? {} : { 'Content-Type': 'application/json' }),
    ...(init?.headers as Record<string, string> | undefined),
  }
  const token = getToken()
  if (token) headers['Authorization'] = `Bearer ${token}`
  const secret = sessionStorage.getItem(DESKTOP_SECRET_KEY)
  if (secret) headers['X-Catflap-Desktop'] = secret

  const resp = await fetch(path, {
    ...init,
    headers,
  })
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
  const json = (await resp.json()) as ApiResponse<T>
  if (json.code === 401) {
    clearToken()
    if (!location.hash.startsWith('#/login')) location.hash = '#/login'
    throw new Error(json.msg || '未登录')
  }
  if (json.code === 428) {
    if (!location.hash.startsWith('#/setup')) location.hash = '#/setup'
    throw new Error(json.msg || '系统尚未初始化')
  }
  if (json.code !== 0) throw new Error(json.msg || '请求失败')
  return { data: json.data as T, msg: json.msg ?? '' }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  return (await requestWithMsg<T>(path, init)).data
}

// ===================== Auth =====================

// 密码长度区间，和后端 auth.MinPasswordLength / MaxPasswordLength 一致。
// 下限 6：加了用户名之后攻击者要同时猜中两个，6 位够用。
// 上限 24：主要是防"把整段文字粘进密码框"，不是安全上限。
export const MIN_PASSWORD_LENGTH = 6
export const MAX_PASSWORD_LENGTH = 24

export const getAuthStatus = () =>
  request<{ initialized: boolean }>('/api/auth/status')

// username 留空后端会回落到默认（admin）—— 老配置没有用户名这个概念
export const setupPassword = (username: string, password: string) =>
  request<{ token: string; username: string }>('/api/auth/setup', {
    method: 'POST',
    body: JSON.stringify({ username, password }),
  })

// login 的 remember=true 会让后端签发 7 天有效的 token；默认只给 24 小时。
// 后端 modules/auth/auth_config.go 里的两个常量是这里注释的依据。
//
// username 留空也能登：升级上来的老配置里不存在用户名，后端会放行。
// 返回的 username 是**后端里实际生效的那个**（留空登录时是 admin），
// 不是输入框里的原文，直接存下来给顶栏用。
export const login = (username: string, password: string, remember = false) =>
  request<{ token: string; username: string }>('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ username, password, remember }),
  })

// 改完密码旧 token 全部作废，后端会回一个新的，记得存下来顶替旧的。
// newUsername 留空表示只改密码、不动用户名。
export const changePassword = (oldPassword: string, newPassword: string, newUsername = '') =>
  request<{ token: string; username: string }>('/api/auth/password', {
    method: 'PUT',
    body: JSON.stringify({ oldPassword, newPassword, newUsername }),
  })

export const getVersion = () => request<{ version: string }>('/api/version')

// ===================== 组件（功能）管理 =====================

/**
 * 列出后端注册的全部组件、它们的开关状态与健康状态。
 *
 * 界面不维护任何「有哪些功能」的清单：新增一个后端组件（只加一个
 * modules/<x>/component.go）就会自动出现在功能管理页里，侧栏也能跟着显示。
 */
export const getComponents = () => request<ComponentsView>('/api/system/components')

/**
 * 开关一个组件。立即生效（路由闸门每次请求都读当前状态），不需要重启。
 * 失败时后端走业务错误码，request 会抛出带 msg 的 Error。
 */
export const setComponentEnabled = (name: string, enabled: boolean) =>
  request<unknown>('/api/system/component/enable', {
    method: 'POST',
    body: JSON.stringify({ name, enabled }),
  })

// ===================== 一键更新 =====================

export const getUpdateStatus = () => request<UpdateStatus>('/api/system/update')

export const checkUpdate = () => request<UpdateStatus>('/api/system/update/check', { method: 'POST' })

// 立刻返回，下载进度和结果轮询 getUpdateStatus 看；下载校验完后端会自己重启
export const installUpdate = () => request<unknown>('/api/system/update/install', { method: 'POST' })

export const setUpdateConfig = (body: UpdateConfig) =>
  request<UpdateStatus>('/api/system/update/config', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

// 手动上传程序文件：后端校验是这台机器能跑的 Catflap 才换上，换完自己重启
export const uploadUpdate = (file: File) => {
  const form = new FormData()
  form.append('file', file)
  return requestWithMsg<unknown>('/api/system/update/upload', {
    method: 'POST',
    body: form,
  })
}

// ===================== 运行日志 =====================

export const getLogDays = () => request<string[]>('/api/system/log/days')

/**
 * 分类摘要：只回各模块的条数与异常数，不读内容。
 * 折叠态用它 —— 为了显示「穿透 1204 条」去下载 2MB 日志是不划算的。
 */
export const getLogSummary = (params: { day?: string; file?: string }) => {
  const q = new URLSearchParams()
  if (params.day) q.set('day', params.day)
  if (params.file) q.set('file', params.file)
  return request<LogSummary>(`/api/system/log/summary?${q.toString()}`)
}

export const getLogs = (params: {
  day: string
  file?: 'info' | 'err'
  level?: string
  keyword?: string
  limit?: number
}) => {
  const q = new URLSearchParams({ day: params.day })
  if (params.file) q.set('file', params.file)
  if (params.level) q.set('level', params.level)
  if (params.keyword) q.set('keyword', params.keyword)
  if (params.limit) q.set('limit', String(params.limit))
  return request<{ list: LogEntry[]; truncated: boolean }>(`/api/system/log?${q}`)
}

export const getStunConfig = () => request<StunConfig>('/api/stun/config')

/** STUN 服务器列表：查看 + 保存（保存后后端立即重测选优） */
export const getStunServers = () =>
  request<{ servers: string[]; best: string }>('/api/stun/servers')

export const updateStunServers = (servers: string[]) =>
  requestWithMsg<{ servers: string[]; best: string }>('/api/stun/servers', {
    method: 'PUT',
    body: JSON.stringify({ servers }),
  })

/**
 * 把服务器列表恢复成内置的那一批（恢复后后端立即重测选优）。
 *
 * 存在的意义是给「把列表改坏了」留一条回头路：列表一旦被删空，打洞彻底起不来，
 * 而手动重打十几行 host:port 没人愿意做。
 */
export const resetStunServers = () =>
  requestWithMsg<{ servers: string[]; best: string }>('/api/stun/servers/reset', {
    method: 'POST',
  })

/** 打洞走哪张网卡、STUN 域名找谁解析 */
export const getStunNetwork = () => request<StunNetwork>('/api/stun/network')

// 保存后立刻生效；指定的网卡没连上时也会保存，msg 里说明现在的状况
export const updateStunNetwork = (body: StunNetworkConfig) =>
  requestWithMsg<unknown>('/api/stun/network', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const getNatType = () => request<NatTypeInfo>('/api/stun/nat-type')

export const detectNatType = () => request<NatTypeInfo>('/api/stun/nat-type/detect', { method: 'POST' })

export interface SSEHandle {
  close: () => void
}

// subscribeStunStatus 用 fetch 流式读取 SSE，替代浏览器原生 EventSource——
// 因为受保护端点需要 Authorization/桌面 secret 头，而 EventSource 无法自定义请求头。
export function subscribeStunStatus(onEvent: (data: string) => void): SSEHandle {
  const controller = new AbortController()
  let closed = false
  let retry = 1000

  const run = async () => {
    while (!closed) {
      try {
        const headers: Record<string, string> = { Accept: 'text/event-stream' }
        const token = getToken()
        if (token) headers['Authorization'] = `Bearer ${token}`
        const secret = sessionStorage.getItem(DESKTOP_SECRET_KEY)
        if (secret) headers['X-Catflap-Desktop'] = secret

        const resp = await fetch('/api/stun/status/events', {
          headers,
          signal: controller.signal,
        })
        if (resp.status === 401) {
          clearToken()
          if (!location.hash.startsWith('#/login')) location.hash = '#/login'
          return
        }
        if (!resp.ok || !resp.body) throw new Error(`HTTP ${resp.status}`)
        retry = 1000

        const reader = resp.body.getReader()
        const decoder = new TextDecoder()
        let buffer = ''
        while (!closed) {
          const { value, done } = await reader.read()
          if (done) break
          buffer += decoder.decode(value, { stream: true })
          let idx: number
          while ((idx = buffer.indexOf('\n\n')) >= 0) {
            const raw = buffer.slice(0, idx)
            buffer = buffer.slice(idx + 2)
            const data = raw
              .split('\n')
              .filter((l) => l.startsWith('data:'))
              .map((l) => l.slice(5).replace(/^ /, ''))
              .join('\n')
            if (data) onEvent(data)
          }
        }
      } catch {
        if (closed) return
      }
      if (closed) return
      await new Promise((r) => setTimeout(r, retry))
      retry = Math.min(retry * 2, 15000)
    }
  }
  run()

  return {
    close: () => {
      closed = true
      controller.abort()
    },
  }
}

/** 本机接在哪几个局域网上。只读网卡，不发包 */
export const listLanSubnets = () => request<LanSubnet[]>('/api/stun/lan/subnets')

/** 扫一段局域网。要往两百多个地址发连接，两三秒才回得来 */
export const scanLan = (cidr: string) =>
  request<LanHost[]>('/api/stun/lan/scan', {
    method: 'POST',
    body: JSON.stringify({ cidr }),
  })

export const addStunDevice = (body: { name: string; ip: string }) =>
  request<{ id: number }>('/api/stun/device/add', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateStunDevice = (body: { deviceId: number; name: string; ip: string }) =>
  request<unknown>('/api/stun/device/update', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const deleteStunDevice = (deviceId: number) =>
  request<unknown>('/api/stun/device/delete', {
    method: 'DELETE',
    body: JSON.stringify({ deviceId }),
  })

export interface StunServicePayload {
  deviceId: number
  name: string
  /**
   * 访问控制（IP 门禁）—— 和反代站点同一套结构。
   * 新建时不传会按默认处理：黑名单模式 + 空规则。
   */
  access?: { mode: string; rules?: string[] }
  internalPort: number
  protocol: string
  useUpnp: boolean
  /** 仅影响链接展示成 http:// 还是 https://，不改变转发行为 */
  https: boolean
  /** 对外域名，留空回落公网 IP */
  domain: string
  /** 由 Catflap 在洞口终结 TLS，仅 TCP 有效 */
  tlsTerminate: boolean
  /** 绑定证书 ID，0 表示按 SNI 自动匹配 */
  certId: number
  /** 内网服务本身就说 TLS（自签也算），转发时用 tls.Dial */
  backendHttps: boolean
  /**
   * 打洞用的本机端口；0 = 每次随机。
   *
   * 外部端口是 NAT 照着本机端口分配的，本机端口随机外部端口就每次都变，
   * 配好的「域名+端口」过一会儿就失效。钉住本机端口外部端口才稳。
   */
  punchLocalPort: number
  enabled: boolean
  description: string
  /** 选用的全局通知渠道 ID */
  channelIds: string[]
  /**
   * 自定义通知正文（#{xxx} 占位符）。
   *
   * 通知方式和 token 在通知页全局配，服务只管内容和模板。
   * 这个字段以前没进报文 —— 界面上写了模板，点保存后落盘里根本没有它。
   */
  notifyTemplate?: string
  /** 旧的内联 webhook，已被通知中心取代（保留做回退，界面不再编辑） */
  webhookconfig?: WebhookConfig
  redirect?: RedirectConfig
  mcEntry?: MCEntryConfig
}

export const addStunService = (body: StunServicePayload) =>
  request<{ id: number }>('/api/stun/service/add', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateStunService = (body: StunServicePayload & { serviceId: number }) =>
  request<unknown>('/api/stun/service/update', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const deleteStunService = (deviceId: number, serviceId: number) =>
  request<unknown>('/api/stun/service/delete', {
    method: 'DELETE',
    body: JSON.stringify({ deviceId, serviceId }),
  })

/**
 * 立即把服务当前的外网地址写到入口域名上；平时端口一变会自动同步。
 *
 * 返回后端那句完整的话：这一步可能只做成一半（规则写进去了，入口域名的解析记录没建上），
 * 界面自己拼不出「还差什么」，拼了也会和后端各说各的。
 */
export const syncStunRedirect = (deviceId: number, serviceId: number) =>
  requestWithMsg<{ target: string; keepPath: boolean }>('/api/stun/redirect/sync', {
    method: 'POST',
    body: JSON.stringify({ deviceId, serviceId }),
  })

/** 入口/落地这两条解析记录现在各是什么样；入口那条要现场去服务商查，慢一点 */
export const inspectStunRedirect = (deviceId: number, serviceId: number) =>
  request<RedirectInspection>('/api/stun/redirect/inspect', {
    method: 'POST',
    body: JSON.stringify({ deviceId, serviceId }),
  })

/** 删掉服务商那边 Catflap 写的那条规则 */
export const removeStunRedirect = (deviceId: number, serviceId: number) =>
  request<unknown>('/api/stun/redirect', {
    method: 'DELETE',
    body: JSON.stringify({ deviceId, serviceId }),
  })

/** 立即把当前外部端口写进 MC 入口那条 SRV；后端那句话里会带上域名 A 记录有没有补上 */
export const syncStunMCEntry = (deviceId: number, serviceId: number) =>
  requestWithMsg<unknown>('/api/stun/mc-entry/sync', {
    method: 'POST',
    body: JSON.stringify({ deviceId, serviceId }),
  })

// ===================== DDNS =====================

export const getDdnsConfig = () => request<DdnsConfig>('/api/ddns/config')

/**
 * DDNS 里配置过的**主域列表**（去重）—— 域名输入控件的主域下拉用。
 * 没配 DDNS 时返回空数组，DomainInput 会退化成普通完整域名输入。
 */
export const getDdnsZones = () =>
  getDdnsConfig().then((c) => {
    const set = new Set<string>()
    for (const r of c.records ?? []) {
      const d = (r.domain || '').trim().toLowerCase()
      if (d) set.add(d)
    }
    return [...set]
  })

/** 本机网卡列表，给「本地网卡」这个 IP 来源当选项 */
export const getDdnsInterfaces = () => request<NetInterface[]>('/api/ddns/interfaces')

/**
 * 系统支持哪些 DNS 服务商、各自要哪些凭据字段。
 *
 * 添加/编辑服务商的表单完全由它渲染 —— 后端加一家服务商（只加一个
 * register_*.go），这里和界面都不用动。
 */
export const getDdnsProviderTypes = () =>
  request<DdnsProviderTypeSpec[]>('/api/ddns/provider/types')

export const updateDdnsSettings = (body: { intervalSec: number }) =>
  request<unknown>('/api/ddns/settings', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const addDdnsProvider = (body: { name: string; type: string; credential: Record<string, string> }) =>
  request<DdnsProvider>('/api/ddns/provider/add', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateDdnsProvider = (body: { id: number; name: string; credential: Record<string, string> }) =>
  request<unknown>('/api/ddns/provider/update', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const deleteDdnsProvider = (id: number) =>
  request<unknown>('/api/ddns/provider/delete', {
    method: 'DELETE',
    body: JSON.stringify({ id }),
  })

export interface DdnsRecordPayload {
  enabled: boolean
  providerId: number
  name: string
  domain: string
  subDomain: string
  recordType: string
  ipSource: string
  ipSourceArg: string
  ttl: number
  proxied: boolean
}

export const addDdnsRecord = (body: DdnsRecordPayload) =>
  request<DdnsRecord>('/api/ddns/record/add', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateDdnsRecord = (body: DdnsRecordPayload & { id: number }) =>
  request<unknown>('/api/ddns/record/update', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const deleteDdnsRecord = (id: number) =>
  request<unknown>('/api/ddns/record/delete', {
    method: 'DELETE',
    body: JSON.stringify({ id }),
  })

export const syncDdnsRecord = (id: number) =>
  request<unknown>('/api/ddns/record/sync', {
    method: 'POST',
    body: JSON.stringify({ id }),
  })

// ===================== Webhook =====================

export const getWebhookTemplates = () => request<WebhookTemplate[]>('/api/webhook/templates')

export const addWebhookTemplate = (body: { name: string; description: string; config: WebhookConfig }) =>
  request<WebhookTemplate>('/api/webhook/template/add', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateWebhookTemplate = (body: { id: string; name: string; description: string; config: WebhookConfig }) =>
  request<WebhookTemplate>('/api/webhook/template/update', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const deleteWebhookTemplate = (id: string) =>
  request<unknown>('/api/webhook/template/delete', {
    method: 'DELETE',
    body: JSON.stringify({ id }),
  })

// ===================== 证书 =====================

export const getCertList = () =>
  request<{ list: Certificate[]; count: number }>('/api/cert/list').then((r) => r.list ?? [])

/** DNS-01 可选服务商，复用 DDNS 里已配好凭证的那些 */
export const getCertProviders = () =>
  request<{ list: AcmeProvider[]; count: number }>('/api/cert/providers').then((r) => r.list ?? [])

export interface CertPayload {
  name: string
  source: CertSource
  domains: string[]
  enabled: boolean
  isDefault: boolean
  certFile: string
  keyFile: string
  acme: {
    directory: string
    email: string
    providerId: number
    httpPort: number
    eabKeyId: string
    /** 留空表示沿用原值 */
    eabHmac: string
    renewDays: number
  }
}

export const addCert = (body: CertPayload) =>
  request<Certificate>('/api/cert/create', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateCert = (body: CertPayload & { id: number }) =>
  request<Certificate>('/api/cert/update', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const deleteCert = (id: number) =>
  request<unknown>('/api/cert/remove', {
    method: 'DELETE',
    body: JSON.stringify({ id }),
  })

/** 粘贴 PEM 文本上传 */
export const uploadCertPEM = (id: number, certPem: string, keyPem: string) =>
  request<Certificate>('/api/cert/upload', {
    method: 'POST',
    body: JSON.stringify({ id, certPem, keyPem }),
  })

/** 立即签发/续期。ACME 全流程要几分钟，接口只负责启动，结果靠刷新列表看 */
export const issueCert = (id: number) =>
  request<unknown>('/api/cert/issue', {
    method: 'POST',
    body: JSON.stringify({ id }),
  })

// ===================== 反向代理 =====================

/** 入口设置 + 监听状态 + 站点表 */
export const getProxyConfig = () => request<ProxyConfig>('/api/proxy/config')

export interface EntryPayload {
  enabled: boolean
  /** HTTP 入口端口，0 = 不开 */
  httpPort: number
  /** HTTPS 入口端口，0 = 不开 */
  httpsPort: number
  /** 0 表示按 SNI 自动匹配 */
  certId: number
  /** 主域名，如 example.com；留空则站点必须写全域名 */
  baseDomain: string
}

export const saveProxyEntry = (body: EntryPayload) =>
  request<ProxyEntry>('/api/proxy/entry', {
    method: 'POST',
    body: JSON.stringify(body),
  })

/** 外网错误页：默认脱敏（不列域名、不说后端地址），也可换成自定义 HTML */
export const saveErrorPage = (body: { mode: string; customHtml: string }) =>
  requestWithMsg<unknown>('/api/proxy/errorpage', {
    method: 'POST',
    body: JSON.stringify(body),
  })

/** 是否允许把管理后台通过反代挂出去（远程管理用，默认关） */
export const saveExposeAdmin = (enabled: boolean) =>
  requestWithMsg<unknown>('/api/proxy/expose-admin', {
    method: 'POST',
    body: JSON.stringify({ enabled }),
  })

/** 单独一个接口：它即点即生效，不该顺带把监听重开一遍 */
export const setProxyAccessLog = (enabled: boolean) =>
  request<unknown>('/api/proxy/accesslog', {
    method: 'POST',
    body: JSON.stringify({ enabled }),
  })

// ===================== Cloudflare Tunnel（独立模块）=====================

export const getTunnelConfig = () => request<TunnelConfigView>('/api/tunnel/config')

export const saveTunnel = (body: TunnelSavePayload) =>
  request<TunnelRuntimeState>('/api/tunnel', { method: 'POST', body: JSON.stringify(body) })

export const restartTunnel = () =>
  request<TunnelRuntimeState>('/api/tunnel/restart', { method: 'POST' })

export interface ProxySitePayload {
  /**
   * 站点后端在哪台设备上。**按 id 关联，改设备 IP 时归属不变。**
   * 后端在没传时会按 backend 的 IP 反查兜底，但前端有准的就该给准的。
   */
  deviceId: number
  hosts: string[]
  pathPrefix: string
  stripPrefix: boolean
  backend: string
  backendHttps: boolean
  /** 单独占一个端口；0 = 挂在默认入口上 */
  listenPort: number
  /** 对外走 HTTPS */
  https: boolean
  /** 0 表示按 SNI 自动匹配 */
  certId: number
  enabled: boolean
  description: string
  /** IP 门禁；mode 为空串表示不限制 */
  access: ProxyAccess
}

export const addProxySite = (body: ProxySitePayload) =>
  request<ProxySite>('/api/proxy/site', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateProxySite = (body: ProxySitePayload & { id: number }) =>
  request<ProxySite>('/api/proxy/site', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const deleteProxySite = (id: number) =>
  request<unknown>('/api/proxy/site', {
    method: 'DELETE',
    body: JSON.stringify({ id }),
  })

/** 立即拨一次后端。不要求先保存，填到一半就能点 */
export const testProxySite = (body: ProxySitePayload) =>
  request<ProxyProbeResult>('/api/proxy/site/test', {
    method: 'POST',
    body: JSON.stringify(body),
  })

/** 网络唤醒：一台可被唤醒的机器 */
export interface WolTarget {
  id: string
  name: string
  mac: string
  broadcast: string
  port: number
  note?: string
  createdAt?: string
}

export const getWolTargets = () =>
  request<{ targets: WolTarget[] }>('/api/wol/targets')

export const addWolTarget = (body: Omit<WolTarget, 'createdAt'>) =>
  requestWithMsg<unknown>('/api/wol/target/add', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateWolTarget = (body: WolTarget) =>
  requestWithMsg<unknown>('/api/wol/target/update', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const deleteWolTarget = (id: string) =>
  requestWithMsg<unknown>('/api/wol/target/delete', {
    method: 'DELETE',
    body: JSON.stringify({ id }),
  })

/**
 * 发送唤醒包。
 *
 * 注意返回里的 `hint`：后端只保证「包发出去了」，**不能**确认目标是否醒来
 * （目标可能没开 WOL、广播地址不对、或跨了网段）。所以这里不返回成功布尔值，
 * 而是把那句提示原样交给界面转述，别让用户以为点完就该亮屏。
 */
export const wakeWolTarget = (id: string) =>
  requestWithMsg<{ sent: boolean; hint: string }>('/api/wol/wake', {
    method: 'POST',
    body: JSON.stringify({ id }),
  })

/** 计划任务 */
export interface TaskItem {
  id: string
  name: string
  enabled: boolean
  kind: 'interval' | 'daily'
  intervalMin: number
  hour: number
  minute: number
  action: 'ddns' | 'wol'
  target: string
  lastRun?: string
  lastResult?: string
  lastError?: string
  nextRun?: string
}

export const getTasks = () => request<{ tasks: TaskItem[] }>('/api/task/tasks')

export const addTask = (body: Omit<TaskItem, 'lastRun' | 'lastResult' | 'lastError' | 'nextRun'>) =>
  request<TaskItem>('/api/task/task/add', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateTask = (body: TaskItem) =>
  requestWithMsg<unknown>('/api/task/task/update', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const deleteTask = (id: string) =>
  requestWithMsg<unknown>('/api/task/task/delete', {
    method: 'DELETE',
    body: JSON.stringify({ id }),
  })

/**
 * 立刻执行一次。
 *
 * 后端是同步执行的（动作本身秒级完成），所以这里能直接拿到 ok / error。
 * 手动与定时走同一条路径，不存在「手动能跑、定时没反应」这种两套逻辑。
 */
export const runTask = (id: string) =>
  requestWithMsg<{ ok: boolean; lastRun?: string; error?: string }>('/api/task/task/run', {
    method: 'POST',
    body: JSON.stringify({ id }),
  })

// ===================== 通知中心 =====================

export interface NotifyChannelSavePayload {
  id: string
  name: string
  type: NotifyChannelType
  enabled: boolean
  config: Record<string, string>
}

export interface NotifyRuleSavePayload {
  id: string
  name: string
  event: string
  channelIds: string[]
  enabled: boolean
}

export async function getNotifyConfig(): Promise<NotifyConfigView> {
  const data = await request<NotifyConfigView>('/api/notify/config')
  // Go 的 nil 切片会序列化成 null，前端拿到 null 取 .length 就是 TypeError，
  // 整个页面白屏且看不出是哪个字段。后端已经给空切片了，这里再兜一层 ——
  // 这种崩法排查成本太高，不值得只靠一边保证。
  data.channels = (data.channels ?? []).map((c) => ({ ...c, usedByRules: c.usedByRules ?? [] }))
  data.rules = data.rules ?? []
  data.events = data.events ?? []
  return data
}

/** 哪些穿透服务在用哪些渠道（独立接口，方便通知页直接拉） */
export const getNotifyServiceRefs = () => request<NotifyServiceRef[]>('/api/notify/service-refs')

export const saveNotifyChannel = (body: NotifyChannelSavePayload) =>
  request<{ id: string }>('/api/notify/channel', { method: 'POST', body: JSON.stringify(body) })

export const deleteNotifyChannel = (id: string) =>
  request<unknown>('/api/notify/channel/delete', { method: 'POST', body: JSON.stringify({ id }) })

export const testNotifyChannel = (body: { id?: string; type?: string; config?: Record<string, string> }) =>
  requestWithMsg<unknown>('/api/notify/channel/test', { method: 'POST', body: JSON.stringify(body) })

export interface NotifyServiceTestResult {
  channelId: string
  channelName: string
  ok: boolean
  error?: string
}

/**
 * 用服务自己的配置（渠道 + 模板）试发一条。
 *
 * 用户最想知道的就是"我这样配到底能不能收到"，所以走真实事件那条
 * 渲染 + 发送路径，逐渠道回结果 —— 三个渠道里两个通一个不通时，
 * 只回一句"发送失败"他根本不知道该去改哪个。
 */
export const testNotifyService = (body: {
  channelIds: string[]
  template?: string
  fields?: Record<string, string>
}) =>
  request<{ results: NotifyServiceTestResult[]; sent: number; total: number }>(
    '/api/notify/service-test',
    { method: 'POST', body: JSON.stringify(body) },
  )

export const saveNotifyRule = (body: NotifyRuleSavePayload) =>
  request<unknown>('/api/notify/rule', { method: 'POST', body: JSON.stringify(body) })

export const deleteNotifyRule = (id: string) =>
  request<unknown>('/api/notify/rule/delete', { method: 'POST', body: JSON.stringify({ id }) })

// ===================== 流量统计 =====================

/**
 * 实时速率 + 累计值。会被仪表盘每 2 秒调一次。
 *
 * 后端只读内存里的滚动窗口，不碰磁盘 —— 所以调这么频没问题。
 */
export const getTrafficRealtime = () => request<TrafficRealtime>('/api/traffic/realtime')

/** 近 N 天趋势（默认 7 天） */
/**
 * 流量趋势。**hours 优先**（近 1 小时 / 近 24 小时这类），没传用 days。
 * 聚合粒度后端按范围自动定：≤48 小时按小时，更大按天。
 */
export const getTrafficTrend = (params: { hours?: number; days?: number } = {}) => {
  const q = new URLSearchParams()
  if (params.hours) q.set('hours', String(params.hours))
  else q.set('days', String(params.days ?? 7))
  return request<TrafficTrend>(`/api/traffic/trend?${q.toString()}`)
}

// ===================== 访问审计与 IP 管理 =====================

export const getAuditConfig = () => request<AuditConfigView>('/api/audit/config')

/** 概览：指标卡 + 趋势 + 攻击类型 + 地区 + 站点，一次拿齐 */
export const getAuditOverview = (hours = 24) =>
  request<AuditOverview>(`/api/audit/overview?hours=${hours}`)

export const getAuditIPs = (
  params: {
    sort?: string
    suspicious?: boolean
    limit?: number
    page?: number
    /** 关键词：IP / 归属地 / 路径 / 主机名都能搜 */
    q?: string
    /** 只看这一天；留空 = 累计（跨天累加） */
    day?: string
  } = {},
) => {
  const q = new URLSearchParams()
  if (params.sort) q.set('sort', params.sort)
  if (params.suspicious) q.set('suspicious', '1')
  if (params.limit) q.set('limit', String(params.limit))
  if (params.page) q.set('page', String(params.page))
  if (params.q) q.set('q', params.q)
  if (params.day) q.set('day', params.day)
  return request<{
    list: AuditIPStat[]
    total: number
    page: number
    limit: number
    /** 当前看的是哪天；空 = 累计 */
    day: string
    /** 有访问记录的日期，给「按日期查看」下拉用 */
    days: string[]
    geoEnabled: boolean
  }>(`/api/audit/ips?${q.toString()}`)
}

export const getAuditRecords = (
  params: { day?: string; ip?: string; siteId?: number; ref?: string; limit?: number } = {},
) => {
  const q = new URLSearchParams()
  if (params.day) q.set('day', params.day)
  if (params.ip) q.set('ip', params.ip)
  // siteId：反代站点的日志模块用 —— "最近谁访问过我这个站点"
  if (params.siteId) q.set('siteId', String(params.siteId))
  // ref：穿透服务的日志模块用 —— "s<设备ID>-<服务ID>"（穿透没有站点 ID 这个概念）
  if (params.ref) q.set('ref', params.ref)
  if (params.limit) q.set('limit', String(params.limit))
  return request<{ list: AuditRecord[]; day?: string }>(`/api/audit/records?${q.toString()}`)
}

/** 日志弹窗用的记录类型（就是 AuditRecord，起个别名给组件用） */
export type AuditRecordLike = AuditRecord

export const blockIP = (body: { ip: string; reason: string; minutes: number }) =>
  requestWithMsg<unknown>('/api/audit/block', { method: 'POST', body: JSON.stringify(body) })

export const unblockIP = (ip: string) =>
  requestWithMsg<unknown>('/api/audit/unblock', { method: 'POST', body: JSON.stringify({ ip }) })

export const blockIPBatch = (body: { ips: string[]; reason: string; minutes: number }) =>
  request<{ ok: number; failed: Record<string, string>; total: number }>('/api/audit/block/batch', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const saveAuditSettings = (body: {
  enabled: boolean
  keepDays: number
  maxFileMB: number
  autoBlock: boolean
  autoBlockThreshold: number
  autoBlockMinutes: number
  whitelist: string[]
  trustedProxies: string[]
}) => requestWithMsg<unknown>('/api/audit/settings', { method: 'POST', body: JSON.stringify(body) })

/**
 * 保存「永不封禁」名单（白名单）。
 *
 * 走独立的 /api/audit/whitelist：它只改名单与自动拉黑，
 * 不碰保留天数、单天上限那些 —— 免得改名单时把整份设置覆盖回去。
 */
export const saveAuditWhitelist = (body: {
  whitelist: string[]
  autoBlock: boolean
  autoBlockThreshold: number
  autoBlockMinutes: number
  trustedProxies?: string[]
}) => requestWithMsg<unknown>('/api/audit/whitelist', { method: 'POST', body: JSON.stringify(body) })

// ===================== 反代复用局域网扫描 =====================
//
// 和穿透那边是同一套后端实现（core/lanscan），只是填的字段不同：
// 那边点端口是"建服务"，这边是"填站点 backend"。

export const getProxyLanSubnets = () => request<LanSubnet[]>('/api/proxy/lan/subnets')

export const scanProxyLan = (cidr: string) =>
  request<LanHost[]>('/api/proxy/lan/scan', { method: 'POST', body: JSON.stringify({ cidr }) })

/**
 * 站点表单的可选项：已配的 DDNS 域名、本机网段、本机地址。
 * 在 api 层聚合（proxy 不直接依赖 ddns）。
 */
export const getProxySiteOptions = () => request<ProxySiteOptions>('/api/proxy/site-options')

/** 站点的一键开关（启用 / 显示在主页） */
export const setProxySiteFlag = (id: number, flag: 'enabled', value: boolean) =>
  request<unknown>('/api/proxy/site/flag', {
    method: 'POST',
    body: JSON.stringify({ id, flag, value }),
  })

/** 登录尝试（谁在试着进后台） */
export const getAuditLogins = (
  params: { day?: string; ip?: string; limit?: number; page?: number } = {},
) => {
  const q = new URLSearchParams()
  if (params.day) q.set('day', params.day)
  if (params.ip) q.set('ip', params.ip)
  if (params.limit) q.set('limit', String(params.limit))
  if (params.page) q.set('page', String(params.page))
  return request<{
    list: LoginEvent[]
    /** 当前看的是哪天 */
    day: string
    /** 有登录记录的那些日期（给「按日期看」的下拉用） */
    days: string[]
    /** 当天总条数，给分页用 */
    total: number
    /** 当天汇总。**不随分页变化** —— 它回答的是"这天总共怎样" */
    summary: LoginSummary
  }>(`/api/audit/logins?${q.toString()}`)
}

// ===================== 设备（三个功能共用一份） =====================

export const getDevices = () => request<Device[]>('/api/device/list')

export const saveDevice = (id: number, name: string, ip: string) =>
  request<Device>('/api/device', { method: id ? 'PUT' : 'POST', body: JSON.stringify({ id, name, ip }) })

/** 删一台设备。cascade=true 时连带删掉它上面的服务 */
export const deleteDevice = (id: number, cascade = false) =>
  request<unknown>('/api/device', { method: 'DELETE', body: JSON.stringify({ id, cascade }) })

// ===================== 端口转发 =====================

export const saveForward = (f: Partial<ProxyForward>) =>
  request<ProxyForward>('/api/proxy/forward', {
    method: f.id ? 'PUT' : 'POST',
    body: JSON.stringify(f),
  })

export const deleteForward = (id: number) =>
  request<unknown>('/api/proxy/forward', { method: 'DELETE', body: JSON.stringify({ id }) })

/** 拖动排序后保存设备顺序 */
export const reorderDevices = (ids: number[]) =>
  request<unknown>('/api/device/reorder', { method: 'POST', body: JSON.stringify({ ids }) })

/** 扫一台机器上开了哪些端口（服务表单里选好设备后点扫描） */
export const scanHostPorts = (ip: string) =>
  request<LanPort[]>('/api/device/scan-ports', { method: 'POST', body: JSON.stringify({ ip }) })

/** 拖动排序后保存站点（服务）顺序 */
export const reorderSites = (ids: number[]) =>
  request<unknown>('/api/proxy/sites/reorder', { method: 'POST', body: JSON.stringify({ ids }) })

/** 拖动排序：某台设备下的穿透服务换个顺序 */
export const reorderStunServices = (deviceId: number, ids: number[]) =>
  request<unknown>('/api/stun/services/reorder', {
    method: 'POST',
    body: JSON.stringify({ deviceId, ids }),
  })

/**
 * 一条规则（站点 / 转发 / 穿透服务）归属到哪台设备。
 *
 * **先按 deviceId，找不到再按 IP**：
 * 这是"改设备 IP 不丢归属"的关键 —— 老数据只有 IP（deviceId 是 0），新数据按 id 关联。
 * 两边都试一遍，任何一边对得上就算归到那台设备，不会因为 IP 变了就掉进"未归到设备"。
 *
 * @param rule     规则对象（有 deviceId 和/或 addr）
 * @param addr    规则里的 "ip:port" 字符串（站点的 backend / 转发的 target / 穿透不需要）
 * @param devices 设备列表
 */
export function deviceOf<T extends { id: number; ip: string }>(
  rule: { deviceId?: number },
  addr: string,
  devices: T[],
): T | undefined {
  if (rule.deviceId) {
    const d = devices.find((x) => x.id === rule.deviceId)
    if (d) return d
  }
  // 兜底：老数据按 IP 字符串匹配（站点/转发），穿透服务没有 addr 就跳过
  const ip = String(addr ?? '').split(':')[0]
  if (!ip) return undefined
  return devices.find((x) => x.ip === ip)
}

// ===================== 备份与恢复 =====================

/** 可备份的一项（模块或资产） */
export interface BackupItem {
  name: string
  label: string
  /** 现在有没有内容；没有的不让用户勾 */
  exists: boolean
  /** 含凭据（token / 密钥 / 口令哈希）—— 备份包是明文，要提示 */
  sensitive?: boolean
}

export interface BackupModulesView {
  modules: BackupItem[]
  assets: BackupItem[]
}

/** 备份包清单（预览时看到的东西） */
export interface BackupManifest {
  version: number
  appVersion?: string
  createdAt?: string
  modules?: { name: string; label: string; file: string; size: number; exists: boolean }[]
  assets?: { kind: string; path: string; size: number }[]
  sensitive?: boolean
}

export interface BackupImportResult {
  restoredModules: string[]
  restoredAssets: string[]
  skipped: string[]
}

/** 可备份的模块与资产清单 */
export const getBackupModules = () => request<BackupModulesView>('/api/backup/modules')

/**
 * 预览备份包：先看清单再决定恢复哪些模块。
 * 走 multipart（文件），不用 JSON。
 */
export const previewBackup = (file: File) => {
  const fd = new FormData()
  fd.append('file', file)
  return request<BackupManifest>('/api/backup/preview', { method: 'POST', body: fd })
}

/**
 * 恢复。内部会先给现有 config/ 打快照，失败自动回滚。
 *
 * @param modules 要恢复哪些模块；空数组 = 全部
 */
export const importBackup = (file: File, modules: string[] = [], assets: string[] = []) => {
  const fd = new FormData()
  fd.append('file', file)
  modules.forEach((m) => fd.append('modules', m))
  assets.forEach((a) => fd.append('assets', a))
  return request<BackupImportResult>('/api/backup/import', { method: 'POST', body: fd })
}

/**
 * 导出备份包，触发浏览器下载。
 *
 * 单独一个函数（而不是走 request）因为返回的是二进制 zip，
 * 要读 Content-Disposition 里的文件名再自己触发下载。
 */
export async function exportBackup(modules: string[] = [], assets: string[] = []): Promise<string> {
  const token = getToken()
  const res = await fetch('/api/backup/export', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
    },
    body: JSON.stringify({ modules, assets }),
  })
  if (!res.ok) {
    let msg = `导出失败（HTTP ${res.status}）`
    try {
      const j = await res.json()
      if (j?.msg) msg = j.msg
    } catch {
      // 响应不是 JSON（比如网关返回的 HTML），用默认提示
    }
    throw new Error(msg)
  }

  // 从 Content-Disposition 拿文件名，服务端已经生成好了
  let filename = 'catflap-backup.zip'
  const cd = res.headers.get('Content-Disposition') || ''
  const m = /filename="([^"]+)"/.exec(cd)
  if (m) filename = m[1]

  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
  return filename
}
