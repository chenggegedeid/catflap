/**
 * 请求封装。
 *
 * 只做三件事：带上 token、**把后端那句话取出来**、把各种失败变成同一种形状。
 *
 * 为什么要单独一层：以前每个调用点各写各的 fetch，于是错误处理五花八门 ——
 * 有的地方弹 alert、有的地方静默、有的地方把 HTML 当 JSON 解析然后崩在
 * `.length` 上。统一之后"失败时该说什么"只有一处决定。
 */

import { getToken } from './token'
import { getUnlockToken } from './unlock'

export class ApiError extends Error {
  /** HTTP 状态码；0 表示没拿到响应（断网 / 超时） */
  readonly status: number
  /** 后端返回的原始 body，排查时有用 */
  readonly body: string

  constructor(message: string, status: number, body = '') {
    super(message)
    this.name = 'ApiError'
    this.status = status
    this.body = body
  }
}

/** 后端统一响应形状 */
interface BackendResponse<T> {
  code: number
  data: T
  msg: string
}

const DEFAULT_TIMEOUT = 12_000

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers: Record<string, string> = {
    ...(init.body instanceof FormData ? {} : { 'Content-Type': 'application/json' }),
    ...((init.headers as Record<string, string> | undefined) ?? {}),
  }
  const token = getToken()
  if (token) headers.Authorization = `Bearer ${token}`
  // 分组解锁 token：服务端据此决定要不要下发 lock 分组的内容。
  // 放在 header 而不是 cookie，是为了让 fetch 走的每条路径都带上（后端两者都认）
  const unlock = getUnlockToken()
  if (unlock) headers['X-Catflap-Unlock'] = unlock

  // 超时控制：导航页可能挂在外网，卡住的请求不能让用户一直对着转圈
  const ctrl = new AbortController()
  const timer = setTimeout(() => ctrl.abort(), DEFAULT_TIMEOUT)

  let res: Response
  try {
    res = await fetch(path, { ...init, headers, signal: ctrl.signal, credentials: 'same-origin' })
  } catch (e) {
    clearTimeout(timer)
    // fetch 只在网络层失败时抛，HTTP 4xx/5xx 不抛 —— 这里就是"根本没连上"
    throw new ApiError(e instanceof Error && e.name === 'AbortError' ? '请求超时' : '连不上服务器', 0)
  }
  clearTimeout(timer)

  const text = await res.text()

  // 401 = token 过期。这是唯一需要全局处理的错误码，交给上层决定跳不跳登录
  if (res.status === 401) {
    throw new ApiError('登录已过期，请重新登录', 401, text)
  }

  let payload: BackendResponse<T> | null = null
  try {
    payload = JSON.parse(text) as BackendResponse<T>
  } catch {
    // 拿到了 HTML（比如被反代挡了、或者请求打到了别的服务）比拿到错误 JSON 更常见
    throw new ApiError(`服务器返回了意外内容（HTTP ${res.status}）`, res.status, text.slice(0, 200))
  }

  if (!res.ok || (payload.code !== 0 && payload.code !== 200)) {
    throw new ApiError(payload.msg || `请求失败（HTTP ${res.status}）`, res.status, text)
  }

  return payload.data
}

/** 只要 data */
export function apiGet<T>(path: string): Promise<T> {
  return request<T>(path, { method: 'GET' })
}

/** 要 data 也要后端那句话（有些接口做了一半，那句"还差什么"只有后端知道） */
export async function apiSend<T>(method: 'PUT' | 'POST' | 'DELETE', path: string, body?: unknown): Promise<{ data: T; msg: string }> {
  const res = await fetch(path, buildInit(method, body))
  return unwrap<T>(res)
}

/** 上传文件（JSON 会让浏览器写死 boundary，所以走单独的入口） */
export async function apiUpload<T>(path: string, file: File, field = 'file'): Promise<{ data: T; msg: string }> {
  const fd = new FormData()
  fd.append(field, file)
  const res = await fetch(path, { method: 'POST', body: fd, headers: authHeaders(), credentials: 'same-origin' })
  return unwrap<T>(res)
}

function buildInit(method: string, body?: unknown): RequestInit {
  return {
    method,
    body: body === undefined ? undefined : JSON.stringify(body),
    headers: { 'Content-Type': 'application/json', ...authHeaders() },
    credentials: 'same-origin',
  }
}

function authHeaders(): Record<string, string> {
  const out: Record<string, string> = {}
  const token = getToken()
  if (token) out.Authorization = `Bearer ${token}`
  const unlock = getUnlockToken()
  if (unlock) out['X-Catflap-Unlock'] = unlock
  return out
}

async function unwrap<T>(res: Response): Promise<{ data: T; msg: string }> {
  const text = await res.text()
  if (res.status === 401) throw new ApiError('登录已过期，请重新登录', 401, text)
  let payload: BackendResponse<T>
  try {
    payload = JSON.parse(text) as BackendResponse<T>
  } catch {
    throw new ApiError(`服务器返回了意外内容（HTTP ${res.status}）`, res.status, text.slice(0, 200))
  }
  if (!res.ok || (payload.code !== 0 && payload.code !== 200)) {
    throw new ApiError(payload.msg || `请求失败（HTTP ${res.status}）`, res.status, text)
  }
  return { data: payload.data, msg: payload.msg }
}
