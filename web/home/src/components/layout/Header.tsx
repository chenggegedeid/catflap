/**
 * 页头：站点标题 + 时钟（含农历）。
 *
 * 计时器只在 showTime 打开时才起 —— 关了还每秒 setState 是白烧电。
 * 农历用的是 Intl 的 chinese 日历，浏览器不支持时静默省略，不报错。
 */

import { useEffect, useState } from 'react'

function lunarDay(now: Date): string {
  try {
    // 浏览器不支持 Chinese 日历时这里会抛 —— 农历只是锦上添花，不该让页头崩掉
    const s = new Intl.DateTimeFormat('zh-CN-u-ca-chinese', { day: 'numeric', month: 'long' }).format(now)
    // 形如 "九月十一"，去掉可能的闰月标记前缀
    return s.replace(/^闰/, '')
  } catch {
    return ''
  }
}

export function Header({ title, showTime }: { title: string; showTime: boolean }) {
  const [now, setNow] = useState(() => new Date())

  useEffect(() => {
    if (!showTime) return
    const timer = window.setInterval(() => setNow(new Date()), 1000)
    return () => window.clearInterval(timer)
  }, [showTime])

  const time = now.toLocaleTimeString('zh-CN', { hour12: false })
  const date = now.toLocaleDateString('zh-CN', { month: 'long', day: 'numeric', weekday: 'long' })
  const lunar = showTime ? lunarDay(now) : ''

  return (
    <header className="flex flex-col items-center gap-1 text-white drop-shadow">
      <h1 className="text-3xl font-bold tracking-tight sm:text-4xl">{title || 'Catflap'}</h1>
      {showTime && (
        <p className="flex items-baseline gap-2 text-sm text-white/85">
          <span className="text-xl font-semibold tabular-nums text-white">{time}</span>
          <span>
            {date}
            {lunar && <span className="ml-1 text-white/70">{lunar}</span>}
          </span>
        </p>
      )}
    </header>
  )
}
