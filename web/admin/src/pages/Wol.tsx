import { useEffect, useMemo, useState } from 'react'
import { Plus, Power, Trash2 } from 'lucide-react'

import { Card } from '../components/Card'
import { modalBackdrop } from '../components/modal'
import * as api from '../lib/api'

/**
 * 网络唤醒（WOL）。
 *
 * 页面上刻意把「广播地址」这一栏说得很细：唤醒包是靠二层广播送到目标网卡的，
 * 目标此刻没有 IP、也不响应 ARP，填它的 IP 地址是**送不到**的。
 * 这个坑的表现是「点了没反应、也没报错」，不解释清楚用户只会以为功能坏了。
 */
export function Wol() {
  const [targets, setTargets] = useState<api.WolTarget[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      const res = await api.getWolTargets()
      setTargets(res.targets ?? [])
    } catch (e) {
      setError(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const wake = async (t: api.WolTarget) => {
    setNotice('')
    setError('')
    try {
      const res = await api.wakeWolTarget(t.id)
      // 后端明确返回「发了，但不保证醒」，这里原样转述给用户
      setNotice(res.data?.hint ?? `已向 ${t.name} 发送唤醒包`)
    } catch (e) {
      setError(e instanceof Error ? e.message : '唤醒失败')
    }
  }

  const remove = async (t: api.WolTarget) => {
    if (!confirm(`确定删除「${t.name}」？`)) return
    try {
      await api.deleteWolTarget(t.id)
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : '删除失败')
    }
  }

  const sorted = useMemo(
    () => [...targets].sort((a, b) => a.name.localeCompare(b.name, 'zh')),
    [targets],
  )

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-lg font-bold text-content">网络唤醒</h2>
        <p className="mt-1 text-xs text-muted">
          发一个魔术包把局域网里的机器叫醒。需要先在目标机器的 BIOS / 网卡设置里打开 WOL。
        </p>
      </div>

      {error && (
        <div className="rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">
          {error}
        </div>
      )}
      {notice && (
        <div className="rounded-md border border-blue-200 bg-blue-50 px-3 py-2 text-xs text-blue-700">
          {notice}
        </div>
      )}

      <Card>
        {loading ? (
          <div className="px-4 py-8 text-center text-xs text-muted">加载中…</div>
        ) : sorted.length === 0 ? (
          <div className="px-4 py-8 text-center text-xs text-muted">
            还没有添加任何机器。点右上角「添加机器」开始。
          </div>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-line text-left text-xs text-muted">
                <th className="px-4 py-2 font-semibold">名称</th>
                <th className="px-4 py-2 font-semibold">MAC</th>
                <th className="px-4 py-2 font-semibold">广播地址</th>
                <th className="px-4 py-2 font-semibold">端口</th>
                <th className="px-4 py-2 text-right font-semibold">操作</th>
              </tr>
            </thead>
            <tbody>
              {sorted.map((t) => (
                <tr key={t.id} className="border-b border-line last:border-0">
                  <td className="px-4 py-2.5">
                    <div className="font-medium text-content">{t.name}</div>
                    {t.note && <div className="mt-0.5 text-xs text-muted">{t.note}</div>}
                  </td>
                  <td className="px-4 py-2.5 font-mono text-xs text-content">{t.mac}</td>
                  <td className="px-4 py-2.5 font-mono text-xs text-content">{t.broadcast}</td>
                  <td className="px-4 py-2.5 text-xs text-content">{t.port || 9}</td>
                  <td className="px-4 py-2.5">
                    <div className="flex items-center justify-end gap-2">
                      <button
                        type="button"
                        onClick={() => void wake(t)}
                        className="rounded-md border border-line px-2.5 py-1.5 text-xs font-semibold text-content transition hover:bg-surface-2"
                        title="发送唤醒包"
                      >
                        <Power className="h-3.5 w-3.5" />
                        唤醒
                      </button>
                      <button
                        type="button"
                        onClick={() => void remove(t)}
                        className="rounded-md border border-line px-2 py-1.5 text-xs text-muted transition hover:border-rose-200 hover:text-rose-600"
                        title="删除"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
          </div>
        )}
      </Card>

      <AddTargetModal onSaved={load} />
    </div>
  )
}

/**
 * 添加机器。只做新增，不做编辑 —— 编辑在这个场景里价值很低：
 * 换网卡改一次 MAC，删掉重加比打开编辑框更快，还少一套表单状态要维护。
 */
function AddTargetModal({ onSaved }: { onSaved: () => void }) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [mac, setMac] = useState('')
  const [broadcast, setBroadcast] = useState('192.168.1.255')
  const [port, setPort] = useState('9')
  const [note, setNote] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const submit = async () => {
    setSaving(true)
    setError('')
    try {
      const p = parseInt(port, 10)
      await api.addWolTarget({
        id: '',
        name,
        mac,
        broadcast,
        port: Number.isFinite(p) ? p : 9,
        note,
      })
      setName('')
      setMac('')
      setNote('')
      setOpen(false)
      onSaved()
    } catch (e) {
      setError(e instanceof Error ? e.message : '添加失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <div className="flex justify-end">
        <button type="button" onClick={() => setOpen(true)} className="flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-2 text-xs font-semibold text-white shadow-sm transition hover:bg-blue-700 disabled:opacity-60">
          <Plus className="h-4 w-4" />
          添加机器
        </button>
      </div>

      {open && (
        <div
          className={modalBackdrop}
          onMouseDown={(e) => {
            if (e.target === e.currentTarget) setOpen(false)
          }}
        >
          <div className="w-full max-w-lg rounded-lg bg-surface p-5 shadow-2xl ring-1 ring-line">
            <div className="text-base font-bold text-content">添加机器</div>

            <div className="mt-4 space-y-3">
              <Field label="名称" value={name} onChange={setName} placeholder="书房台式机" />
              <Field
                label="MAC 地址"
                value={mac}
                onChange={setMac}
                placeholder="00:11:22:33:44:55"
                hint="冒号、横线、点分隔都认，也可以连着写 12 位"
              />
              <Field
                label="广播地址"
                value={broadcast}
                onChange={setBroadcast}
                placeholder="192.168.1.255"
                hint="⚠️ 填子网的广播地址，不是机器的 IP —— 唤醒包靠二层广播送达，机器此刻没有 IP、也不响应 ARP。跨网段的话广播过不去，需要在目标网段里放一个转发器。"
              />
              <Field label="端口" value={port} onChange={setPort} placeholder="9" />
              <Field label="备注" value={note} onChange={setNote} placeholder="选填" />
            </div>

            {error && (
              <div className="mt-3 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">
                {error}
              </div>
            )}

            <div className="mt-5 flex items-center justify-end gap-2">
              <button type="button" onClick={() => setOpen(false)} className="rounded-md border border-line px-2.5 py-1.5 text-xs font-semibold text-content transition hover:bg-surface-2">
                取消
              </button>
              <button
                type="button"
                onClick={() => void submit()}
                disabled={saving || !name || !mac || !broadcast}
                className="flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-2 text-xs font-semibold text-white shadow-sm transition hover:bg-blue-700 disabled:opacity-60"
              >
                {saving ? '保存中...' : '保存'}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}

function Field({
  label,
  value,
  onChange,
  placeholder,
  hint,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  placeholder?: string
  hint?: string
}) {
  return (
    <label className="block">
      <span className="text-xs font-semibold text-content">{label}</span>
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="mt-1 w-full rounded-md border border-line px-3 py-2 text-sm outline-none transition focus:border-blue-400"
      />
      {hint && <span className="mt-1 block text-xs text-muted">{hint}</span>}
    </label>
  )
}
