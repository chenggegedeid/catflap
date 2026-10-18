/**
 * 穿透服务弹窗（新增 / 编辑）。
 *
 * 这是原本 Stun.tsx 里最大的一块 —— **单个组件 1354 行**。现在独立成文件，
 * 里面按 Tab 分成基本信息 / Webhook / 重定向三段，各自仍是这个文件的局部组件。
 *
 * 为什么不再往下切成三个文件：这三段共享同一份 `form` state 与 setForm，
 * 硬拆成三个文件就要把 state 与校验逻辑提到父层或塞进 context，
 * 反而更绕。等哪天某个 Tab 复杂到需要独立复用时再分不迟。
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  CornerUpRight, Gamepad2, Radar, RotateCw, Send, ShieldCheck, Trash2, TriangleAlert, X,
} from 'lucide-react'
import { modalBackdrop } from '../../components/modal'
import { DomainInput } from '../../components/DomainInput'
import * as api from '../../lib/api'
import { hasCapability, loadProviderTypes } from '../../lib/providerTypes'
import { ProviderCap } from '../../types'
import type {
  Certificate,
  DdnsProvider,
  DdnsProviderTypeSpec,
  Device,
  LanPort,
  MCEntryConfig,
  NotifyChannel,
  RedirectConfig,
  RedirectInspection,
  StunService,
  StunStatusEvent,
} from '../../types'
import {
  type ServiceFormState,
  type ServiceModalTab,
  certDomainsOf,
  emptyMCEntry,
  emptyRedirect,
  fmtTime,
  emptyService,
  fieldInputCls,
  fieldLabelCls,
  matchCertDomain,
  NOTIFY_PLACEHOLDERS,
  NOTIFY_TEMPLATES,
  pickCertDomain,
  serviceToForm,
  setDefaultChannelIds,
} from './shared'

export function ServiceModal({
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
      <div className="flex max-h-[92vh] w-full max-w-3xl flex-col rounded-2xl bg-surface text-content shadow-2xl ring-1 ring-line">
        <div className="flex items-center justify-between px-6 py-5">
          <div className="text-base font-bold text-content">{initial ? '编辑服务' : '添加服务'}</div>
          <button
            type="button"
            onClick={onCancel}
            className="grid h-7 w-7 place-items-center rounded-full text-muted transition hover:bg-surface-2 hover:text-content"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="flex gap-4 overflow-x-auto border-b border-line px-4 sm:gap-8 sm:px-6">
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
                  : 'border-transparent text-muted hover:text-content'
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
                <div className="flex flex-wrap gap-2">
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
                  <div className="mt-2 flex flex-wrap gap-1 rounded-xl bg-surface-2 p-2">
                    {hostPorts.map((p) => (
                      <button
                        key={p.port}
                        type="button"
                        onClick={() => setForm((prev) => ({ ...prev, internalPort: String(p.port) }))}
                        className="rounded bg-surface px-2 py-1 text-[11px] font-semibold text-content ring-1 ring-line transition hover:bg-blue-50 hover:text-blue-600 hover:ring-blue-300"
                      >
                        {p.port}
                        {p.name && <span className="ml-1 opacity-70">{p.name}</span>}
                      </button>
                    ))}
                  </div>
                )}

                <div className="mt-1 text-[11px] text-muted">
                  服务会加在这台设备下。机器没在列表里就去「设备」页加一台。
                </div>
              </label>

              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
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
                  <span className="ml-1 font-normal text-muted">（可选，建议填）</span>
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
                <p className="mt-1 text-xs leading-5 text-muted">
                  外部端口是路由器照着这个本机端口分配的。不填的话本机端口每次随机，
                  外部端口也跟着一直变，配好的「域名+端口」过一会儿就失效；
                  填一个没被占用的端口（如 30000-60000），外部端口就稳定下来了，
                  不开 UPnP 也能长期使用。
                </p>
              </label>

              {/* 协议决定了下面能配什么：证书、HTTPS、入口重定向都只有 TCP 才有 */}
              <div>
                <div className={fieldLabelCls}>协议</div>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
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
                          : 'border-line hover:border-line'
                      }`}
                    >
                      <div
                        className={`text-sm font-bold ${form.protocol === o.value ? 'text-blue-600' : 'text-content'}`}
                      >
                        {o.value}
                      </div>
                      <div className="mt-0.5 text-[11px] text-muted">{o.hint}</div>
                    </button>
                  ))}
                </div>
              </div>

              <div className="space-y-3 rounded-xl border border-line/80 bg-surface-2/50 p-3">
                <div className="flex items-center gap-1.5 text-xs font-bold text-content">
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
                  <div className="mt-1 text-[11px] text-muted">
                    {ddnsZones.length > 0
                      ? '主域来自 DDNS 解析记录；删掉那条解析后这里的主域就没了，服务会回落到公网 IP'
                      : '要在 DDNS 页解析到你的公网 IP'}
                  </div>
                </label>

                {form.protocol === 'TCP' && (
                  <>
                    {/* 洞口一终结 TLS，非 HTTPS 的连接全会被当成敲错地址的浏览器回 302，
                        MC、SSH 这类直接连不上——这是最容易配错的一项，旁边必须写明 */}
                    <label className="flex cursor-pointer items-start gap-2 rounded-xl bg-surface px-3 py-2 ring-1 ring-line">
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
                        <span className="block text-xs font-semibold text-content">加 HTTPS 证书</span>
                        <span className="block text-[11px] text-muted">
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
                    <label className="flex cursor-pointer items-start gap-2 rounded-xl bg-surface px-3 py-2 ring-1 ring-line">
                      <input
                        type="checkbox"
                        checked={form.internalHttps}
                        onChange={(e) => setForm((p) => ({ ...p, internalHttps: e.target.checked }))}
                        className="mt-0.5 h-3.5 w-3.5"
                      />
                      <span>
                        <span className="block text-xs font-semibold text-content">内网服务本身就是 HTTPS</span>
                        <span className="block text-[11px] text-muted">
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
                    className="flex cursor-pointer items-center gap-2 rounded-xl bg-surface-2 px-3 py-2 text-xs font-medium text-content"
                  >
                    <input
                      type="checkbox"
                      checked={form[opt.key]}
                      onChange={(e) => setForm((p) => ({ ...p, [opt.key]: e.target.checked }))}
                      className="h-3.5 w-3.5"
                    />
                    {opt.label}
                    {opt.hint && <span className="text-[10px] text-muted">（UDP 无效）</span>}
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
              <div className="mb-2 flex flex-wrap items-center gap-2 text-sm font-bold text-content">
                <Send className="h-4 w-4 text-blue-500" />
                通知渠道
              </div>
              <div className="mb-3 rounded-xl bg-surface-2 px-3 py-2.5 text-[11px] leading-relaxed text-muted">
                渠道在「通知」页里统一配置（推送地址、token 都存那儿），这里只选用 ——
                同一个群机器人被多个服务用的时候，改一次全都跟着变。
                <a href="#/notify" className="ml-1 text-blue-600 underline-offset-2 hover:underline">
                  去配置渠道 ↗
                </a>
              </div>

              {channelsLoading ? (
                <div className="rounded-xl border border-line px-3 py-6 text-center text-xs text-muted">
                  正在加载渠道…
                </div>
              ) : channels.length === 0 ? (
                <div className="rounded-xl border border-dashed border-line px-3 py-6 text-center text-xs text-muted">
                  还没有任何通知渠道。
                  <a href="#/notify" className="ml-1 text-blue-600 underline-offset-2 hover:underline">
                    去添加一个
                  </a>
                  ，之后这个服务就能选它来发通知了。
                </div>
              ) : (
                <div className="space-y-1.5 rounded-xl border border-line p-3">
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
                      <span className={ch.enabled ? 'text-content' : 'text-muted'}>
                        {ch.name}
                        {!ch.enabled && '（已停用，勾了也不会发）'}
                      </span>
                    </label>
                  ))}
                  {(form.channelIds ?? []).length === 0 && (
                    <div className="pt-1 text-[11px] text-muted">
                      一个都不选 = 这个服务不发通知。
                    </div>
                  )}
                </div>
              )}

              {/* 自定义通知正文：怎么发在通知页配好了，这里只写"说什么" */}
              <div className="mt-4">
                <div className="mb-2 flex flex-wrap items-center justify-between gap-2">
                  <div className="text-sm font-bold text-content">通知内容（可选）</div>
                  <select
                    value=""
                    onChange={(e) => {
                      const tpl = NOTIFY_TEMPLATES.find((t) => t.key === e.target.value)
                      if (tpl) setForm((p) => ({ ...p, notifyTemplate: tpl.body }))
                      e.target.value = ''
                    }}
                    className="rounded-lg border border-line bg-surface px-2 py-1 text-[11px] outline-none focus:border-blue-400"
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
                  className="w-full resize-y rounded-xl border border-line bg-surface px-3 py-2 font-mono text-xs outline-none focus:border-blue-400"
                />
                <div className="mt-1.5 text-[11px] leading-relaxed text-muted">
                  可用占位符（发的时候会自动换成真实值）：
                  <div className="mt-1 flex flex-wrap gap-1">
                    {NOTIFY_PLACEHOLDERS.map((p) => (
                      <span
                        key={p}
                        className="rounded bg-surface-2 px-1.5 py-0.5 font-mono text-[10px] text-muted ring-1 ring-line"
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
                <span className="text-[11px] text-muted">
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


          {/*
            站点级 IP 名单已移除。穿透本来就直接暴露在公网上，
            同样"不知道谁会来"—— 过滤统一交给全局黑名单（支持网段）+ 自动拉黑。
            Access 字段仍在数据模型里保留，老配置读进来不会报错，只是不再生效。
          */}

          {activeTab === 'redirect' && form.protocol === 'TCP' && (
            <div className="mx-auto max-w-2xl">
              {/* 一个服务只用一种入口：网页走 307，MC 走 SRV。选哪个就只显示哪块，另一个自动关掉 */}
              <div className="mb-4 inline-flex w-full flex-wrap gap-1 rounded-xl bg-surface-2 p-1">
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
                      entryKind === o.key ? 'bg-surface text-blue-600 shadow-sm' : 'text-muted hover:text-content'
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
                <div className="rounded-xl bg-surface-2 p-4 text-xs leading-6 text-muted">
                  外部端口会变。想让别人用一个固定的名字找到这个服务，选一种入口：
                  <div className="mt-1">
                    · <span className="font-semibold text-content">网页</span>：浏览器访问入口域名，自动跳到当前端口
                  </div>
                  <div>
                    · <span className="font-semibold text-content">MC Java 版</span>：朋友在 MC 里只填域名，不用填端口
                  </div>
                  <div className="text-muted">SSH、远程桌面这类两种都用不了，只能用「域名 + 当前端口」。</div>
                </div>
              )}

              {entryKind === 'web' && (
                <>
                  <div className="mb-4 rounded-xl bg-surface-2 p-3 text-xs leading-5 text-muted">
                    端口会变，入口域名不变：浏览器访问入口域名，Cloudflare 自动跳到当前端口。
                    <span className="text-muted">只对网页有用，MC、SSH 这类不认跳转。</span>
                    <div className="mt-2 flex flex-wrap items-center gap-2 font-mono text-[11px] text-content">
                      <span className="rounded-md bg-surface px-2 py-1 ring-1 ring-line">
                        {form.redirect.entryHost.trim() || '入口域名'}
                      </span>
                      <span className="text-muted">→</span>
                      <span className="rounded-md bg-surface px-2 py-1 ring-1 ring-line">
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
                    <div className="mb-4 rounded-xl border border-line p-3">
                      <div className="mb-2 flex items-center justify-between gap-2">
                        <div className="text-xs font-semibold text-muted">Cloudflare 上的解析记录</div>
                        <button
                          type="button"
                          disabled={inspecting}
                          onClick={() => void loadInspect()}
                          className="inline-flex items-center gap-1 rounded-lg px-1.5 py-0.5 text-[11px] font-semibold text-muted transition hover:bg-surface-2 hover:text-content disabled:opacity-50"
                        >
                          <RotateCw className={`h-3 w-3 ${inspecting ? 'animate-spin' : ''}`} />
                          {inspecting ? '查询中' : '刷新'}
                        </button>
                      </div>

                      {inspectErr && <div className="text-[11px] leading-5 text-rose-500">查不了：{inspectErr}</div>}
                      {!inspectErr && !inspect && (
                        <div className="text-[11px] text-muted">
                          {inspecting ? '正在去 Cloudflare 查…' : '点右上角刷新看现在什么样'}
                        </div>
                      )}

                      {inspect && (
                        <div className="space-y-2.5">
                          <div>
                            <div className="flex flex-wrap items-center gap-1.5 text-[11px]">
                              <span className="rounded bg-surface-2 px-1.5 py-0.5 font-semibold text-muted">
                                入口
                              </span>
                              <span className="font-mono text-content">{inspect.entry.host}</span>
                              {inspect.entry.found && (
                                <span className="font-mono text-muted">
                                  {inspect.entry.type} {inspect.entry.content}
                                </span>
                              )}
                              {inspect.entry.byCatflap && (
                                <span className="text-muted">· Catflap 建的</span>
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
                              <span className="rounded bg-surface-2 px-1.5 py-0.5 font-semibold text-muted">
                                落地
                              </span>
                              <span className="font-mono text-content">
                                {inspect.landingHost || '还没定'}
                              </span>
                              {inspect.landing.managed && inspect.landing.lastIP && (
                                <span className="font-mono text-muted">A {inspect.landing.lastIP}</span>
                              )}
                            </div>
                            <div className="mt-1 text-[11px] leading-5">
                              {!inspect.landingHost ? (
                                <span className="text-muted">
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

                      <div className="mt-2.5 border-t border-line pt-2 text-[11px] leading-5 text-muted">
                        两条的要求正好相反：入口那条要开小黄云，落地那条要关 —— 开了的话 Cloudflare
                        不转发打洞出来的高位端口，一样访问不了。
                      </div>
                    </div>
                  )}

                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    <label className="col-span-2 block sm:col-span-1">
                      <div className="mb-1 text-xs font-semibold text-muted">Cloudflare 账号</div>
                      <select
                        value={String(form.redirect.providerId || 0)}
                        onChange={(e) => updateRedirect({ providerId: Number(e.target.value) || 0 })}
                        className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-surface-2 disabled:text-muted"
                      >
                        <option value="0">请选择</option>
                        {redirectCapableProviders.map((p) => (
                          <option key={p.id} value={String(p.id)}>
                            {p.name}
                          </option>
                        ))}
                      </select>
                      <div className="mt-1 text-[11px] text-muted">
                        {redirectCapableProviders.length === 0
                          ? '"DDNS" 页面里还没有 Cloudflare 账号，先去那边加一个'
                          : '令牌要有 DNS 编辑 + 单一重定向编辑两项权限'}
                      </div>
                    </label>

                    <label className="col-span-2 block sm:col-span-1">
                      <div className="mb-1 text-xs font-semibold text-muted">入口域名</div>
                      <input
                        value={form.redirect.entryHost}
                        onChange={(e) => updateRedirect({ entryHost: e.target.value })}
                        placeholder="catflap.example.com"
                        className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-surface-2 disabled:text-muted"
                      />
                      <div className="mt-1 text-[11px] text-muted">
                        外面记的就是这个，Catflap 会自己建好；别和基础配置里的对外域名填成同一个
                      </div>
                    </label>

                    <label className="col-span-2 block sm:col-span-1">
                      <div className="mb-1 text-xs font-semibold text-muted">主域名</div>
                      <input
                        value={form.redirect.zoneDomain}
                        onChange={(e) => updateRedirect({ zoneDomain: e.target.value })}
                        placeholder={redirectGuessedZone || 'example.com'}
                        className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-surface-2 disabled:text-muted"
                      />
                      <div className="mt-1 text-[11px] text-muted">
                        {redirectGuessedZone
                          ? `留空就按 ${redirectGuessedZone} 算；域名多一层（如 a.b.co.uk）才需要填`
                          : '留空按入口域名的后两段算'}
                      </div>
                    </label>
                  </div>

                  {initial ? (
                    <div className="mt-4 rounded-xl border border-line p-3">
                      <div className="mb-2 text-xs font-semibold text-muted">最近一次同步</div>
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
                          <div className="mt-1 text-muted">
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
                        <div className="text-xs text-muted">还没同步过</div>
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
                          className="inline-flex items-center gap-1.5 rounded-xl bg-surface-2 px-3 py-1.5 text-xs font-semibold text-content transition hover:bg-rose-50 hover:text-rose-500 disabled:opacity-50"
                        >
                          <Trash2 className="h-3.5 w-3.5" />
                          删掉 Cloudflare 上的规则
                        </button>
                        {redirectDirty ? (
                          <span className="text-[11px] font-semibold text-amber-600">
                            上面改的还没保存 —— 点右下角「保存」，几秒后自己就同步了，不用回来点这两个
                          </span>
                        ) : (
                          <span className="text-[11px] text-muted">
                            端口一变会自动同步，这里是手动再跑一遍
                          </span>
                        )}
                      </div>
                    </div>
                  ) : (
                    <div className="mt-4 text-[11px] text-muted">先把服务保存了，再回来点同步</div>
                  )}
                </>
              )}

              {entryKind === 'mc' && (
                <div>
                  <div className="mb-4 rounded-xl bg-surface-2 p-3 text-xs leading-5 text-muted">
                    朋友在 MC 里只填域名，不用填端口；端口变了 Catflap 自动改 SRV 记录。
                    <span className="text-muted">基岩版不认 SRV，用不上。</span>
                    <div className="mt-2 flex flex-wrap items-center gap-2 font-mono text-[11px] text-content">
                      <span className="text-muted">朋友填</span>
                      <span className="rounded-md bg-surface px-2 py-1 font-semibold ring-1 ring-line">
                        {form.mcEntry.host.trim() || 'mc.example.com'}
                      </span>
                    </div>
                  </div>

                  <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                    <label className="col-span-2 block sm:col-span-1">
                      <div className={fieldLabelCls}>Cloudflare 账号</div>
                      <select
                        value={String(form.mcEntry.providerId || 0)}
                        onChange={(e) => updateMCEntry({ providerId: Number(e.target.value) || 0 })}
                        className={`${fieldInputCls} disabled:bg-surface-2 disabled:text-muted`}
                      >
                        <option value="0">请选择</option>
                        {redirectCapableProviders.map((p) => (
                          <option key={p.id} value={String(p.id)}>
                            {p.name}
                          </option>
                        ))}
                      </select>
                      <div className="mt-1 text-[11px] text-muted">
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
                        className={`${fieldInputCls} disabled:bg-surface-2 disabled:text-muted`}
                      />
                      <div className="mt-1 text-[11px] text-muted">朋友在 MC 里填的就是它</div>
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
                                on ? 'border-blue-300 bg-blue-50/60 ring-1 ring-blue-200' : 'border-line hover:border-line'
                              }`}
                            >
                              <div className={`text-xs font-bold ${on ? 'text-blue-600' : 'text-content'}`}>{o.title}</div>
                              <div className="mt-0.5 text-[11px] text-muted">{o.hint}</div>
                            </button>
                          )
                        })}
                      </div>

                      {!!form.mcEntry.target && (
                        <select
                          value={form.mcEntry.target}
                          onChange={(e) => updateMCEntry({ target: e.target.value })}
                          className={`${fieldInputCls} mt-2 disabled:bg-surface-2 disabled:text-muted`}
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

                      <div className="mt-2 font-mono text-[11px] text-muted">
                        _minecraft._tcp.{form.mcEntry.host.trim() || 'mc.example.com'} →{' '}
                        {form.mcEntry.target || form.mcEntry.host.trim() || 'mc.example.com'}:端口
                      </div>
                    </div>
                  </div>

                  {initial ? (
                    <div className="mt-4 rounded-xl border border-line p-3">
                      <div className="mb-2 text-xs font-semibold text-muted">最近一次同步</div>
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
                      {!status?.mcEntryStatus && <div className="text-xs text-muted">还没同步过</div>}

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
                          className={`text-[11px] ${mcEntryDirty ? 'font-semibold text-amber-600' : 'text-muted'}`}
                        >
                          {mcEntryDirty ? '上面改的还没保存 —— 点右下角「保存」，几秒后自己就同步了' : '端口一变会自动同步'}
                        </span>
                      </div>
                    </div>
                  ) : (
                    <div className="mt-4 text-[11px] text-muted">先把服务保存了，再回来点同步</div>
                  )}
                </div>
              )}
            </div>
          )}
        </div>

        {err && <div className="px-6 pb-2 text-xs text-rose-500">{err}</div>}
        <div className="flex justify-end gap-2 border-t border-line px-6 py-4">
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

