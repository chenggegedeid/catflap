import { describe, expect, it } from 'vitest'
import { defaultCategoryId, resolveActive, sortedCategories } from './view'
import type { HomeConfig } from '../types'

function cfg(cats: { id: string; order: number; name?: string }[]): HomeConfig {
  return {
    title: '',
    showTime: false,
    wallpaper: { mode: 'default', resolution: '1080', blur: 0, custom: '', css: '' },
    networkPrefer: 'wanV4',
    defaultSearchEngineId: 'bing',
    searchEngines: [],
    categories: cats.map((c) => ({ id: c.id, name: c.name ?? c.id, order: c.order })),
    apps: [],
    sites: null,
  }
}

describe('sortedCategories', () => {
  it('按 order 升序，不改原数组', () => {
    const c = cfg([
      { id: 'b', order: 2 },
      { id: 'a', order: 1 },
    ])
    expect(sortedCategories(c).map((x) => x.id)).toEqual(['a', 'b'])
    // 原数组顺序不该被就地改掉（组件里可能同时拿它做别的）
    expect(c.categories[0].id).toBe('b')
  })
})

describe('defaultCategoryId', () => {
  // 这条是产品规矩的判据：首页没有「全部」，默认就落在第一个分组
  it('默认是第一个分组，不是"全部"', () => {
    expect(defaultCategoryId(cfg([{ id: 'work', order: 1 }, { id: 'fun', order: 2 }]))).toBe('work')
  })

  it('没有分组时返回空串', () => {
    expect(defaultCategoryId(cfg([]))).toBe('')
    expect(defaultCategoryId(null)).toBe('')
  })
})

describe('resolveActive', () => {
  it('选中的还在就用它', () => {
    expect(resolveActive(cfg([{ id: 'a', order: 1 }, { id: 'b', order: 2 }]), 'b')).toBe('b')
  })

  // 删掉当前分组后不能掉进"全部"视图 —— 那会让用户突然看到所有东西
  it('选中的被删了，落回第一个而不是空串', () => {
    expect(resolveActive(cfg([{ id: 'a', order: 1 }]), 'gone')).toBe('a')
  })

  it('空串也落回第一个', () => {
    expect(resolveActive(cfg([{ id: 'a', order: 1 }]), '')).toBe('a')
  })
})
