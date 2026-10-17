import { describe, expect, it } from 'vitest'
import { looksLikeUrl, matchApps, matchEngines, searchAll } from './search'
import type { AppView, SearchEngine } from '../types'

const apps: AppView[] = [
  {
    id: 'nas', name: '内网服务', icon: '', color: '', categoryId: 'c1', order: 1, size: '1x1', type: 'static',
    addresses: { wanV4: '', wanV6: '', lan: 'http://192.168.9.6:5000' },
  },
  {
    id: 'dd', name: 'DDNS 控制台', icon: '', color: '', categoryId: 'c1', order: 2, size: '1x1', type: 'static',
    addresses: { wanV4: 'https://ddns.example.com', wanV6: '', lan: '' },
  },
]

const engines: SearchEngine[] = [
  { id: 'bing', name: '必应', shortName: 'B', url: 'https://www.bing.com/search?q=', color: '', order: 1 },
  { id: 'google', name: '谷歌', shortName: 'G', url: 'https://www.google.com/search?q=', color: '', order: 2 },
]

describe('looksLikeUrl', () => {
  it('带点的像网址，中文不像', () => {
    expect(looksLikeUrl('nas.example.com')).toBe(true)
    expect(looksLikeUrl('https://a.b/c')).toBe(true)
    expect(looksLikeUrl('内网服务')).toBe(false)
    expect(looksLikeUrl('nas example')).toBe(false)
  })
})

describe('matchApps', () => {
  it('名字开头命中', () => {
    const hits = matchApps(apps, 'ddns')
    expect(hits).toHaveLength(1)
    expect(hits[0].label).toBe('DDNS 控制台')
  })

  it('地址也能搜到 —— 用户记住的往往是域名而不是名字', () => {
    const hits = matchApps(apps, '192.168.9')
    expect(hits).toHaveLength(1)
    expect(hits[0].label).toBe('内网服务')
  })

  it('匹配不上的不硬塞', () => {
    expect(matchApps(apps, 'zzz')).toEqual([])
  })
})

describe('matchEngines', () => {
  it('引擎名和缩写都能命中', () => {
    expect(matchEngines(engines, '必应')[0].refId).toBe('bing')
    expect(matchEngines(engines, 'g')[0].refId).toBe('google')
  })
})

describe('searchAll', () => {
  it('像网址的永远排第一，不被应用匹配挤下去', () => {
    const hits = searchAll('ddns.example.com', apps, engines)
    expect(hits[0].kind).toBe('url')
    expect(hits[0].url).toBe('https://ddns.example.com')
  })

  it('没有命中时兜底交给默认引擎 —— 敲了字总是想去个地方', () => {
    const hits = searchAll('完全不存在的东西', apps, engines, engines[0])
    expect(hits).toHaveLength(1)
    expect(hits[0].url).toContain('bing.com')
    expect(hits[0].url).toContain(encodeURIComponent('完全不存在的东西'))
  })

  it('空输入不给任何结果（也不能兜底）', () => {
    expect(searchAll('  ', apps, engines, engines[0])).toEqual([])
  })
})
