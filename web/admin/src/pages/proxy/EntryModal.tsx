import { useState } from 'react'
import * as api from '../../lib/api'
import type { Certificate, ProxyEntry } from '../../types'
import { BlockedPortWarn, CertSelect, Modal } from './Modal'
import { errText, inputCls, labelCls, portBlocked } from './shared'

export function EntryModal({
  entry,
  certs,
  onCancel,
  onSubmit,
}: {
  entry: ProxyEntry
  certs: Certificate[]
  onCancel: () => void
  onSubmit: (payload: Omit<api.EntryPayload, 'enabled'>) => Promise<void>
}) {
  const [httpPort, setHttpPort] = useState(entry.httpPort ? String(entry.httpPort) : '')
  const [httpsPort, setHttpsPort] = useState(entry.httpsPort ? String(entry.httpsPort) : '')
  const [certId, setCertId] = useState(String(entry.certId || 0))
  // ?? ''：老后端不回这个字段，少了它下面每个 .trim() 都会炸
  const [baseDomain, setBaseDomain] = useState(entry.baseDomain ?? '')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const httpsOn = (Number(httpsPort) || 0) > 0

  const submit = async () => {
    setErr('')
    setBusy(true)
    try {
      await onSubmit({
        httpPort: Number(httpPort) || 0,
        httpsPort: Number(httpsPort) || 0,
        certId: httpsOn ? Number(certId) || 0 : 0,
        baseDomain: baseDomain.trim(),
      })
    } catch (e) {
      setErr(errText(e))
      setBusy(false)
    }
  }

  return (
    <Modal
      title="默认入口"
      onCancel={onCancel}
      footer={
        <>
          <button
            type="button"
            onClick={onCancel}
            className="rounded-xl px-4 py-2 text-sm text-muted transition hover:bg-surface-2"
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
        </>
      }
    >
      <div className="rounded-xl bg-surface-2 px-3 py-2.5 text-[11px] leading-relaxed text-muted ring-1 ring-line/70">
        没单独指定端口的服务都挂在这两个端口上，靠域名区分——
        就是 nginx 里一堆 server 块共用 <span className="font-mono">listen 80</span> /{' '}
        <span className="font-mono">listen 443 ssl</span> 的那个意思
      </div>

      <div className="rounded-xl bg-blue-50/70 px-3 py-2.5 text-[11px] leading-relaxed text-content ring-1 ring-blue-200/70">
        <div className="mb-1 font-semibold text-content">想「只开一个端口」就填这儿</div>
        把 HTTPS 端口填成一个固定的高位端口（比如 <span className="font-mono">8443</span>），
        所有二级域名的服务都不指定端口、挂上来按域名分发 —— 对外就只有这一个端口要放通。
        <div className="mt-1.5">
          本机有公网 IPv6 时（没有 IPv4 公网、只有 IPv6 的家宽很常见），
          <span className="font-semibold">这个端口直接就能从外网访问，不需要打洞</span>：
          在「DDNS 域名解析」里加一条 <span className="font-mono">AAAA</span> 记录、
          IP 来源选「本地网卡」，再把域名的 IPv6 解析指向本机即可。
          路由器上记得放行这个端口的 IPv6 入站。
        </div>
      </div>

      <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
        <label className="block">
          <div className={labelCls}>HTTP 端口</div>
          <input
            autoFocus
            value={httpPort}
            onChange={(e) => setHttpPort(e.target.value)}
            placeholder="80"
            className={`${inputCls} font-mono`}
          />
        </label>
        <label className="block">
          <div className={labelCls}>HTTPS 端口</div>
          <input
            value={httpsPort}
            onChange={(e) => setHttpsPort(e.target.value)}
            placeholder="443"
            className={`${inputCls} font-mono`}
          />
        </label>
      </div>
      <div className="-mt-1 text-[11px] leading-relaxed text-muted">
        留空表示不开这个入口，两个都留空就没入口了。1024 以下的端口要管理员权限
      </div>

      {portBlocked(Number(httpPort)) && <BlockedPortWarn label="HTTP 端口" port={Number(httpPort)} />}
      {portBlocked(Number(httpsPort)) && <BlockedPortWarn label="HTTPS 端口" port={Number(httpsPort)} />}

      {httpsOn && (
        <label className="block">
          <div className={labelCls}>默认证书</div>
          <CertSelect value={certId} certs={certs} onChange={setCertId} />
          <div className="mt-1 text-[11px] leading-relaxed text-muted">
            自动匹配会按访问的域名挑证书。指定一张就是所有站点都用它，证书得盖住全部域名——
            <span className="font-mono">*.{baseDomain.trim() || 'example.com'}</span> 这种通配最省事
          </div>
        </label>
      )}

      <label className="block">
        <div className={labelCls}>主域名（可选）</div>
        <input
          value={baseDomain}
          onChange={(e) => setBaseDomain(e.target.value)}
          placeholder="example.com"
          className={`${inputCls} font-mono`}
        />
        <div className="mt-1 text-[11px] leading-relaxed text-muted">
          填了之后加站点只写前缀：<span className="font-mono">nas</span> 就是{' '}
          <span className="font-mono">nas.{baseDomain.trim() || 'example.com'}</span>。写全域名的照样认
        </div>
      </label>

      {err && <div className="rounded-xl bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>}
    </Modal>
  )
}

// ===================== 站点 =====================

