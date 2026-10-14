import { getDdnsProviderTypes } from './api'
import type { DdnsProviderTypeSpec, DnsProviderType } from '../types'

/**
 * 服务商类型表的进程内缓存。
 *
 * 这份数据是「系统支持哪些服务商」，进程生命周期内不会变，但有好几处要用：
 * DDNS 页的表单、STUN 页筛选「谁能写重定向规则」。每处各拉一次既浪费请求，
 * 又会让几处判断依据出现先后差异（刚加的服务商在一处已经能选、另一处还不能）。
 *
 * 失败时把缓存清掉，下次访问会重试；返回空数组让调用方走「暂时不知道」的分支，
 * 而不是抛出去把整页打挂 —— 一个下拉框拉不到不该让人看不见已经配好的记录。
 */
let cache: Promise<DdnsProviderTypeSpec[]> | null = null

export function loadProviderTypes(): Promise<DdnsProviderTypeSpec[]> {
  if (!cache) {
    cache = getDdnsProviderTypes().catch(() => {
      cache = null
      return [] as DdnsProviderTypeSpec[]
    })
  }
  return cache
}

/** 仅供测试/登出后清空用 */
export function resetProviderTypesCache() {
  cache = null
}

/** 找某个服务商类型的能力声明；未注册的类型返回 undefined */
export function findProviderType(
  list: DdnsProviderTypeSpec[],
  type: DnsProviderType,
): DdnsProviderTypeSpec | undefined {
  return list.find((t) => t.type === type)
}

/** 某个服务商类型是否具备某项能力（cap 取 ProviderCap 里的值） */
export function hasCapability(
  list: DdnsProviderTypeSpec[],
  type: DnsProviderType,
  cap: string,
): boolean {
  return findProviderType(list, type)?.capabilities?.includes(cap) ?? false
}

/** 服务商类型的展示名；未知类型退回显示原始 key，别显示成空白 */
export function providerTypeLabel(list: DdnsProviderTypeSpec[], type: DnsProviderType): string {
  return findProviderType(list, type)?.label ?? type
}
