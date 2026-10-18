import { useMemo, useState } from 'react'
import { TriangleAlert, X } from 'lucide-react'
import { modalBackdrop } from '../../components/modal'
import * as api from '../../lib/api'
import type { AcmeProvider, CertSource, Certificate } from '../../types'
import {
  DIRECTORIES, HTTP01_WARNING, SELF_SIGNED_NOTE, SOURCES,
  type CertFormState, initialForm, parseDomains, sourceMetaMap,
} from './shared'

export function CertModal({
  initial,
  providers,
  onCancel,
  onSubmit,
}: {
  initial?: Certificate
  providers: AcmeProvider[]
  onCancel: () => void
  onSubmit: (payload: api.CertPayload) => Promise<void>
}) {
  const [form, setForm] = useState<CertFormState>(() => initialForm(initial, providers))
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const meta = sourceMetaMap.get(form.source) ?? SOURCES[0]
  const isDns01 = form.source === 'acme-dns01'
  const isHttp01 = form.source === 'acme-http01'
  const isAcme = !!meta.acme
  const isPath = form.source === 'path'
  const isSelfSigned = form.source === 'self-signed'

  const domains = useMemo(() => parseDomains(form.domainsRaw), [form.domainsRaw])
  const hasWildcard = domains.some((d) => d.startsWith('*.'))

  const submit = async () => {
    if (!form.name.trim()) {
      setErr('请填写证书名称')
      return
    }
    if (isPath && (!form.certFile.trim() || !form.keyFile.trim())) {
      setErr('请填写证书和私钥的文件路径')
      return
    }
    if (isAcme && domains.length === 0) {
      setErr('自动签发至少需要填写一个域名')
      return
    }
    if (isDns01 && !form.providerId) {
      setErr('DNS-01 需要选择一个 DNS 服务商')
      return
    }
    if (isHttp01 && hasWildcard) {
      setErr('HTTP-01 不支持通配符域名，通配符证书请改用 DNS-01')
      return
    }

    const directory = form.directoryChoice === 'custom' ? form.directoryCustom.trim() : form.directoryChoice

    setErr('')
    setBusy(true)
    try {
      await onSubmit({
        name: form.name.trim(),
        source: form.source,
        domains,
        enabled: form.enabled,
        isDefault: form.isDefault,
        certFile: form.certFile.trim(),
        keyFile: form.keyFile.trim(),
        acme: {
          directory,
          email: form.email.trim(),
          providerId: isDns01 ? Number(form.providerId) || 0 : 0,
          httpPort: isHttp01 ? Number(form.httpPort) || 80 : 0,
          eabKeyId: form.eabKeyId.trim(),
          eabHmac: form.eabHmac.trim(),
          renewDays: Number(form.renewDays) || 30,
        },
      })
    } catch (e) {
      setErr(e instanceof Error ? e.message : '操作失败')
      setBusy(false)
    }
  }

  const input =
    'w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-surface-2 disabled:text-muted'
  const labelText = 'mb-1 text-xs font-semibold text-muted'

  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="max-h-full w-full max-w-lg overflow-y-auto rounded-2xl bg-surface p-6 text-content shadow-2xl ring-1 ring-line">
        <div className="mb-5 flex items-center justify-between">
          <div className="text-base font-bold text-content">{initial ? '编辑证书' : '添加证书'}</div>
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
            <div className={labelText}>证书名称</div>
            <input
              autoFocus
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              placeholder="如 主域名通配符"
              className={input}
            />
          </label>

          <label className="block">
            <div className={labelText}>证书来源</div>
            <select
              value={form.source}
              onChange={(e) => {
                // 换来源要顺手清掉报错：上一条错误是针对旧来源的，
                // 留在那里会变成「已经改用 DNS-01 了还在说 HTTP-01 不支持通配符」
                setErr('')
                setForm((p) => ({ ...p, source: e.target.value as CertSource }))
              }}
              className={input}
            >
              {SOURCES.map((s) => (
                <option key={s.value} value={s.value}>
                  {s.label}
                </option>
              ))}
            </select>
            <div className="mt-1.5 text-[11px] leading-relaxed text-muted">{meta.hint}</div>
          </label>

          {/* HTTP-01 的限制是协议层面的，常驻展示，不等报错才说 */}
          {isHttp01 && (
            <div className="flex gap-2 rounded-xl bg-amber-50 px-3 py-2.5 text-[11px] leading-relaxed text-amber-700 ring-1 ring-amber-200">
              <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <span>{HTTP01_WARNING}</span>
            </div>
          )}

          {/* 自签换不来什么，配之前就得知道，别配完了才发现地址栏还是红的 */}
          {isSelfSigned && (
            <div className="flex gap-2 rounded-xl bg-amber-50 px-3 py-2.5 text-[11px] leading-relaxed text-amber-700 ring-1 ring-amber-200">
              <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <span>{SELF_SIGNED_NOTE}</span>
            </div>
          )}

          <label className="block">
            <div className={labelText}>
              覆盖域名
              {isAcme ? '' : isSelfSigned ? '（可以不填）' : '（留空则从证书里自动读取）'}
            </div>
            <textarea
              rows={3}
              value={form.domainsRaw}
              onChange={(e) => setForm((p) => ({ ...p, domainsRaw: e.target.value }))}
              placeholder={'一行一个，如\nexample.com\n*.example.com'}
              className={`${input} font-mono`}
            />
            <div className="mt-1 text-[11px] text-muted">
              {domains.length > 0
                ? `将覆盖 ${domains.length} 个域名：${domains.join('、')}`
                : isSelfSigned
                  ? '不填也行——本机的 IP 会自动写进证书，敲 IP 访问时名字对得上'
                  : '用换行或逗号分隔'}
            </div>
            {hasWildcard && (
              <div className="mt-1 text-[11px] text-muted">
                通配符只覆盖一层：<span className="font-mono">*.example.com</span> 能覆盖{' '}
                <span className="font-mono">a.example.com</span>，但不覆盖{' '}
                <span className="font-mono">example.com</span> 和{' '}
                <span className="font-mono">a.b.example.com</span>
              </div>
            )}
          </label>

          {isPath && (
            <>
              <label className="block">
                <div className={labelText}>证书文件路径（fullchain.pem）</div>
                <input
                  value={form.certFile}
                  onChange={(e) => setForm((p) => ({ ...p, certFile: e.target.value }))}
                  placeholder="/etc/letsencrypt/live/example.com/fullchain.pem"
                  className={`${input} font-mono`}
                />
              </label>
              <label className="block">
                <div className={labelText}>私钥文件路径（privkey.pem）</div>
                <input
                  value={form.keyFile}
                  onChange={(e) => setForm((p) => ({ ...p, keyFile: e.target.value }))}
                  placeholder="/etc/letsencrypt/live/example.com/privkey.pem"
                  className={`${input} font-mono`}
                />
              </label>
            </>
          )}

          {isAcme && (
            <>
              <label className="block">
                <div className={labelText}>ACME 服务</div>
                <select
                  value={form.directoryChoice}
                  onChange={(e) => setForm((p) => ({ ...p, directoryChoice: e.target.value }))}
                  className={input}
                >
                  {DIRECTORIES.map((d) => (
                    <option key={d.value} value={d.value}>
                      {d.label}
                    </option>
                  ))}
                </select>
                {form.directoryChoice === 'custom' && (
                  <input
                    value={form.directoryCustom}
                    onChange={(e) => setForm((p) => ({ ...p, directoryCustom: e.target.value }))}
                    placeholder="https://acme.zerossl.com/v2/DV90"
                    className={`${input} mt-2 font-mono`}
                  />
                )}
                {form.directoryChoice.includes('staging') && (
                  <div className="mt-1 text-[11px] leading-5 text-muted">
                    这里签出来的证书浏览器不认，只用来验证 DNS 权限和域名填对了没有。
                    跑通之后改回正式环境，会重新签一张能用的。
                  </div>
                )}
                {/*
                  正式环境一小时内失败 5 次就签不了了，只能干等。
                  第一次配 DNS-01 十有八九要试几轮，这句得在他动手前说
                */}
                {form.directoryChoice === '' && (
                  <div className="mt-1 text-[11px] leading-5 text-muted">
                    正式环境一小时内验证失败 5 次就会被暂时挡住，只能等。
                    第一次配、或者反复签不出来，先用上面的测试环境试通再回来。
                  </div>
                )}
              </label>

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <label className="block">
                  <div className={labelText}>联系邮箱（可选）</div>
                  <input
                    value={form.email}
                    onChange={(e) => setForm((p) => ({ ...p, email: e.target.value }))}
                    placeholder="you@example.com"
                    className={input}
                  />
                </label>
                <label className="block">
                  <div className={labelText}>提前续期天数</div>
                  <input
                    type="number"
                    min={1}
                    max={89}
                    value={form.renewDays}
                    onChange={(e) => setForm((p) => ({ ...p, renewDays: e.target.value }))}
                    className={input}
                  />
                </label>
              </div>

              {isDns01 && (
                <label className="block">
                  <div className={labelText}>DNS 服务商</div>
                  <select
                    value={form.providerId}
                    onChange={(e) => setForm((p) => ({ ...p, providerId: e.target.value }))}
                    className={input}
                  >
                    {providers.length === 0 && <option value="">请先在 DDNS 页面添加服务商</option>}
                    {providers.map((p) => (
                      <option key={p.id} value={String(p.id)} disabled={!p.supportsDns01}>
                        {p.name}
                        {p.supportsDns01 ? '' : '（暂不支持 DNS-01）'}
                      </option>
                    ))}
                  </select>
                  <div className="mt-1 text-[11px] text-muted">
                    复用 DDNS 里已配好的凭证，签发时自动写入 TXT 记录，验证完成后自动清理
                  </div>
                </label>
              )}

              {isHttp01 && (
                <label className="block">
                  <div className={labelText}>验证监听端口</div>
                  <input
                    type="number"
                    value={form.httpPort}
                    onChange={(e) => setForm((p) => ({ ...p, httpPort: e.target.value }))}
                    className={input}
                  />
                  <div className="mt-1 text-[11px] text-muted">
                    CA 始终从 80 端口发起验证。这里只在你用端口转发把外部 80 转到本机其它端口时才需要改。
                  </div>
                </label>
              )}

              <details className="rounded-xl bg-surface-2 px-3 py-2">
                <summary className="cursor-pointer text-xs font-semibold text-muted">
                  外部账户绑定 EAB（ZeroSSL 等需要）
                </summary>
                <div className="mt-2 space-y-2">
                  <input
                    value={form.eabKeyId}
                    onChange={(e) => setForm((p) => ({ ...p, eabKeyId: e.target.value }))}
                    placeholder="EAB Key ID"
                    className={`${input} font-mono`}
                  />
                  <input
                    type="password"
                    value={form.eabHmac}
                    onChange={(e) => setForm((p) => ({ ...p, eabHmac: e.target.value }))}
                    placeholder={initial?.acme.hasEabHmac ? '••••••••（已配置，留空不修改）' : 'EAB HMAC Key'}
                    className={`${input} font-mono`}
                  />
                </div>
              </details>
            </>
          )}

          {form.source === 'upload' && !initial && (
            <div className="rounded-xl bg-blue-50 px-3 py-2.5 text-[11px] leading-relaxed text-blue-700 ring-1 ring-blue-200">
              保存后会出现在列表里，再点「上传 PEM」把证书和私钥贴进去。
            </div>
          )}

          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2">
            <label className="flex cursor-pointer items-center gap-2 rounded-xl bg-surface-2 px-3 py-2 text-xs font-medium text-content">
              <input
                type="checkbox"
                checked={form.enabled}
                onChange={(e) => setForm((p) => ({ ...p, enabled: e.target.checked }))}
                className="h-3.5 w-3.5"
              />
              启用
            </label>
            <label className="flex cursor-pointer items-center gap-2 rounded-xl bg-surface-2 px-3 py-2 text-xs font-medium text-content">
              <input
                type="checkbox"
                checked={form.isDefault}
                onChange={(e) => setForm((p) => ({ ...p, isDefault: e.target.checked }))}
                className="h-3.5 w-3.5"
              />
              设为默认证书
            </label>
          </div>
          <div className="text-[11px] text-muted">
            默认证书用于客户端没带 SNI（比如直接用 IP 访问）或没有域名匹配时兜底，全局只能有一张。
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
            {busy ? '保存中...' : '保存'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ===================== 上传 PEM 弹窗 =====================

