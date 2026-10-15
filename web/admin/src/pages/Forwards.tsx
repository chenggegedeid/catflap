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
import { ArrowRight, Pencil, Plus, Trash2 } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import * as api from '../lib/api'
import type { Device, ProxyConfig, ProxyForward } from '../types'

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
                端口转发 <span className="ml-1 font-normal text-slate-400">{list.length}</span>
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
          <div className="mb-3 rounded-xl bg-slate-50 px-3 py-2.5 text-[11px] leading-relaxed text-slate-500 ring-1 ring-slate-200/70">
            把本机某个端口收到的流量原样转到内网某台机器 —— 不看域名，收到什么转什么。
            适合数据库、游戏服务器、监控这类不是 HTTP 的服务。
            <br />
            要按域名分流的去「反向代理」加站点。
          </div>

          {list.length === 0 ? (
            <div className="rounded-xl border border-dashed border-slate-200 py-10 text-center">
              <ArrowRight className="mx-auto h-8 w-8 text-slate-300" />
              <p className="mt-2 text-sm text-slate-500">还没有端口转发</p>
              <p className="mt-1 text-[11px] text-slate-400">
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
  const port = String(f.target ?? '').split(':')[1]
  const dev = devices.find((d) => d.ip === ip)

  return (
    <div className="flex flex-wrap items-center gap-2 rounded-lg bg-slate-50/60 px-2.5 py-2 text-xs">
      {/* 状态点兼启停按钮：和站点/穿透卡片同一套交互 */}
      <button
        onClick={onToggle}
        title={f.enabled ? '点一下停用' : '点一下启用'}
        className={`h-2 w-2 shrink-0 rounded-full transition ${
          f.enabled ? 'bg-emerald-500' : 'bg-slate-300'
        }`}
      />
      <span className="font-mono font-semibold text-slate-700">:{f.listenPort}</span>
      <ArrowRight className="h-3 w-3 shrink-0 text-slate-300" />
      <span className="min-w-0 flex-1 truncate text-slate-600">
        {/* 目标按 IP 归属到设备，有名字就显示名字 */}
        {dev ? dev.name : ip}
        <span className="ml-1 font-mono text-slate-400">:{port}</span>
      </span>
      <span className="shrink-0 rounded bg-slate-100 px-1 py-0.5 text-[10px] font-bold text-slate-500">
        {f.protocol || 'tcp'}
      </span>
      {!f.enabled && (
        <span className="shrink-0 rounded bg-slate-100 px-1.5 py-0.5 text-[10px] font-bold text-slate-500">
          已停用
        </span>
      )}
      {f.description && (
        <span className="shrink-0 text-[11px] text-slate-400">{f.description}</span>
      )}
      <button
        onClick={onEdit}
        className="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-slate-100 text-slate-600 transition hover:bg-slate-200"
        title="编辑"
      >
        <Pencil className="h-3 w-3" />
      </button>
      <button
        onClick={onRemove}
        className="grid h-6 w-6 shrink-0 place-items-center rounded-md bg-rose-50 text-rose-500 transition hover:bg-rose-100"
        title="删除"
      >
        <Trash2 className="h-3 w-3" />
      </button>
    </div>
  )
}

function ForwardModal({
  initial,
  onCancel,
  onSubmit,
}: {
  initial?: ProxyForward
  onCancel: () => void
  onSubmit: () => void
}) {
  const [listenPort, setListenPort] = useState(
    initial?.listenPort ? String(initial.listenPort) : '',
  )
  const [target, setTarget] = useState(initial?.target ?? '')
  const [protocol, setProtocol] = useState(initial?.protocol || 'tcp')
  const [enabled, setEnabled] = useState(initial?.enabled ?? true)
  const [description, setDescription] = useState(initial?.description ?? '')
  const [err, setErr] = useState('')
  const [busy, setBusy] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setErr('')
    try {
      await api.saveForward({
        id: initial?.id,
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
        className="w-full max-w-md rounded-2xl bg-white p-5 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="mb-3 text-sm font-bold text-slate-800">
          {initial ? '编辑端口转发' : '添加端口转发'}
        </h3>
        <form onSubmit={submit} className="space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <label className="block">
              <div className="mb-1 text-xs font-semibold text-slate-500">本机监听端口</div>
              <input
                value={listenPort}
                onChange={(e) => setListenPort(e.target.value)}
                placeholder="8081"
                className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 font-mono text-sm outline-none focus:border-blue-400"
              />
            </label>
            <label className="block">
              <div className="mb-1 text-xs font-semibold text-slate-500">协议</div>
              <select
                value={protocol}
                onChange={(e) => setProtocol(e.target.value)}
                className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
              >
                <option value="tcp">TCP</option>
                <option value="udp">UDP</option>
              </select>
            </label>
          </div>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">目标地址</div>
            <input
              value={target}
              onChange={(e) => setTarget(e.target.value)}
              placeholder="192.168.1.20:80"
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 font-mono text-sm outline-none focus:border-blue-400"
            />
            <div className="mt-1 text-[11px] text-slate-400">
              目标那台机器要在「设备」里加过，否则保存时会被拒
            </div>
          </label>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">备注（可选）</div>
            <input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="给外面的监控用"
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="flex items-center gap-2 text-sm text-slate-600">
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
              className="rounded-xl px-4 py-2 text-sm text-slate-500 transition hover:bg-slate-100"
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
