/**
 * 穿透（STUN）页面。
 *
 * 这个页面曾经是 **3010 行的单文件**，里面塞了服务弹窗（1354 行）、设备弹窗、
 * NAT 检测、日志、STUN 服务器五个弹窗和一堆表单转换。现在已经按职责拆到
 * `./stun/` 下各自的文件，这里只留**列表与编排**：
 *   - 设备树 + 服务列表怎么摆
 *   - 点哪个按钮弹哪个窗（弹窗的 state 也在这里，因为要互斥）
 *   - 与各弹窗之间的数据流转
 *
 * 加新弹窗时：在 ./stun/ 下新建一个文件，然后在这里 import 并加一个 state 即可。
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertCircle, ChevronRight, ClipboardCopy, Computer, ExternalLink, FileText, Globe,
  Info, LoaderCircle, Pencil, Plus, Power, RotateCw, Server, Trash2, Wifi,
} from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import { LanScanModal } from '../components/LanScanModal'
import { CardBadge, ServiceCard, iconBtn } from '../components/ServiceCard'
import { AccessLogModal } from '../components/AccessLogModal'
import type { ServiceState } from '../components/ServiceCard'
import * as api from '../lib/api'
import type {
  LanHost,
  LanPort,
  LanSubnet,
  StunConfig,
  StunDevice,
  StunService,
  StunStatusEvent,
} from '../types'

import { DeviceModal, type DeviceFormState } from './stun/DeviceModal'
import { LogModal } from './stun/LogModal'
import { NatTypeModal } from './stun/NatTypeModal'
import { ServiceModal } from './stun/ServiceModal'
import { StunServersModal } from './stun/StunServersModal'
import {
  type ServiceFormState,
  formToPayloadTLS,
  getWebhookStatus,
  phaseLabel,
  serviceToPayload,
} from './stun/shared'

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
      // 站点级名单已废弃，始终传「不限制」；过滤统一走全局封禁表 / 白名单
      access: { mode: '', rules: [] },
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
          <div className="mt-1 text-xs text-muted">{loadErr}</div>
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
    return <Card><div className="py-12 text-center text-sm text-muted">正在加载...</div></Card>
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
                off ? 'bg-fill text-muted hover:bg-fill' : 'bg-emerald-50 text-emerald-600 hover:bg-emerald-100'
              }`}
            >
              {busy ? <LoaderCircle className="h-3 w-3 animate-spin" /> : <Power className="h-3 w-3" />}
            </button>
            <button
              onClick={() => setServiceModal({ open: true, deviceId: getDeviceId(dev), initial: svc })}
              className={`${iconBtn} bg-surface-2 text-content hover:bg-fill`}
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
              className={`${iconBtn} bg-surface-2 text-content hover:bg-fill`}
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
            <span className="text-muted">已停用，外部地址不通</span>
          ) : fullAddr ? (
            <div className="flex min-w-0 items-center gap-1">
              <button
                onClick={() => copy(fullAddr)}
                title={`点击复制：${fullAddr}`}
                className="flex min-w-0 flex-1 items-center gap-1 text-left"
              >
                <ClipboardCopy className="h-3 w-3 shrink-0 text-muted" />
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
            <span className="text-muted">{phaseLabel[phase] || '等待外部地址…'}</span>
          )
        }
        meta={
          <>
            <span className="font-mono text-[11px] text-muted">: {svc.internalPort}</span>
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
              <span className="text-muted">最优 STUN:</span>
              <span className="max-w-[220px] break-all text-right font-mono font-semibold text-blue-500 sm:max-w-none">
                {config.bestStun || '--'}
              </span>
              <button
                type="button"
                onClick={() => setStunServersOpen(true)}
                title="编辑 STUN 服务器列表"
                className="flex items-center gap-1 rounded-md border border-line px-2 py-1 text-content transition hover:border-blue-200 hover:bg-blue-50 hover:text-blue-600"
              >
                <Server className="h-3.5 w-3.5" /> 服务器
              </button>
              <button
                type="button"
                onClick={() => setNatTypeOpen(true)}
                title="查看 NAT 类型详情"
                className="flex items-center gap-1 rounded-md border border-line px-2 py-1 text-content transition hover:border-blue-200 hover:bg-blue-50 hover:text-blue-600"
              >
                <Info className="h-3.5 w-3.5" /> NAT 类型
              </button>
              <button
                onClick={refresh}
                className="ml-2 flex items-center gap-1 rounded-md text-muted transition hover:text-blue-500"
              >
                <RotateCw className="h-3.5 w-3.5" /> 刷新
              </button>
            </div>
          }
        />
        <div className="flex flex-wrap items-center gap-2 rounded-xl bg-surface-2 p-4">
          <div className="flex min-w-[140px] flex-col items-center gap-1 rounded-xl bg-sky-50 px-4 py-3 ring-1 ring-sky-200">
            <div className="flex items-center gap-1.5">
              <Computer className="h-4 w-4 text-sky-500" />
              <span className="text-xs font-semibold text-sky-600">本机 IP</span>
            </div>
            <span className="font-mono text-xs text-content">{config.localIP || '--'}</span>
          </div>
          {[...(config.natRouterList || [])]
            .sort((a, b) => a.natLevel - b.natLevel)
            .map((r) => (
              <div key={r.natLevel} className="flex items-center gap-2">
                <ChevronRight className="h-4 w-4 text-muted" />
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
                  <span className="font-mono text-xs text-content">{r.lanIP}</span>
                </div>
              </div>
            ))}
          <ChevronRight className="h-4 w-4 text-muted" />
          <div className="flex min-w-[140px] flex-col items-center gap-1 rounded-xl bg-emerald-50 px-4 py-3 ring-1 ring-emerald-200">
            <div className="flex items-center gap-1.5">
              <Globe className="h-4 w-4 text-emerald-500" />
              <span className="text-xs font-semibold text-emerald-600">公网 IP</span>
            </div>
            <span className="font-mono text-xs text-content">{config.publicIP || '--'}</span>
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
        <div className="text-[11px] leading-relaxed text-muted">
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
              <Server className="mx-auto h-8 w-8 text-muted" />
              <p className="mt-2 text-sm text-muted">还没有内网穿透服务</p>
              <p className="mt-1 text-[11px] text-muted">
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
                <div className="flex items-center gap-2 border-b border-line px-3 py-2">
                  <span className="grid h-7 w-7 shrink-0 place-items-center rounded-lg bg-emerald-50 text-emerald-600">
                    <Server className="h-3.5 w-3.5" />
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-[13px] font-bold text-content">
                      {dev ? dev.name : '未归到设备'}
                    </p>
                    {dev && <p className="font-mono text-[10px] text-muted">{dev.ip}</p>}
                  </div>
                  <span className="shrink-0 text-[11px] text-muted">{svcs.length} 个服务</span>
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
            className="max-w-[min(90vw,34rem)] break-words rounded-lg bg-inverse/90 px-4 py-2 text-sm leading-relaxed text-on-inverse shadow-lg"
          >
            {t.text}
          </div>
        ))}
      </div>
    </div>
  )
}
