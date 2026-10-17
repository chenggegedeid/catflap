/**
 * 图标。
 *
 * 两件事：
 * 1. 统一套一层白色圆角方块底（连续圆角，约 22%）—— 各站点的图标风格千奇百怪，
 *    直接铺在壁纸上的话整页会很散；套进同一个形状里立刻就整齐了。
 * 2. **加载失败要退化**，不是裂图也不是空洞。退化成站点名首字 + 一个由名字
 *    算出来的稳定底色（同一个名字永远同一个颜色，不会因为重渲染换来换去）。
 */

import { useState } from 'react'
import * as Lucide from 'lucide-react'
import { fallbackBg, iconFallback, iconSrc } from '../../lib/icons'
import { findLibIcon } from '../../lib/iconLibrary'

export function AppIcon({ name, icon, color }: { name: string; icon: string; color?: string }) {
  const [broken, setBroken] = useState(false)
  const src = icon ? iconSrc(icon) : ''

  /**
   * 内置图标库（`lib:<名字>`）直接渲染，不发网络请求。
   *
   * 放在最前面判断：这类图标永远可用，不该走"加载失败 → 退化"那条路。
   * 用 lucide 的命名空间动态取（图标名是数据，不是写死的组件）。
   */
  const lib = findLibIcon(icon)
  if (lib) {
    const Icon = (Lucide as unknown as Record<string, React.ElementType>)[lib.icon]
    if (Icon) {
      return (
        <span
          className="grid h-full w-full place-items-center text-white"
          style={{ background: `linear-gradient(135deg, ${lib.from}, ${lib.to})` }}
          aria-hidden
        >
          <Icon className="h-[55%] w-[55%]" strokeWidth={2} />
        </span>
      )
    }
    // 图标名写错了（将来删改图标库时可能遇到）→ 落回下面的退化逻辑
  }

  if (!src || broken) {
    const { initial, hue } = iconFallback(name)
    return (
      <span
        className="grid h-full w-full place-items-center text-base font-bold text-white"
        style={{ background: fallbackBg(hue) }}
        aria-hidden
      >
        {initial}
      </span>
    )
  }

  return (
    <img
      src={src}
      alt=""
      loading="lazy"
      onError={() => setBroken(true)}
      className="h-full w-full object-contain p-[18%]"
      style={{ background: color ? undefined : '#fff' }}
    />
  )
}
