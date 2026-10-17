/**
 * 壁纸层。
 *
 * 三种来源：内置渐变（default）、Bing 每日图（bing）、自己上传（custom）。
 *
 * 两个容易踩的点：
 * 1. **自定义 CSS 是用户写的**，只能塞进 <style>，不能用 style 属性 ——
 *    那里写不了 `radial-gradient(...)` 这种多行的规则体。
 *    这段内容来自管理员（要登录才能改），不是访客输入的，所以不做转义；
 *    但它仍然是"把一段 CSS 插进页面"，因此只挂在 .default-wallpaper 这一条规则里，
 *    不让它有机会影响别的元素。
 * 2. 图片加载失败要**静默退回渐变**。外链图挂掉是常事，
 *    弹个裂图图标在首页正中间很难看。
 */

import { useEffect, useState } from 'react'
import { getBingWallpaper } from '../../api/home'
import type { Wallpaper } from '../../types'

export function Background({ wallpaper }: { wallpaper?: Wallpaper }) {
  const mode = wallpaper?.mode ?? 'default'
  const blur = wallpaper?.blur ?? 0
  const custom = wallpaper?.custom ?? ''
  const userCss = wallpaper?.css ?? ''

  const [bingUrl, setBingUrl] = useState('')
  const [bingFailed, setBingFailed] = useState(false)

  useEffect(() => {
    if (mode !== 'bing') return
    let alive = true
    getBingWallpaper((wallpaper?.resolution ?? '1080') as 'uhd' | '1080')
      .then((r) => {
        if (alive && r?.url) setBingUrl(r.url)
        else if (alive) setBingFailed(true)
      })
      .catch(() => {
        if (alive) setBingFailed(true)
      })
    return () => {
      alive = false
    }
  }, [mode, wallpaper?.resolution])

  // 用户自己写的 CSS。
  //
  // 类名写两遍是为了把特异度提上去 —— App.css 里那条内置渐变也是单类名，
  // 单类名盖不住它（谁在后面谁赢，不可靠）。这条约定和 App.css 里的注释是一致的。
  // 只作用在这一个元素上，别让它有机会影响页面其它地方。
  const userStyle = userCss.trim() ? `.default-wallpaper.default-wallpaper{${userCss}}` : ''

  return (
    <>
      {userStyle && <style>{userStyle}</style>}

      <div className="pointer-events-none fixed inset-0 -z-10 overflow-hidden">
        {/* 兜底渐变：任何模式下都垫在底下，图没出来时页面不会是黑的 */}
        <div className="default-wallpaper absolute inset-0" />

        {mode === 'custom' && custom && (
          <img
            src={custom.startsWith('/') ? custom : `/${custom}`}
            alt=""
            className="absolute inset-0 h-full w-full object-cover"
            style={{ filter: blur > 0 ? `blur(${blur}px)` : undefined }}
          />
        )}

        {mode === 'bing' && bingUrl && !bingFailed && (
          <img
            src={bingUrl}
            alt=""
            onError={() => setBingFailed(true)}
            className="absolute inset-0 h-full w-full object-cover"
            style={{ filter: blur > 0 ? `blur(${blur}px)` : undefined }}
          />
        )}

        {/* 压一层暗色。
            25% 太浅 —— 内置渐变是淡蓝白，白字压上去对比度只有 1.6:1 左右
            （WCAG AA 要求 4.5:1），标题和卡片名几乎看不见。
            45% 是淡渐变上实测能读清的最低值；自定义壁纸偏亮时也够用，
            偏暗时也不至于被压黑。 */}
        <div className="absolute inset-0 bg-slate-900/45" />
      </div>
    </>
  )
}
