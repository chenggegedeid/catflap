import {
  ArrowRight, FileText, Pencil, Power, ShieldCheck, Trash2, Zap,
} from 'lucide-react'
import { CardBadge, ServiceCard, iconBtn } from '../../components/ServiceCard'
import { ResolvedAddress } from '../../components/ResolvedAddress'
import type { ProxyEntry, ProxyListener, ProxySite } from '../../types'
import { portText, siteAddr, siteURL } from './shared'

export function ListenerChip({ l, certName }: { l: ProxyListener; certName: string }) {
  return (
    <span
      title={l.tls ? `证书：${certName}` : undefined}
      className={`inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-[11px] ring-1 ${
        l.running ? 'bg-surface text-content ring-line' : 'bg-rose-50 text-rose-600 ring-rose-200'
      }`}
    >
      <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${l.running ? 'bg-emerald-500' : 'bg-rose-500'}`} />
      <span className="font-mono font-semibold">:{l.port}</span>
      {l.tls ? (
        <span className="inline-flex items-center gap-0.5 font-semibold text-violet-600">
          <ShieldCheck className="h-3 w-3" />
          HTTPS
        </span>
      ) : (
        <span className="text-muted">HTTP</span>
      )}
      <span className="text-muted">{l.siteCount} 站点</span>
    </span>
  )
}

// ===================== 主页面 =====================

export function SiteCard({
  site: s,
  entry,
  testingId,
  flagBusy,
  onTest,
  onToggleFlag,
  onEdit,
  onRemove,
  onLogs,
  onCopy,
}: {
  site: ProxySite
  entry?: ProxyEntry
  testingId: number
  flagBusy: number
  onTest: () => void
  onToggleFlag: (flag: 'enabled') => void
  onEdit: () => void
  onRemove: () => void
  /** 打开这个站点的访问日志 */
  onLogs: () => void
  /** 复制外部地址。不传则地址只展示、不可复制 */
  onCopy?: (text: string) => void
}) {
  const url = siteURL(entry, s)
  const addr = siteAddr(entry, s)
  // 停用的站点本来就不该跑，别拿红色状态去吓人
  const off = s.enabled === false

  // **和穿透/转发共用同一张卡**（components/ServiceCard）。
  // 反代特有的两样通过 props 传：名字用域名（可点开），第三行放后端端口 + 连通性测试。
  return (
    <ServiceCard
      // **服务名称优先**：填了名称显示名称（和穿透一致，仪表盘也用它），
      // 没填才退回域名 —— 大屏上"设备·名称"比一长串域名好认得多
      name={s.description || s.hosts?.[0] || '任意域名'}
      state={off ? 'off' : 'running'}
      stateHint={off ? '已停用' : '在跑'}
      draggable={false}
      icons={
        <>
          <button
            onClick={() => onToggleFlag('enabled')}
            disabled={flagBusy === s.id}
            title={off ? '点一下启用' : '点一下停用'}
            className={`${iconBtn} disabled:opacity-40 ${
              off ? 'bg-fill text-muted hover:bg-fill' : 'bg-emerald-50 text-emerald-600 hover:bg-emerald-100'
            }`}
          >
            <Power className="h-3 w-3" />
          </button>
          <button onClick={onEdit} className={`${iconBtn} bg-surface-2 text-content hover:bg-fill`} title="编辑">
            <Pencil className="h-3 w-3" />
          </button>
          {/* 访问日志：谁访问过这个站点。数据来自访问审计，和穿透同一个弹窗 */}
          <button onClick={onLogs} className={`${iconBtn} bg-surface-2 text-content hover:bg-fill`} title="访问日志（谁访问过它）">
            <FileText className="h-3 w-3" />
          </button>
          <button onClick={onRemove} className={`${iconBtn} bg-rose-50 text-rose-500 hover:bg-rose-100`} title="删除">
            <Trash2 className="h-3 w-3" />
          </button>
        </>
      }
      subtitle={
        // 和穿透页共用 ResolvedAddress：地址文字可点跳转、旁边复制图标、
        // 端口非默认时带角标、非 HTTP 服务不提供跳转。
        // 没拿到 URL 时（比如站点停用）退回纯文本展示域名。
        url ? (
          <ResolvedAddress
            value={{
              scheme: addr?.https ? 'https' : 'http',
              host: s.hosts?.[0] ?? '',
              port: addr?.port ?? 0,
              path: s.pathPrefix || undefined,
            }}
            onCopy={onCopy ?? (() => {})}
          />
        ) : (
          <span className="font-mono text-xs font-semibold text-content">
            {s.hosts?.[0] ?? <span className="text-muted">任意域名</span>}
            {addr && <span className="text-muted">{portText(addr.https, addr.port)}</span>}
            {s.pathPrefix && <span className="text-muted">{s.pathPrefix}</span>}
          </span>
        )
      }
      meta={
        <>
          {/* 后端：只显示端口（IP 已经在设备头上了，不用重复） */}
          <span className="inline-flex min-w-0 items-center gap-1 font-mono text-[11px] text-muted">
            <ArrowRight className="h-3 w-3 shrink-0 text-muted" />
            <span className="truncate">:{String(s.backend ?? '').split(':')[1] ?? s.backend}</span>
          </span>
          {s.backendHttps && <CardBadge tone="emerald" title="内网服务本身是 HTTPS">TLS</CardBadge>}
          {s.https && (
            <CardBadge tone="violet" title="对外走 HTTPS">HTTPS</CardBadge>
          )}
          {s.hosts.length > 1 && (
            <CardBadge tone="slate">+{s.hosts.length - 1} 个域名</CardBadge>
          )}
          {s.stripPrefix && <CardBadge tone="amber" title="转发前剥掉路径前缀">剥前缀</CardBadge>}
          <button
            onClick={onTest}
            disabled={testingId === s.id}
            className="ml-auto grid h-5 w-5 shrink-0 place-items-center rounded text-muted hover:bg-blue-50 hover:text-blue-600 disabled:opacity-40"
            title="测试后端能不能连上"
          >
            <Zap className={`h-3 w-3 ${testingId === s.id ? 'animate-pulse' : ''}`} />
          </button>
        </>
      }
    />
  )
}
