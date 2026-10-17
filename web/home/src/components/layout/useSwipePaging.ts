/**
 * 触屏左右滑动切换分组。
 *
 * 为什么要有：滚轮翻页在触屏上根本不存在（没有 wheel 事件），
 * 手机上除了点左栏就没有任何"翻页"方式 —— 而滑动切页是移动端
 * 导航页的通用手势，用户会本能地试。
 *
 * 三条不能省的判断：
 * 1. **必须主要是横向**：纵向滑动是滚页面/拖拽，不能抢
 * 2. **位移要够**：小于阈值不响应，否则点一下卡片就被当成滑动了
 * 3. **拖拽中不处理**：正在拖卡片时手指也在动，那是排序不是翻页
 */

import { useEffect, useRef } from 'react'

const MIN_DX = 60
const MAX_DY = 60

export function useSwipePaging(
  enabled: boolean,
  /** 正在拖拽时不响应（拖拽本身也在用手指） */
  busy: boolean,
  onChange: (dir: 1 | -1) => void,
) {
  const start = useRef<{ x: number; y: number } | null>(null)
  // 回调放 ref：避免父组件每次重渲染都重新绑监听
  const cb = useRef(onChange)
  cb.current = onChange

  useEffect(() => {
    if (!enabled) return

    const onStart = (e: TouchEvent) => {
      if (e.touches.length !== 1) return
      start.current = { x: e.touches[0].clientX, y: e.touches[0].clientY }
    }

    const onEnd = (e: TouchEvent) => {
      const s = start.current
      start.current = null
      if (!s) return
      const t = e.changedTouches[0]
      if (!t) return
      const dx = t.clientX - s.x
      const dy = t.clientY - s.y

      // 主要是横向，且幅度够大 —— 否则是滚动/点击，不管
      if (Math.abs(dx) < MIN_DX) return
      if (Math.abs(dy) > MAX_DY) return
      if (Math.abs(dx) <= Math.abs(dy)) return

      cb.current(dx < 0 ? 1 : -1)
    }

    const el = document
    el.addEventListener('touchstart', onStart, { passive: true })
    el.addEventListener('touchend', onEnd, { passive: true })
    return () => {
      el.removeEventListener('touchstart', onStart)
      el.removeEventListener('touchend', onEnd)
    }
  }, [enabled, busy])
}
