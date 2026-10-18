import { useState } from 'react'
import { X } from 'lucide-react'
import { modalBackdrop } from '../../components/modal'
import type { Certificate } from '../../types'

export function UploadModal({
  cert,
  onCancel,
  onSubmit,
}: {
  cert: Certificate
  onCancel: () => void
  onSubmit: (certPem: string, keyPem: string) => Promise<void>
}) {
  const [certPem, setCertPem] = useState('')
  const [keyPem, setKeyPem] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const submit = async () => {
    if (!certPem.trim() || !keyPem.trim()) {
      setErr('证书和私钥都要填')
      return
    }
    setErr('')
    setBusy(true)
    try {
      await onSubmit(certPem, keyPem)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '上传失败')
      setBusy(false)
    }
  }

  const box =
    'w-full rounded-xl border border-line bg-surface px-3 py-2 font-mono text-[11px] leading-relaxed outline-none focus:border-blue-400'

  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="max-h-full w-full max-w-2xl overflow-y-auto rounded-2xl bg-surface p-6 text-content shadow-2xl ring-1 ring-line">
        <div className="mb-5 flex items-center justify-between">
          <div className="text-base font-bold text-content">上传 PEM — {cert.name}</div>
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
            <div className="mb-1 text-xs font-semibold text-muted">
              证书链（fullchain）
              <span className="ml-1 font-normal text-muted">含中间证书，否则部分客户端会报证书链不完整</span>
            </div>
            <textarea
              rows={8}
              value={certPem}
              onChange={(e) => setCertPem(e.target.value)}
              placeholder="-----BEGIN CERTIFICATE-----"
              className={box}
            />
          </label>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-muted">私钥</div>
            <textarea
              rows={6}
              value={keyPem}
              onChange={(e) => setKeyPem(e.target.value)}
              placeholder="-----BEGIN PRIVATE KEY-----"
              className={box}
            />
          </label>
          <div className="text-[11px] text-muted">
            私钥只写到本机 data/cert/ 目录（权限 0600），不会出现在任何接口返回里。
          </div>
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
            {busy ? '校验中...' : '上传'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ===================== 主页面 =====================

