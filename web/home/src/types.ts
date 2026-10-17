// 与后端 modules/home/model.go / hydrate.go / api/home_api 对应
//
// 这个文件是前后端的契约。**改这里要同步改 Go 那边的 struct**，
// 反过来也一样 —— 两边字段名对不上时不会报错，只会静默变成 undefined。

export type NetworkPrefer = 'wanV4' | 'wanV6' | 'lan'
export type WallpaperMode = 'default' | 'bing' | 'custom'
export type WallpaperResolution = '1080' | 'uhd'
export type AppType = 'stun' | 'static'

/**
 * 卡片尺寸，宽×高。
 *
 * 1×2 = 宽 1 列高 2 行（竖长条），2×1 = 宽 2 列高 1 行（横扁条）。
 * 和后端 modules/home/model.go 的 Size 字段一致。
 */
export type CardSize = '1x1' | '1x2' | '2x1' | '2x2' | '2x4'

export interface Wallpaper {
  mode: WallpaperMode
  resolution: WallpaperResolution
  blur: number
  /** 自己上传的壁纸，形如 data/wallpaper/xxx.jpg；和 mode 分开存，切走再切回来不用重传 */
  custom: string
  /** 默认背景的样式，.default-wallpaper 那条规则大括号里的内容；空 = 用内置的 */
  css: string
}

export interface SearchEngine {
  id: string
  name: string
  shortName: string
  url: string
  color: string
  icon?: string
  order: number
}

/**
 * 分组的可见性。
 *
 * - public 谁都能看（未登录也行）
 * - login  登录后才能看
 * - lock   即使登录了也要再输一次**分组独立口令**
 *
 * requireAuth 是老字段（true = 现在的 login），迁移后由 visibility 表达。
 */
export type Visibility = 'public' | 'login' | 'lock'

export interface Category {
  id: string
  name: string
  order: number
  /** 老字段：要登录后才看得见。新字段是 visibility，后端会迁移 */
  requireAuth?: boolean
  visibility?: Visibility
  /** visibility=lock 时，这个分组是否已经解锁（后端按解锁状态过滤内容） */
  unlocked?: boolean
  icon?: string
  createdAt?: string
  updatedAt?: string
}

export interface AppAddresses {
  wanV4: string
  wanV6: string
  lan: string
}

export interface AppView {
  id: string
  name: string
  icon: string
  color: string
  categoryId: string
  /** 排第几，1 开始。流式网格按它排，再按当前列数算位置 */
  order: number
  /** 卡片尺寸；空 = 1x1 */
  size: string
  type: AppType
  /** 未登录时后端会把内网地址留空 —— 见 get_home_config 的脱敏 */
  addresses: AppAddresses
  online?: boolean
  createdAt?: string
  updatedAt?: string
}

export interface SiteOption {
  id: number
  name: string
  host: string
}

export interface HomeConfig {
  title: string
  showTime: boolean
  wallpaper: Wallpaper
  networkPrefer: NetworkPrefer
  defaultSearchEngineId: string
  searchEngines: SearchEngine[]
  categories: Category[]
  apps: AppView[]
  /** 只给登录的人（后端过滤，未登录为 null） */
  sites: SiteOption[] | null
  /**
   * 因为没权限而被后端藏起来的分组数量（可选：后端还没发这个字段时用 0）。
   *
   * **只有数量、没有名单** —— 连"你藏了几个"都不该让访客知道，
   * 那本身也是信息。占位分组由前端据此渲染。
   */
  hiddenCategoryCount?: number
  /**
   * 分类切换方式：stream（一条长流，默认）或 paged（一屏一组）。
   * paged 解决"内容不足一屏时流式滚动没有可滚的余地"的问题。
   */
  pageMode?: string

  /**
   * 底部托盘的应用（按配置顺序）。
   *
   * 服务端已按"这次能看到什么"过滤过 —— 访客的托盘里不会有私密分组的应用，
   * 删掉的应用也不会留幽灵。前端直接渲染即可，不用再判断权限。
   */
  dock?: AppView[]
  /** 这次请求有没有登录态；只用于展示，真正的过滤在服务端 */
  authed?: boolean
}

/** 登录状态 */
export interface AuthStatus {
  authed: boolean
  username?: string
}
