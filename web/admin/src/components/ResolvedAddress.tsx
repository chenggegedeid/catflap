/**
 * 服务的外部地址：可点击跳转、可复制。
 *
 * 反代页和穿透页原本各写一套，行为还不一样：
 *   - 反代：整块是 <a href>，点域名直接跳（对）
 *   - 穿透：地址是个 button，点了是**复制**；跳转藏在右边一个小图标里，
 *     手机上很难点中。而且非 HTTP 服务（Minecraft / RDP）也照样 window.open，
 *     跳过去必然打不开。
 *
 * 这里统一成一份，两边都用：
 *   - 地址文字本身可点击跳转（HTTP/HTTPS 才行）
 *   - 旁边一个复制小图标（复制纯 host:port，不带协议 —— 粘到别处更好用）
 *   - 对外端口不是该协议的默认端口时，端口部分单独加个角标，提醒「这个端口得带上」
 *   - 非 HTTP 服务：显示协议前缀（如 rdp://），并**禁用跳转** —— 浏览器打不开，
 *     点了只会得到一个没用的空白页，不如直接告诉用户该用什么客户端连
 */
import { ClipboardCopy, ExternalLink } from 'lucide-react'

/** 已知协议 → 默认端口。端口不等于它时才需要角标提醒 */
const DEFAULT_PORT: Record<string, number> = { http: 80, https: 443 }

/** 能直接用浏览器打开的协议。其余的一律不提供跳转 */
const BROWSER_OPENABLE = new Set(['http', 'https'])

export interface ResolvedAddressValue {
  /** 协议，如 https。空串表示未知/非 HTTP */
  scheme: string
  /** 主机名或 IP */
  host: string
  /** 对外端口，0 = 未知 */
  port: number
  /** 路径前缀，可空 */
  path?: string
}

/** 拼出完整 URL。端口是协议默认值时省略 */
export function formatURL(a: ResolvedAddressValue): string {
  if (!a.host) return ''
  const scheme = a.scheme || 'http'
  const def = DEFAULT_PORT[scheme]
  const port = a.port > 0 && a.port !== def ? `:${a.port}` : ''
  return `${scheme}://${a.host}${port}${a.path ?? ''}`
}

/** 复制用的文本：不带协议、一定带端口（粘到 SSH / RDP 客户端里更好用） */
export function formatCopyText(a: ResolvedAddressValue): string {
  if (!a.host) return ''
  return a.port > 0 ? `${a.host}:${a.port}` : a.host
}

export function ResolvedAddress({
  value,
  onCopy,
  /** 地址还没拿到（比如洞口没打通）。这行只显示提示文案 */
  fallback,
  className = '',
}: {
  value: ResolvedAddressValue | null
  onCopy: (text: string) => void
  fallback?: string
  className?: string
}) {
  if (!value || !value.host) {
    return <span className={`text-[11px] text-muted ${className}`}>{fallback ?? '等待外部地址…'}</span>
  }

  const scheme = value.scheme || 'http'
  const openable = BROWSER_OPENABLE.has(scheme)
  const url = formatURL(value)
  const copyText = formatCopyText(value)
  const def = DEFAULT_PORT[scheme]
  const showPortHint = value.port > 0 && value.port !== def

  return (
    <div className={`flex min-w-0 items-center gap-1 ${className}`}>
      {openable ? (
        <a
          href={url}
          target="_blank"
          rel="noopener noreferrer"
          title={`打开 ${url}`}
          className="flex min-w-0 items-center gap-1 font-mono text-xs font-semibold text-blue-600 hover:text-blue-700"
        >
          <span className="truncate">
            {value.host}
            {showPortHint ? <span className="text-muted">:{value.port}</span> : null}
            {value.path ? <span className="text-muted">{value.path}</span> : null}
          </span>
          <ExternalLink className="h-3 w-3 shrink-0 text-muted" />
        </a>
      ) : (
        // 非 HTTP：只展示，不提供跳转。点了也打不开，反而让人以为服务坏了
        <span
          title={`${scheme} 服务，浏览器打不开；用对应客户端连 ${copyText}`}
          className="flex min-w-0 items-center font-mono text-xs font-semibold text-content"
        >
          <span className="truncate">
            {scheme}://{value.host}:{value.port || '?'}
          </span>
        </span>
      )}

      <button
        onClick={() => onCopy(copyText)}
        title={`点击复制：${copyText}`}
        className="shrink-0 rounded p-0.5 text-muted transition hover:bg-surface-2 hover:text-content"
      >
        <ClipboardCopy className="h-3 w-3" />
      </button>
    </div>
  )
}
