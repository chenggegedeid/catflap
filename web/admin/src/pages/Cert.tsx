import { useCallback, useEffect, useState } from 'react'
import {
  AlertCircle,
  CircleCheck,
  Clock,
  Pencil,
  Plus,
  RefreshCw,
  ShieldCheck,
  TriangleAlert,
  Upload,
  Trash2,
} from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import { CertModal } from './cert/CertModal'
import { UploadModal } from './cert/UploadModal'
import {
  certStatus, fmtDate, sourceLabel, useToast,
} from './cert/shared'
import * as api from '../lib/api'
import type { AcmeProvider, Certificate } from '../types'

// ===================== 来源元信息 =====================

export function Cert() {
  const [certs, setCerts] = useState<Certificate[] | null>(null)
  const [providers, setProviders] = useState<AcmeProvider[]>([])
  const [loadErr, setLoadErr] = useState('')
  const [issuingId, setIssuingId] = useState<number | null>(null)
  const { toasts, show: toast } = useToast()

  const [editModal, setEditModal] = useState<{ open: boolean; initial?: Certificate }>({ open: false })
  const [uploadModal, setUploadModal] = useState<Certificate | null>(null)

  const refresh = useCallback(async () => {
    try {
      const [list, provs] = await Promise.all([api.getCertList(), api.getCertProviders()])
      setCerts(list)
      setProviders(provs)
      setLoadErr('')
    } catch (e) {
      setLoadErr(e instanceof Error ? e.message : String(e))
    }
  }, [])

  useEffect(() => {
    refresh()
    // 签发是异步的，靠轮询把 notAfter / lastError 的变化捞回来
    const t = window.setInterval(refresh, 10000)
    return () => window.clearInterval(t)
  }, [refresh])

  const list = certs ?? []
  const active = list.filter((c) => c.enabled && c.loaded && !c.expired).length
  const expiring = list.filter((c) => c.loaded && !c.expired && c.daysLeft <= 15).length
  const problem = list.filter((c) => c.enabled && (c.expired || !!c.lastError)).length

  const submitCert = async (payload: api.CertPayload) => {
    if (editModal.initial) {
      await api.updateCert({ ...payload, id: editModal.initial.id })
      toast('证书已更新')
    } else {
      await api.addCert(payload)
      toast(payload.source === 'upload' ? '已创建，接着上传 PEM' : '证书已添加')
    }
    setEditModal({ open: false })
    await refresh()
  }

  const submitUpload = async (certPem: string, keyPem: string) => {
    if (!uploadModal) return
    await api.uploadCertPEM(uploadModal.id, certPem, keyPem)
    setUploadModal(null)
    toast('证书已上传并生效')
    await refresh()
  }

  const removeCert = async (c: Certificate) => {
    if (!window.confirm(`确认删除证书「${c.name}」？\n本机保存的 PEM 也会一并清除。`)) return
    try {
      await api.deleteCert(c.id)
      toast('证书已删除')
      await refresh()
    } catch (e) {
      toast('删除失败: ' + (e instanceof Error ? e.message : ''))
    }
  }

  const issue = async (c: Certificate) => {
    setIssuingId(c.id)
    try {
      await api.issueCert(c.id)
      // 自签是本机算的，毫秒级就好了；ACME 要跟 CA 来回跑，说个时间免得用户以为卡住
      toast(c.source === 'self-signed' ? '已重新生成' : '已开始签发，全程约需 1–3 分钟')
      await refresh()
    } catch (e) {
      toast('签发失败: ' + (e instanceof Error ? e.message : ''))
    } finally {
      setIssuingId(null)
    }
  }

  if (loadErr && !certs) {
    return (
      <Card>
        <div className="py-12 text-center text-rose-500">
          <AlertCircle className="mx-auto h-8 w-8" />
          <div className="mt-2 text-sm font-semibold">数据加载失败</div>
          <div className="mt-1 text-xs text-muted">{loadErr}</div>
          <button onClick={refresh} className="mt-4 rounded-xl bg-blue-500 px-4 py-2 text-xs font-semibold text-white">
            重试
          </button>
        </div>
      </Card>
    )
  }

  if (!certs) {
    return (
      <Card>
        <div className="py-12 text-center text-sm text-muted">正在加载...</div>
      </Card>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="text-base font-bold text-content">证书管理</div>
      </div>

      {/* 这页存在的意义，一句话说清楚 */}
      <Card>
        <div className="flex gap-3 text-xs leading-relaxed text-muted">
          <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-blue-500" />
          <div>
            这里的证书用于在 <span className="font-semibold text-content">STUN 洞口终结 TLS</span>
            ：开启后外部访问是 <span className="font-mono">https://</span>，而不是明文 HTTP。
            在 STUN 页面为每个服务填好对外域名并打开「洞口终结 TLS」即可使用，
            证书续期时只热替换内存中的指针，<span className="font-semibold text-content">不会断开已有连接，也不需要重启穿透</span>。
          </div>
        </div>
      </Card>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-blue-50 text-blue-500">
              <ShieldCheck className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-muted">证书总数</div>
              <div className="text-2xl font-bold text-content">{list.length}</div>
            </div>
          </div>
        </Card>
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-emerald-50 text-emerald-500">
              <CircleCheck className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-muted">生效中</div>
              <div className="text-2xl font-bold text-content">{active}</div>
            </div>
          </div>
        </Card>
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-amber-50 text-amber-500">
              <Clock className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-muted">15 天内到期</div>
              <div className="text-2xl font-bold text-content">{expiring}</div>
            </div>
          </div>
        </Card>
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-rose-50 text-rose-500">
              <TriangleAlert className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-muted">过期 / 异常</div>
              <div className="text-2xl font-bold text-content">{problem}</div>
            </div>
          </div>
        </Card>
      </div>

      <Card className="!p-0">
        <div className="px-5 pt-4">
          <CardHeader
            title="证书列表"
            action={
              <button
                onClick={() => setEditModal({ open: true })}
                className="flex items-center gap-1 rounded-md bg-blue-500 px-2.5 py-1 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 hover:bg-blue-600"
              >
                <Plus className="h-3 w-3" /> 添加证书
              </button>
            }
          />
        </div>

        {/* 手机上列太窄会把中文压成一列一个字，给张表一个下限，窄屏改成横向滚动 */}
        <div className="overflow-x-auto">
          <table className="w-full min-w-[820px] text-sm">
            <thead>
              <tr className="bg-surface-2/60 text-xs text-muted">
                <th className="px-5 py-2.5 text-left font-medium">名称</th>
                <th className="px-3 py-2.5 text-left font-medium">来源</th>
                <th className="px-3 py-2.5 text-left font-medium">覆盖域名</th>
                <th className="px-3 py-2.5 text-left font-medium">状态</th>
                <th className="px-3 py-2.5 text-left font-medium">有效期至</th>
                <th className="px-3 py-2.5 text-left font-medium">签发者</th>
                <th className="px-5 py-2.5 text-right font-medium">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line">
              {list.length === 0 && (
                <tr>
                  <td colSpan={7} className="px-5 py-8 text-center text-sm text-muted">
                    还没有证书，点右上角「添加证书」开始
                  </td>
                </tr>
              )}
              {list.map((c) => {
                const s = certStatus(c)
                const Icon = s.icon
                const busy = issuingId === c.id
                // 自签也能点这个按钮：换了网段 / DHCP 换了地址之后，
                // 得重签一张把新 IP 写进证书里
                const selfSigned = c.source === 'self-signed'
                const canReissue = selfSigned || c.source === 'acme-dns01' || c.source === 'acme-http01'
                return (
                  <tr key={c.id} className="align-top text-content transition hover:bg-surface-2/60">
                    <td className="px-5 py-3">
                      <div className="flex items-center gap-1.5 font-semibold">
                        {c.name}
                        {c.isDefault && (
                          <span className="rounded bg-violet-50 px-1.5 py-0.5 text-[10px] font-bold text-violet-600">
                            默认
                          </span>
                        )}
                      </div>
                      {c.lastError && (
                        <div className="mt-0.5 max-w-xs text-[11px] leading-relaxed text-rose-500" title={c.lastError}>
                          {c.lastError}
                        </div>
                      )}
                    </td>
                    <td className="px-3 py-3 text-xs text-muted">{sourceLabel(c.source)}</td>
                    <td className="px-3 py-3">
                      <div className="flex max-w-xs flex-wrap gap-1">
                        {c.domains.length === 0 && <span className="text-xs text-muted">--</span>}
                        {c.domains.map((d) => (
                          <span key={d} className="rounded bg-surface-2 px-1.5 py-0.5 font-mono text-[11px] text-content">
                            {d}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td className="px-3 py-3">
                      <span className={`inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] font-semibold ${s.tone}`}>
                        <Icon className={`h-3 w-3 ${s.label === '等待签发' ? 'animate-spin' : ''}`} />
                        {s.label}
                      </span>
                    </td>
                    <td className="px-3 py-3 text-xs">
                      <div className="text-content">{fmtDate(c.notAfter)}</div>
                      {c.notAfter && !c.expired && (
                        <div className={c.daysLeft <= 15 ? 'text-amber-600' : 'text-muted'}>剩 {c.daysLeft} 天</div>
                      )}
                    </td>
                    {/* 自签证书的签发者就是它自己，直接显示会是「nas.home.lan」，
                        看着像某个叫这名字的 CA。说清楚是谁签的更有用 */}
                    <td className="px-3 py-3 text-xs text-muted">
                      {selfSigned ? 'Catflap 自签' : c.issuer || '--'}
                    </td>
                    <td className="px-5 py-3 text-right">
                      <div className="inline-flex items-center gap-1">
                        {canReissue && (
                          <button
                            onClick={() => issue(c)}
                            disabled={busy || !c.enabled}
                            className="grid h-6 w-6 place-items-center rounded-md text-muted hover:bg-blue-50 hover:text-blue-600 disabled:opacity-40"
                            title={
                              !c.enabled
                                ? '证书已禁用'
                                : selfSigned
                                  ? '重新生成（换了网段、IP 变了之后点一下，把新 IP 写进证书）'
                                  : '立即签发 / 续期'
                            }
                          >
                            <RefreshCw className={`h-3 w-3 ${busy ? 'animate-spin' : ''}`} />
                          </button>
                        )}
                        {c.source === 'upload' && (
                          <button
                            onClick={() => setUploadModal(c)}
                            className="grid h-6 w-6 place-items-center rounded-md text-muted hover:bg-blue-50 hover:text-blue-600"
                            title="上传 PEM"
                          >
                            <Upload className="h-3 w-3" />
                          </button>
                        )}
                        <button
                          onClick={() => setEditModal({ open: true, initial: c })}
                          className="grid h-6 w-6 place-items-center rounded-md text-muted hover:bg-emerald-50 hover:text-emerald-600"
                          title="编辑"
                        >
                          <Pencil className="h-3 w-3" />
                        </button>
                        <button
                          onClick={() => removeCert(c)}
                          className="grid h-6 w-6 place-items-center rounded-md text-muted hover:bg-rose-50 hover:text-rose-500"
                          title="删除"
                        >
                          <Trash2 className="h-3 w-3" />
                        </button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>

        <div className="border-t border-line px-5 py-3 text-xs text-muted">共 {list.length} 张证书</div>
      </Card>

      {editModal.open && (
        <CertModal
          initial={editModal.initial}
          providers={providers}
          onCancel={() => setEditModal({ open: false })}
          onSubmit={submitCert}
        />
      )}
      {uploadModal && (
        <UploadModal cert={uploadModal} onCancel={() => setUploadModal(null)} onSubmit={submitUpload} />
      )}

      <div className="pointer-events-none fixed bottom-6 left-1/2 z-[60] flex -translate-x-1/2 flex-col items-center gap-2">
        {toasts.map((t) => (
          <div key={t.id} className="rounded-lg bg-inverse/90 px-4 py-2 text-sm text-on-inverse shadow-lg">
            {t.text}
          </div>
        ))}
      </div>
    </div>
  )
}
