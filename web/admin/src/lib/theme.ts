/**
 * 主题：跟随系统 / 浅色 / 深色。
 *
 * 存在理由和「每页条数」一样 —— 记住用户的选择。默认**跟随系统**：
 * 新装的用户直接得到符合系统偏好的外观，不用先去设置里翻。
 */

export type ThemeMode = 'system' | 'light' | 'dark'

const THEME_KEY = 'catflap.theme'

export const getThemeMode = (): ThemeMode => {
  try {
    const v = localStorage.getItem(THEME_KEY)
    if (v === 'light' || v === 'dark' || v === 'system') return v
  } catch {
    // 隐私模式下读不到，按跟随系统处理
  }
  return 'system'
}

export const setThemeMode = (mode: ThemeMode): void => {
  try {
    localStorage.setItem(THEME_KEY, mode)
  } catch {
    // 存不进去最多是下次回到跟随系统
  }
}

/** 系统现在偏好深色吗 */
export const systemPrefersDark = (): boolean =>
  typeof window !== 'undefined' &&
  typeof window.matchMedia === 'function' &&
  window.matchMedia('(prefers-color-scheme: dark)').matches

/** 这个模式下，界面最终应该是深色吗 */
export const resolveDark = (mode: ThemeMode): boolean =>
  mode === 'dark' || (mode === 'system' && systemPrefersDark())

/**
 * 把主题落到 <html> 上。
 *
 * 用 class 而不是在 body 上改样式：Tailwind 的 dark 变体是 `&:where(.dark, .dark *)`，
 * 必须挂在祖先元素上整棵子树才生效。
 */
export const applyTheme = (mode: ThemeMode): void => {
  const dark = resolveDark(mode)
  const root = document.documentElement
  root.classList.toggle('dark', dark)
  // 让浏览器把表单控件、滚动条也跟着变（否则深色页面里弹出浅色的原生下拉会很突兀）
  root.style.colorScheme = dark ? 'dark' : 'light'
}

/** 循环切换：浅色 → 深色 → 跟随系统 */
export const nextThemeMode = (mode: ThemeMode): ThemeMode => {
  if (mode === 'light') return 'dark'
  if (mode === 'dark') return 'system'
  return 'light'
}

/** 当前档位叫什么（给按钮的 title 和无障碍标签用） */
export const themeLabel = (mode: ThemeMode): string => {
  if (mode === 'light') return '浅色'
  if (mode === 'dark') return '深色'
  return '跟随系统'
}
