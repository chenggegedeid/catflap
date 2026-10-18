import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  AlertTriangle,
  Globe,
  Repeat,
  RotateCw,
  ServerCog,
  ShieldCheck,
  Workflow,
} from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import * as api from '../lib/api'


import { TrafficSecurityCard } from '../components/TrafficSecurityCard'
import type {
  Certificate,
  DdnsConfig,
  NatTypeInfo,
  PageKey,
  ProxyConfig,
  StunConfig,
  StunDevice,
  StunStatusEvent,
} from '../types'

// Dashboard 不接 props，App 监听 hashchange，改 hash 就跳页
function go(page: PageKey) {
  window.location.hash = `#/${page}`
}

const phaseLabel: Record<string, string> = {
  PROBING: '探测中',
  RUNNING: '穿透成功',
  RESTARTING: '重启中',
  FAILED: '探测失败',
  STOPPED: '已停止',
}

function getDeviceId(d: StunDevice): number {
  return d.DeviceID ?? d.deviceId ?? d.id
}

/**
 * 和后端 PublicScheme、STUN 页的 openAddress 保持同一套判断：
 * 洞口终结 TLS 时外面那一段就是 https；不终结却勾了「转发给内网时也用 HTTPS」
 * 时洞口在替内网加密、外面是明文，写成 https:// 只会给出一个必报
 * ERR_SSL_PROTOCOL_ERROR 的链接。
 */

function ddnsFullName(subDomain: string, domain: string) {
  const sub = (subDomain || '').trim()
  if (!sub || sub === '@') return domain
  return `${sub}.${domain}`
}

/** 需要用户去处理的一条，按严重程度排前面 */
interface Alert {
  key: string
  level: 'error' | 'warn'
  text: string
  page: PageKey
}

export function Dashboard() {
  const [config, setConfig] = useState<StunConfig | null>(null)
  const [natType, setNatType] = useState<NatTypeInfo | null>(null)
  const [ddns, setDdns] = useState<DdnsConfig | null>(null)
  const [certs, setCerts] = useState<Certificate[]>([])
  const [proxy, setProxy] = useState<ProxyConfig | null>(null)
  const [version, setVersion] = useState('')
  const [statusMap, setStatusMap] = useState<Record<string, StunStatusEvent>>({})
  const [loadErr, setLoadErr] = useState('')
  const [refreshedAt, setRefreshedAt] = useState<Date | null>(null)

  // 每块都单独 catch：证书页没配、反代没开都不该让整个仪表盘空白
  const refresh = useCallback(async () => {
    try {
      const cfg = await api.getStunConfig()
      setConfig(cfg)
      setLoadErr('')
    } catch (e) {
      setLoadErr(e instanceof Error ? e.message : String(e))
    }
    api.getNatType().then(setNatType).catch(() => {})
    api.getDdnsConfig().then(setDdns).catch(() => {})
    api.getCertList().then(setCerts).catch(() => {})
    api.getProxyConfig().then(setProxy).catch(() => {})
    api.getVersion().then((v) => setVersion(v.version)).catch(() => {})
    setRefreshedAt(new Date())
  }, [])

  useEffect(() => {
    refresh()
    const t = window.setInterval(refresh, 30000)
    return () => window.clearInterval(t)
  }, [refresh])

  // 外部端口是漂的，只有这条 SSE 是实时的，别的都靠 30s 轮询
  useEffect(() => {
    const es = api.subscribeStunStatus((data) => {
      try {
        const evt = JSON.parse(data) as StunStatusEvent
        setStatusMap((p) => ({ ...p, [evt.key]: evt }))
      } catch {
        // 单条事件坏了不影响后面的
      }
    })
    return () => es.close()
  }, [])


  // 把设备摊平成一条条对外服务，每条都带上它此刻真实的外部地址
  const rows = useMemo(() => {
    const list = []
    for (const d of config?.devices ?? []) {
      for (const svc of d.services ?? []) {
        const key = `${getDeviceId(d)}-${svc.id}`
        const status = statusMap[key]
        const phase = status?.phaseStr || 'STOPPED'
        const externalPort = status?.externalPort ?? 0
        // 配了对外域名就用域名——终结 TLS 时证书是按域名签的，用 IP 访问必然报证书错
        const externalHost = svc.domain || config?.publicIP || ''
        list.push({
          key,
          svc,
          deviceName: d.name,
          internal: `${d.ip}:${svc.internalPort}`,
          phase,
          running: phase === 'RUNNING',
          lastError: status?.lastError || '',
          external: externalPort > 0 && externalHost ? `${externalHost}:${externalPort}` : '',
        })
      }
    }
    return list
  }, [config, statusMap])

  const enabledRows = rows.filter((r) => r.svc.enabled)
  const runningCount = enabledRows.filter((r) => r.running).length

  const alerts = useMemo<Alert[]>(() => {
    const out: Alert[] = []

    for (const r of rows) {
      if (!r.svc.enabled) continue
      if (!r.running) {
        out.push({
          key: `hole-${r.key}`,
          level: 'error',
          text: `「${r.svc.name || '未命名服务'}」没穿透成功（${phaseLabel[r.phase] || r.phase}）${r.lastError ? '：' + r.lastError : ''}`,
          page: 'stun',
        })
      }
      // 终结 TLS 时证书是按域名签的，域名留空会回落公网 IP，
      // 而证书永远盖不住 IP，点开只会看到 ERR_CERT_COMMON_NAME_INVALID
      if (r.svc.tlsTerminate && !r.svc.domain) {
        out.push({
          key: `nodomain-${r.key}`,
          level: 'warn',
          text: `「${r.svc.name || '未命名服务'}」开了 TLS 终结却没填对外域名，生成的链接会用公网 IP，浏览器会报证书错误`,
          page: 'stun',
        })
      }
    }

    for (const c of certs) {
      if (!c.enabled) continue
      if (c.lastError) {
        out.push({ key: `cert-err-${c.id}`, level: 'error', text: `证书「${c.name}」上次签发失败：${c.lastError}`, page: 'cert' })
      } else if (c.expired) {
        out.push({ key: `cert-exp-${c.id}`, level: 'error', text: `证书「${c.name}」已过期`, page: 'cert' })
      } else if (c.daysLeft > 0 && c.daysLeft <= 15) {
        out.push({ key: `cert-soon-${c.id}`, level: 'warn', text: `证书「${c.name}」还剩 ${c.daysLeft} 天到期`, page: 'cert' })
      }
    }

    for (const r of ddns?.records ?? []) {
      if (r.enabled && r.lastStatus === 'failed') {
        out.push({
          key: `ddns-${r.id}`,
          level: 'error',
          text: `域名 ${ddnsFullName(r.subDomain, r.domain)} 同步失败${r.lastMessage ? '：' + r.lastMessage : ''}`,
          page: 'ddns',
        })
      }
    }

    for (const l of proxy?.listeners ?? []) {
      if (!l.running) {
        out.push({
          key: `proxy-${l.port}`,
          level: 'error',
          text: `反向代理 ${l.port} 端口没监听起来${l.lastError ? '：' + l.lastError : ''}`,
          page: 'reverse-proxy',
        })
      }
    }

    return out.sort((a, b) => (a.level === b.level ? 0 : a.level === 'error' ? -1 : 1))
  }, [rows, certs, ddns, proxy])

  if (loadErr && !config) {
    return (
      <Card>
        <div className="py-12 text-center text-rose-500">
          <AlertTriangle className="mx-auto h-8 w-8" />
          <div className="mt-2 text-sm font-semibold">数据加载失败</div>
          <div className="mt-1 text-xs text-muted">{loadErr}</div>
          <button onClick={refresh} className="mt-4 rounded-xl bg-blue-500 px-4 py-2 text-xs font-semibold text-white">
            重试
          </button>
        </div>
      </Card>
    )
  }

  return (
    <div className="space-y-4">
      {/*
        流量与安全：只留信号。

        这里**曾经**直接渲染 <TrafficPanel /> —— 那块有 587 行，实时速率、趋势
        四档、各服务 Top、可疑来源全塞在仪表盘最上面，结果是往下翻好几屏才看得
        到「当前状态」和「需要处理」。现在它独立成「流量统计」页（#/traffic），
        这里换成一张紧凑的联合卡：流量与异常画在同一条时间轴上，回答「现在的
        流量是正常访问还是有人在扫我」，详情各自链接过去。
      */}
      <TrafficSecurityCard />

      {/* 概览 + 快捷操作 */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-12">
        <Card className="lg:col-span-7">
          <CardHeader
            title="当前状态"
            action={
              <button
                onClick={refresh}
                className="flex items-center gap-1 text-xs text-muted transition hover:text-blue-500"
              >
                <RotateCw className="h-3.5 w-3.5" /> 刷新
              </button>
            }
          />
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3 sm:grid-cols-4">
            <div className="rounded-xl bg-surface-2 p-3">
              <div className="text-xs text-muted">公网 IP</div>
              <div className="mt-1 break-all font-mono text-sm font-bold text-content">
                {config?.publicIP || '--'}
              </div>
            </div>
            <div className="rounded-xl bg-surface-2 p-3">
              <div className="text-xs text-muted">NAT 层数</div>
              <div className="mt-1 text-sm font-bold text-content">
                {config ? `${(config.natRouterList || []).length} 层` : '--'}
              </div>
              <div className="mt-0.5 truncate text-[11px] text-muted" title={natType?.udp?.natType || ''}>
                {natType?.udp?.natType || '未检测'}
              </div>
            </div>
            <div className="rounded-xl bg-surface-2 p-3">
              <div className="text-xs text-muted">穿透中</div>
              <div
                className={`mt-1 text-sm font-bold ${
                  enabledRows.length > 0 && runningCount === enabledRows.length ? 'text-emerald-600' : 'text-content'
                }`}
              >
                {runningCount} / {enabledRows.length}
              </div>
              <div className="mt-0.5 text-[11px] text-muted">已启用的服务</div>
            </div>
            <div className="rounded-xl bg-surface-2 p-3">
              <div className="text-xs text-muted">最优 STUN</div>
              <div className="mt-1 break-all font-mono text-[11px] font-semibold text-content">
                {config?.bestStun || '--'}
              </div>
            </div>
          </div>
        </Card>

        <Card className="lg:col-span-5">
          <CardHeader title="快捷操作" />
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-2 sm:grid-cols-5">
            {[
              { icon: ServerCog, label: '设备', tone: 'bg-blue-50 text-blue-600', page: 'stun' as PageKey },
              { icon: Workflow, label: '服务', tone: 'bg-emerald-50 text-emerald-600', page: 'stun' as PageKey },
              { icon: Globe, label: 'DDNS', tone: 'bg-violet-50 text-violet-600', page: 'ddns' as PageKey },
              { icon: Repeat, label: '反向代理', tone: 'bg-cyan-50 text-cyan-600', page: 'reverse-proxy' as PageKey },
              { icon: ShieldCheck, label: '证书', tone: 'bg-rose-50 text-rose-500', page: 'cert' as PageKey },
            ].map((q) => {
              const Icon = q.icon
              return (
                <button
                  key={q.label}
                  type="button"
                  onClick={() => go(q.page)}
                  className="flex flex-col items-center gap-1.5 rounded-2xl border border-line/70 bg-surface p-3 text-xs font-semibold text-content transition hover:-translate-y-0.5 hover:shadow-md"
                >
                  <span className={`grid h-9 w-9 place-items-center rounded-xl ${q.tone}`}>
                    <Icon className="h-4 w-4" />
                  </span>
                  <span>{q.label}</span>
                </button>
              )
            })}
          </div>
        </Card>
      </div>

      {/* 需要处理——没问题时整块不出现，不占地方 */}
      {alerts.length > 0 && (
        <Card>
          <CardHeader title={`需要处理（${alerts.length}）`} />
          <ul className="space-y-2">
            {alerts.map((a) => (
              <li key={a.key}>
                <button
                  type="button"
                  onClick={() => go(a.page)}
                  className={`flex w-full items-start gap-2 rounded-xl border px-3 py-2 text-left text-xs transition hover:shadow-sm ${
                    a.level === 'error'
                      ? 'border-rose-200 bg-rose-50 text-rose-700 hover:bg-rose-100'
                      : 'border-amber-200 bg-amber-50 text-amber-700 hover:bg-amber-100'
                  }`}
                >
                  <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  <span className="flex-1">{a.text}</span>
                </button>
              </li>
            ))}
          </ul>
        </Card>
      )}

      <div className="flex justify-end gap-3 pt-1 text-xs text-muted">
        {version && <span>Catflap {version}</span>}
        {refreshedAt && <span>最后刷新 {refreshedAt.toLocaleTimeString()}</span>}
      </div>
    </div>
  )
}
