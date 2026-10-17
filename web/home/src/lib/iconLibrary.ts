/**
 * 内置图标库。
 *
 * 为什么要有：抓 favicon 不是总能成功（站点没 favicon、内网服务、被墙），
 * 上传又太重 —— 而一个导航页里，NAS / 下载机 / 路由器这类条目根本没有
 * 可抓的 favicon。Sun-Panel 就是给一套内置图标让人直接挑，这个思路对。
 *
 * 存法：`icon` 字段写 `lib:<名字>`，AppIcon 认这个前缀直接渲染，
 * 不发网络请求。好处是**和上传/抓取的图标共用同一个字段**，不用加新字段，
 * 也不会因为后端换了目录就失效。
 */

export interface LibIcon {
  /** 存进 icon 字段的名字（不含前缀） */
  key: string
  /** 界面上显示的名字 */
  label: string
  /** lucide 图标名 */
  icon: string
  /** 底色（渐变），统一视觉 */
  from: string
  to: string
}

export interface IconGroup {
  name: string
  icons: LibIcon[]
}

export const ICON_LIBRARY: IconGroup[] = [
  {
    name: 'NAS 与存储',
    icons: [
      { key: 'nas', label: 'NAS', icon: 'Server', from: '#3b82f6', to: '#1d4ed8' },
      { key: 'disk', label: '硬盘', icon: 'HardDrive', from: '#64748b', to: '#334155' },
      { key: 'folder', label: '文件夹', icon: 'Folder', from: '#eab308', to: '#a16207' },
      { key: 'cloud', label: '网盘', icon: 'Cloud', from: '#38bdf8', to: '#0284c7' },
      { key: 'backup', label: '备份', icon: 'DatabaseBackup', from: '#14b8a6', to: '#0f766e' },
      { key: 'photo', label: '相册', icon: 'Images', from: '#f472b6', to: '#be185d' },
    ],
  },
  {
    name: '影音',
    icons: [
      { key: 'video', label: '影音', icon: 'Film', from: '#8b5cf6', to: '#6d28d9' },
      { key: 'music', label: '音乐', icon: 'Music', from: '#f43f5e', to: '#be123c' },
      { key: 'tv', label: '电视', icon: 'Tv', from: '#0ea5e9', to: '#0369a1' },
      { key: 'cast', label: '投屏', icon: 'Cast', from: '#22c55e', to: '#15803d' },
      { key: 'download', label: '下载', icon: 'Download', from: '#f97316', to: '#c2410c' },
      { key: 'play', label: '播放', icon: 'PlayCircle', from: '#a855f7', to: '#7e22ce' },
    ],
  },
  {
    name: '网络与运维',
    icons: [
      { key: 'router', label: '路由', icon: 'Router', from: '#10b981', to: '#047857' },
      { key: 'wifi', label: '无线', icon: 'Wifi', from: '#06b6d4', to: '#0e7490' },
      { key: 'shield', label: '安全', icon: 'Shield', from: '#ef4444', to: '#b91c1c' },
      { key: 'terminal', label: '终端', icon: 'Terminal', from: '#475569', to: '#0f172a' },
      { key: 'monitor', label: '监控', icon: 'Activity', from: '#84cc16', to: '#4d7c0f' },
      { key: 'globe', label: '网站', icon: 'Globe', from: '#6366f1', to: '#4338ca' },
    ],
  },
  {
    name: '常用',
    icons: [
      { key: 'home', label: '首页', icon: 'Home', from: '#f59e0b', to: '#b45309' },
      { key: 'book', label: '阅读', icon: 'BookOpen', from: '#ec4899', to: '#9d174d' },
      { key: 'chat', label: '聊天', icon: 'MessageCircle', from: '#2dd4bf', to: '#0d9488' },
      { key: 'mail', label: '邮件', icon: 'Mail', from: '#60a5fa', to: '#2563eb' },
      { key: 'calendar', label: '日历', icon: 'Calendar', from: '#fb7185', to: '#e11d48' },
      { key: 'settings', label: '设置', icon: 'Settings', from: '#94a3b8', to: '#475569' },
      { key: 'code', label: '开发', icon: 'Code', from: '#4ade80', to: '#16a34a' },
      { key: 'game', label: '游戏', icon: 'Gamepad2', from: '#c084fc', to: '#9333ea' },
      { key: 'shop', label: '购物', icon: 'ShoppingCart', from: '#fbbf24', to: '#d97706' },
      { key: 'tool', label: '工具', icon: 'Wrench', from: '#a1a1aa', to: '#52525b' },
    ],
  },
]

/** 存进 icon 字段的前缀 */
export const LIB_PREFIX = 'lib:'

/** 图标库每个条目的完整值（写进 icon 字段） */
export function libIconValue(key: string): string {
  return `${LIB_PREFIX}${key}`
}

/** 判断一个 icon 值是不是内置图标库里的 */
export function isLibIcon(icon: string): boolean {
  return !!icon && icon.startsWith(LIB_PREFIX)
}

/** 从 icon 值里取出图标库条目的定义；找不到返回 null */
export function findLibIcon(icon: string): LibIcon | null {
  if (!isLibIcon(icon)) return null
  const key = icon.slice(LIB_PREFIX.length)
  for (const g of ICON_LIBRARY) {
    for (const it of g.icons) {
      if (it.key === key) return it
    }
  }
  return null
}
