import { describe, expect, it } from 'vitest'
import {
  entriesOf,
  hasEntry,
  mergeAddresses,
  preferredEntry,
  summarize,
} from './addresses'

describe('mergeAddresses', () => {
  it('v6 与内网：手填优先，自动兜底', () => {
    const out = mergeAddresses(
      { wanV6: 'auto-v6', lan: 'auto-lan' },
      { wanV6: 'manual-v6', lan: 'manual-lan' },
    )
    expect(out.wanV6).toBe('manual-v6')
    expect(out.lan).toBe('manual-lan')
  })

  it('手填没填的那一项，退回自动探测', () => {
    expect(mergeAddresses({ wanV6: 'auto-v6' }, { lan: 'manual-lan' }).wanV6).toBe('auto-v6')
  })

  /**
   * 这条是全套规则里唯一容易写错的地方，单独守着。
   *
   * 公网 v4 来自穿透，隧道重打之后地址会变；用户上次手填的很可能是个死地址。
   * 所以 v4 不能跟手填走 —— 必须以实时探测为准。
   */
  it('公网 v4 是例外：手填再怎么填，也以自动探测为准', () => {
    const out = mergeAddresses({ wanV4: 'auto-v4' }, { wanV4: 'manual-v4' })
    expect(out.wanV4).toBe('auto-v4')
  })

  it('自动探测没有 v4 时，才用手填的顶上', () => {
    expect(mergeAddresses({ wanV6: 'x' }, { wanV4: 'manual-v4' }).wanV4).toBe('manual-v4')
  })

  it('两边都没有就是空串，不是 undefined', () => {
    expect(mergeAddresses({}, {}).wanV4).toBe('')
  })
})

describe('entriesOf 与 summarize', () => {
  it('只列有值的，顺序固定 v4 → v6 → 内网', () => {
    const out = entriesOf({ lan: 'l', wanV4: 'a' })
    expect(out.map((e) => e.kind)).toEqual(['wanV4', 'lan'])
  })

  it('一个都没有时给「未配置地址」，不是空字符串', () => {
    expect(entriesOf({})).toEqual([])
    expect(summarize([])).toBe('未配置地址')
  })

  it('副标题用短名', () => {
    expect(summarize(entriesOf({ wanV4: 'a', wanV6: 'b', lan: 'c' }))).toBe('v4 · v6 · 内网')
  })
})

describe('preferredEntry', () => {
  const addr = { wanV4: 'v4-url', wanV6: 'v6-url', lan: 'lan-url' }

  it('偏好什么就给什么', () => {
    expect(preferredEntry(addr, 'lan')).toBe('lan-url')
  })

  it('偏好的那个没有，按兜底顺序往后找', () => {
    expect(preferredEntry({ wanV6: 'v6-url' }, 'wanV4')).toBe('v6-url')
    expect(preferredEntry({ lan: 'lan-url' }, 'wanV4')).toBe('lan-url')
  })

  it('全都没有返回 null —— 调用方据此把卡片置灰', () => {
    expect(preferredEntry({}, 'wanV4')).toBeNull()
    expect(preferredEntry(null, 'wanV4')).toBeNull()
  })
})

describe('hasEntry', () => {
  it('按类型判断有没有配', () => {
    expect(hasEntry({ wanV4: 'a' }, 'wanV4')).toBe(true)
    expect(hasEntry({ wanV4: 'a' }, 'lan')).toBe(false)
    expect(hasEntry(undefined, 'lan')).toBe(false)
  })
})
