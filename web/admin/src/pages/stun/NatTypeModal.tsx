/**
 * NAT 类型检测弹窗：探测 UDP / TCP 分别是哪种 NAT。
 */
import { useCallback, useEffect, useState } from 'react'
import { AlertCircle, Info, LoaderCircle, RotateCw, Server, Wifi, X } from 'lucide-react'
import { modalBackdrop } from '../../components/modal'
import * as api from '../../lib/api'
import type { NatDetectResult, NatTypeInfo } from '../../types'

const mappingLabels: Record<string, string> = {
  EIM: '端点无关映射',
  ADM: '地址相关映射',
  APDM: '地址和端口相关映射',
  unknown: '未知',
}

const filteringLabels: Record<string, string> = {
  EIF: '端点无关过滤',
  ADF: '地址相关过滤',
  APDF: '地址和端口相关过滤',
  unknown: '未知',
}

function behaviorText(value: string, labels: Record<string, string>) {
  if (!value) return '--'
  const label = labels[value]
  return label ? `${label} (${value})` : value
}

function natTypeTone(natType: string) {
  if (natType.includes('NAT1')) return { panel: 'border-emerald-200 bg-emerald-50/40', badge: 'bg-emerald-100 text-emerald-700' }
  if (natType.includes('NAT2')) return { panel: 'border-sky-200 bg-sky-50/40', badge: 'bg-sky-100 text-sky-700' }
  if (natType.includes('NAT3')) return { panel: 'border-amber-200 bg-amber-50/40', badge: 'bg-amber-100 text-amber-700' }
  if (natType.includes('NAT4') || natType.includes('阻断')) return { panel: 'border-rose-200 bg-rose-50/40', badge: 'bg-rose-100 text-rose-700' }
  if (natType.includes('公网')) return { panel: 'border-teal-200 bg-teal-50/40', badge: 'bg-teal-100 text-teal-700' }
  return { panel: 'border-line bg-surface-2/70', badge: 'bg-surface-2 text-content' }
}

function NatProtocolResult({ protocol, result }: { protocol: 'UDP' | 'TCP'; result: NatDetectResult | null }) {
  const tone = natTypeTone(result?.natType ?? '')
  const ProtocolIcon = protocol === 'UDP' ? Wifi : Server

  if (!result) {
    return (
      <section className="rounded-lg border border-dashed border-line bg-surface-2/70 p-4">
        <div className="flex items-center gap-2 text-sm font-bold text-content">
          <ProtocolIcon className="h-4 w-4 text-muted" /> {protocol}
        </div>
        <div className="mt-5 text-center text-sm text-muted">尚无检测结果</div>
      </section>
    )
  }

  return (
    <section className={`rounded-lg border p-4 ${tone.panel}`}>
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <span className="grid h-8 w-8 shrink-0 place-items-center rounded-md bg-surface text-content shadow-sm ring-1 ring-line/80">
            <ProtocolIcon className="h-4 w-4" />
          </span>
          <div>
            <div className="text-sm font-bold text-content">{protocol}</div>
            <div className="text-[11px] text-muted">{protocol === 'UDP' ? 'Mapping + Filtering' : 'Mapping'}</div>
          </div>
        </div>
        <span className={`shrink-0 rounded-md px-2 py-1 text-[11px] font-bold ${tone.badge}`}>
          {result.mappingErr ? '部分失败' : '检测完成'}
        </span>
      </div>

      <div className="mt-4">
        <div className="text-[11px] font-semibold text-muted">NAT 类型</div>
        <div className="mt-1 break-words text-sm font-bold leading-5 text-content">{result.natType || '无法确定'}</div>
      </div>

      <dl className="mt-4 grid gap-2 text-xs sm:grid-cols-2">
        <div className="min-w-0 rounded-md bg-surface/80 px-3 py-2 ring-1 ring-line/70">
          <dt className="text-[11px] font-semibold text-muted">Mapping</dt>
          <dd className="mt-1 break-words font-medium leading-5 text-content">{behaviorText(result.mapping, mappingLabels)}</dd>
          {result.mapping && result.mapping !== 'unknown' && (
            <dd className="mt-1.5 text-[10px] leading-relaxed text-muted">
              {result.mapping === 'EIM' && '✓ 最佳：同一端口对所有目标使用相同映射，打洞成功率最高'}
              {result.mapping === 'ADM' && '△ 中等：不同目标 IP 使用不同映射，需要端口预测'}
              {result.mapping === 'APDM' && '✗ 困难：每个目标 IP+端口都不同，打洞成功率低'}
            </dd>
          )}
        </div>
        <div className="min-w-0 rounded-md bg-surface/80 px-3 py-2 ring-1 ring-line/70">
          <dt className="text-[11px] font-semibold text-muted">Filtering</dt>
          <dd className="mt-1 break-words font-medium leading-5 text-content">
            {protocol === 'TCP' ? 'TCP 不适用' : behaviorText(result.filtering, filteringLabels)}
          </dd>
          {protocol === 'TCP' ? (
            <dd className="mt-1.5 text-[10px] leading-relaxed text-muted">
              TCP 面向连接特性导致 Filtering 无法准确测试
            </dd>
          ) : result.filtering && result.filtering !== 'unknown' ? (
            <dd className="mt-1.5 text-[10px] leading-relaxed text-muted">
              {result.filtering === 'EIF' && '✓ 宽松：任何外部地址都能连入'}
              {result.filtering === 'ADF' && '△ 中等：需要先向目标发包建立连接'}
              {result.filtering === 'APDF' && '○ 严格：需要精确匹配端口，但 EIM 下仍可打洞'}
            </dd>
          ) : null}
        </div>
      </dl>
      {(result.mappingErr || result.filteringErr) && (
        <div className="mt-3 space-y-1 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">
          {result.mappingErr && <div className="break-words">Mapping: {result.mappingErr}</div>}
          {result.filteringErr && <div className="break-words">Filtering: {result.filteringErr}</div>}
        </div>
      )}
    </section>
  )
}

export function NatTypeModal({ onClose }: { onClose: () => void }) {
  const [data, setData] = useState<NatTypeInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [detecting, setDetecting] = useState(false)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      setData(await api.getNatType())
    } catch (e) {
      setError(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  const detect = async () => {
    setDetecting(true)
    setError('')
    try {
      setData(await api.detectNatType())
    } catch (e) {
      setError(e instanceof Error ? e.message : '重新检测失败')
    } finally {
      setDetecting(false)
    }
  }

  useEffect(() => {
    let active = true
    api.getNatType()
      .then((result) => {
        if (active) setData(result)
      })
      .catch((e) => {
        if (active) setError(e instanceof Error ? e.message : '加载失败')
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [])

  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="flex max-h-[88vh] w-full max-w-2xl flex-col rounded-lg bg-surface shadow-2xl ring-1 ring-line">
        <div className="flex items-center justify-between gap-4 border-b border-line px-5 py-4">
          <div className="flex min-w-0 items-center gap-3">
            <span className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-blue-50 text-blue-600 ring-1 ring-blue-100">
              <Info className="h-4.5 w-4.5" />
            </span>
            <div className="min-w-0">
              <div className="text-base font-bold text-content">NAT 类型详情</div>
              <div className="mt-0.5 text-xs text-muted">RFC 5780 行为检测</div>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            title="关闭"
            aria-label="关闭"
            className="grid h-8 w-8 place-items-center rounded-md text-muted transition hover:bg-surface-2 hover:text-content"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="overflow-y-auto p-5">
          {loading ? (
            <div className="flex min-h-48 items-center justify-center gap-2 text-sm text-muted">
              <LoaderCircle className="h-4 w-4 animate-spin" /> 正在加载
            </div>
          ) : error ? (
            <div className="flex min-h-48 flex-col items-center justify-center gap-3 text-sm text-rose-600">
              <AlertCircle className="h-6 w-6" />
              <span>{error}</span>
              <button
                type="button"
                onClick={load}
                className="flex items-center gap-1 rounded-md border border-line px-3 py-1.5 text-xs font-semibold text-content hover:bg-surface-2"
              >
                <RotateCw className="h-3.5 w-3.5" /> 重试
              </button>
            </div>
          ) : (
            <div className="space-y-3">
              {data?.error && (
                <div className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800">
                  <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
                  <span className="break-words">部分探测未完成：{data.error}</span>
                </div>
              )}
              <div className="grid gap-3 md:grid-cols-2">
                <NatProtocolResult protocol="UDP" result={data?.udp ?? null} />
                <NatProtocolResult protocol="TCP" result={data?.tcp ?? null} />
              </div>
            </div>
          )}
        </div>
        {!loading && !error && (
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line px-5 py-3">
            <span className="text-xs text-muted">重新检测通常需要数秒</span>
            <button
              type="button"
              onClick={detect}
              disabled={detecting}
              className="flex min-w-[104px] items-center justify-center gap-1.5 rounded-md bg-blue-600 px-3 py-2 text-xs font-semibold text-white shadow-sm transition hover:bg-blue-700 disabled:cursor-wait disabled:opacity-60"
            >
              <RotateCw className={`h-3.5 w-3.5 ${detecting ? 'animate-spin' : ''}`} />
              {detecting ? '检测中...' : '重新检测'}
            </button>
          </div>
        )}
      </div>
    </div>
  )
}

// StunServersModal 编辑 STUN 服务器列表（每行一个），保存后后端立即重测选优。
// 实测有些服务商的 STUN 会很快掐掉 TCP 连接（比如 sonetel），导致 TCP 打洞
// 在 25s 保活检查时必然失败。把这类服务器从列表里去掉，换成长连接稳定的。
