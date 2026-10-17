/**
 * 导航页用到的后端接口。
 *
 * 这一层只描述"有哪些接口、出入参是什么"，不含任何 UI 逻辑 ——
 * 组件不该自己拼 URL，否则改一个接口要满仓库找字符串。
 */

import { apiGet, apiSend, apiUpload } from './client'
import type {
  AppView,
  AuthStatus,
  CardSize,
  Category,
  HomeConfig,
  NetworkPrefer,
  SearchEngine,
  Visibility,
  Wallpaper,
} from '../types'

export const getConfig = (): Promise<HomeConfig> => apiGet<HomeConfig>('/api/home/config')

export const getAuthStatus = (): Promise<AuthStatus> => apiGet<AuthStatus>('/api/auth/status')

/** Bing 今日壁纸。加时间戳是故意的：这个接口有缓存，不带参会一直拿到旧图 */
export const getBingWallpaper = (resolution: 'uhd' | '1080'): Promise<{ url: string; copyright: string }> =>
  apiGet(`/api/home/bing-wallpaper?resolution=${resolution}&t=${Date.now()}`)

/* ============ 应用 ============ */

export interface AppPayload {
  name: string
  icon?: string
  color?: string
  categoryId?: string
  addresses?: { wanV4?: string; wanV6?: string; lan?: string }
  /** 排在第几个（1 开始）；不传 = 排到最后 */
  order?: number
}

export const addApp = (body: AppPayload) => apiSend<AppView>('POST', '/api/home/app/add', body)
export const updateApp = (id: string, body: Partial<AppPayload>) =>
  apiSend<AppView>('PUT', '/api/home/app/update', { id, ...body })
export const deleteApp = (id: string) => apiSend<unknown>('DELETE', '/api/home/app/delete', { id })

/** 重排：ids 是完整的新顺序，没点名的会按原顺序接到末尾 */
export const reorderApps = (ids: string[]) => apiSend<unknown>('PUT', '/api/home/app/reorder', { ids })

/** 改尺寸（右键菜单「修改大小」用） */
export const resizeApp = (id: string, size: CardSize) =>
  apiSend<AppView>('PUT', '/api/home/app/resize', { id, size })

/** 移动到某个分类 */
export const moveApp = (id: string, categoryId: string) =>
  apiSend<AppView>('PUT', '/api/home/app/category', { id, categoryId })

/* ============ 分类 ============ */

export const addCategory = (name: string, visibility: Visibility = 'public') =>
  apiSend<Category>('POST', '/api/home/category/add', { name, visibility })
export const updateCategory = (id: string, patch: { name?: string; visibility?: Visibility; password?: string }) =>
  apiSend<Category>('PUT', '/api/home/category/update', { id, ...patch })
export const deleteCategory = (id: string) => apiSend<unknown>('DELETE', '/api/home/category/delete', { id })
export const reorderCategories = (ids: string[]) =>
  apiSend<unknown>('PUT', '/api/home/category/reorder', { ids })

/** 用口令解锁一个分组。返回短期 token，前端存 sessionStorage */
export const unlockCategory = (id: string, password: string) =>
  apiSend<{ token: string }>('POST', '/api/home/category/unlock', { id, password })

/* ============ 外观与设置 ============ */

export const updateWallpaper = (body: Wallpaper) => apiSend<unknown>('PUT', '/api/home/wallpaper', body)
export const uploadWallpaper = (file: File) => apiUpload<{ path: string }>('/api/home/wallpaper/upload', file)
export const updateNetworkPrefer = (networkPrefer: NetworkPrefer) =>
  apiSend<unknown>('PUT', '/api/home/network', { networkPrefer })

/* ============ 图标 ============ */

export const uploadIcon = (file: File) => apiUpload<{ path: string }>('/api/home/icon/upload', file)
export const fetchIconFromURL = (url: string) =>
  apiSend<{ path: string }>('POST', '/api/home/icon/fetch', { url })

/* ============ 搜索 ============ */

export const getSearchHistory = (): Promise<string[]> => apiGet<string[]>('/api/home/search-history')
export const addSearchHistory = (keyword: string) =>
  apiSend<string[]>('POST', '/api/home/search-history/add', { keyword })
export const clearSearchHistory = () => apiSend<string[]>('DELETE', '/api/home/search-history/clear')

export const addSearchEngine = (body: Omit<SearchEngine, 'order'>) =>
  apiSend<SearchEngine>('POST', '/api/home/search-engine/add', body)
export const updateSearchEngine = (body: Omit<SearchEngine, 'order'>) =>
  apiSend<SearchEngine>('PUT', '/api/home/search-engine/update', body)
export const deleteSearchEngine = (id: string) =>
  apiSend<unknown>('DELETE', '/api/home/search-engine/delete', { id })

/** 跳到登录页（后台的入口） */
export const gotoLogin = (): void => {
  window.location.href = '/catflap/'
}

/** 退出登录 */
export const logout = (): Promise<void> => apiSend<unknown>('POST', '/api/auth/logout').then(() => undefined)
