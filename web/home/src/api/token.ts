/**
 * token 的存放。
 *
 * 导航页是**公开页面**：访客本来就没登录，所以这里读不到 token 是常态，不是错误。
 * 和后台那份共用同一个 key —— 从后台登过之后，导航页这边自动就有权限了，
 * 不用在公开页面上再登一次。
 */

const TOKEN_KEY = 'catflap_token'

export function getToken(): string {
  try {
    return localStorage.getItem(TOKEN_KEY) ?? ''
  } catch {
    // 隐私模式下 localStorage 可能直接抛，别让它把首屏搞崩
    return ''
  }
}

export function setToken(t: string): void {
  try {
    if (t) localStorage.setItem(TOKEN_KEY, t)
    else localStorage.removeItem(TOKEN_KEY)
  } catch {
    /* 存不了就算了，只是这次会话没有登录态 */
  }
}

export function clearToken(): void {
  try {
    localStorage.removeItem(TOKEN_KEY)
  } catch {
    /* 同上 */
  }
}
