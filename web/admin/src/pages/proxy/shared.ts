import { useCallback, useRef, useState } from 'react'
import type { Certificate, ProxyEntry, ProxySite } from '../../types'

export interface Toast {
  id: number
  text: string
}

export function useToast() {
  const [toasts, setToasts] = useState<Toast[]>([])
  const idRef = useRef(0)
  const show = useCallback((text: string) => {
    const id = ++idRef.current
    setToasts((p) => [...p, { id, text }])
    window.setTimeout(() => setToasts((p) => p.filter((t) => t.id !== id)), 3200)
  }, [])
  return { toasts, show }
}

// 后端端口没填时的默认值：内网服务绝大多数跑在 80 上。
// 端口不能留空 —— 空端口拼出的 "ip:" 会让后端解析不出端口、转发失败。
export const DEFAULT_BACKEND_PORT = 80

export const errText = (e: unknown) => (e instanceof Error ? e.message : String(e))

/** 80 和 443 不用写出来，写了反而显得脏 */
export const portText = (https: boolean, port: number) => ((https ? port === 443 : port === 80) ? '' : `:${port}`)

/**
 * 浏览器自己写死不让连的端口，抄自 Chromium 的 net::kRestrictedPorts
 * （net/base/port_util.cc），Firefox 那份几乎一模一样。
 *
 * 全是上古协议的端口（IRC 6665-6669、NFS 2049、SMTP 25……），历史上被拿来
 * 做跨协议攻击，从 2000 年代禁到现在。这件事阴在：反代监听这些端口不会报错，
 * 后端探测是通的，curl 也拿得到 200，唯独浏览器甩一句 ERR_UNSAFE_PORT，
 * 配的人完全不知道是谁的问题。所以在填的时候就要说出来。
 *
 * 不含 0——这里 0 表示「留空」，不是真端口。
 */
export const BROWSER_BLOCKED_PORTS = new Set([
  1, 7, 9, 11, 13, 15, 17, 19, 20, 21, 22, 23, 25, 37, 42, 43, 53, 69, 77, 79, 87, 95, 101, 102,
  103, 104, 109, 110, 111, 113, 115, 117, 119, 123, 135, 137, 139, 143, 161, 179, 389, 427, 465,
  512, 513, 514, 515, 526, 530, 531, 532, 540, 548, 554, 556, 563, 587, 601, 636, 989, 990, 993,
  995, 1719, 1720, 1723, 2049, 3659, 4045, 5060, 5061, 6000, 6566, 6665, 6666, 6667, 6668, 6669,
  6697, 10080,
])

export const portBlocked = (port: number) => port > 0 && BROWSER_BLOCKED_PORTS.has(port)

/**
 * 「按域名自动匹配」在客户端没发 SNI 的时候还能不能挑出一张证书。
 *
 * 逐条对着 modules/cert.Manager.Lookup 的第 ④⑤ 步：先找默认证书，
 * 再看是不是只有一张可用的。两条都不满足，握手就地失败——浏览器那头
 * 只有一句 ERR_SSL_PROTOCOL_ERROR，什么都看不出来，所以得提前说。
 *
 * 敲 IP 的地址栏是不发 SNI 的（RFC 6066 只允许域名），所以「没填域名的
 * HTTPS 站点」正好撞在这个坑上。
 */
export const autoCertNeedsSNI = (certs: Certificate[]) => {
  const usable = certs.filter((c) => c.enabled && c.loaded)
  return !usable.some((c) => c.isDefault) && usable.length !== 1
}

/** 淡黄提示条。只是提醒，不拦保存——挡不挡得住由用户自己判断 */
export function siteAddr(entry: ProxyEntry | undefined, site: ProxySite): { https: boolean; port: number } | null {
  if (site.listenPort > 0) return { https: site.https, port: site.listenPort }
  // 入口信息还没加载出来时（首帧）拿不到端口，返回空比报错好
  if (!entry) return null
  if (site.https && entry.httpsPort > 0) return { https: true, port: entry.httpsPort }
  if (entry.httpPort > 0) return { https: false, port: entry.httpPort }
  return null
}

/** 可点的访问链接，用第一个域名。通配域名拼不出具体地址，返回空 */
export function siteURL(entry: ProxyEntry | undefined, site: ProxySite): string {
  const addr = siteAddr(entry, site)
  const host = site.hosts?.[0] ?? ''
  if (!addr || !host || host.startsWith('*')) return ''
  return `${addr.https ? 'https' : 'http'}://${host}${portText(addr.https, addr.port)}${site.pathPrefix}`
}

// 不带宽度，留给调用方定。想窄的那几个（代理地址前面的 http/https 下拉）
// 不能写成 `${inputCls} w-24`——Tailwind 的 w-full 在样式表里排在 w-24 后面，
// 类名写在后面也不管用，只会把整行撑满、把后面的输入框顶出弹窗。
export const fieldCls =
  'rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-surface-2 disabled:text-muted'
export const inputCls = `w-full ${fieldCls}`
export const labelCls = 'mb-1 text-xs font-semibold text-muted'

