import { useEffect, useState } from 'react'
import {
  ArrowRight, Globe, HardDrive, Plus, Radar, TriangleAlert, X, Zap,
} from 'lucide-react'
import { DomainInput } from '../../components/DomainInput'
import * as api from '../../lib/api'
import type {
  Certificate, Device, LanPort, ProxyEntry, ProxySite,
} from '../../types'
import {
  BlockedPortWarn, CertSelect, Modal, Warn,
} from './Modal'
import {
  autoCertNeedsSNI, DEFAULT_BACKEND_PORT, errText, fieldCls, inputCls,
  labelCls, portBlocked, portText,
} from './shared'

export interface SiteFormState {
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
}

export function SiteModal({
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
    // 站点级 IP 名单已废弃：过滤统一走全局封禁表 / 白名单。
    // 这里显式传「不限制」，是为了兼容仍在读这个字段的旧配置 ——
    // 老配置里如果还存着 deny 规则，不能让它继续生效（否则用户改不掉）。
    access: { mode: '', rules: [] },
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
      const p = payload()
      // 保存前先拨一次后端：**连不通就不存**。
      //
      // 以前是「先存后测」，结果是用户点保存看到「已保存」，转头访问却 502，
      // 又得回来改配置。现在反过来：不通就挡在这儿，把提示留在界面上，
      // 顺手把后端 HTTPS 那个勾探正 —— 探测发现后端说的是 TLS 而我们没勾，
      // 就直接存成勾上，省得存完还是连不上。
      const r = await api.testProxySite(p)
      if (!r.reachable) {
        setErr(r.message || '后端连不上，没有保存')
        setBusy(false)
        return
      }
      if (r.mismatch) {
        const fixed = { ...p, backendHttps: r.scheme === 'https' }
        setForm((prev) => ({ ...prev, backendHttps: r.scheme === 'https' }))
        await onSubmit(fixed)
      } else {
        await onSubmit(p)
      }
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
            className="mr-auto flex items-center gap-1.5 rounded-xl px-3 py-2 text-sm text-muted transition hover:bg-surface-2 disabled:opacity-40"
          >
            <Zap className={`h-3.5 w-3.5 ${testing ? 'animate-pulse' : ''}`} />
            {testing ? '测试中...' : '测试后端'}
          </button>
          <button
            type="button"
            onClick={onCancel}
            className="rounded-xl px-4 py-2 text-sm text-muted transition hover:bg-surface-2"
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
      <div className="flex items-center gap-2 rounded-xl bg-surface-2 px-3 py-2.5 text-xs ring-1 ring-line/70">
        <Globe className="h-3.5 w-3.5 shrink-0 text-blue-500" />
        <span className="truncate font-mono text-content">
          {shownHttps ? 'https://' : 'http://'}
          {hosts.length > 0 ? fullHost(hosts[0]) : portForward ? '任意域名' : '域名'}
          {shownPort > 0 && <span className="text-muted">{portText(shownHttps, shownPort)}</span>}
          {hosts.length > 1 && <span className="text-muted"> 等 {hosts.length} 个域名</span>}
        </span>
        <ArrowRight className="h-3.5 w-3.5 shrink-0 text-muted" />
        <HardDrive className="h-3.5 w-3.5 shrink-0 text-emerald-500" />
        <span className="truncate font-mono text-content">
          {form.backendHttps ? 'https://' : 'http://'}
          {form.backend.trim() || '内网服务'}
        </span>
      </div>

      {/* ==== 这台服务跑在哪 + 叫什么 ====
          顺序和内网穿透的添加表单一致：先选设备，再填名称/端口，
          最后才是对外怎么访问（域名、HTTPS、证书）。
          "代理地址"其实就是 设备 + 端口，所以并进这一块。 */}
      {/* ==== 后端这一侧 ==== */}
      <div className="!mt-5 border-t border-line pt-4">
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
          <div className="mt-2 flex flex-wrap gap-1 rounded-xl bg-surface-2 p-2">
            {hostPorts.map((p) => (
              <button
                key={p.port}
                type="button"
                onClick={() => setBackendPort(String(p.port))}
                className="rounded bg-surface px-2 py-1 text-[11px] font-semibold text-content ring-1 ring-line transition hover:bg-blue-50 hover:text-blue-600 hover:ring-blue-300"
              >
                {p.port}
                {p.name && <span className="ml-1 opacity-70">{p.name}</span>}
              </button>
            ))}
          </div>
        )}

        <div className="mt-1 text-[11px] leading-relaxed text-muted">
          选设备 + 填端口就好，不用记 IP。机器没在列表里就去「设备」页加一台。
        </div>
      </div>


      {/* 服务名称 + 后端端口：和内网穿透表单同一个两列排法 */}
      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
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
        {/* 内网端口 + 后端是否 HTTPS：两个控件挨着，因为它们说的是同一台内网机器 */}
        <div className="col-span-2 sm:col-span-1">
          <div className={labelCls}>内网端口</div>
          <div className="flex items-center gap-2">
            <input
              value={form.backendPort}
              onChange={(e) => setBackendPort(e.target.value)}
              placeholder="8080"
              inputMode="numeric"
              className={`${inputCls} font-mono`}
            />
            {/*
              **后端（内网那台机器）是不是 HTTPS。**
              以前界面上**根本没有这个入口** —— 只能靠探测自动改。于是探测提示
              「请勾上『后端是 HTTPS』」，而用户根本找不到那个勾（界面上唯一一个
              HTTPS 勾管的是对外那一侧）。这就是那个 BUG。
              现在补上，并放在内网端口旁边：它属于内网那一侧，不该跟对外的证书混在一起。
            */}
            <label className="flex shrink-0 cursor-pointer items-center gap-1.5 rounded-lg bg-surface px-2.5 py-2 text-[11px] font-semibold whitespace-nowrap text-content ring-1 ring-line transition hover:bg-surface-2">
              <input
                type="checkbox"
                checked={form.backendHttps}
                onChange={(e) => setForm((p) => ({ ...p, backendHttps: e.target.checked }))}
                className="h-3.5 w-3.5"
              />
              HTTPS
            </label>
          </div>
          {/* 提示紧贴在这一行下面，说的是「后端是不是 HTTPS」 */}
          <div className="mt-1.5 text-[11px] leading-relaxed text-muted">
            内网那台服务本身走 HTTPS 就勾上（自签证书也算）。点了「测试后端」会自动探
          </div>
        </div>
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
                  className="grid h-8 w-8 shrink-0 place-items-center rounded-lg text-muted transition hover:bg-rose-50 hover:text-rose-500"
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

        <div className="mt-1 text-[11px] leading-relaxed text-muted">
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

      {/* 外网端口 + 对外是否加证书：这两个也是一对，管的是浏览器到 Catflap 这一侧 */}
      <div>
        <div className={labelCls}>外网端口</div>
        <div className="flex items-center gap-2">
          <input
            value={form.listenPort}
            onChange={(e) => setForm((p) => ({ ...p, listenPort: e.target.value }))}
            placeholder={shownPort > 0 ? String(shownPort) : '端口'}
            className={`${inputCls} font-mono`}
          />
          <label className="flex shrink-0 cursor-pointer items-center gap-1.5 rounded-lg bg-surface px-2.5 py-2 text-[11px] font-semibold whitespace-nowrap text-content ring-1 ring-line transition hover:bg-surface-2">
            <input
              type="checkbox"
              checked={form.https}
              onChange={(e) => setForm((p) => ({ ...p, https: e.target.checked }))}
              className="h-3.5 w-3.5"
            />
            加证书（HTTPS）
          </label>
        </div>
        {/* 端口的提示，紧跟端口那一行 */}
        <div className="mt-1.5 text-[11px] leading-relaxed text-muted">
          {portHint}
        </div>

        {portBlocked(customPort) && (
          <div className="mt-2">
            <BlockedPortWarn label="外网端口" port={customPort} />
          </div>
        )}

        {/* 证书选择器收进这一块里：只有勾了「加证书」才需要挑证书，
            放在紧下面才不会让人以为它是个独立设置 */}
        {form.https && (
          <div className="mt-3">
            <label className="block">
              <div className={labelCls}>证书</div>
              <CertSelect
                value={form.certId}
                certs={certs}
                onChange={(v) => setForm((p) => ({ ...p, certId: v }))}
              />
              {/* 证书的提示，紧跟证书选择 */}
              <div className="mt-1 text-[11px] leading-relaxed text-muted">
                自动匹配就是按访问用的域名挑证书，一般不用动
              </div>
            </label>
          </div>
        )}
      </div>

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

      <label className="!mt-5 block border-t border-line pt-4">
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
          <label className="flex cursor-pointer items-center gap-2 rounded-xl bg-surface-2 px-3 py-2.5 text-xs text-content">
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


      {/*
        站点级 IP 名单已移除。
        理由见 CHANGELOG 2.5.0：面向互联网的服务**没法提前知道谁会来**，
        提前配网段没有意义；真正起作用的是全局黑名单（支持网段）+ 自动拉黑。
        过滤统一走一处（审计页的封禁列表 / 白名单），不再每个服务各配一份。

        Access 字段仍在数据模型里保留，老配置读进来不会报错，只是不再生效。
      */}

      {err && <div className="rounded-xl bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>}
    </Modal>

    </>
  )
}


// ===================== 监听端口一览 =====================

/** 一个端口一块，对着 netstat 看到的那几行 */
