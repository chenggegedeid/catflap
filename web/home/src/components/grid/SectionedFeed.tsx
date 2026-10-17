/**
 * 分组流。
 *
 * 极光那种"上下翻"的手感，本质上**不是翻页，是一条长流**：
 * 所有分组连着排，滚到哪儿，左栏就高亮哪一组。
 *
 * 为什么这样比"一次只显示一组"舒服：
 *   - 想找的东西在下一组时，往下滚就行，不用先点左栏再滚
 *   - 触控板上一个惯性滑动就能跨好几组，比"点、点、点"快
 *   - 分组短的时候不会出现大片空白（一次只显示一组时，短分组下面全是空的）
 *
 * 高亮的判定用**视口中线**：哪一组跨过了屏幕中间，就算当前组。
 * 用 IntersectionObserver 的阈值做会抖（两组同时可见时要选一个），
 * 手动量 scroll 位置反而更稳、更好解释。
 */

import { useEffect, useRef, type ReactNode } from 'react'

export function Section({
  id,
  title,
  children,
  registerRef,
}: {
  id: string
  title: string
  children: ReactNode
  registerRef: (id: string, el: HTMLElement | null) => void
}) {
  return (
    <section
      id={`cat-${id}`}
      ref={(el) => registerRef(id, el)}
      // scroll-mt 让"滚到这一组"时标题不会被顶部挡住
      className="scroll-mt-4"
    >
      {title && (
        // 分组名只是"你在哪一段"的路标，别做成大标题抢卡片的名字
        <h2 className="mb-1.5 px-1 text-[11px] font-medium tracking-wide text-white/55">{title}</h2>
      )}
      {children}
    </section>
  )
}

/**
 * 监听滚动，报告当前该高亮哪一组。
 *
 * 判据：所有 section 里，**顶部还没越过中线的最靠下的那个**。
 * 换句话说"已经滚过去的部分里，最后开始的那一组"。
 */
export function useScrollSpy(ids: string[], onChange: (id: string) => void) {
  const refs = useRef(new Map<string, HTMLElement>())

  useEffect(() => {
    if (ids.length === 0) return

    let raf = 0
    const compute = () => {
      raf = 0
      const mid = window.innerHeight * 0.35 // 偏上一点：更像"我正在看的这一组"
      let current = ids[0]
      for (const id of ids) {
        const el = refs.current.get(id)
        if (!el) continue
        const top = el.getBoundingClientRect().top
        if (top - mid <= 0) current = id
        else break
      }
      onChange(current)
    }

    // scroll 事件每秒能来上百次，用 rAF 合并成每帧一次
    const onScroll = () => {
      if (raf) return
      raf = window.requestAnimationFrame(compute)
    }

    compute()
    window.addEventListener('scroll', onScroll, { passive: true })
    window.addEventListener('resize', onScroll)
    return () => {
      window.removeEventListener('scroll', onScroll)
      window.removeEventListener('resize', onScroll)
      if (raf) window.cancelAnimationFrame(raf)
    }
  }, [ids, onChange])

  return {
    registerRef: (id: string, el: HTMLElement | null) => {
      if (el) refs.current.set(id, el)
      else refs.current.delete(id)
    },
  }
}
