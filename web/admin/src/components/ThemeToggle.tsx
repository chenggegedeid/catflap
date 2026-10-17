import { useEffect, useState } from 'react'
import { Monitor, Moon, Sun } from 'lucide-react'
import {
  type ThemeMode,
  applyTheme,
  getThemeMode,
  nextThemeMode,
  resolveDark,
  setThemeMode,
  themeLabel,
} from '../lib/theme'

/**
 * ThemeToggle 主题切换：一个按钮循环三档（浅色 → 深色 → 跟随系统）。
 *
 * 为什么是循环按钮而不是下拉：顶栏本来就很挤，下拉在手机上还容易误触。
 * 点一下切一档，图标本身就能表明当前档位。
 *
 * 移动端只显示图标（文字用 hidden sm:inline 藏掉），桌面端才带文字。
 */
export function ThemeToggle() {
  const [mode, setMode] = useState<ThemeMode>(getThemeMode)

  // 档位或系统偏好变了都要重新落到 <html> 上
  useEffect(() => {
    applyTheme(mode)
  }, [mode])

  // 选了「跟随系统」时，系统主题变了要跟着变
  useEffect(() => {
    if (mode !== 'system') return
    const mq = window.matchMedia('(prefers-color-scheme: dark)')
    const onChange = () => applyTheme('system')
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [mode])

  const cycle = () => {
    const next = nextThemeMode(mode)
    setThemeMode(next)
    setMode(next)
  }

  // 图标表示**当前的档位**，不是"点下去会变成什么" ——
  // 后者会让人以为显示月亮时已经是深色了
  const Icon = mode === 'light' ? Sun : mode === 'dark' ? Moon : Monitor
  const label = themeLabel(mode)

  return (
    <button
      type="button"
      onClick={cycle}
      title={`主题：${label}（点击切换）`}
      aria-label={`主题：${label}，点击切换`}
      className="flex shrink-0 items-center gap-1.5 rounded-lg px-2 py-1.5 text-xs font-semibold text-muted transition ring-1 ring-line hover:bg-surface-2 dark:text-muted dark:ring-slate-700 dark:hover:bg-inverse"
    >
      <Icon className="h-4 w-4" />
      {/* 窄屏只留图标，桌面才显示当前档位 */}
      <span className="hidden sm:inline">{label}</span>
    </button>
  )
}

/** 给需要在 JS 里判断当前是否深色的地方用 */
export const isDarkNow = (): boolean => resolveDark(getThemeMode())
