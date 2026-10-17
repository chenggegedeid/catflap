import { describe, expect, it } from 'vitest'

/**
 * 抽成纯函数测的是**判据本身**，不是 hook 的挂载 ——
 * 这类"位移够不够、是不是横向"的阈值最容易写反，写反了表现是
 * "点一下卡片就翻页了"或者"怎么滑都没反应"。
 */
function judge(dx: number, dy: number): 1 | -1 | 0 {
  const MIN_DX = 60
  const MAX_DY = 60
  if (Math.abs(dx) < MIN_DX) return 0
  if (Math.abs(dy) > MAX_DY) return 0
  if (Math.abs(dx) <= Math.abs(dy)) return 0
  return dx < 0 ? 1 : -1
}

describe('滑动翻页判据', () => {
  it('左滑 → 下一组', () => {
    expect(judge(-100, 5)).toBe(1)
  })

  it('右滑 → 上一组', () => {
    expect(judge(100, -5)).toBe(-1)
  })

  it('位移太小不响应（否则点一下卡片就翻页）', () => {
    expect(judge(-30, 2)).toBe(0)
    expect(judge(40, 0)).toBe(0)
  })

  it('纵向为主的不抢（那是滚动页面）', () => {
    expect(judge(-70, 100)).toBe(0)
    expect(judge(-100, 120)).toBe(0)
  })

  it('斜着滑但纵向偏移过大也不响应', () => {
    expect(judge(-80, 90)).toBe(0)
  })
})
