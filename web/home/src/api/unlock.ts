/**
 * 解锁 token 的存放。
 *
 * **只存 sessionStorage**：关标签、关浏览器就没了，下次打开要重新输口令 ——
 * 这正是你要的语义（"关闭浏览器就需要重新验证"）。
 *
 * localStorage 不行：那会变成"这台电脑以后都不用再输"，
 * 和分组锁的目的正好相反（锁防的就是"别人坐到这台电脑前"）。
 *
 * 后端那份 token 另有 12 小时时限，双保险：只靠前端存，别人拿到 token 可以一直用；
 * 只靠后端时限，同一个浏览器里换个人坐过来还是能看。两条都要有。
 */

const UNLOCK_KEY = 'catflap_unlock'

export function getUnlockToken(): string {
  try {
    return sessionStorage.getItem(UNLOCK_KEY) ?? readCookie(UNLOCK_KEY)
  } catch {
    // 隐私模式下 sessionStorage 会抛，退回 cookie（后端也种了）
    return ''
  }
}

export function setUnlockToken(t: string): void {
  try {
    if (t) sessionStorage.setItem(UNLOCK_KEY, t)
    else sessionStorage.removeItem(UNLOCK_KEY)
  } catch {
    /* 存不了就只靠后端种的 cookie，功能不丢 */
  }
}

export function clearUnlockToken(): void {
  try {
    sessionStorage.removeItem(UNLOCK_KEY)
  } catch {
    /* 同上 */
  }
}

function readCookie(name: string): string {
  const m = document.cookie.match(new RegExp('(?:^|; )' + name + '=([^;]*)'))
  return m ? decodeURIComponent(m[1]) : ''
}
