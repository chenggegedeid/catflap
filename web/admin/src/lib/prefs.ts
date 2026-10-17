/**
 * 界面偏好的本地持久化。
 *
 * 存在的理由：列表的「每页多少条」以前只活在组件的 useState 里，
 * 刷新页面就打回默认值 —— 用户调到 10，一刷新又变回 50 把屏幕占满。
 * 这种设置必须记得住，否则等于每次都要重设一遍。
 */

const PAGE_LIMIT_KEY = 'catflap.pageLimit'

/**
 * 默认每页 10 条。
 *
 * 定这么小是因为列表里有 IPv6 地址和一整行说明，一条就占不少高度；
 * 默认 50 会让单个列表把整页撑满，其它内容要滚很久才看得到。
 */
export const DEFAULT_PAGE_LIMIT = 10

/**
 * 每页显示多少条。三个列表（访问来源 / 黑名单 / 登录尝试）共用这一个值：
 * 用户在一处调了密度，别处就是同样的密度 —— 而且刷新后仍然记得。
 */
export const getPageLimit = (): number => {
  try {
    const n = Number(localStorage.getItem(PAGE_LIMIT_KEY))
    if (Number.isFinite(n) && n > 0) return Math.min(Math.round(n), 500)
  } catch {
    // localStorage 不可用（隐私模式 / 被禁用）就用默认值，不影响使用
  }
  return DEFAULT_PAGE_LIMIT
}

/** 记住每页多少条 */
export const setPageLimit = (n: number): void => {
  try {
    localStorage.setItem(PAGE_LIMIT_KEY, String(n))
  } catch {
    // 存不进去最多是下次回到默认，不该让界面出错
  }
}
