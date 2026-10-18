import { setPageLimit } from '../../lib/prefs'
import type { AuditIPStat } from '../../types'

export const fmtBytes = (n: number): string => {
  if (!n) return '0 B'
  const u = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < u.length - 1) {
    v /= 1024
    i++
  }
  return `${v < 10 && i > 0 ? v.toFixed(1) : v.toFixed(0)} ${u[i]}`
}

export const fmtCount = (n: number): string => {
  if (n >= 1e8) return `${(n / 1e8).toFixed(1)} 亿`
  if (n >= 1e4) return `${(n / 1e4).toFixed(1)} 万`
  return String(n)
}

/** 封禁时长选项。0 = 永久 —— 放在最后，因为它最需要想清楚 */
export const durations = [
  { label: '1 小时', minutes: 60 },
  { label: '1 天', minutes: 60 * 24 },
  { label: '7 天', minutes: 60 * 24 * 7 },
  { label: '永久', minutes: 0 },
]

/**
 * 改每页条数：先记进 localStorage，再更新组件状态。
 *
 * 顺序不能反 —— 也正因为以前压根没存，才会出现「调到 10、刷新又变 50」。
 */
export function applyLimit(setter: (n: number) => void) {
  return (n: number) => {
    setPageLimit(n)
    setter(n)
  }
}

export function isSuspicious(x: AuditIPStat): boolean {
  if (x.errors < 50 || x.requests === 0) return false
  return x.errors / x.requests >= 0.8
}

export function blockUntil(expiresAt?: string): string {
  if (!expiresAt) return '永久'
  const d = new Date(expiresAt)
  if (Number.isNaN(d.getTime()) || d.getUTCFullYear() <= 1) return '永久'
  return `至 ${d.toLocaleString('zh-CN')}`
}

/** IPCard 一份 IP / 网段名单的编辑器（黑白名单共用） */
