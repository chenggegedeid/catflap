import { useState } from 'react'
import { X } from 'lucide-react'
import { modalBackdrop } from '../../components/modal'
import { findProviderType } from '../../lib/providerTypes'
import type { DdnsProvider } from '../../types'
import { type ProviderFormState, type ProviderTypeSpec } from './shared'

export function ProviderModal({
  types,
  initial,
  onCancel,
  onSubmit,
}: {
  /** 可选服务商类型表，由后端注册表给出 */
  types: ProviderTypeSpec[]
  initial?: DdnsProvider
  onCancel: () => void
  onSubmit: (form: ProviderFormState) => Promise<void>
}) {
  const [form, setForm] = useState<ProviderFormState>(() => ({
    name: initial?.name ?? '',
    type: initial?.type ?? types[0]?.type ?? '',
    credential: {},
  }))
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const meta = findProviderType(types, form.type) ?? types[0]
  const isEdit = !!initial

  const submit = async () => {
    if (!form.name.trim()) {
      setErr('请填写服务商名称')
      return
    }
    if (!meta) {
      setErr('服务商列表还没加载出来，请稍后重试')
      return
    }
    // 新增时所有字段必填；编辑时留空表示沿用原凭证
    if (!isEdit) {
      for (const f of meta.fields) {
        if (!(form.credential[f.key] ?? '').trim()) {
          setErr(`请填写${f.label}`)
          return
        }
      }
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
          <div className="text-base font-bold text-content">{initial ? '编辑服务商' : '添加服务商'}</div>
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
            <div className="mb-1 text-xs font-semibold text-muted">服务商名称</div>
            <input
              autoFocus
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              placeholder="如 我的 Cloudflare"
              className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-muted">服务商类型</div>
            <select
              value={form.type}
              disabled={isEdit || types.length === 0}
              onChange={(e) =>
                setForm((p) => ({ ...p, type: e.target.value as DdnsProvider['type'], credential: {} }))
              }
              className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-surface-2 disabled:text-muted"
            >
              {types.length === 0 && <option value="">正在读取服务商列表…</option>}
              {types.map((m) => (
                <option key={m.type} value={m.type}>
                  {m.label}
                </option>
              ))}
            </select>
            {isEdit && <div className="mt-1 text-[11px] text-muted">类型创建后不可更改</div>}
            {meta?.docsUrl && (
              <a
                href={meta.docsUrl}
                target="_blank"
                rel="noreferrer"
                className="mt-1 inline-block text-[11px] text-blue-500 hover:underline"
              >
                去哪拿这些凭据 →
              </a>
            )}
          </label>
          {(meta?.fields ?? []).map((f) => (
            <label key={f.key} className="block">
              <div className="mb-1 text-xs font-semibold text-muted">
                {f.label}
                {isEdit && <span className="ml-1 font-normal text-muted">（留空表示不修改）</span>}
              </div>
              <input
                type={f.secret ? 'password' : 'text'}
                value={form.credential[f.key] ?? ''}
                onChange={(e) =>
                  setForm((p) => ({ ...p, credential: { ...p.credential, [f.key]: e.target.value } }))
                }
                placeholder={isEdit ? '••••••••（已配置）' : f.placeholder ?? f.label}
                className="w-full rounded-xl border border-line bg-surface px-3 py-2 font-mono text-sm outline-none focus:border-blue-400"
              />
              {f.help && <div className="mt-1 text-[11px] leading-relaxed text-muted">{f.help}</div>}
            </label>
          ))}
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

// ===================== 记录弹窗 =====================

