/**
 * 反向代理页面。
 *
 * 原来 1677 行塞在一个文件里：站点弹窗（580 行）、入口弹窗、站点卡片、
 * 通用弹窗壳、一堆常量。现在按职责拆到 `./proxy/` 下，这里只留**列表与编排**：
 *   - 站点列表怎么摆、怎么搜
 *   - 点哪个按钮弹哪个窗（弹窗 state 在这里，因为要互斥）
 *   - 与各弹窗之间的数据流转
 *
 * 加新弹窗时：在 ./proxy/ 下新建文件，然后在这里 import 并加一个 state 即可。
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  AlertCircle,
  ArrowRight,
  Globe,
  HardDrive,
  Plus,
  Settings2,
  } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import { EntryModal } from './proxy/EntryModal'
import { ListenerChip, SiteCard } from './proxy/SiteCard'
import { SiteModal } from './proxy/SiteModal'
import { errText, useToast } from './proxy/shared'
import * as api from '../lib/api'
import type {
  Certificate,
  Device,
  ProxyConfig,
  ProxySite,
} from '../types'
import { AccessLogModal } from '../components/AccessLogModal'

// ===================== 小工具 =====================

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
          <div className="mt-1 text-xs text-muted">{loadErr}</div>
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
        <div className="py-12 text-center text-sm text-muted">正在加载...</div>
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
    ? 'bg-fill'
    : listeners.length === 0
      ? 'bg-fill'
      : failed.length === 0
        ? 'bg-emerald-500'
        : 'bg-amber-400'

  return (
    <div className="space-y-4">
      <div>
        <div className="text-base font-bold text-content">反向代理</div>
        <div className="mt-1 text-xs text-muted">
          在本机监听端口，按域名把请求转给内网的机器。就是 nginx 干的活
        </div>
      </div>

      {/* 监听状态：哪些端口开着、各挂了几个服务 */}
      <Card className="!py-3.5">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <span className={`h-2 w-2 shrink-0 rounded-full ${stateDot}`} />
          <span className="text-sm font-semibold text-content">{stateText}</span>

          {entry.baseDomain && <span className="font-mono text-[11px] text-muted">*.{entry.baseDomain}</span>}

          <button
            onClick={toggleAccessLog}
            title="点一下开关访问日志"
            className={`rounded-md px-1.5 py-0.5 text-[11px] font-semibold transition ${
              entry.accessLog
                ? 'bg-fill text-content hover:bg-fill'
                : 'text-muted hover:bg-surface-2 hover:text-muted'
            }`}
          >
            访问日志
          </button>

          <div className="ml-auto flex items-center gap-1">
            <button
              onClick={toggleRunning}
              className="rounded-lg px-2.5 py-1 text-xs font-semibold text-muted hover:bg-surface-2"
            >
              {entry.enabled ? '停止' : '启动'}
            </button>
            <button
              onClick={() => setEntryModal(true)}
              className="flex items-center gap-1 rounded-lg px-2.5 py-1 text-xs font-semibold text-muted hover:bg-surface-2"
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
          <div className="mt-2 text-[11px] text-muted">
            站点都停用了，或者默认入口两个端口都没填——点右上角「默认入口」看看
          </div>
        )}

        {listeners.length > 0 && failed.length === 0 && (
          <div className="mt-2 text-[11px] text-muted">
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
                服务 <span className="ml-1 font-normal text-muted">{sites.length}</span>
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
                  <div className="break-all text-center font-mono text-[11px] text-muted">{example}</div>
                  <div className="text-[10px] text-muted">浏览器</div>
                </div>

                <ArrowRight className="h-4 w-4 shrink-0 text-muted" />

                <div className="flex w-28 flex-col items-center gap-1.5">
                  <div className="grid h-11 w-11 place-items-center rounded-2xl bg-inverse text-[10px] font-bold text-on-inverse">
                    LS
                  </div>
                  <div className="font-mono text-[11px] text-muted">{entryPort ? `:${entryPort}` : '未设端口'}</div>
                  <div className="text-[10px] text-muted">Catflap</div>
                </div>

                <ArrowRight className="h-4 w-4 shrink-0 text-muted" />

                <div className="flex w-28 flex-col items-center gap-1.5">
                  <div className="grid h-11 w-11 place-items-center rounded-2xl bg-emerald-50 text-emerald-500">
                    <HardDrive className="h-5 w-5" />
                  </div>
                  <div className="break-all text-center font-mono text-[11px] text-muted">192.168.1.20:5000</div>
                  <div className="text-[10px] text-muted">内网服务</div>
                </div>
              </div>

              <button
                onClick={() => setSiteModal({ open: true })}
                className="mt-7 rounded-xl bg-blue-500 px-4 py-2 text-xs font-semibold text-white shadow-md shadow-blue-500/20 hover:bg-blue-600"
              >
                添加第一个服务
              </button>
              <div className="mt-2.5 text-[11px] text-muted">
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
                className={`rounded-xl border border-line/70 bg-surface transition ${
                  g.device && overDev === g.device.id && dragDev !== g.device.id
                    ? 'ring-2 ring-blue-300'
                    : ''
                } ${g.device && dragDev === g.device.id ? 'opacity-40' : ''}`}
              >
                {/* 设备头：和穿透页的设备卡片同一套样式 */}
                <div className="flex items-center gap-2 border-b border-line px-3 py-2">
                  <span className="grid h-7 w-7 shrink-0 place-items-center rounded-lg bg-blue-50 text-blue-500">
                    <HardDrive className="h-3.5 w-3.5" />
                  </span>
                  <div className="min-w-0 flex-1">
                    <p className="truncate text-[13px] font-bold text-content">
                      {g.device?.name ?? '未归到设备'}
                    </p>
                    <p className="font-mono text-[10px] text-muted">
                      {g.device?.ip ?? '这台机器还没加进设备列表'}
                    </p>
                  </div>
                  <span className="shrink-0 text-[11px] text-muted">
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
          <div key={t.id} className="rounded-lg bg-inverse/90 px-4 py-2 text-sm text-on-inverse shadow-lg">
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
