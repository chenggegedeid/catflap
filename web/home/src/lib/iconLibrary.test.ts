import { describe, expect, it } from 'vitest'
import { ICON_LIBRARY, findLibIcon, isLibIcon, libIconValue } from './iconLibrary'

describe('iconLibrary', () => {
  it('值带前缀，能认出来', () => {
    const v = libIconValue('nas')
    expect(v).toBe('lib:nas')
    expect(isLibIcon(v)).toBe(true)
    expect(isLibIcon('/data/icon/x.png')).toBe(false)
    expect(isLibIcon('')).toBe(false)
  })

  it('能按 key 找到条目', () => {
    const it = findLibIcon('lib:nas')
    expect(it?.label).toBe('NAS')
    expect(it?.icon).toBeTruthy()
  })

  it('找不到的返回 null（不该抛）', () => {
    expect(findLibIcon('lib:不存在的图标')).toBeNull()
    expect(findLibIcon('https://x.com/favicon.ico')).toBeNull()
  })

  // 这条防的是"图标库里写了 lucide 没有的名字" —— 一旦发生会静默退化成首字
  it('库里每个图标的 key 唯一，且 lucide 图标名非空', async () => {
    const Lucide = await import('lucide-react')
    const seen = new Set<string>()
    for (const g of ICON_LIBRARY) {
      for (const it of g.icons) {
        expect(seen.has(it.key), `key 重复：${it.key}`).toBe(false)
        seen.add(it.key)
        expect(it.icon, `${it.key} 缺 lucide 图标名`).toBeTruthy()
        const comp = (Lucide as unknown as Record<string, unknown>)[it.icon]
        expect(comp, `lucide 里没有这个图标：${it.icon}（${it.key}）`).toBeTruthy()
      }
    }
  })
})
