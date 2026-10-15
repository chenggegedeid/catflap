/**
 * 后台页面标识（侧栏条目、hash 路由里的 key）。
 *
 * 真正的定义在 pages/registry.tsx —— 那是**唯一**描述「后台有哪些页面」的
 * 地方（标题、图标、分组、归属哪个后端组件都在那儿）。这里只做转发，让老代码
 * 继续从 '../types' 拿它。
 *
 * 这么绕一下是为了「新增页面只改一处」：以前加一个页面要同时动这里的联合类型、
 * nav.ts 的列表、App.tsx 的 switch，而 App.tsx 那个 switch 漏一个 case 时
 * 页面不会报错，只会静默渲染成占位符 —— 正是 M3 要消掉的那类问题。
 */
export type { PageKey } from './pages/registry'

// ===================== 组件（功能）注册表 =====================

/**
 * 一个后端组件的元信息，来自 GET /api/system/components。
 *
 * 字段与后端 core/component.Descriptor 一一对应。它由后端注册表从组件实例上
 * 探测可选接口自动拼出来，前端不需要维护任何一份「有哪些功能」的清单。
 */
export interface ComponentInfo {
  name: string
  /**
   * 配置结构声明。后端 SchemaProvider 尚未被界面消费（表单还是各页自己写的），
   * 这里先留着类型口子，等 M4 之后接自动表单时再用。
   */
  schema?: unknown
  /** 是否支持在界面上开关（后端组件有没有实现 Enablable） */
  enableable: boolean
  /** 当前是否启用；不支持开关时该值无意义 */
  enabled: boolean
  /** 该组件声明的定时任务名 */
  tasks?: string[]
  /** 是否提供健康状态 */
  healthAware: boolean
}

export type ComponentHealthStatus = 'unknown' | 'healthy' | 'degraded' | 'down'

/** 组件健康状态。后端保证快速返回（只读缓存，不做网络探测） */
export interface ComponentHealth {
  status: ComponentHealthStatus
  /** 给用户看的一句话，健康时为空 */
  message?: string
  details?: Record<string, unknown>
  checkedAt: string
}

export interface ComponentsView {
  components: ComponentInfo[]
  /** 按组件名索引的健康状态；未提供健康检查的组件不会出现在这里 */
  health?: Record<string, ComponentHealth>
  /** 当前被关掉的组件名 */
  disabled: string[]
}

export interface LogEntry {
  time: string
  level: string
  caller: string
  message: string
}

/** 一键更新：'' 空闲 / downloading / restarting / failed */
export interface UpdateJob {
  state: '' | 'downloading' | 'restarting' | 'failed'
  /** 正在从哪个源下，如「ghfast.top」「GitHub 直连」 */
  source: string
  done: number
  total: number
  error: string
}

/** 下载源：'' 内置（几个加速镜像轮着试，最后直连）/ official 只直连 GitHub / custom 自己填的前缀 */
export type UpdateSource = '' | 'official' | 'custom'

export interface UpdateConfig {
  source: UpdateSource
  custom: string
}

export interface UpdateStatus {
  current: string
  latest: string
  hasUpdate: boolean
  /** Release 说明，markdown 原文 */
  notes: string
  releaseUrl: string
  checkedAt?: string
  checkError: string
  /** 不能一键更新的原因（没有这个平台的文件、飞牛……），空串表示可以 */
  manualReason: string
  /** 不能手动上传更新的原因，空串表示可以 */
  uploadReason: string
  /** 本机对应的发布文件名，没有对应的是空串 */
  asset: string
  config: UpdateConfig
  /** 后端进程启动时间，变了说明重启完成 */
  startedAt: string
  job: UpdateJob
}

export interface StunService {
  id: number
  name: string
  internalPort: number
  protocol: string
  upnpMappedPort: number
  useUpnp: boolean
  /**
   * 打洞用的本机端口；0 = 每次随机。
   * 外部端口是 NAT 照着本机端口分配的，本机端口随机外部端口就每次都变，
   * 配好的「域名+端口」过一会儿就失效。钉住本机端口外部端口才稳。
   */
  punchLocalPort: number
  /** 仅影响链接展示成 http:// 还是 https://，不改变转发行为 */
  https: boolean
  /** 该服务对外域名，如 fw.example.com；留空回落公网 IP */
  domain: string
  /** 由 Catflap 在洞口终结 TLS（仅 TCP 有效） */
  tlsTerminate: boolean
  /** 绑定的证书 ID，0 表示按域名自动匹配 */
  certId: number
  /** 内网服务本身就说 TLS（自签也算），转发时用 tls.Dial */
  backendHttps: boolean
  enabled: boolean
  description: string
  webhookconfig?: WebhookConfig
  redirect?: RedirectConfig
  mcEntry?: MCEntryConfig
}

/** MC Java 版联机：_minecraft._tcp.<host> 这条 SRV 跟着外部端口走，朋友只填域名 */
export interface MCEntryConfig {
  enabled: boolean
  providerId: number
  /** 朋友在 MC 里填的域名，如 mc.example.com */
  host: string
  /** 主域名；留空按 host 的后两段取 */
  zoneDomain: string
  /**
   * SRV 指向的主机。空 = 指向 host 自己（DDNS 里没有就自动补一条 A 记录）；
   * 填了 = 指向这个已经解析好的域名，不再补记录。
   */
  target: string
}

/**
 * 让一个固定的域名，始终指向这个服务当前的外网地址。
 * zone / ruleset / rule 三个 ID 都不用填，后端自己查。
 */
export interface RedirectConfig {
  enabled: boolean
  /** 用 DDNS 里已经配好的服务商，token 不用再贴一遍 */
  providerId: number
  /** 外面访问用的名字，如 catflap.example.com */
  entryHost: string
  /** 主域名；留空按 entryHost 的后两段取 */
  zoneDomain: string
}

/**
 * 入口重定向靠的那两条解析记录现在各是什么样。
 *
 * 两条的要求正好相反：入口域名要开橙云（不开，重定向规则轮不到执行），
 * 落地域名要关橙云（开了，Cloudflare 的代理不转发打洞出来的高位端口）。
 * 配错了两边都不报错，只体现为访问不了，所以得摆出来看。
 */
export interface RedirectInspection {
  entry: EntryRecordState
  /** 307 之后真正要连的那个域名 */
  landingHost: string
  landing: LandingRecordState
}

/** 入口域名那条解析记录的现状；内容是个永远不变的占位地址，不归 DDNS 管 */
export interface EntryRecordState {
  host: string
  found: boolean
  type: string
  content: string
  /** 橙云。灰云的话请求根本不经过 Cloudflare，重定向不会执行 */
  proxied: boolean
  /** 这条是 Catflap 建的，不是用户手加的 */
  byCatflap: boolean
  /** Catflap 建这条记录用的占位地址 */
  wantIP: string
  /** 查不了这条记录时的说明，多半是 Token 没有 DNS 权限 */
  warn: string
}

/** 落地域名在 DDNS 那边由哪条记录管着 */
export interface LandingRecordState {
  /** 没有的话公网 IP 一变就得手动去改 */
  managed: boolean
  name: string
  lastIP: string
  status: string
  message: string
  at: string
}

/** 本机接着的一段局域网，给扫描前挑网段用 */
export interface LanSubnet {
  cidr: string
  /** 网卡名，如「以太网」「VMware Network Adapter VMnet8」 */
  iface: string
  /** 本机在这段网里的地址 */
  localIP: string
  /** 这段网有多少个可用地址 */
  hosts: number
}

/** 扫出来的一个开着的端口，name 是认得出来的话给的说明 */
export interface LanPort {
  port: number
  name: string
}

/** 扫描扫到的一台机器 */
export interface LanHost {
  ip: string
  /** 反查到的名字，查不到就是空 */
  name: string
  ports: LanPort[]
  /** 这台就是 Catflap 自己跑着的机器 */
  self: boolean
  /** 设备列表里已经有这个 IP 了，别让用户又加一遍 */
  added: boolean
  /** 已经加过的话，当时起的名字 */
  addedName: string
}

export interface WebhookConfig {
  enabled: boolean
  onlyWhenChanged: boolean
  url: string
  method: string
  headers: string
  body: string
  disableSuccessCheck: boolean
  successContains: string
  proxy: string
}

export interface WebhookTemplate {
  id: string
  name: string
  description: string
  builtin: boolean
  config: WebhookConfig
  createdAt?: string
  updatedAt?: string
}

export interface StunDevice {
  id: number
  DeviceID?: number
  deviceId?: number
  name: string
  ip: string
  services: StunService[]
}

export interface NatRouter {
  natLevel: number
  lanIP: string
  ipType: 'private' | 'cgn'
}

export interface NatDetectResult {
  natType: string
  mapping: string
  filtering: string
  mappingErr?: string
  filteringErr?: string
}

export interface NatTypeInfo {
  udp: NatDetectResult | null
  tcp: NatDetectResult | null
  error?: string
}

export interface StunConfig {
  localIP: string
  publicIP: string
  bestStun: string
  stunServerList?: string[]
  natRouterList: NatRouter[]
  devices: StunDevice[]
}

/** 网络设置的模式：'' 自动/内置，'system' 跟随系统，'custom' 指定 */
export type NetworkMode = '' | 'system' | 'custom'

export interface StunNetworkConfig {
  ifaceMode: NetworkMode
  iface: string
  dnsMode: NetworkMode
  dns: string[]
}

export interface OutboundIface {
  name: string
  localIP: string
  gateway: string
  /** TUN / VPN 这类隧道，自动模式不会选 */
  tunnel: boolean
  /** 自动模式会选的就是它 */
  auto: boolean
}

export interface StunNetwork {
  config: StunNetworkConfig
  ifaces: OutboundIface[]
  defaultDns: string[]
  /** 现在实际在用的出口 */
  current: OutboundIface
}

export interface StunStatusLog {
  createdAt: string
  phaseStr: string
  message: string
}

export interface StunStatusEvent {
  key: string
  deviceName?: string
  serviceName?: string
  phaseStr: string
  restartCount?: number
  externalPort?: number
  lastError?: string
  logs?: StunStatusLog[]
  /** ok / failed / stale（端口已经变了、还没写过去），空表示还没同步过 */
  redirectStatus?: string
  redirectTarget?: string
  redirectError?: string
  /** 服务商是否接受了「保留原始路径」的写法 */
  redirectKeepPath?: boolean
  redirectAt?: string
  /** MC 入口（SRV）：ok / failed，空表示还没同步过 */
  mcEntryStatus?: string
  mcEntryPort?: number
  mcEntryError?: string
  mcEntryAt?: string
}

/**
 * DNS 服务商类型的键，与后端 core/provider 注册表里的 Descriptor.Key 一一对应。
 *
 * 刻意**不做成字面量联合**：可选的服务商由后端注册表决定，前端跟着
 * /api/ddns/provider/types 走。写死成 'cloudflare' | 'alidns' | … 的话，
 * 后端加一家就变成「前端也要改一行」——而 A3 要消掉的正是这件事。
 */
export type DnsProviderType = string
// 'A+AAAA' = 双栈：一条记录同时同步 A 和 AAAA，不必建两条
export type DnsRecordType = 'A' | 'AAAA' | 'A+AAAA'
export type DdnsRecordStatus = 'pending' | 'success' | 'failed' | 'skipped'
export type IpSourceType = 'stun' | 'web' | 'dns' | 'interface' | 'custom'

/** 服务商能力标记，与后端 provider.Capability 的常量值一致 */
export const ProviderCap = {
  /** 支持 ACME DNS-01，可签通配符证书 */
  DNS01: 'dns-01',
  /** 支持写边缘重定向规则 */
  Redirect: 'dns-redirect',
  /** 支持写 SRV 记录 */
  SRV: 'dns-srv',
  Wildcard: 'dns-wildcard',
  /** 支持小云朵代理开关 */
  Proxied: 'dns-proxied',
  /** 只支持 A 记录，不能写 AAAA */
  IPv4Only: 'dns-ipv4-only',
} as const

/** 一个凭据输入框的声明，后端 component.Field 的子集 */
export interface ProviderFieldSpec {
  key: string
  label: string
  type?: string
  required?: boolean
  placeholder?: string
  help?: string
  options?: { value: string; label: string }[]
  /** 敏感字段：界面打码，后端不回传明文 */
  secret?: boolean
}

/** 一个可选的服务商类型。来自 GET /api/ddns/provider/types */
export interface DdnsProviderTypeSpec {
  type: DnsProviderType
  label: string
  docsUrl?: string
  capabilities: string[]
  fields: ProviderFieldSpec[]
}

/** 本机一块网卡，和它上面能写进 DNS 的地址 */
export interface NetInterface {
  name: string
  ipv4: string[]
  ipv6: string[]
  /** 有公网地址。只有 192.168 / fd00:: 的网卡写进公网 DNS，外面连不上 */
  hasPublic: boolean
}

export interface DdnsProvider {
  id: number
  name: string
  type: DnsProviderType
  hasCredential: boolean
}

export interface DdnsRecord {
  enabled: boolean
  id: number
  providerId: number
  name: string
  domain: string
  subDomain: string
  recordType: DnsRecordType
  ipSource: IpSourceType
  ipSourceArg: string
  ttl: number
  proxied: boolean
  lastIP: string
  lastStatus: DdnsRecordStatus
  lastMessage: string
  lastCheckAt: string
  lastSyncAt: string
  createdAt: string
  updatedAt: string
}

export interface DdnsConfig {
  intervalSec: number
  providers: DdnsProvider[]
  records: DdnsRecord[]
}

// ===================== 证书 =====================

/**
 * upload     手动粘贴/上传 PEM
 * path       读本机文件路径（外部 certbot / acme.sh 续期后自动热重载）
 * acme-dns01 自动签发，写 TXT 验证，支持通配符
 * acme-http01 自动签发，需公网 80 端口可入站
 */
export type CertSource = 'upload' | 'path' | 'acme-dns01' | 'acme-http01' | 'self-signed'

export interface AcmeOptions {
  directory: string
  email: string
  providerId: number
  httpPort: number
  eabKeyId: string
  /** HMAC 属于密钥，后端只回是否已配置 */
  hasEabHmac: boolean
  renewDays: number
}

export interface Certificate {
  id: number
  name: string
  source: CertSource
  domains: string[]
  enabled: boolean
  isDefault: boolean
  certFile: string
  keyFile: string
  acme: AcmeOptions
  notBefore?: string
  notAfter?: string
  issuer?: string
  lastError?: string
  lastIssueAt?: string
  /** 是否已真正装载进内存对外提供服务 */
  loaded: boolean
  daysLeft: number
  expired: boolean
  createdAt?: string
  updatedAt?: string
}

export interface AcmeProvider {
  id: number
  name: string
  type: DnsProviderType
  /** false 时该服务商还没实现 TXT 写入，不能用于 DNS-01 */
  supportsDns01: boolean
}

// ===================== 反向代理 =====================

/**
 * 默认入口，对应 nginx 的 listen 80 / listen 443 ssl。
 *
 * 站点不单独指定端口时就挂在这两个上面。端口填 0 表示不开这个入口。
 * 这里是用户的意愿；实际跑没跑起来看 listeners。
 */
export interface ProxyEntry {
  enabled: boolean
  /** HTTP 入口端口，0 = 不开 */
  httpPort: number
  /** HTTPS 入口端口，0 = 不开 */
  httpsPort: number
  /** HTTPS 入口的默认证书，0 表示按 SNI 自动匹配 */
  certId: number
  accessLog: boolean
  /** 主域名，如 example.com；加站点时只填前缀就能补全 */
  baseDomain: string
}

/** 此刻实际开着的一个监听端口，对照 netstat 看到的那一行 */
export interface ProxyListener {
  port: number
  /** 这个端口是 https */
  tls: boolean
  /** 0 表示按 SNI 自动匹配 */
  certId: number
  /** 挂在这个端口上的站点数 */
  siteCount: number
  running: boolean
  lastError: string
}

/** 一个站点，对应 nginx 的一个 server 块 */
export interface ProxySite {
  id: number
  /** 域名，分流依据，至少一个。对应 nginx 的 server_name a.com www.a.com */
  hosts: string[]
  pathPrefix: string
  /** 转发前剥掉前缀，后端需自己支持 base path */
  stripPrefix: boolean
  backend: string
  /** 后端本身就说 TLS（自签也算） */
  backendHttps: boolean
  /** 单独占一个端口；0 = 挂在默认入口上 */
  listenPort: number
  /** 对外走 HTTPS */
  https: boolean
  /** 0 表示按 SNI 自动匹配 */
  certId: number
  enabled: boolean
  description: string
  /** IP 门禁 */
  access?: ProxyAccess
  updatedAt?: string
}

/**
 * 站点级 IP 门禁。
 *
 * 服务暴露到公网后，光靠「域名没人知道」不算防护——域名会出现在证书透明
 * 日志里，扫描器也在扫整个网段。
 */
export interface ProxyAccess {
  /** '' = 谁都能访问；allow = 白名单；deny = 黑名单 */
  mode: '' | 'allow' | 'deny'
  /** IP 或 CIDR 网段，如 203.0.113.0/24；家宽 IP 会变，写网段更耐用 */
  rules: string[]
}

export interface ProxyConfig {
  entry: ProxyEntry
  listeners: ProxyListener[]
  sites: ProxySite[]
}

export interface ProxyProbeResult {
  reachable: boolean
  /** 实际探到的协议：http / https */
  scheme: string
  /** 和用户勾的不一致 */
  mismatch: boolean
  latencyMs: number
  message: string
}
