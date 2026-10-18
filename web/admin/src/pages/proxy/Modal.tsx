import type { ReactNode } from 'react'
import { TriangleAlert, X } from 'lucide-react'
import { modalBackdrop } from '../../components/modal'
import type { Certificate } from '../../types'

export function Warn({ children }: { children: ReactNode }) {
  return (
    <div className="flex gap-2 rounded-xl bg-amber-50 px-3 py-2.5 text-[11px] leading-relaxed text-amber-700 ring-1 ring-amber-200">
      <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
      <span>{children}</span>
    </div>
  )
}

/** 端口落在浏览器禁用名单上时的说辞，两个表单共用 */
export function BlockedPortWarn({ label, port }: { label: string; port: number }) {
  return (
    <Warn>
      {label} <span className="font-mono font-semibold">{port}</span>{' '}
      在浏览器的禁用端口名单里，Chrome / Firefox 会直接拒连，报{' '}
      <span className="font-mono">ERR_UNSAFE_PORT</span>。反代照样监听得好好的、
      <span className="font-mono">curl</span> 也能通，只有浏览器进不来——换一个端口
    </Warn>
  )
}

/**
 * 站点对外的地址：自己占端口就用那个，否则挂在默认入口上。
 *
 * 挂默认入口且勾了 HTTPS 时，HTTP 入口上其实也有一份——
 * 但用户真正会用的是 HTTPS 那个，这里就显示它。
 */
const fieldCls =
  'rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-surface-2 disabled:text-muted'
const inputCls = `w-full ${fieldCls}`

export function Modal({
  title,
  onCancel,
  children,
  footer,
}: {
  title: string
  onCancel: () => void
  children: React.ReactNode
  footer: React.ReactNode
}) {
  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="max-h-full w-full max-w-lg overflow-y-auto rounded-2xl bg-surface p-6 text-content shadow-2xl ring-1 ring-line">
        <div className="mb-5 flex items-center justify-between">
          <div className="text-base font-bold text-content">{title}</div>
          <button
            type="button"
            onClick={onCancel}
            className="grid h-7 w-7 place-items-center rounded-full text-muted transition hover:bg-surface-2 hover:text-content"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="space-y-3">{children}</div>
        <div className="mt-6 flex justify-end gap-2">{footer}</div>
      </div>
    </div>
  )
}

export function CertSelect({
  value,
  certs,
  onChange,
}: {
  value: string
  certs: Certificate[]
  onChange: (v: string) => void
}) {
  return (
    <select value={value} onChange={(e) => onChange(e.target.value)} className={inputCls}>
      <option value="0">按域名自动匹配</option>
      {certs.map((c) => (
        <option key={c.id} value={String(c.id)}>
          {c.name}
          {c.domains.length > 0 ? `（${c.domains.join('、')}）` : ''}
        </option>
      ))}
    </select>
  )
}

// ===================== 默认入口设置 =====================

