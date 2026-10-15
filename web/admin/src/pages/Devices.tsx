/**
 * 设备：内网里那些机器。
 *
 * **穿透、反向代理、端口转发共用这一份** —— 同一台机器只维护一次。
 * 设备按 IP 标识，服务（站点/穿透服务/端口转发）通过 IP 归属到它。
 *
 * 这个页面是整个流程的起点：先有设备，才能加服务。
 */
import { useCallback, useEffect, useMemo, useState, type FormEvent } from 'react'
import { HardDrive, Pencil, Plus, Server, Trash2 } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import * as api from '../lib/api'
import type { Device, ProxyConfig, StunConfig } from '../types'

export function Devices() {
  const [devices, setDevices] = useState<Device[]>([])
  const [modal, setModal] = useState<{ open: boolean; initial?: Device }>({ open: false })
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

  const remove = async (d: Device) => {
    if (!window.confirm(`确认删除设备「${d.name}」？`)) return
    try {
      await api.deleteDevice(d.id)
      setToast('已删除')
      await load()
    } catch (e) {
      setToast(e instanceof Error ? e.message : '删除失败')
    }
  }

  return (
    <div className="space-y-4">
      <Card className="!p-0">
        <div className="px-5 pt-4">
          <CardHeader
            title={
              <span>
                设备 <span className="ml-1 font-normal text-slate-400">{devices.length}</span>
              </span>
            }
            action={
              <button
                onClick={() => setModal({ open: true })}
                className="flex items-center gap-1 rounded-md bg-blue-500 px-2.5 py-1 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 transition hover:bg-blue-600"
              >
                <Plus className="h-3 w-3" /> 添加设备
              </button>
            }
          />
        </div>
        <div className="px-5 pb-5">
          <div className="mb-3 rounded-xl bg-slate-50 px-3 py-2.5 text-[11px] leading-relaxed text-slate-500 ring-1 ring-slate-200/70">
            设备按 IP 标识。加好之后，内网穿透、反向代理、端口转发都能直接选它 ——
            同一台机器不用在三处各填一遍。
            <br />
            服务是靠后端地址里的 IP 自动归属到设备的，改了 IP 会跟着换设备。
          </div>

          {devices.length === 0 ? (
            <div className="rounded-xl border border-dashed border-slate-200 py-10 text-center">
              <Server className="mx-auto h-8 w-8 text-slate-300" />
              <p className="mt-2 text-sm text-slate-500">还没有设备</p>
              <p className="mt-1 text-[11px] text-slate-400">
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
                <DeviceCard key={d.id} device={d} onEdit={() => setModal({ open: true, initial: d })} onDelete={() => remove(d)} />
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
    <div className="flex flex-col rounded-xl border border-slate-200/80 p-3 transition hover:border-blue-200 hover:shadow-md">
      <div className="mb-2 flex items-center gap-2 border-b border-slate-100 pb-2">
        <span className="grid h-8 w-8 shrink-0 place-items-center rounded-lg bg-blue-50 text-blue-500">
          <HardDrive className="h-4 w-4" />
        </span>
        <div className="min-w-0 flex-1">
          <p className="truncate text-sm font-bold text-slate-800">{device.name}</p>
          <p className="font-mono text-[11px] text-slate-400">{device.ip}</p>
        </div>
        <button
          onClick={onEdit}
          className="grid h-6 w-6 place-items-center rounded-md bg-slate-100 text-slate-600 transition hover:bg-slate-200"
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
    return <p className="text-[11px] text-slate-400">这台机器上还没有服务</p>
  }

  return (
    <div className="space-y-1">
      {items.map((it, i) => (
        <div key={i} className="flex items-center gap-1.5 text-[11px]">
          <span className="shrink-0 rounded bg-slate-100 px-1 py-0.5 font-bold text-slate-500">
            {it.kind}
          </span>
          <span className="min-w-0 flex-1 truncate text-slate-600">{it.label}</span>
          {it.port && <span className="font-mono text-slate-400">:{it.port}</span>}
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
      <div className="w-full max-w-sm rounded-2xl bg-white p-5 shadow-xl" onClick={(e) => e.stopPropagation()}>
        <h3 className="mb-3 text-sm font-bold text-slate-800">{initial ? '编辑设备' : '添加设备'}</h3>
        <form onSubmit={submit} className="space-y-3">
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">名字</div>
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="群晖 NAS（留空就用 IP）"
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">IP</div>
            <input
              value={ip}
              onChange={(e) => setIp(e.target.value)}
              placeholder="192.168.1.20"
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 font-mono text-sm outline-none focus:border-blue-400"
            />
          </label>
          {err && <div className="rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>}
          <div className="flex justify-end gap-2">
            <button type="button" onClick={onCancel} className="rounded-xl px-4 py-2 text-sm text-slate-500 transition hover:bg-slate-100">
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
