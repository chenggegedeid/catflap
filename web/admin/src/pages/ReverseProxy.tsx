import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import {
  Radar,
  AlertCircle,
  ArrowRight,
  ExternalLink,
  Globe,
  HardDrive,
  FileText,
  Pencil,
  Power,
  Plus,
  Settings2,
  ShieldCheck,
  TriangleAlert,
  Trash2,
  X,
  Zap,
} from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import { modalBackdrop } from '../components/modal'
import * as api from '../lib/api'
import type {
  Certificate,
  Device,
  LanPort,
  ProxyConfig,
  ProxyEntry,
  ProxyListener,
  ProxySite,
} from '../types'
import { CardBadge, ServiceCard, iconBtn } from '../components/ServiceCard'
import { DomainInput } from '../components/DomainInput'
import { AccessControlEditor } from '../components/AccessControlEditor'
import { AccessLogModal } from '../components/AccessLogModal'

// ===================== 小工具 =====================

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
    window.setTimeout(() => setToasts((p) => p.filter((t) => t.id !== id)), 3200)
  }, [])
  return { toasts, show }
}

// 后端端口没填时的默认值：内网服务绝大多数跑在 80 上。
// 端口不能留空 —— 空端口拼出的 "ip:" 会让后端解析不出端口、转发失败。
const DEFAULT_BACKEND_PORT = 80

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e))

/** 80 和 443 不用写出来，写了反而显得脏 */
export const portText = (https: boolean, port: number) => ((https ? port === 443 : port === 80) ? '' : `:${port}`)

/**
 * 浏览器自己写死不让连的端口，抄自 Chromium 的 net::kRestrictedPorts
 * （net/base/port_util.cc），Firefox 那份几乎一模一样。
 *
 * 全是上古协议的端口（IRC 6665-6669、NFS 2049、SMTP 25……），历史上被拿来
 * 做跨协议攻击，从 2000 年代禁到现在。这件事阴在：反代监听这些端口不会报错，
 * 后端探测是通的，curl 也拿得到 200，唯独浏览器甩一句 ERR_UNSAFE_PORT，
 * 配的人完全不知道是谁的问题。所以在填的时候就要说出来。
 *
 * 不含 0——这里 0 表示「留空」，不是真端口。
 */
const BROWSER_BLOCKED_PORTS = new Set([
  1, 7, 9, 11, 13, 15, 17, 19, 20, 21, 22, 23, 25, 37, 42, 43, 53, 69, 77, 79, 87, 95, 101, 102,
  103, 104, 109, 110, 111, 113, 115, 117, 119, 123, 135, 137, 139, 143, 161, 179, 389, 427, 465,
  512, 513, 514, 515, 526, 530, 531, 532, 540, 548, 554, 556, 563, 587, 601, 636, 989, 990, 993,
  995, 1719, 1720, 1723, 2049, 3659, 4045, 5060, 5061, 6000, 6566, 6665, 6666, 6667, 6668, 6669,
  6697, 10080,
])

const portBlocked = (port: number) => port > 0 && BROWSER_BLOCKED_PORTS.has(port)

/**
 * 「按域名自动匹配」在客户端没发 SNI 的时候还能不能挑出一张证书。
 *
 * 逐条对着 modules/cert.Manager.Lookup 的第 ④⑤ 步：先找默认证书，
 * 再看是不是只有一张可用的。两条都不满足，握手就地失败——浏览器那头
 * 只有一句 ERR_SSL_PROTOCOL_ERROR，什么都看不出来，所以得提前说。
 *
 * 敲 IP 的地址栏是不发 SNI 的（RFC 6066 只允许域名），所以「没填域名的
 * HTTPS 站点」正好撞在这个坑上。
 */
const autoCertNeedsSNI = (certs: Certificate[]) => {
  const usable = certs.filter((c) => c.enabled && c.loaded)
  return !usable.some((c) => c.isDefault) && usable.length !== 1
}

/** 淡黄提示条。只是提醒，不拦保存——挡不挡得住由用户自己判断 */
function Warn({ children }: { children: ReactNode }) {
  return (
    <div className="flex gap-2 rounded-xl bg-amber-50 px-3 py-2.5 text-[11px] leading-relaxed text-amber-700 ring-1 ring-amber-200">
      <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
      <span>{children}</span>
    </div>
  )
}

/** 端口落在浏览器禁用名单上时的说辞，两个表单共用 */
function BlockedPortWarn({ label, port }: { label: string; port: number }) {
  return (
    <Warn>
      {label} <span className="font-mono font-semibold">{port}</span>{' '}
      在浏览器的禁用端口名单里，Chrome / Firefox 会直接拒连，报{' '}
      <span className="font-mono">ERR_UNSAFE_PORT</span>。反代照样监听得好好的、
      <span className="font-mono">curl</span> 也能通，只有浏览器进不来——换一个端口
    </Warn>
  )
}

/**
 * 站点对外的地址：自己占端口就用那个，否则挂在默认入口上。
 *
 * 挂默认入口且勾了 HTTPS 时，HTTP 入口上其实也有一份——
 * 但用户真正会用的是 HTTPS 那个，这里就显示它。
 */
export function siteAddr(entry: ProxyEntry | undefined, site: ProxySite): { https: boolean; port: number } | null {
  if (site.listenPort > 0) return { https: site.https, port: site.listenPort }
  // 入口信息还没加载出来时（首帧）拿不到端口，返回空比报错好
  if (!entry) return null
  if (site.https && entry.httpsPort > 0) return { https: true, port: entry.httpsPort }
  if (entry.httpPort > 0) return { https: false, port: entry.httpPort }
  return null
}

/** 可点的访问链接，用第一个域名。通配域名拼不出具体地址，返回空 */
export function siteURL(entry: ProxyEntry | undefined, site: ProxySite): string {
  const addr = siteAddr(entry, site)
  const host = site.hosts?.[0] ?? ''
  if (!addr || !host || host.startsWith('*')) return ''
  return `${addr.https ? 'https' : 'http'}://${host}${portText(addr.https, addr.port)}${site.pathPrefix}`
}

// 不带宽度，留给调用方定。想窄的那几个（代理地址前面的 http/https 下拉）
// 不能写成 `${inputCls} w-24`——Tailwind 的 w-full 在样式表里排在 w-24 后面，
// 类名写在后面也不管用，只会把整行撑满、把后面的输入框顶出弹窗。
const fieldCls =
  'rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400'
const inputCls = `w-full ${fieldCls}`
const labelCls = 'mb-1 text-xs font-semibold text-slate-500'

function Modal({
  title,
  onCancel,
  children,
  footer,
}: {
  title: string
  onCancel: () => void
  children: React.ReactNode
  footer: React.ReactNode
}) {
  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="max-h-full w-full max-w-lg overflow-y-auto rounded-2xl bg-white p-6 text-slate-700 shadow-2xl ring-1 ring-slate-200">
        <div className="mb-5 flex items-center justify-between">
          <div className="text-base font-bold text-slate-800">{title}</div>
          <button
            type="button"
            onClick={onCancel}
            className="grid h-7 w-7 place-items-center rounded-full text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="space-y-3">{children}</div>
        <div className="mt-6 flex justify-end gap-2">{footer}</div>
      </div>
    </div>
  )
}

function CertSelect({
  value,
  certs,
  onChange,
}: {
  value: string
  certs: Certificate[]
  onChange: (v: string) => void
}) {
  return (
    <select value={value} onChange={(e) => onChange(e.target.value)} className={inputCls}>
      <option value="0">按域名自动匹配</option>
      {certs.map((c) => (
        <option key={c.id} value={String(c.id)}>
          {c.name}
          {c.domains.length > 0 ? `（${c.domains.join('、')}）` : ''}
        </option>
      ))}
    </select>
  )
}

// ===================== 默认入口设置 =====================

function EntryModal({
  entry,
  certs,
  onCancel,
  onSubmit,
}: {
  entry: ProxyEntry
  certs: Certificate[]
  onCancel: () => void
  onSubmit: (payload: Omit<api.EntryPayload, 'enabled'>) => Promise<void>
}) {
  const [httpPort, setHttpPort] = useState(entry.httpPort ? String(entry.httpPort) : '')
  const [httpsPort, setHttpsPort] = useState(entry.httpsPort ? String(entry.httpsPort) : '')
  const [certId, setCertId] = useState(String(entry.certId || 0))
  // ?? ''：老后端不回这个字段，少了它下面每个 .trim() 都会炸
  const [baseDomain, setBaseDomain] = useState(entry.baseDomain ?? '')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const httpsOn = (Number(httpsPort) || 0) > 0

  const submit = async () => {
    setErr('')
    setBusy(true)
    try {
      await onSubmit({
        httpPort: Number(httpPort) || 0,
        httpsPort: Number(httpsPort) || 0,
        certId: httpsOn ? Number(certId) || 0 : 0,
        baseDomain: baseDomain.trim(),
      })
    } catch (e) {
      setErr(errText(e))
      setBusy(false)
    }
  }

  return (
    <Modal
      title="默认入口"
      onCancel={onCancel}
      footer={
        <>
          <button
            type="button"
            onClick={onCancel}
            className="rounded-xl px-4 py-2 text-sm text-slate-500 transition hover:bg-slate-100"
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
        </>
      }
    >
      <div className="rounded-xl bg-slate-50 px-3 py-2.5 text-[11px] leading-relaxed text-slate-500 ring-1 ring-slate-200/70">
        没单独指定端口的服务都挂在这两个端口上，靠域名区分——
        就是 nginx 里一堆 server 块共用 <span className="font-mono">listen 80</span> /{' '}
        <span className="font-mono">listen 443 ssl</span> 的那个意思
      </div>

      <div className="rounded-xl bg-blue-50/70 px-3 py-2.5 text-[11px] leading-relaxed text-slate-600 ring-1 ring-blue-200/70">
        <div className="mb-1 font-semibold text-slate-700">想「只开一个端口」就填这儿</div>
        把 HTTPS 端口填成一个固定的高位端口（比如 <span className="font-mono">8443</span>），
        所有二级域名的服务都不指定端口、挂上来按域名分发 —— 对外就只有这一个端口要放通。
        <div className="mt-1.5">
          本机有公网 IPv6 时（没有 IPv4 公网、只有 IPv6 的家宽很常见），
          <span className="font-semibold">这个端口直接就能从外网访问，不需要打洞</span>：
          在「DDNS 域名解析」里加一条 <span className="font-mono">AAAA</span> 记录、
          IP 来源选「本地网卡」，再把域名的 IPv6 解析指向本机即可。
          路由器上记得放行这个端口的 IPv6 入站。
        </div>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <label className="block">
          <div className={labelCls}>HTTP 端口</div>
          <input
            autoFocus
            value={httpPort}
            onChange={(e) => setHttpPort(e.target.value)}
            placeholder="80"
            className={`${inputCls} font-mono`}
          />
        </label>
        <label className="block">
          <div className={labelCls}>HTTPS 端口</div>
          <input
            value={httpsPort}
            onChange={(e) => setHttpsPort(e.target.value)}
            placeholder="443"
            className={`${inputCls} font-mono`}
          />
        </label>
      </div>
      <div className="-mt-1 text-[11px] leading-relaxed text-slate-400">
        留空表示不开这个入口，两个都留空就没入口了。1024 以下的端口要管理员权限
      </div>

      {portBlocked(Number(httpPort)) && <BlockedPortWarn label="HTTP 端口" port={Number(httpPort)} />}
      {portBlocked(Number(httpsPort)) && <BlockedPortWarn label="HTTPS 端口" port={Number(httpsPort)} />}

      {httpsOn && (
        <label className="block">
          <div className={labelCls}>默认证书</div>
          <CertSelect value={certId} certs={certs} onChange={setCertId} />
          <div className="mt-1 text-[11px] leading-relaxed text-slate-400">
            自动匹配会按访问的域名挑证书。指定一张就是所有站点都用它，证书得盖住全部域名——
            <span className="font-mono">*.{baseDomain.trim() || 'example.com'}</span> 这种通配最省事
          </div>
        </label>
      )}

      <label className="block">
        <div className={labelCls}>主域名（可选）</div>
        <input
          value={baseDomain}
          onChange={(e) => setBaseDomain(e.target.value)}
          placeholder="example.com"
          className={`${inputCls} font-mono`}
        />
        <div className="mt-1 text-[11px] leading-relaxed text-slate-400">
          填了之后加站点只写前缀：<span className="font-mono">nas</span> 就是{' '}
          <span className="font-mono">nas.{baseDomain.trim() || 'example.com'}</span>。写全域名的照样认
        </div>
      </label>

      {err && <div className="rounded-xl bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>}
    </Modal>
  )
}

// ===================== 站点 =====================

interface SiteFormState {
  hosts: string[]
  pathPrefix: string
  stripPrefix: boolean
  /** 选中的设备（它的 IP + 下面的端口 = 完整的代理地址） */
  deviceId: number
  backendPort: string
  backend: string
  backendHttps: boolean
  listenPort: string
  https: boolean
  certId: string
  enabled: boolean
  description: string
  accessMode: '' | 'allow' | 'deny'
  accessRules: string
}

function SiteModal({
  initial,
  entry,
  certs,
  prefillDeviceId = 0,
  onCancel,
  onSubmit,
}: {
  initial?: ProxySite
  /** 新建时预选的设备（设备头上的「+ 加服务」带进来） */
  prefillDeviceId?: number
  entry: ProxyEntry
  certs: Certificate[]
  onCancel: () => void
  onSubmit: (payload: api.ProxySitePayload) => Promise<void>
}) {
  const [form, setForm] = useState<SiteFormState>(() => ({
    hosts: initial?.hosts?.length ? [...initial.hosts] : [''],
    pathPrefix: initial?.pathPrefix ?? '',
    stripPrefix: initial?.stripPrefix ?? false,
    deviceId: initial?.deviceId ?? prefillDeviceId,
    backendPort: '',
    backend: initial?.backend ?? '',
    backendHttps: initial?.backendHttps ?? false,
    listenPort: initial?.listenPort ? String(initial.listenPort) : '',
    https: initial?.https ?? false,
    certId: String(initial?.certId ?? 0),
    enabled: initial?.enabled ?? true,
    description: initial?.description ?? '',
    accessMode: initial?.access?.mode ?? '',
    accessRules: (initial?.access?.rules ?? []).join('\n'),
  }))
  const [busy, setBusy] = useState(false)
  const [testing, setTesting] = useState(false)
  const [probe, setProbe] = useState<{ ok: boolean; text: string } | null>(null)
  const [err, setErr] = useState('')

  // 设备列表（服务要挂在某台设备上）+ 扫这台设备得到的端口
  const [devices, setDevices] = useState<Device[]>([])
  // 主域候选（DDNS 解析记录去重 ∪ 入口 baseDomain）—— 域名输入的下拉用。
  // 加载逻辑放在下面 baseDomain 声明之后（它依赖 entry）。
  const [hostZones, setHostZones] = useState<string[]>([])
  const [hostPorts, setHostPorts] = useState<LanPort[]>([])
  const [scanning, setScanning] = useState(false)

  useEffect(() => {
    let alive = true
    api
      .getDevices()
      .then((l) => {
        if (!alive) return
        setDevices(l)
        // 编辑已有服务时：从 backend 的 IP 反查出设备，端口填进框
        const ip = String(initial?.backend ?? '').split(':')[0]
        const port = String(initial?.backend ?? '').split(':')[1] ?? ''
        if (ip) {
          const d = l.find((x) => x.ip === ip)
          if (d) {
            setForm((p) => ({ ...p, deviceId: d.id, backendPort: port }))
          } else if (port) {
            // IP 没匹配到设备：至少把端口填上，让用户自己选一台
            setForm((p) => ({ ...p, backendPort: port }))
          }
        }
      })
      .catch(() => {})
    return () => {
      alive = false
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [initial?.id])


  const baseDomain = entry.baseDomain ?? ''

  // 主域候选：DDNS 解析记录去重 ∪ 入口 baseDomain —— 域名输入的下拉用
  useEffect(() => {
    let alive = true
    api
      .getDdnsZones()
      .then((z) => {
        if (!alive) return
        const set = new Set(z)
        if (baseDomain) set.add(baseDomain)
        setHostZones([...set])
      })
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [baseDomain])
  // 要补的那截主域名，判据和后端 expandHost 一致：没有点才补
  const suffixOf = (h: string) => {
    const t = h.trim()
    return baseDomain && t && !t.includes('.') ? `.${baseDomain}` : ''
  }
  const fullHost = (h: string) => h.trim().toLowerCase() + suffixOf(h)
  const hosts = form.hosts.map((h) => h.trim()).filter(Boolean)

  const setHost = (i: number, v: string) =>
    setForm((p) => ({ ...p, hosts: p.hosts.map((h, k) => (k === i ? v : h)) }))
  /** 选了设备 → 拼出完整代理地址；换设备时提醒一下（这是换到另一台机器） */
  const pickDevice = (id: number) => {
    const d = devices.find((x) => x.id === id)
    if (!d) {
      setForm((p) => ({ ...p, deviceId: 0, backend: '' }))
      return
    }
    const prev = devices.find((x) => x.id === form.deviceId)
    if (prev && prev.id !== d.id) {
      // 换机器是件大事（服务会跑到别处），确认一下，别手滑点错
      if (!window.confirm(`要把这个服务从「${prev.name}」换到「${d.name}」吗？`)) return
    }
    setForm((p) => ({
      ...p,
      deviceId: d.id,
      // 端口没填时补默认 80 —— 不能拼成 "192.168.9.8:"（空冒号会让后端解析不出端口），
      // 也不能只写 IP（后端拿不到端口）。内网服务绝大多数是 80，填了不对用户自己改。
      backend: `${d.ip}:${p.backendPort || DEFAULT_BACKEND_PORT}`,
    }))
    setHostPorts([])
  }

  /** 填端口 → 立刻拼进 backend */
  const setBackendPort = (port: string) => {
    const d = devices.find((x) => x.id === form.deviceId)
    setForm((p) => ({
      ...p,
      backendPort: port,
      backend: d ? `${d.ip}:${port || DEFAULT_BACKEND_PORT}` : p.backend,
    }))
  }

  /** 扫这台设备开了哪些端口（只探这一台，一两秒） */
  const scanThisDevice = async () => {
    const d = devices.find((x) => x.id === form.deviceId)
    if (!d) return
    setScanning(true)
    setErr('')
    try {
      const ports = await api.scanHostPorts(d.ip)
      setHostPorts(ports)
      if (ports.length === 0) {
        setErr(`没探到 ${d.name}（${d.ip}）上开放的端口。机器开着吗？`)
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : '扫描失败')
    } finally {
      setScanning(false)
    }
  }

  const addHost = () => setForm((p) => ({ ...p, hosts: [...p.hosts, ''] }))
  const dropHost = (i: number) => setForm((p) => ({ ...p, hosts: p.hosts.filter((_, k) => k !== i) }))

  const customPort = Number(form.listenPort) || 0
  // 挂默认入口时，选 HTTPS 只是「额外挂到 HTTPS 入口」，HTTP 入口上一直都在
  const shownPort = customPort || (form.https && entry.httpsPort > 0 ? entry.httpsPort : entry.httpPort)
  const shownHttps = customPort ? form.https : form.https && entry.httpsPort > 0
  // 挂默认入口，但那个入口没开——这条配了也不会有人听
  const orphan = !customPort && shownPort === 0
  // 占了端口又不填域名 = 当端口转发使：这个端口上来的请求全收
  const portForward = hosts.length === 0 && customPort > 0
  // 没域名 + 自动匹配 + 挑不出兜底证书 = 这个端口上的 HTTPS 一次都握不成
  const noCertWithoutDomain =
    form.https && hosts.length === 0 && Number(form.certId) === 0 && autoCertNeedsSNI(certs)

  // 端口这一格到底会落在哪，一句话说清。用户最容易误解的就是
  // 「留空」和「选了 HTTPS 却在 80 上也开着」这两件事。
  const portHint = customPort
    ? `这个服务单独占 ${customPort}，只有它在上面`
    : entry.httpPort > 0 && form.https && entry.httpsPort > 0
      ? `留空 = 跟默认入口走，:${entry.httpPort} 和 :${entry.httpsPort} 上都能访问`
      : shownPort > 0
        ? `留空 = 跟默认入口走，落在 :${shownPort}`
        : '留空 = 跟默认入口走'

  const payload = (): api.ProxySitePayload => ({
    // deviceId：让这条站点**按 id 归到设备**，改设备 IP 时归属不会丢。
    // 后端在没传时会按 backend 的 IP 反查兜底，但前端有准的就直接给。
    deviceId: form.deviceId,
    hosts: hosts.map(fullHost),
    pathPrefix: form.pathPrefix.trim(),
    stripPrefix: form.stripPrefix,
    backend: form.backend.trim(),
    backendHttps: form.backendHttps,
    listenPort: customPort,
    https: form.https,
    certId: form.https ? Number(form.certId) || 0 : 0,
    enabled: form.enabled,
    description: form.description.trim(),
    // 一行一条规则；空行和多余空格丢掉，后端只认干净的 IP / CIDR
    access: {
      mode: form.accessMode,
      rules: form.accessRules
        .split('\n')
        .map((s) => s.trim())
        .filter(Boolean),
    },
  })

  const runTest = async () => {
    setTesting(true)
    setProbe(null)
    try {
      const r = await api.testProxySite(payload())
      setProbe({ ok: r.reachable && !r.mismatch, text: r.message })
      // 探到的协议和勾选不符就顺手改掉，省得用户自己回去找那个勾
      if (r.reachable && r.mismatch) setForm((p) => ({ ...p, backendHttps: r.scheme === 'https' }))
    } catch (e) {
      setProbe({ ok: false, text: errText(e) })
    } finally {
      setTesting(false)
    }
  }

  const submit = async () => {
    setErr('')
    setBusy(true)
    try {
      await onSubmit(payload())
    } catch (e) {
      setErr(errText(e))
      setBusy(false)
    }
  }

  return (
    <>
      <Modal
        title={initial ? '编辑服务' : '添加服务'}
      onCancel={onCancel}
      footer={
        <>
          <button
            type="button"
            onClick={runTest}
            disabled={testing || !form.backend.trim()}
            className="mr-auto flex items-center gap-1.5 rounded-xl px-3 py-2 text-sm text-slate-500 transition hover:bg-slate-100 disabled:opacity-40"
          >
            <Zap className={`h-3.5 w-3.5 ${testing ? 'animate-pulse' : ''}`} />
            {testing ? '测试中...' : '测试后端'}
          </button>
          <button
            type="button"
            onClick={onCancel}
            className="rounded-xl px-4 py-2 text-sm text-slate-500 transition hover:bg-slate-100"
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
        </>
      }
    >
      {/* 这条服务在干什么，一眼看完 */}
      <div className="flex items-center gap-2 rounded-xl bg-slate-50 px-3 py-2.5 text-xs ring-1 ring-slate-200/70">
        <Globe className="h-3.5 w-3.5 shrink-0 text-blue-500" />
        <span className="truncate font-mono text-slate-700">
          {shownHttps ? 'https://' : 'http://'}
          {hosts.length > 0 ? fullHost(hosts[0]) : portForward ? '任意域名' : '域名'}
          {shownPort > 0 && <span className="text-slate-400">{portText(shownHttps, shownPort)}</span>}
          {hosts.length > 1 && <span className="text-slate-400"> 等 {hosts.length} 个域名</span>}
        </span>
        <ArrowRight className="h-3.5 w-3.5 shrink-0 text-slate-300" />
        <HardDrive className="h-3.5 w-3.5 shrink-0 text-emerald-500" />
        <span className="truncate font-mono text-slate-700">
          {form.backendHttps ? 'https://' : 'http://'}
          {form.backend.trim() || '内网服务'}
        </span>
      </div>

      {/* ==== 这台服务跑在哪 + 叫什么 ====
          顺序和内网穿透的添加表单一致：先选设备，再填名称/端口，
          最后才是对外怎么访问（域名、HTTPS、证书）。
          "代理地址"其实就是 设备 + 端口，所以并进这一块。 */}
      {/* ==== 后端这一侧 ==== */}
      <div className="!mt-5 border-t border-slate-100 pt-4">
        <div className={labelCls}>设备</div>
        <div className="flex flex-wrap gap-2">
          {/* 设备：只选，不手填 IP */}
          <select
            value={form.deviceId || ''}
            onChange={(e) => pickDevice(Number(e.target.value) || 0)}
            className={`${fieldCls} min-w-0 flex-1`}
          >
            <option value="">选择设备…</option>
            {devices.map((d) => (
              <option key={d.id} value={d.id}>
                {d.name}（{d.ip}）
              </option>
            ))}
          </select>

          {/* 扫**这台设备**上的端口（不是全局扫描） */}
          <button
            type="button"
            onClick={scanThisDevice}
            disabled={!form.deviceId || scanning}
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
                onClick={() => setBackendPort(String(p.port))}
                className="rounded bg-white px-2 py-1 text-[11px] font-semibold text-slate-600 ring-1 ring-slate-200 transition hover:bg-blue-50 hover:text-blue-600 hover:ring-blue-300"
              >
                {p.port}
                {p.name && <span className="ml-1 opacity-70">{p.name}</span>}
              </button>
            ))}
          </div>
        )}

        <div className="mt-1 text-[11px] leading-relaxed text-slate-400">
          选设备 + 填端口就好，不用记 IP。机器没在列表里就去「设备」页加一台。
        </div>
      </div>


      {/* 服务名称 + 后端端口：和内网穿透表单同一个两列排法 */}
      <div className="grid grid-cols-2 gap-3">
        <label className="col-span-2 block sm:col-span-1">
          {/* 这个字段就是"服务名称"：卡片、仪表盘都优先显示它（穿透的 name 对应物） */}
          <div className={labelCls}>服务名称</div>
          <input
            value={form.description}
            onChange={(e) => setForm((p) => ({ ...p, description: e.target.value }))}
            placeholder="如 阿里云盘"
            className={inputCls}
          />
        </label>
        <label className="col-span-2 block sm:col-span-1">
          <div className={labelCls}>内网端口</div>
          <input
            value={form.backendPort}
            onChange={(e) => setBackendPort(e.target.value)}
            placeholder="8080"
            inputMode="numeric"
            className={`${inputCls} font-mono`}
          />
        </label>
      </div>

      {/* ==== 对外这一侧：域名 + 端口 + 协议 + 证书 ==== */}
      <div>
        <div className={labelCls}>域名{customPort > 0 && <span className="font-normal">（可选）</span>}</div>
        <div className="space-y-2">
          {form.hosts.map((h, i) => (
            <div key={i} className="flex items-center gap-2">
              {/* 子域手填 + 主域从 DDNS 选（和穿透同一个控件）；主域候选里
                  也带上入口的 baseDomain —— 它本来就是补全用的主域 */}
              <DomainInput
                value={h}
                zones={hostZones}
                onChange={(v) => setHost(i, v)}
                placeholder={baseDomain ? 'nas' : 'nas.example.com'}
                className="flex-1"
              />
              {form.hosts.length > 1 && (
                <button
                  type="button"
                  onClick={() => dropHost(i)}
                  className="grid h-8 w-8 shrink-0 place-items-center rounded-lg text-slate-400 transition hover:bg-rose-50 hover:text-rose-500"
                >
                  <X className="h-4 w-4" />
                </button>
              )}
            </div>
          ))}
        </div>
        <button
          type="button"
          onClick={addHost}
          className="mt-2 flex items-center gap-1 rounded-lg px-2 py-1 text-[11px] text-blue-500 transition hover:bg-blue-50"
        >
          <Plus className="h-3 w-3" />
          添加域名
        </button>

        <div className="mt-1 text-[11px] leading-relaxed text-slate-400">
          {portForward ? (
            form.https ? (
              <>
                不填域名 = 这个端口上来的请求全收，等于 nginx 里不写{' '}
                <span className="font-mono">server_name</span> 的 server 块。
                但别指望敲 IP 能进——HTTPS 要先握手，敲 IP 不发 SNI，证书也对不上 IP，得用域名访问
              </>
            ) : (
              <>
                不填域名 = 这个端口上来的请求全收，敲 IP 也能进，就是个端口转发。
                等于 nginx 里不写 <span className="font-mono">server_name</span> 的 server 块
              </>
            )
          ) : (
            <>
              几个域名指向同一个后端就写几行，等于 nginx 的{' '}
              <span className="font-mono">server_name a.com www.a.com</span>
            </>
          )}
        </div>
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <label className="block">
          <div className={labelCls}>端口</div>
          <input
            value={form.listenPort}
            onChange={(e) => setForm((p) => ({ ...p, listenPort: e.target.value }))}
            placeholder={shownPort > 0 ? String(shownPort) : '端口'}
            className={`${inputCls} font-mono`}
          />
        </label>
      </div>
      {/* 协议不再用下拉选：和内网穿透同一个样子 —— 一个「加 HTTPS 证书」开关。
          勾上就按域名配证书（自动匹配即可），不勾就是明文 HTTP。 */}
      <label className="flex cursor-pointer items-start gap-2 rounded-xl bg-white px-3 py-2.5 ring-1 ring-slate-200">
        <input
          type="checkbox"
          checked={form.https}
          onChange={(e) => setForm((p) => ({ ...p, https: e.target.checked }))}
          className="mt-0.5 h-3.5 w-3.5"
        />
        <span>
          <span className="block text-xs font-semibold text-slate-700">HTTPS（加证书）</span>
          <span className="block text-[11px] text-slate-400">
            勾了按域名配证书对外走 HTTPS；不勾就是明文 HTTP
          </span>
        </span>
      </label>
      <div className="!mt-1.5 text-[11px] leading-relaxed text-slate-400">{portHint}</div>

      {portBlocked(customPort) && <BlockedPortWarn label="端口" port={customPort} />}

      {form.https && (
        <label className="block">
          <div className={labelCls}>证书</div>
          <CertSelect value={form.certId} certs={certs} onChange={(v) => setForm((p) => ({ ...p, certId: v }))} />
          <div className="mt-1 text-[11px] leading-relaxed text-slate-400">自动匹配就是按访问的域名挑，一般不用动</div>
        </label>
      )}

      {noCertWithoutDomain && (
        <Warn>
          没填域名，证书又是「自动匹配」——自动匹配是按访问用的域名挑证书的，而这条服务没有域名可挑：
          现在还没有默认证书，可用证书也不止一张，握手会直接失败，浏览器只报{' '}
          <span className="font-mono">ERR_SSL_PROTOCOL_ERROR</span>。
          上面挑一张具体证书，或者去「证书」页把一张设为默认。
          一张证书都不想管的话，「证书」页可以现签一张自签的——浏览器会拦一道警告，但 HTTPS 能通
        </Warn>
      )}

      {orphan && (
        <Warn>默认入口一个都没开，这条服务存下来也没人监听。去「默认入口」填个端口，或者在上面给它单独填一个</Warn>
      )}

      {probe && (
        <div
          className={`rounded-xl px-3 py-2 text-[11px] leading-relaxed ring-1 ${
            probe.ok ? 'bg-emerald-50 text-emerald-700 ring-emerald-200' : 'bg-amber-50 text-amber-700 ring-amber-200'
          }`}
        >
          {probe.text}
        </div>
      )}

      <label className="!mt-5 block border-t border-slate-100 pt-4">
        <div className={labelCls}>路径（可选）</div>
        <input
          value={form.pathPrefix}
          onChange={(e) => setForm((p) => ({ ...p, pathPrefix: e.target.value }))}
          placeholder="留空 = 整站"
          className={`${inputCls} font-mono`}
        />
      </label>

      {!!form.pathPrefix.trim() && (
        <>
          <label className="flex cursor-pointer items-center gap-2 rounded-xl bg-slate-50 px-3 py-2.5 text-xs text-slate-600">
            <input
              type="checkbox"
              checked={form.stripPrefix}
              onChange={(e) => setForm((p) => ({ ...p, stripPrefix: e.target.checked }))}
              className="h-3.5 w-3.5"
            />
            转发前剥掉这个前缀
          </label>
          {form.stripPrefix && (
            <div className="flex gap-2 rounded-xl bg-amber-50 px-3 py-2.5 text-[11px] leading-relaxed text-amber-700 ring-1 ring-amber-200">
              <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <span>后端得自己支持 base path，否则页面里的资源会 404。不确定就别勾，给它单独一个域名</span>
            </div>
          )}
        </>
      )}


      {/* ===================== 访问控制 =====================
           用共用组件（components/AccessControlEditor）—— 和穿透同一套模板，
           并且能从访问审计里抓到的 IP 一键加入名单（IPv4 / IPv6 都支持）。 */}
      <AccessControlEditor
        mode={form.accessMode}
        rules={form.accessRules}
        onChange={(next) =>
          setForm((p) => ({ ...p, accessMode: next.mode, accessRules: next.rules }))
        }
      />

      {err && <div className="rounded-xl bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>}
    </Modal>

    </>
  )
}


// ===================== 监听端口一览 =====================

/** 一个端口一块，对着 netstat 看到的那几行 */
function ListenerChip({ l, certName }: { l: ProxyListener; certName: string }) {
  return (
    <span
      title={l.tls ? `证书：${certName}` : undefined}
      className={`inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-[11px] ring-1 ${
        l.running ? 'bg-white text-slate-600 ring-slate-200' : 'bg-rose-50 text-rose-600 ring-rose-200'
      }`}
    >
      <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${l.running ? 'bg-emerald-500' : 'bg-rose-500'}`} />
      <span className="font-mono font-semibold">:{l.port}</span>
      {l.tls ? (
        <span className="inline-flex items-center gap-0.5 font-semibold text-violet-600">
          <ShieldCheck className="h-3 w-3" />
          HTTPS
        </span>
      ) : (
        <span className="text-slate-400">HTTP</span>
      )}
      <span className="text-slate-400">{l.siteCount} 站点</span>
    </span>
  )
}

// ===================== 主页面 =====================

export function ReverseProxy() {
  const [cfg, setCfg] = useState<ProxyConfig | null>(null)
  const [certs, setCerts] = useState<Certificate[]>([])
  const [loadErr, setLoadErr] = useState('')
  const [testingId, setTestingId] = useState<number | null>(null)
  // 访问日志弹窗：点站点卡片上的「日志」打开，看"谁访问过这个服务"
  const [logSite, setLogSite] = useState<ProxySite | null>(null)
  const { toasts, show: toast } = useToast()

  const [entryModal, setEntryModal] = useState(false)
  const [siteModal, setSiteModal] = useState<{
    open: boolean
    initial?: ProxySite
    /** 设备头「+ 加服务」预选的设备（只对新建生效） */
    preDeviceId?: number
  }>({ open: false })

  const refresh = useCallback(async () => {
    try {
      setCfg(await api.getProxyConfig())
      setLoadErr('')
    } catch (e) {
      setLoadErr(errText(e))
    }
  }, [])

  useEffect(() => {
    refresh()
    const t = window.setInterval(refresh, 10000)
    return () => window.clearInterval(t)
  }, [refresh])

  // 证书只有弹窗用得上，拿不到不该拦住整页
  useEffect(() => {
    let alive = true
    api
      .getCertList()
      .then((l) => alive && setCerts(l))
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [])

  const entry = cfg?.entry
  const listeners = useMemo(() => cfg?.listeners ?? [], [cfg])
  const sites = useMemo(() => cfg?.sites ?? [], [cfg])

  // 设备是公共的一份（穿透/反代/端口转发共用），站点按后端 IP 归属到设备
  const [devices, setDevices] = useState<Device[]>([])
  useEffect(() => {
    let alive = true
    api
      .getDevices()
      .then((l) => alive && setDevices(l))
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [cfg])

  // 拖动排序：设备之间、设备内的服务之间都能拖
  const [dragDev, setDragDev] = useState(0)
  const [overDev, setOverDev] = useState(0)
  const [dragSite, setDragSite] = useState(0)
  const [overSite, setOverSite] = useState(0)

  /** 设备之间换顺序 */
  const commitDeviceOrder = async (from: number, to: number) => {
    const ids = devices.map((d) => d.id)
    const i = ids.indexOf(from)
    const j = ids.indexOf(to)
    if (i < 0 || j < 0 || i === j) return
    const [moved] = ids.splice(i, 1)
    ids.splice(j, 0, moved)
    // 先本地改，界面立刻跟上
    setDevices(devices.slice().sort((a, b) => ids.indexOf(a.id) - ids.indexOf(b.id)))
    try {
      await api.reorderDevices(ids)
      // 再拉一次：设备顺序是公共的（穿透页也用），以服务端为准
      await refresh()
    } catch (e) {
      toast('排序保存失败: ' + errText(e))
      void refresh()
    }
  }

  /** 站点（服务）之间换顺序 */
  const commitSiteOrder = async (from: number, to: number) => {
    const ids = sites.map((s2) => s2.id)
    const i = ids.indexOf(from)
    const j = ids.indexOf(to)
    if (i < 0 || j < 0 || i === j) return
    const [moved] = ids.splice(i, 1)
    ids.splice(j, 0, moved)
    try {
      await api.reorderSites(ids)
      await refresh()
    } catch (e) {
      toast('排序保存失败: ' + errText(e))
    }
  }

  /**
   * 站点按设备分组。
   *
   * 归属是**按后端地址的 IP 推导**的，不是存了个 deviceId ——
   * 这样改了 backend 的 IP 就自动换设备，不会出现两边不一致。
   */
  const groups = useMemo(() => {
    type Group = { device?: Device; sites: typeof sites }
    const byDev = new Map<number, Group>()
    const rest: Group = { sites: [] }

    for (const s of sites) {
      // 按 deviceId 找设备（改了 IP 也还是同一台），找不到再按 backend 里的 IP 兜底
      const d = api.deviceOf(s, s.backend, devices)
      if (!d) {
        rest.sites.push(s)
        continue
      }
      const dev = devices.find((x) => x.id === d.id) ?? d
      let g = byDev.get(dev.id)
      if (!g) {
        g = { device: dev, sites: [] }
        byDev.set(dev.id, g)
      }
      g.sites.push(s)
    }

    // **分组顺序要跟着设备表走**，不能按站点在数组里出现的先后 ——
    // 后者是「站点在配置里的先后」，跟设备拖动毫无关系：设备顺序改了（sortOrder 已存盘），
    // 界面却还是老样子，表现就是「拖动没反应」。
    // devices 来自 device.List()，后端已按 sortOrder 排好，直接照它的次序输出。
    const devOrder = devices.map((d) => d.id)
    const out = [...byDev.values()].sort((a, b) => {
      const ia = devOrder.indexOf(a.device?.id ?? -1)
      const ib = devOrder.indexOf(b.device?.id ?? -1)
      return (ia < 0 ? devOrder.length : ia) - (ib < 0 ? devOrder.length : ib)
    })
    // 没归到设备的放在最后，并且只在真有的时候才显示
    if (rest.sites.length > 0) out.push(rest)
    return out
  }, [sites, devices])
  const certName = useCallback(
    (id: number) => (id === 0 ? '按域名自动匹配' : (certs.find((c) => c.id === id)?.name ?? `#${id}`)),
    [certs],
  )

  const saveEntry = async (payload: Omit<api.EntryPayload, 'enabled'>) => {
    // 改设置不该顺手把停掉的反代拉起来
    await api.saveProxyEntry({ ...payload, enabled: entry?.enabled ?? true })
    setEntryModal(false)
    toast('已保存')
    await refresh()
  }

  const toggleRunning = async () => {
    if (!entry) return
    try {
      await api.saveProxyEntry({
        enabled: !entry.enabled,
        httpPort: entry.httpPort,
        httpsPort: entry.httpsPort,
        certId: entry.certId,
        baseDomain: entry.baseDomain ?? '',
      })
      toast(entry.enabled ? '已停止' : '已启动')
    } catch (e) {
      toast(errText(e))
    }
    await refresh()
  }

  const toggleAccessLog = async () => {
    if (!entry) return
    try {
      await api.setProxyAccessLog(!entry.accessLog)
      await refresh()
    } catch (e) {
      toast(errText(e))
    }
  }

  const submitSite = async (payload: api.ProxySitePayload) => {
    if (siteModal.initial) {
      await api.updateProxySite({ ...payload, id: siteModal.initial.id })
      toast('已更新')
    } else {
      await api.addProxySite(payload)
      toast('已添加')
    }
    setSiteModal({ open: false })
    await refresh()
  }

  // 切换中锁定按钮：后端改完要重开监听，要好几秒。
  // 这几秒按钮上什么都不变的话用户以为没点着，再点一下又切回去了
  const [flagBusy, setFlagBusy] = useState(0)

  /**
   * 只剩「启停」一个开关了。
   * 以前还有「添加到主页」（把站点同步到导航页），导航页砍掉后没了。
   */
  const toggleSiteFlag = async (s: ProxySite, flag: 'enabled') => {
    const next = !s.enabled
    setFlagBusy(s.id)
    try {
      await api.setProxySiteFlag(s.id, flag, next)
      toast(next ? '已启用' : '已停用')
      await refresh()
    } catch (e) {
      toast('操作失败: ' + errText(e))
    } finally {
      setFlagBusy(0)
    }
  }

  /**
   * 照着这条服务再开一个。
   *
   * ID 清零就是为了"新增"而不是"覆盖" —— 和穿透页的复制服务一个道理。
   * 域名要换（同一个域名不能挂两条），所以清掉让用户重填；
   * 其余（证书、门禁、后端、HTTPS 设置）都保留，省得再勾一遍。
   */

  const removeSite = async (s: ProxySite) => {
    if (!window.confirm(`确认删除服务 ${s.hosts?.[0] ?? `:${s.listenPort}`}？`)) return
    try {
      await api.deleteProxySite(s.id)
      toast('已删除')
      await refresh()
    } catch (e) {
      toast(errText(e))
    }
  }

  const testSite = async (s: ProxySite) => {
    setTestingId(s.id)
    try {
      const probe = await api.testProxySite({
        deviceId: s.deviceId ?? 0,
        hosts: s.hosts ?? [],
        pathPrefix: s.pathPrefix,
        stripPrefix: s.stripPrefix,
        backend: s.backend,
        backendHttps: s.backendHttps,
        listenPort: s.listenPort,
        https: s.https,
        certId: s.certId,
        enabled: s.enabled,
        description: s.description,
        access: s.access ?? { mode: '', rules: [] },
      })
      toast(`${s.backend}：${probe.message}`)
    } catch (e) {
      toast('测试失败：' + errText(e))
    } finally {
      setTestingId(null)
    }
  }

  if (loadErr && !cfg) {
    return (
      <Card>
        <div className="py-12 text-center text-rose-500">
          <AlertCircle className="mx-auto h-8 w-8" />
          <div className="mt-2 text-sm font-semibold">数据加载失败</div>
          <div className="mt-1 text-xs text-slate-500">{loadErr}</div>
          <button onClick={refresh} className="mt-4 rounded-xl bg-blue-500 px-4 py-2 text-xs font-semibold text-white">
            重试
          </button>
        </div>
      </Card>
    )
  }

  if (!cfg || !entry) {
    return (
      <Card>
        <div className="py-12 text-center text-sm text-slate-400">正在加载...</div>
      </Card>
    )
  }

  const example = entry.baseDomain ? `nas.${entry.baseDomain}` : 'nas.example.com'
  const entryPort = entry.httpPort || entry.httpsPort
  const failed = listeners.filter((l) => !l.running)

  // 状态一句话：停了 / 没端口可听 / 全起来了 / 有几个没起来
  const stateText = !entry.enabled
    ? '已停止'
    : listeners.length === 0
      ? '没有监听'
      : failed.length === 0
        ? '运行中'
        : `${failed.length} 个端口没起来`
  const stateDot = !entry.enabled
    ? 'bg-slate-300'
    : listeners.length === 0
      ? 'bg-slate-300'
      : failed.length === 0
        ? 'bg-emerald-500'
        : 'bg-amber-400'

  return (
    <div className="space-y-4">
      <div>
        <div className="text-base font-bold text-slate-800">反向代理</div>
        <div className="mt-1 text-xs text-slate-500">
          在本机监听端口，按域名把请求转给内网的机器。就是 nginx 干的活
        </div>
      </div>

      {/* 监听状态：哪些端口开着、各挂了几个服务 */}
      <Card className="!py-3.5">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <span className={`h-2 w-2 shrink-0 rounded-full ${stateDot}`} />
          <span className="text-sm font-semibold text-slate-700">{stateText}</span>

          {entry.baseDomain && <span className="font-mono text-[11px] text-slate-400">*.{entry.baseDomain}</span>}

          <button
            onClick={toggleAccessLog}
            title="点一下开关访问日志"
            className={`rounded-md px-1.5 py-0.5 text-[11px] font-semibold transition ${
              entry.accessLog
                ? 'bg-slate-200 text-slate-600 hover:bg-slate-300'
                : 'text-slate-300 hover:bg-slate-100 hover:text-slate-500'
            }`}
          >
            访问日志
          </button>

          <div className="ml-auto flex items-center gap-1">
            <button
              onClick={toggleRunning}
              className="rounded-lg px-2.5 py-1 text-xs font-semibold text-slate-500 hover:bg-slate-100"
            >
              {entry.enabled ? '停止' : '启动'}
            </button>
            <button
              onClick={() => setEntryModal(true)}
              className="flex items-center gap-1 rounded-lg px-2.5 py-1 text-xs font-semibold text-slate-500 hover:bg-slate-100"
            >
              <Settings2 className="h-3 w-3" /> 默认入口
            </button>
          </div>
        </div>

        {listeners.length > 0 && (
          <div className="mt-2.5 flex flex-wrap gap-1.5">
            {listeners.map((l) => (
              <ListenerChip key={l.port} l={l} certName={certName(l.certId)} />
            ))}
          </div>
        )}

        {failed.map(
          (l) =>
            l.lastError && (
              <div
                key={l.port}
                className="mt-2 rounded-xl bg-rose-50 px-3 py-2 text-[11px] leading-relaxed text-rose-600"
              >
                <span className="font-mono font-semibold">:{l.port}</span> {l.lastError}
              </div>
            ),
        )}

        {entry.enabled && listeners.length === 0 && sites.length > 0 && (
          <div className="mt-2 text-[11px] text-slate-400">
            站点都停用了，或者默认入口两个端口都没填——点右上角「默认入口」看看
          </div>
        )}

        {listeners.length > 0 && failed.length === 0 && (
          <div className="mt-2 text-[11px] text-slate-400">
            要让外网也能访问：到 STUN 页给这些端口各加一条 TCP 服务
          </div>
        )}
      </Card>

      {/* 服务列表：**按设备分组** —— 和穿透页的「设备列表」同一套布局。
          站点靠后端地址的 IP 自动归属到设备，不用手动挂。 */}
      <Card className="!p-0">
        <div className="px-5 pt-4">
          <CardHeader
            title={
              <span>
                服务 <span className="ml-1 font-normal text-slate-400">{sites.length}</span>
              </span>
            }
            action={
              <button
                onClick={() => setSiteModal({ open: true })}
                className="flex items-center gap-1 rounded-md bg-blue-500 px-2.5 py-1 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 hover:bg-blue-600"
              >
                <Plus className="h-3 w-3" /> 添加服务
              </button>
            }
          />
        </div>

        {sites.length === 0 ? (
          // 空状态直接把这东西怎么工作画出来，比写一段话管用
          <div className="px-5 pb-10 pt-4">
            <div className="mx-auto flex max-w-xl flex-col items-center">
              <div className="flex w-full items-center justify-center gap-2 sm:gap-4">
                <div className="flex w-28 flex-col items-center gap-1.5">
                  <div className="grid h-11 w-11 place-items-center rounded-2xl bg-blue-50 text-blue-500">
                    <Globe className="h-5 w-5" />
                  </div>
                  <div className="break-all text-center font-mono text-[11px] text-slate-500">{example}</div>
                  <div className="text-[10px] text-slate-400">浏览器</div>
                </div>

                <ArrowRight className="h-4 w-4 shrink-0 text-slate-300" />

                <div className="flex w-28 flex-col items-center gap-1.5">
                  <div className="grid h-11 w-11 place-items-center rounded-2xl bg-slate-900 text-[10px] font-bold text-white">
                    LS
                  </div>
                  <div className="font-mono text-[11px] text-slate-500">{entryPort ? `:${entryPort}` : '未设端口'}</div>
                  <div className="text-[10px] text-slate-400">Catflap</div>
                </div>

                <ArrowRight className="h-4 w-4 shrink-0 text-slate-300" />

                <div className="flex w-28 flex-col items-center gap-1.5">
                  <div className="grid h-11 w-11 place-items-center rounded-2xl bg-emerald-50 text-emerald-500">
                    <HardDrive className="h-5 w-5" />
                  </div>
                  <div className="break-all text-center font-mono text-[11px] text-slate-500">192.168.1.20:5000</div>
                  <div className="text-[10px] text-slate-400">内网服务</div>
                </div>
              </div>

              <button
                onClick={() => setSiteModal({ open: true })}
                className="mt-7 rounded-xl bg-blue-500 px-4 py-2 text-xs font-semibold text-white shadow-md shadow-blue-500/20 hover:bg-blue-600"
              >
                添加第一个服务
              </button>
              <div className="mt-2.5 text-[11px] text-slate-400">
                一个服务一个域名，域名解析到本机就行。要单独的端口、单独的证书也行
              </div>
            </div>
          </div>
        ) : (
          /*
           * 服务列表用**卡片**，和穿透页的服务卡片同一套布局。
           *
           * 为什么不用表格：两边都是"一堆可开关的入口"，操作方式应该一致 ——
           * 用户在一个页面学会的操作，到另一个页面就该能直接用。
           * 表格在小屏上还会把中文压成一列一个字，卡片不会。
           */
          <div className="space-y-3 px-5 py-4">
            {groups.map((g, gi) => (
              <div
                key={g.device?.id ?? `rest-${gi}`}
                draggable={!!g.device}
                onDragStart={() => g.device && setDragDev(g.device.id)}
                onDragEnd={() => {
                  setDragDev(0)
                  setOverDev(0)
                }}
                onDragOver={(e) => {
                  e.preventDefault()
                  if (g.device && g.device.id !== overDev) setOverDev(g.device.id)
                }}
                onDrop={(e) => {
                  e.preventDefault()
                  if (dragDev && g.device) void commitDeviceOrder(dragDev, g.device.id)
                }}
                className={`rounded-xl border border-slate-200/70 bg-white transition ${
                  g.device && overDev === g.device.id && dragDev !== g.device.id
                    ? 'ring-2 ring-blue-300'
                    : ''
                } ${g.device && dragDev === g.device.id ? 'opacity-40' : ''}`}
              >
                {/* 设备头：和穿透页的设备卡片同一套样式 */}
                <div className="flex items-center gap-2 border-b border-slate-100 px-3 py-2">
                  <span className="grid h-7 w-7 shrink-0 place-items-center rounded-lg bg-blue-50 text-blue-500">
                    <HardDrive className="h-3.5 w-3.5" />
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-[13px] font-bold text-slate-800">
                      {g.device?.name ?? '未归到设备'}
                    </p>
                    <p className="font-mono text-[10px] text-slate-400">
                      {g.device?.ip ?? '这台机器还没加进设备列表'}
                    </p>
                  </div>
                  <span className="shrink-0 text-[11px] text-slate-400">
                    {g.sites.length} 个服务
                  </span>
                  {/* 和穿透页设备头同一个按钮：在这台设备下直接加服务，表单里预选好设备 */}
                  {g.device && (
                    <button
                      onClick={() =>
                        setSiteModal({ open: true, preDeviceId: g.device!.id })
                      }
                      className="flex shrink-0 items-center gap-1 rounded-md px-2 py-1 text-[11px] font-semibold text-blue-600 ring-1 ring-blue-200 transition hover:bg-blue-50"
                      title={`在 ${g.device.name} 上添加服务`}
                    >
                      <Plus className="h-3 w-3" /> 加服务
                    </button>
                  )}
                </div>

                <div className="grid grid-cols-[repeat(auto-fill,minmax(min(16rem,100%),1fr))] gap-2.5 p-2.5">
                  {g.sites.map((s) => (
                    <div
                      key={s.id}
                      draggable
                      onDragStart={() => setDragSite(s.id)}
                      onDragEnd={() => {
                        setDragSite(0)
                        setOverSite(0)
                      }}
                      onDragOver={(e) => {
                        e.preventDefault()
                        if (s.id !== overSite) setOverSite(s.id)
                      }}
                      onDrop={(e) => {
                        e.preventDefault()
                        if (dragSite) void commitSiteOrder(dragSite, s.id)
                      }}
                      className={`cursor-grab transition ${
                        overSite === s.id && dragSite !== s.id ? 'ring-2 ring-blue-300' : ''
                      } ${dragSite === s.id ? 'opacity-40' : ''}`}
                    >
                    <SiteCard
                      key={s.id}
                      site={s}
                      onLogs={() => setLogSite(s)}
                      entry={entry}
                      testingId={testingId ?? 0}
                      flagBusy={flagBusy}
                      onTest={() => testSite(s)}
                      onToggleFlag={(flag) => toggleSiteFlag(s, flag)}
                      onEdit={() => setSiteModal({ open: true, initial: s })}
                      onRemove={() => removeSite(s)}
                    />
                    </div>
                  ))}
                </div>
              </div>
            ))}
          </div>
        )}
      </Card>

      {entryModal && (
        <EntryModal entry={entry} certs={certs} onCancel={() => setEntryModal(false)} onSubmit={saveEntry} />
      )}
      {logSite && (
        <AccessLogModal
          title={logSite.description || logSite.hosts?.[0] || `站点 #${logSite.id}`}
          siteId={logSite.id}
          onClose={() => setLogSite(null)}
        />
      )}

      {siteModal.open && (
        <SiteModal
          initial={siteModal.initial}
          prefillDeviceId={siteModal.preDeviceId ?? 0}
          entry={entry}
          certs={certs}
          onCancel={() => setSiteModal({ open: false })}
          onSubmit={submitSite}
        />
      )}

      <div className="pointer-events-none fixed bottom-6 left-1/2 z-[60] flex -translate-x-1/2 flex-col items-center gap-2">
        {toasts.map((t) => (
          <div key={t.id} className="rounded-lg bg-slate-900/85 px-4 py-2 text-sm text-white shadow-lg">
            {t.text}
          </div>
        ))}
      </div>
    </div>
  )
}

/**
 * 一张站点卡片（服务卡片）。
 *
 * 抽成组件是因为外面要按设备分组包一层 —— 卡片本身不变。
 * 字段上与穿透的服务卡片对齐：状态点 + 名字 + 按钮组 + 徽章 + 后端。
 */
function SiteCard({
  site: s,
  entry,
  testingId,
  flagBusy,
  onTest,
  onToggleFlag,
  onEdit,
  onRemove,
  onLogs,
}: {
  site: ProxySite
  entry?: ProxyEntry
  testingId: number
  flagBusy: number
  onTest: () => void
  onToggleFlag: (flag: 'enabled') => void
  onEdit: () => void
  onRemove: () => void
  /** 打开这个站点的访问日志 */
  onLogs: () => void
}) {
  const url = siteURL(entry, s)
  const addr = siteAddr(entry, s)
  // 停用的站点本来就不该跑，别拿红色状态去吓人
  const off = s.enabled === false

  // **和穿透/转发共用同一张卡**（components/ServiceCard）。
  // 反代特有的两样通过 props 传：名字用域名（可点开），第三行放后端端口 + 连通性测试。
  return (
    <ServiceCard
      // **服务名称优先**：填了名称显示名称（和穿透一致，仪表盘也用它），
      // 没填才退回域名 —— 大屏上"设备·名称"比一长串域名好认得多
      name={s.description || s.hosts?.[0] || '任意域名'}
      state={off ? 'off' : 'running'}
      stateHint={off ? '已停用' : '在跑'}
      draggable={false}
      icons={
        <>
          <button
            onClick={() => onToggleFlag('enabled')}
            disabled={flagBusy === s.id}
            title={off ? '点一下启用' : '点一下停用'}
            className={`${iconBtn} disabled:opacity-40 ${
              off ? 'bg-slate-200 text-slate-500 hover:bg-slate-300' : 'bg-emerald-50 text-emerald-600 hover:bg-emerald-100'
            }`}
          >
            <Power className="h-3 w-3" />
          </button>
          <button onClick={onEdit} className={`${iconBtn} bg-slate-100 text-slate-600 hover:bg-slate-200`} title="编辑">
            <Pencil className="h-3 w-3" />
          </button>
          {/* 访问日志：谁访问过这个站点。数据来自访问审计，和穿透同一个弹窗 */}
          <button onClick={onLogs} className={`${iconBtn} bg-slate-100 text-slate-600 hover:bg-slate-200`} title="访问日志（谁访问过它）">
            <FileText className="h-3 w-3" />
          </button>
          <button onClick={onRemove} className={`${iconBtn} bg-rose-50 text-rose-500 hover:bg-rose-100`} title="删除">
            <Trash2 className="h-3 w-3" />
          </button>
        </>
      }
      subtitle={
        url ? (
          <a
            href={url}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex min-w-0 items-center gap-1 font-mono text-xs font-semibold text-slate-700 hover:text-blue-600"
          >
            <span className="truncate">
              {s.hosts?.[0]}
              {addr && <span className="text-slate-400">{portText(addr.https, addr.port)}</span>}
              {s.pathPrefix && <span className="text-slate-400">{s.pathPrefix}</span>}
            </span>
            <ExternalLink className="h-3 w-3 shrink-0 text-slate-300" />
          </a>
        ) : (
          <span className="font-mono text-xs font-semibold text-slate-700">
            {s.hosts?.[0] ?? <span className="text-slate-400">任意域名</span>}
            {addr && <span className="text-slate-400">{portText(addr.https, addr.port)}</span>}
            {s.pathPrefix && <span className="text-slate-400">{s.pathPrefix}</span>}
          </span>
        )
      }
      meta={
        <>
          {/* 后端：只显示端口（IP 已经在设备头上了，不用重复） */}
          <span className="inline-flex min-w-0 items-center gap-1 font-mono text-[11px] text-slate-500">
            <ArrowRight className="h-3 w-3 shrink-0 text-slate-300" />
            <span className="truncate">:{String(s.backend ?? '').split(':')[1] ?? s.backend}</span>
          </span>
          {s.backendHttps && <CardBadge tone="emerald" title="内网服务本身是 HTTPS">TLS</CardBadge>}
          {s.https && (
            <CardBadge tone="violet" title="对外走 HTTPS">HTTPS</CardBadge>
          )}
          {s.hosts.length > 1 && (
            <CardBadge tone="slate">+{s.hosts.length - 1} 个域名</CardBadge>
          )}
          {s.stripPrefix && <CardBadge tone="amber" title="转发前剥掉路径前缀">剥前缀</CardBadge>}
          <button
            onClick={onTest}
            disabled={testingId === s.id}
            className="ml-auto grid h-5 w-5 shrink-0 place-items-center rounded text-slate-400 hover:bg-blue-50 hover:text-blue-600 disabled:opacity-40"
            title="测试后端能不能连上"
          >
            <Zap className={`h-3 w-3 ${testingId === s.id ? 'animate-pulse' : ''}`} />
          </button>
        </>
      }
    />
  )
}
