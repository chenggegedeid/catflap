/**
 * 三网入口（公网 v4 / 公网 v6 / 内网）的合并与选择。
 *
 * 规则是固定的，别在组件里各写各的：
 *
 *   **手填优先，自动探测兜底。唯一的例外是公网 v4 —— 它始终以实时探测为准。**
 *
 * 例外的原因：v4 那条地址来自穿透，隧道重打之后会变；用户上一次手填的
 * 很可能已经失效，跟着手填走就会连到一个死地址上。v6 和内网一般不会自己变
 * （v6 是家宽分配的地址，内网是路由器里的固定 IP），所以那两个信用户填的。
 */

export type NetworkKind = 'wanV4' | 'wanV6' | 'lan'

/** 一份地址（可能是自动探测来的，也可能是用户手填的） */
export interface AddressSet {
  wanV4?: string
  wanV6?: string
  lan?: string
}

/** 合并后的一个入口 */
export interface Entry {
  kind: NetworkKind
  label: string
  url: string
}

export const NETWORK_LABEL: Record<NetworkKind, string> = {
  wanV4: '公网 v4',
  wanV6: '公网 v6',
  lan: '内网',
}

/** 卡片副标题上的短名 */
const SHORT_LABEL: Record<NetworkKind, string> = {
  wanV4: 'v4',
  wanV6: 'v6',
  lan: '内网',
}

/** 各网络类型的兜底顺序：想要 v4 就 v4 → v6 → 内网，以此类推 */
const FALLBACK: Record<NetworkKind, NetworkKind[]> = {
  wanV4: ['wanV4', 'wanV6', 'lan'],
  wanV6: ['wanV6', 'wanV4', 'lan'],
  lan: ['lan', 'wanV4', 'wanV6'],
}

/**
 * 合并自动探测与手填两份地址，得到最终可用的入口。
 *
 * @param auto   后端 hydrate 出来的（穿透实时探测）
 * @param manual 用户在表单里手填的
 */
export function mergeAddresses(auto?: AddressSet | null, manual?: AddressSet | null): AddressSet {
  const pick = (k: NetworkKind): string => {
    // v4 永远信自动探测；v6 / 内网手填优先
    if (k === 'wanV4') return auto?.[k] || manual?.[k] || ''
    return manual?.[k] || auto?.[k] || ''
  }
  return {
    wanV4: pick('wanV4'),
    wanV6: pick('wanV6'),
    lan: pick('lan'),
  }
}

/** 这个应用当前有哪些可用入口（空的不列，保持顺序稳定） */
export function entriesOf(addr?: AddressSet | null): Entry[] {
  if (!addr) return []
  const order: NetworkKind[] = ['wanV4', 'wanV6', 'lan']
  return order
    .filter((k) => !!addr[k])
    .map((k) => ({ kind: k, label: NETWORK_LABEL[k], url: addr[k]! }))
}

/** 卡片副标题：`v4 · v6 · 内网`；一个都没有时给「未配置地址」 */
export function summarize(entries: Entry[]): string {
  if (entries.length === 0) return '未配置地址'
  return entries.map((e) => SHORT_LABEL[e.kind]).join(' · ')
}

/**
 * 按网络偏好选出「点了就进」的那个入口。
 *
 * 想要的那个没有，就按兜底顺序往后找；全都没有返回 null
 * （调用方据此把卡片置灰，而不是给一个点了没反应的按钮）。
 */
export function preferredEntry(addr: AddressSet | null | undefined, prefer: NetworkKind): string | null {
  if (!addr) return null
  for (const k of FALLBACK[prefer]) {
    const v = addr[k]
    if (v) return v
  }
  return null
}

/** 某个入口有没有配（卡片上的三个小徽标按这个点亮/熄灭） */
export function hasEntry(addr: AddressSet | null | undefined, kind: NetworkKind): boolean {
  return !!addr?.[kind]
}

/**
 * 打开一个地址。
 *
 * `noopener,noreferrer` 不能省：新页面会拿到 `window.opener`，
 * 能反过来操作本页（钓鱼站常用这招）。referrer 也别带 —— 内网地址
 * 没必要泄漏给外部站点。
 */
export function openEntry(url: string): void {
  window.open(url, '_blank', 'noopener,noreferrer')
}
