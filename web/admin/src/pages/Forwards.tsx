/**
 * 端口转发：本机端口 → 内网某台机器的某个端口。
 *
 * **独立成一个页面**（和「设备」一样有导航入口），不塞在反向代理里：
 * 站点是"域名 → 服务"，转发是"端口 → 端口"，不看域名。
 * 混在一起时站点这个概念要同时表达两件事，界面也说不清楚
 * （lucky / 爱快都是分开的）。
 *
 * 目标同样按 IP 归属到设备，所以每条转发上能看出是转到哪台机器。
 */
import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { ArrowRight, Pencil, Plus, Power, Radar, Trash2 } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import * as api from '../lib/api'
import type { Device, LanPort, ProxyConfig, ProxyForward } from '../types'
import { CardBadge, ServiceCard, iconBtn } from '../components/ServiceCard'

// 目标端口没填时的默认值（内网服务多在 80）
const DEFAULT_TARGET_PORT = 80

export function Forwards() {
  const [cfg, setCfg] = useState<ProxyConfig | null>(null)
  const [devices, setDevices] = useState<Device[]>([])
  const [modal, setModal] = useState<{ open: boolean; initial?: ProxyForward }>({ open: false })
  const [err, setErr] = useState('')

  const load = useCallback(async () => {
    try {
      setCfg(await api.getProxyConfig())
    } catch {
      /* 保持空，界面上有引导 */
    }
    try {
      setDevices(await api.getDevices())
    } catch {
      /* 同上 */
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const list = (cfg as unknown as { forwards?: ProxyForward[] } | null)?.forwards ?? []

  const remove = async (f: ProxyForward) => {
    if (!window.confirm(`确认删除端口转发「:${f.listenPort}」？`)) return
    try {
      await api.deleteForward(f.id)
      await load()
    } catch (e) {
      setErr(e instanceof Error ? e.message : '删除失败')
    }
  }

  const toggle = async (f: ProxyForward) => {
    try {
      await api.saveForward({ ...f, enabled: !f.enabled })
      await load()
    } catch (e) {
      setErr(e instanceof Error ? e.message : '操作失败')
    }
  }

  return (
    <div className="space-y-4">
      <Card className="!p-0">
        <div className="px-5 pt-4">
          <CardHeader
            title={
              <span>
                端口转发 <span className="ml-1 font-normal text-muted">{list.length}</span>
              </span>
            }
            action={
              <button
                onClick={() => setModal({ open: true })}
                className="flex items-center gap-1 rounded-md bg-blue-500 px-2.5 py-1 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 hover:bg-blue-600"
              >
                <Plus className="h-3 w-3" /> 添加转发
              </button>
            }
          />
        </div>
        <div className="px-5 pb-5">
          <div className="mb-3 rounded-xl bg-surface-2 px-3 py-2.5 text-[11px] leading-relaxed text-muted ring-1 ring-line/70">
            把本机某个端口收到的流量原样转到内网某台机器 —— 不看域名，收到什么转什么。
            适合数据库、游戏服务器、监控这类不是 HTTP 的服务。
            <br />
            要按域名分流的去「反向代理」加站点。
          </div>

          {list.length === 0 ? (
            <div className="rounded-xl border border-dashed border-line py-10 text-center">
              <ArrowRight className="mx-auto h-8 w-8 text-muted" />
              <p className="mt-2 text-sm text-muted">还没有端口转发</p>
              <p className="mt-1 text-[11px] text-muted">
                非 HTTP 的服务（数据库、游戏、监控）用这个
              </p>
              <button
                onClick={() => setModal({ open: true })}
                className="mt-3 rounded-md bg-blue-500 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-blue-600"
              >
                添加第一条
              </button>
            </div>
          ) : (
            <div className="space-y-1.5">
              {list.map((f) => (
                <ForwardRow
                  key={f.id}
                  f={f}
                  devices={devices}
                  onEdit={() => setModal({ open: true, initial: f })}
                  onToggle={() => toggle(f)}
                  onRemove={() => remove(f)}
                />
              ))}
            </div>
          )}

          {err && (
            <div className="mt-2 rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>
          )}
        </div>
      </Card>

      {modal.open && (
        <ForwardModal
          initial={modal.initial}
          devices={devices}
          onCancel={() => setModal({ open: false })}
          onSubmit={async () => {
            setModal({ open: false })
            await load()
          }}
        />
      )}
    </div>
  )
}

function ForwardRow({
  f,
  devices,
  onEdit,
  onToggle,
  onRemove,
}: {
  f: ProxyForward
  devices: Device[]
  onEdit: () => void
  onToggle: () => void
  onRemove: () => void
}) {
  const ip = String(f.target ?? '').split(':')[0]
  const port = String(f.target ?? '').split(':')[1] ?? ''
  // 按 deviceId 找设备（改了 IP 还是同一台），老数据再按 target 里的 IP 兜底
  const dev = api.deviceOf(f, f.target, devices)
  const off = f.enabled === false

  // **和穿透/反代共用同一张卡**（components/ServiceCard）。
  // 转发本来是一行列表，现在和其它两个页面一致 —— 同一套骨架、同一种拖动。
  // 名字：有备注用备注，没有就用「设备名:目标端口」（转发本身没有名称字段）。
  return (
    <ServiceCard
      name={f.description || `${dev ? dev.name : ip}:${port}`}
      state={off ? 'off' : 'running'}
      stateHint={off ? '已停用' : '监听中'}
      icons={
        <>
          <button
            onClick={onToggle}
            title={off ? '点一下启用' : '点一下停用'}
            className={`${iconBtn} ${
              off ? 'bg-fill text-muted hover:bg-fill' : 'bg-emerald-50 text-emerald-600 hover:bg-emerald-100'
            }`}
          >
            <Power className="h-3 w-3" />
          </button>
          <button onClick={onEdit} className={`${iconBtn} bg-surface-2 text-content hover:bg-fill`} title="编辑">
            <Pencil className="h-3 w-3" />
          </button>
          <button onClick={onRemove} className={`${iconBtn} bg-rose-50 text-rose-500 hover:bg-rose-100`} title="删除">
            <Trash2 className="h-3 w-3" />
          </button>
        </>
      }
      subtitle={
        <span className="inline-flex min-w-0 items-center gap-1 font-mono text-xs text-muted">
          <span className="font-semibold text-content">:{f.listenPort}</span>
          <ArrowRight className="h-3 w-3 shrink-0 text-muted" />
          <span className="truncate">
            {dev ? dev.name : ip}
            {port && <span className="text-muted">:{port}</span>}
          </span>
        </span>
      }
      meta={
        <>
          <CardBadge tone="slate" title="协议">
            {(f.protocol || 'tcp').toUpperCase()}
          </CardBadge>
          {off && <CardBadge tone="slate">已停用</CardBadge>}
        </>
      }
    />
  )
}


function ForwardModal({
  initial,
  devices,
  onCancel,
  onSubmit,
}: {
  initial?: ProxyForward
  devices: Device[]
  onCancel: () => void
  onSubmit: () => void
}) {
  const [listenPort, setListenPort] = useState(
    initial?.listenPort ? String(initial.listenPort) : '',
  )
  const [deviceId, setDeviceId] = useState(0)
  const [targetPort, setTargetPort] = useState('')
  const [target, setTarget] = useState(initial?.target ?? '')
  const [hostPorts, setHostPorts] = useState<LanPort[]>([])
  const [scanning, setScanning] = useState(false)
  const [protocol, setProtocol] = useState(initial?.protocol || 'tcp')
  const [enabled, setEnabled] = useState(initial?.enabled ?? true)
  const [description, setDescription] = useState(initial?.description ?? '')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  // 编辑已有转发时：**优先用这条转发自己记的 deviceId**（改过 IP 也还是同一台），
  // 没有（老数据）才退回从 target 的 IP 反查 —— 和列表、反代、穿透同一套归属逻辑。
  useEffect(() => {
    if (!initial?.target || devices.length === 0) return
    const port = String(initial.target).split(':')[1] ?? ''
    const d = api.deviceOf(initial, initial.target, devices)
    if (d) {
      setDeviceId(d.id)
      setTargetPort(port)
    } else if (port) {
      setTargetPort(port)
    }
  }, [devices, initial?.target])

  /** 选设备 → 拼 target；换设备时确认（这是换到另一台机器） */
  const pickDevice = (id: number) => {
    const d = devices.find((x) => x.id === id)
    if (!d) {
      setDeviceId(0)
      return
    }
    const prev = devices.find((x) => x.id === deviceId)
    if (prev && prev.id !== d.id) {
      if (!window.confirm(`要把这条转发从「${prev.name}」换到「${d.name}」吗？`)) return
    }
    setDeviceId(d.id)
    // 端口为空时补默认 80：空端口拼出的 "ip:" 没有端口，转发会失败
    setTarget(`${d.ip}:${targetPort || DEFAULT_TARGET_PORT}`)
    setHostPorts([])
  }

  const setPort = (port: string) => {
    const d = devices.find((x) => x.id === deviceId)
    setTargetPort(port)
    if (d && port) setTarget(`${d.ip}:${port}`)
  }

  /** 扫这台机器开了哪些端口（只探一台） */
  const scanThis = async () => {
    const d = devices.find((x) => x.id === deviceId)
    if (!d) return
    setScanning(true)
    setErr('')
    try {
      const ports = await api.scanHostPorts(d.ip)
      setHostPorts(ports)
      if (ports.length === 0) setErr(`没探到 ${d.name}（${d.ip}）上开放的端口`)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '扫描失败')
    } finally {
      setScanning(false)
    }
  }

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setErr('')
    try {
      await api.saveForward({
        id: initial?.id,
        deviceId,
        listenPort: Number(listenPort),
        target,
        protocol,
        enabled,
        description,
      })
      onSubmit()
    } catch (e2) {
      // 后台端口、端口冲突这些校验错误由后端返回，照实显示
      setErr(e2 instanceof Error ? e2.message : '保存失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="fixed inset-0 z-50 grid place-items-center bg-black/30 p-4" onClick={onCancel}>
      <div
        className="w-full max-w-md rounded-2xl bg-surface p-5 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="mb-3 text-sm font-bold text-content">
          {initial ? '编辑端口转发' : '添加端口转发'}
        </h3>
        <form onSubmit={submit} className="space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <label className="block">
              <div className="mb-1 text-xs font-semibold text-muted">本机监听端口</div>
              <input
                value={listenPort}
                onChange={(e) => setListenPort(e.target.value)}
                placeholder="8081"
                className="w-full rounded-xl border border-line bg-surface px-3 py-2 font-mono text-sm outline-none focus:border-blue-400"
              />
            </label>
            <label className="block">
              <div className="mb-1 text-xs font-semibold text-muted">协议</div>
              <select
                value={protocol}
                onChange={(e) => setProtocol(e.target.value)}
                className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
              >
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
              </select>
            </label>
          </div>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-muted">目标</div>
            <div className="flex flex-wrap gap-2">
              <select
                value={deviceId || ''}
                onChange={(e) => pickDevice(Number(e.target.value) || 0)}
                className="min-w-0 flex-1 rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
              >
                <option value="">选择设备…</option>
                {devices.map((d) => (
                  <option key={d.id} value={d.id}>
                    {d.name}（{d.ip}）
                  </option>
                ))}
              </select>
              <input
                value={targetPort}
                onChange={(e) => setPort(e.target.value)}
                placeholder="端口"
                inputMode="numeric"
                className="w-24 shrink-0 rounded-xl border border-line bg-surface px-3 py-2 font-mono text-sm outline-none focus:border-blue-400"
              />
              <button
                type="button"
                onClick={scanThis}
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
                    onClick={() => setPort(String(p.port))}
                    className="rounded bg-surface px-2 py-1 text-[11px] font-semibold text-content ring-1 ring-line transition hover:bg-blue-50 hover:text-blue-600"
                  >
                    {p.port}
                    {p.name && <span className="ml-1 opacity-70">{p.name}</span>}
                  </button>
                ))}
              </div>
            )}
            <div className="mt-1 text-[11px] text-muted">
              选设备 + 填端口。机器没在列表里就去「设备」页加一台。
            </div>
          </label>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-muted">备注（可选）</div>
            <input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="给外面的监控用"
              className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="flex items-center gap-2 text-sm text-content">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
              className="h-3.5 w-3.5"
            />
            启用
          </label>
          {err && <div className="rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>}
          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={onCancel}
              className="rounded-xl px-4 py-2 text-sm text-muted transition hover:bg-surface-2"
            >
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
