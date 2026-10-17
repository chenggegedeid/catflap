/**
 * 智能直达的匹配逻辑。
 *
 * 输入一个词，同时从三类来源里找：
 *   1. 应用（导航页上的卡片，含穿透自动带进来的服务）
 *   2. 搜索引擎（输入看起来像网址就直接跳，像搜索就走默认引擎）
 *
 * 相对极光Tab 的优势在这：**我们的服务清单是真的** ——
 * 名字、域名、端口、归属设备，全是后端 hydrate 出来的实数据，
 * 不需要用户先把书签存一遍。
 *
 * 打分规则（从上到下优先）：
 *   完全等于名字 > 名字以输入开头 > 名字包含输入 > 域名/地址包含输入
 * 同分按 order 排，保证结果稳定。
 */

import type { AppView, SearchEngine } from '../types'
import { entriesOf } from './addresses'

export interface SearchHit {
  kind: 'app' | 'engine' | 'url'
  /** 展示用的一行字 */
  label: string
  /** 次要说明（分类名 / 引擎名） */
  sub?: string
  /** 选中后去哪 */
  url: string
  /** 图标（应用有自己的图标；引擎用 shortName 渲染） */
  icon?: string
  color?: string
  shortName?: string
  refId: string
}

/** 看起来像网址吗（带点 + 没有空格，或者显式带了协议） */
export function looksLikeUrl(input: string): boolean {
  const s = input.trim()
  if (!s || /\s/.test(s)) return false
  if (/^https?:\/\//i.test(s)) return true
  return /^[a-z0-9-]+(\.[a-z0-9-]+)+(:\d+)?(\/.*)?$/i.test(s)
}

function score(text: string, q: string): number {
  const t = text.toLowerCase()
  const query = q.toLowerCase()
  if (t === query) return 100
  if (t.startsWith(query)) return 80
  if (t.includes(query)) return 60
  return 0
}

/**
 * 在应用里找。
 *
 * 匹配范围：名字、以及它的**所有入口地址** —— 用户记住的往往是
 * "nas.我的域名.com" 而不是"内网服务"这个名字，两个都得能搜到。
 */
export function matchApps(apps: AppView[], q: string): SearchHit[] {
  if (!q) return []
  const hits: SearchHit[] = []

  for (const a of apps) {
    let s = score(a.name, q)
    const entryList = entriesOf(a.addresses)
    let url = ''
    for (const e of entryList) {
      s = Math.max(s, score(e.url, q) - 10) // 地址命中比名字命中低一档
      if (!url) url = e.url
    }
    if (s <= 0) continue
    if (!url) url = entryList[0]?.url ?? ''
    hits.push({
      kind: 'app',
      label: a.name,
      sub: entryList.length > 0 ? entryList.map((e) => e.url.split('//')[1] ?? e.url).join(' · ') : undefined,
      url: url || a.name,
      icon: a.icon,
      color: a.color,
      refId: a.id,
    })
  }
  return hits
}

/** 引擎命中：输入正好是引擎名（或它的缩写）时，把该引擎提到前面 */
export function matchEngines(engines: SearchEngine[], q: string): SearchHit[] {
  if (!q) return []
  return engines
    .map((e) => ({ e, s: Math.max(score(e.name, q), score(e.shortName, q)) }))
    .filter(({ s }) => s > 0)
    .sort((a, b) => b.s - a.s)
    .map(({ e }) => ({
      kind: 'engine' as const,
      label: `用 ${e.name} 搜索`,
      sub: e.url,
      url: e.url + encodeURIComponent(q),
      icon: e.icon,
      color: e.color,
      shortName: e.shortName,
      refId: e.id,
    }))
}

/**
 * 汇总结果。
 *
 * 看起来像网址 → 第一条永远是"直接打开"，别让匹配把它挤下去；
 * 否则按 应用 > 引擎 排。
 */
export function searchAll(
  input: string,
  apps: AppView[],
  engines: SearchEngine[],
  defaultEngine?: SearchEngine,
): SearchHit[] {
  const q = input.trim()
  if (!q) return []

  const out: SearchHit[] = []

  if (looksLikeUrl(q)) {
    const url = /^https?:\/\//i.test(q) ? q : `https://${q}`
    out.push({ kind: 'url', label: `打开 ${url}`, url, refId: '__url__' })
  }

  out.push(...matchApps(apps, q))
  out.push(...matchEngines(engines, q))

  if (out.length === 0 && defaultEngine) {
    // 没有任何命中：兜底交给默认引擎 —— 用户敲了字总是想"去个地方"
    out.push({
      kind: 'engine',
      label: `用 ${defaultEngine.name} 搜索 "${q}"`,
      url: defaultEngine.url + encodeURIComponent(q),
      icon: defaultEngine.icon,
      color: defaultEngine.color,
      shortName: defaultEngine.shortName,
      refId: defaultEngine.id,
    })
  }

  return out.slice(0, 8)
}
