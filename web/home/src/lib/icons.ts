/**
 * 图标的显示与降级。
 *
 * 图标来源有三种：完整 URL、data URI、本站相对路径（`/data/icon/xxx`）。
 * 老代码里那个判断散在组件中，这里收成一处。
 *
 * 另外一件重要的事：**加载失败时要退化成什么**。
 * 参照极光Tab 的做法（实测它就这么干）—— 图标没加载出来时显示的是
 * 纯色圆角方块，页面依然整齐。而不是浏览器的裂图或者一个空洞。
 */

/** 图标字段 → 可直接给 <img src> 的值 */
export function iconSrc(icon: string): string {
  if (!icon) return ''
  if (/^(https?:|data:)/i.test(icon)) return icon
  if (icon.startsWith('/')) return icon
  return `/${icon}`
}

/**
 * 图标加载失败时用的占位：站点名首字 + 一个稳定的底色。
 *
 * 底色按名字算哈希取，同一个名字永远同一个颜色 —— 不会因为重渲染
 * 而换来换去（那是廉价感的来源之一）。
 */
export function iconFallback(name: string): { initial: string; hue: number } {
  const trimmed = (name || '').trim()
  // 取首个字符，中文取首字、英文取首字母（大写）
  const initial = trimmed ? trimmed[0].toUpperCase() : '?'

  let hash = 0
  for (let i = 0; i < trimmed.length; i++) {
    hash = (hash * 31 + trimmed.charCodeAt(i)) >>> 0
  }
  return { initial, hue: hash % 360 }
}

/** 占位方块的背景（HSL 字符串，直接给 style.background 用） */
export function fallbackBg(hue: number): string {
  // 饱和度与亮度压在中间区间：太艳会跟壁纸打架，太灰又看不出是个块
  return `hsl(${hue} 52% 46%)`
}

/** 名字太长时截断（卡片宽度有限，两行封顶由 CSS 负责，这里只防超长单词） */
export function truncateName(name: string, max = 24): string {
  if (!name) return ''
  return name.length > max ? `${name.slice(0, max - 1)}…` : name
}

/**
 * 默认颜色预设。
 *
 * **关键：存的是 Tailwind 渐变类名，不是色值。**
 * 之前存 hex 要再手写一段"hex → 渐变"的转换，等于同一个视觉规则写两遍，
 * 改一处忘一处。现在前后端都只认这个字符串。
 *
 * 而且这些类名必须在源码里**完整出现**（Tailwind 扫源码生成样式），
 * 所以不能拼接字符串，只能老老实实列全。
 */
export interface ColorPreset {
  id: string
  name: string
  color: string
}

export const DEFAULT_PRESETS: ColorPreset[] = [
  { id: 'blue', name: '蓝', color: 'from-blue-500 to-blue-700' },
  { id: 'indigo', name: '靛', color: 'from-indigo-500 to-indigo-700' },
  { id: 'violet', name: '紫', color: 'from-violet-500 to-violet-700' },
  { id: 'emerald', name: '绿', color: 'from-emerald-500 to-emerald-700' },
  { id: 'amber', name: '橙', color: 'from-amber-500 to-amber-700' },
  { id: 'rose', name: '玫红', color: 'from-rose-500 to-rose-700' },
  { id: 'slate', name: '灰蓝', color: 'from-slate-500 to-slate-700' },
  { id: 'cyan', name: '青', color: 'from-cyan-500 to-cyan-700' },
]
