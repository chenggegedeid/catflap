/**
 * 设备：内网里那些机器。
 *
 * **穿透、反向代理、端口转发共用这一份** —— 同一台机器只维护一次。
 * 设备按 IP 标识，服务（站点/穿透服务/端口转发）通过 IP 归属到它。
 *
 * 这个页面是整个流程的起点：先有设备，才能加服务。
 */
import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react'
import { GripVertical, HardDrive, Pencil, Plus, Radar, Server, Trash2 } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import * as api from '../lib/api'
import type { Device, LanHost, LanSubnet, ProxyConfig, StunConfig } from '../types'
import { LanScanModal } from '../components/LanScanModal'
import { modalBackdrop } from '../components/modal'

export function Devices() {
  const [devices, setDevices] = useState<Device[]>([])
  const [modal, setModal] = useState<{ open: boolean; initial?: Device }>({ open: false })
  // 扫描弹窗：scan() 打开它，addScanned 在用户点某台机器时被调。
  const [scanModal, setScanModal] = useState<{ open: boolean; subnets: LanSubnet[] }>({ open: false, subnets: [] })
  // 删除确认框：设备上还有服务时，把清单给用户看，确认才连带删
  const [deleteConfirm, setDeleteConfirm] = useState<{
    open: boolean
    device?: Device
    detail: string
  }>({ open: false, detail: '' })
  const [deleting, setDeleting] = useState(false)
  const [toast, setToast] = useState('')

  const load = useCallback(async () => {
    try {
      setDevices(await api.getDevices())
    } catch {
      // 读不到就保持空，界面上有加设备的引导
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  // 拖动排序：dragId 是正在拖的那台，overId 是松手时悬停的那台
  const [dragId, setDragId] = useState(0)
  const [overId, setOverId] = useState(0)

  /** 把 dragId 放到 overId 前面，然后提交新顺序 */
  const commitReorder = async (from: number, to: number) => {
    const list = [...devices]
    const i = list.findIndex((d) => d.id === from)
    const j = list.findIndex((d) => d.id === to)
    if (i < 0 || j < 0 || i === j) return
    const [moved] = list.splice(i, 1)
    list.splice(j, 0, moved)
    // 先本地更新，界面立刻跟上；保存失败再刷回来
    setDevices(list)
    try {
      await api.reorderDevices(list.map((d) => d.id))
    } catch (e) {
      setToast(e instanceof Error ? e.message : '排序保存失败')
      await load()
    }
  }

  /**
   * 全局扫描：扫网段找机器，**弹窗列出，用户点哪台加哪台** —— 不再自动全加。
   *
   * 旧版会直接把所有新机器存进设备表（带自定名），结果 LAN 上谁扫谁都是一堆
   * 重复设备。现在用 LanScanModal（和穿透页同一个组件，样式一致）展示，
   * 用户点一台就加一台，不点就不加。
   */
  const scan = async () => {
    setToast('正在扫描局域网…')
    try {
      const subnets = await api.getProxyLanSubnets()
      if (subnets.length === 0) {
        setToast('没找到可扫描的网段')
        return
      }
      setScanModal({ open: true, subnets })
    } catch (e) {
      setToast(e instanceof Error ? e.message : '扫描失败')
    }
  }

  /** 扫描弹窗里点了某台机器 → 加进设备表（已在列表里的会标"已加入"，不重复加） */
  const addScanned = async (host: LanHost) => {
    if (devices.some((d) => d.ip === host.ip)) {
      setToast(`${host.name || host.ip} 已经在设备列表里`)
      return
    }
    try {
      await api.saveDevice(0, host.name || host.ip, host.ip)
      setToast(`已添加 ${host.name || host.ip}`)
      await load()
    } catch (e) {
      setToast('添加失败: ' + (e instanceof Error ? e.message : ''))
    }
  }

  /**
   * 删设备。这台设备上还挂着服务时，后端会返回 409 并列出那些服务 ——
   * 把清单弹给用户看清楚，确认后带 cascade 再删一次（连带删掉那些服务）。
   *
   * 为什么要连带删：设备删了，挂在它上面的服务就成了孤儿 —— 配置还在、
   * 但永远匹配不到设备，于是**静默地不再运行**，比直接删掉更难查。
   */
  const remove = async (d: Device) => {
    try {
      await api.deleteDevice(d.id)
      setToast('已删除')
      await load()
      return
    } catch (e) {
      const msg = e instanceof Error ? e.message : ''
      // 不是"上面还有服务"的错误 → 真失败，直接报
      if (!msg.includes('这台设备上还有')) {
        setToast(msg || '删除失败')
        return
      }
      setDeleteConfirm({ open: true, device: d, detail: msg })
    }
  }

  /** 用户在确认框里点了「一并删除」 */
  const removeCascade = async () => {
    const d = deleteConfirm.device
    if (!d) return
    setDeleting(true)
    try {
      await api.deleteDevice(d.id, true)
      setDeleteConfirm({ open: false, detail: '' })
      setToast('已删除设备及其服务')
      await load()
    } catch (e) {
      setToast(e instanceof Error ? e.message : '删除失败')
      setDeleteConfirm({ open: false, detail: '' })
    } finally {
      setDeleting(false)
    }
  }

  return (
    <div className="space-y-4">
      <Card className="!p-0">
        <div className="px-5 pt-4">
          <CardHeader
            title={
              <span>
                设备 <span className="ml-1 font-normal text-muted">{devices.length}</span>
              </span>
            }
            action={
              <div className="flex items-center gap-1.5">
                {/* 全局扫描在设备页：找出网段里有哪些机器 */}
                <button
                  onClick={scan}
                  className="flex items-center gap-1 rounded-md px-2.5 py-1 text-xs font-semibold text-blue-600 ring-1 ring-blue-200 transition hover:bg-blue-50"
                  title="扫描局域网，把发现的机器加进来"
                >
                  <Radar className="h-3 w-3" /> 扫描
                </button>
                <button
                  onClick={() => setModal({ open: true })}
                  className="flex items-center gap-1 rounded-md bg-blue-500 px-2.5 py-1 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 transition hover:bg-blue-600"
                >
                  <Plus className="h-3 w-3" /> 添加设备
                </button>
              </div>
            }
          />
        </div>
        <div className="px-5 pb-5">
          <div className="mb-3 rounded-xl bg-surface-2 px-3 py-2.5 text-[11px] leading-relaxed text-muted ring-1 ring-line/70">
            设备按 IP 标识。加好之后，内网穿透、反向代理、端口转发都能直接选它 ——
            同一台机器不用在三处各填一遍。
            <br />
            服务是靠后端地址里的 IP 自动归属到设备的，改了 IP 会跟着换设备。
          </div>

          {devices.length === 0 ? (
            <div className="rounded-xl border border-dashed border-line py-10 text-center">
              <Server className="mx-auto h-8 w-8 text-muted" />
              <p className="mt-2 text-sm text-muted">还没有设备</p>
              <p className="mt-1 text-[11px] text-muted">
                先加一台（比如你的群晖、软路由），之后才能在它上面加服务
              </p>
              <button
                onClick={() => setModal({ open: true })}
                className="mt-3 rounded-md bg-blue-500 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-blue-600"
              >
                添加第一台
              </button>
            </div>
          ) : (
            <div className="grid grid-cols-[repeat(auto-fill,minmax(min(18rem,100%),1fr))] gap-3">
              {devices.map((d) => (
                <div
                  key={d.id}
                  draggable
                  onDragStart={() => setDragId(d.id)}
                  onDragEnd={() => {
                    setDragId(0)
                    setOverId(0)
                  }}
                  onDragOver={(e) => {
                    e.preventDefault()
                    if (d.id !== overId) setOverId(d.id)
                  }}
                  onDrop={(e) => {
                    e.preventDefault()
                    if (dragId) void commitReorder(dragId, d.id)
                  }}
                  className={`transition ${
                    overId === d.id && dragId !== d.id ? 'ring-2 ring-blue-300' : ''
                  } ${dragId === d.id ? 'opacity-40' : ''}`}
                >
                  <DeviceCard device={d} onEdit={() => setModal({ open: true, initial: d })} onDelete={() => remove(d)} />
                </div>
              ))}
            </div>
          )}
        </div>
      </Card>

      {toast && (
        <div className="rounded-xl bg-emerald-50 px-3 py-2 text-xs text-emerald-600">{toast}</div>
      )}

      {modal.open && (
        <DeviceModal
          initial={modal.initial}
          onCancel={() => setModal({ open: false })}
          onSubmit={async () => {
            setModal({ open: false })
            setToast('已保存')
            await load()
          }}
        />
      )}

      {deleteConfirm.open && deleteConfirm.device && (
        <div className={modalBackdrop} onMouseDown={(e) => { if (e.target === e.currentTarget) setDeleteConfirm({ open: false, detail: '' }) }}>
          <div className="w-full max-w-md rounded-2xl bg-surface p-5 shadow-xl ring-1 ring-line">
            <h3 className="mb-3 text-sm font-bold text-content">
              删除设备「{deleteConfirm.device.name}」
            </h3>
            {/* detail 是后端返回的清单，逐条列出将被连带删掉的服务 */}
            <pre className="max-h-56 overflow-auto whitespace-pre-wrap rounded-xl bg-rose-50 p-3 text-[11px] leading-relaxed text-rose-700">
{deleteConfirm.detail}
            </pre>
            <div className="mt-4 flex justify-end gap-2">
              <button
                onClick={() => setDeleteConfirm({ open: false, detail: '' })}
                className="rounded-lg px-3 py-1.5 text-xs font-semibold text-content ring-1 ring-line transition hover:bg-surface-2"
              >
                取消
              </button>
              <button
                onClick={removeCascade}
                disabled={deleting}
                className="rounded-lg bg-rose-500 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-rose-600 disabled:opacity-50"
              >
                {deleting ? '删除中…' : '确认，一并删除'}
              </button>
            </div>
          </div>
        </div>
      )}

      {scanModal.open && (
        <LanScanModal
          subnets={scanModal.subnets}
          takenPorts={new Set<string>()}
          addedHosts={new Map(devices.map((d) => [d.ip, d.name]))}
          onCancel={() => setScanModal({ open: false, subnets: [] })}
          onPick={(h) => void addScanned(h)}
          onPickPort={() => {
            /* 设备页只加设备，不关心端口 */
          }}
        />
      )}
    </div>
  )
}

function DeviceCard({
  device,
  onEdit,
  onDelete,
}: {
  device: Device
  onEdit: () => void
  onDelete: () => void
}) {
  return (
    <div className="flex flex-col rounded-xl border border-line/80 p-3 transition hover:border-blue-200 hover:shadow-md">
      <div className="mb-2 flex items-center gap-2 border-b border-line pb-2">
        {/* 手柄：让"可以拖动"看得出来，光靠光标变化不够明显 */}
        <span className="cursor-grab text-muted" title="按住拖动可以排序">
          <GripVertical className="h-4 w-4" />
        </span>
        <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-blue-50 text-blue-500">
          <HardDrive className="h-4 w-4" />
        </span>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-bold text-content">{device.name}</p>
          <p className="font-mono text-[11px] text-muted">{device.ip}</p>
        </div>
        <button
          onClick={onEdit}
          className="grid h-6 w-6 place-items-center rounded-md bg-surface-2 text-content transition hover:bg-fill"
          title="编辑"
        >
          <Pencil className="h-3 w-3" />
        </button>
        <button
          onClick={onDelete}
          className="grid h-6 w-6 place-items-center rounded-md bg-rose-50 text-rose-500 transition hover:bg-rose-100"
          title="删除"
        >
          <Trash2 className="h-3 w-3" />
        </button>
      </div>
      <DeviceServices ip={device.ip} />
    </div>
  )
}

/**
 * 这台设备上跑了什么。
 *
 * 三个功能的服务都按 IP 归到这里 —— 这是"设备共用一份"最直观的收益：
 * 一台机器上跑了什么，一眼看全，不用去三个页面各看一遍。
 */
function DeviceServices({ ip }: { ip: string }) {
  const [stun, setStun] = useState<StunConfig | null>(null)
  const [proxy, setProxy] = useState<ProxyConfig | null>(null)

  useEffect(() => {
    void (async () => {
      try {
        setStun(await api.getStunConfig())
      } catch {
        /* 没有穿透配置就算了 */
      }
      try {
        setProxy(await api.getProxyConfig())
      } catch {
        /* 同上 */
      }
    })()
  }, [])

  const items = useMemo(() => {
    const out: { kind: string; label: string; port?: string }[] = []

    // 穿透服务（设备 IP 对上就是这台机器的）
    for (const d of stun?.devices ?? []) {
      if (d.ip !== ip) continue
      for (const s of d.services ?? []) {
        out.push({ kind: '穿透', label: s.name || '未命名服务', port: String(s.internalPort) })
      }
    }
    // 反代站点（backend 的 IP）
    for (const s of proxy?.sites ?? []) {
      const host = String(s.backend ?? '').split(':')[0]
      if (host !== ip) continue
      out.push({ kind: '站点', label: s.hosts?.[0] || '未命名站点', port: String(s.backend ?? '').split(':')[1] })
    }
    // 端口转发（target 的 IP）
    for (const f of (proxy as unknown as { forwards?: { target: string; description?: string; listenPort: number }[] })?.forwards ?? []) {
      const host = String(f.target ?? '').split(':')[0]
      if (host !== ip) continue
      out.push({ kind: '转发', label: f.description || `端口 ${f.listenPort}`, port: String(f.listenPort) })
    }
    return out
  }, [stun, proxy, ip])

  if (items.length === 0) {
    return <p className="text-[11px] text-muted">这台机器上还没有服务</p>
  }

  return (
    <div className="space-y-1">
      {items.map((it, i) => (
        <div key={i} className="flex items-center gap-1.5 text-[11px]">
          <span className="shrink-0 rounded bg-surface-2 px-1 py-0.5 font-bold text-muted">
            {it.kind}
          </span>
          <span className="min-w-0 flex-1 truncate text-content">{it.label}</span>
          {it.port && <span className="font-mono text-muted">:{it.port}</span>}
        </div>
      ))}
    </div>
  )
}

function DeviceModal({
  initial,
  onCancel,
  onSubmit,
}: {
  initial?: Device
  onCancel: () => void
  onSubmit: () => void
}) {
  const [name, setName] = useState(initial?.name ?? '')
  const [ip, setIp] = useState(initial?.ip ?? '')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!ip.trim()) {
      setErr('IP 必填')
      return
    }
    setBusy(true)
    setErr('')
    try {
      await api.saveDevice(initial?.id ?? 0, name, ip)
      onSubmit()
    } catch (e2) {
      setErr(e2 instanceof Error ? e2.message : '保存失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/30 p-4" onClick={onCancel}>
      <div className="w-full max-w-sm rounded-2xl bg-surface p-5 shadow-xl" onClick={(e) => e.stopPropagation()}>
        <h3 className="mb-3 text-sm font-bold text-content">{initial ? '编辑设备' : '添加设备'}</h3>
        <form onSubmit={submit} className="space-y-3">
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-muted">名字</div>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="群晖 NAS（留空就用 IP）"
              className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-muted">IP</div>
            <input
              value={ip}
              onChange={(e) => setIp(e.target.value)}
              placeholder="192.168.1.20"
              className="w-full rounded-xl border border-line bg-surface px-3 py-2 font-mono text-sm outline-none focus:border-blue-400"
            />
          </label>
          {err && <div className="rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>}
          <div className="flex justify-end gap-2">
            <button type="button" onClick={onCancel} className="rounded-xl px-4 py-2 text-sm text-muted transition hover:bg-surface-2">
              取消
            </button>
            <button
              type="submit"
              disabled={busy}
              className="rounded-xl bg-blue-500 px-4 py-2 text-sm font-semibold text-white transition hover:bg-blue-600 disabled:opacity-50"
            >
              {busy ? '保存中…' : '保存'}
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}
