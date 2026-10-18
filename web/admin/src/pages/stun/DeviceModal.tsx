/**
 * 设备弹窗：新增 / 改一台内网机器。
 *
 * 从 Stun.tsx 抽出来 —— 原来和穿透服务那一大坨挤在同一个文件里，
 * 改设备表单要在 3010 行里翻。
 */
import { useState } from 'react'
import { X } from 'lucide-react'
import { modalBackdrop } from '../../components/modal'
import type { StunDevice } from '../../types'

export interface DeviceFormState {
  name: string
  ip: string
}
export const emptyDevice: DeviceFormState = { name: '', ip: '' }

export function DeviceModal({
  initial,
  prefill,
  onCancel,
  onSubmit,
}: {
  initial?: StunDevice
  /** 扫描结果点「添加」时带过来的 IP。只在新增时用，改的时候以 initial 为准 */
  prefill?: DeviceFormState
  onCancel: () => void
  onSubmit: (form: DeviceFormState) => Promise<void>
}) {
  const [form, setForm] = useState<DeviceFormState>(() =>
    initial ? { name: initial.name, ip: initial.ip } : (prefill ?? emptyDevice),
  )
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const submit = async () => {
    if (!form.name.trim() || !form.ip.trim()) {
      setErr('设备名称和 IP 不能为空')
      return
    }
    setErr('')
    setBusy(true)
    try {
      await onSubmit(form)
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
      <div className="max-h-full w-full max-w-md overflow-y-auto rounded-2xl bg-surface p-6 text-content shadow-2xl ring-1 ring-line">
        <div className="mb-5 flex items-center justify-between">
          <div className="text-base font-bold text-content">{initial ? '编辑设备' : '添加设备'}</div>
          <button
            type="button"
            onClick={onCancel}
            className="grid h-7 w-7 place-items-center rounded-full text-muted transition hover:bg-surface-2 hover:text-content"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="space-y-3">
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-muted">设备名称</div>
            <input
              autoFocus
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              placeholder="如 群晖NAS / 树莓派"
              className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-muted">设备内网 IP</div>
            <input
              value={form.ip}
              onChange={(e) => setForm((p) => ({ ...p, ip: e.target.value }))}
              placeholder="如 192.168.1.100"
              className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          {err && <div className="text-xs text-rose-500">{err}</div>}
        </div>
        <div className="mt-5 flex justify-end gap-2">
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

/**
 * 局域网扫描。
 *
 * 「添加设备」原本要用户自己知道那台机器的内网 IP——得去路由器后台翻 DHCP 列表，
 * 或者跑去那台机器上敲命令。卡在这一步的人，后面打洞根本轮不到。
 * 这里直接把这段网里的机器摆出来选。
 *
 * 网段列表由外面先取好再传进来，弹窗一开下拉框就是满的。
 */

