import { useState, type FormEvent, type ReactNode } from 'react'
import { LockKeyhole, ShieldCheck, User } from 'lucide-react'
import { MIN_PASSWORD_LENGTH, MAX_PASSWORD_LENGTH, login, setupPassword, setToken } from '../lib/api'

export function Login({ onSuccess }: { onSuccess: () => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [remember, setRemember] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!password) {
      setError('请输入密码')
      return
    }
    setLoading(true)
    setError('')
    try {
      // 用户名留空也允许：升级上来的老配置里没有"用户名"这个概念，
      // 后端会放行。这样老用户不会一升级就登不进去。
      const { token } = await login(username, password, remember)
      setToken(token)
      onSuccess()
    } catch (err) {
      setError(err instanceof Error ? err.message : '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthLayout
      icon={<LockKeyhole className="h-7 w-7" />}
      title="登录 Catflap"
      subtitle="请输入用户名和密码"
    >
      <form onSubmit={submit} className="space-y-4">
        <UsernameField value={username} onChange={setUsername} autoFocus />
        <PasswordField value={password} onChange={setPassword} placeholder="密码" />
        <label className="flex cursor-pointer items-center gap-2 text-sm text-slate-500 select-none">
          <input
            type="checkbox"
            checked={remember}
            onChange={(e) => setRemember(e.target.checked)}
            className="h-4 w-4 rounded border-slate-300 accent-indigo-500"
          />
          记住我（7 天内免登录）
        </label>
        {error && <p className="text-sm text-rose-500">{error}</p>}
        <SubmitButton loading={loading}>登录</SubmitButton>
      </form>
    </AuthLayout>
  )
}

export function Setup({ onSuccess }: { onSuccess: () => void }) {
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (password.length < MIN_PASSWORD_LENGTH) {
      setError(`密码至少 ${MIN_PASSWORD_LENGTH} 位`)
      return
    }
    if (password.length > MAX_PASSWORD_LENGTH) {
      setError(`密码最多 ${MAX_PASSWORD_LENGTH} 位`)
      return
    }
    if (password !== confirm) {
      setError('两次输入的密码不一致')
      return
    }
    setLoading(true)
    setError('')
    try {
      const { token } = await setupPassword(username, password)
      setToken(token)
      onSuccess()
    } catch (err) {
      setError(err instanceof Error ? err.message : '设置失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthLayout
      icon={<ShieldCheck className="h-7 w-7" />}
      title="欢迎使用 Catflap"
      subtitle="首次使用，请设置用户名和密码"
    >
      <form onSubmit={submit} className="space-y-4">
        <UsernameField value={username} onChange={setUsername} autoFocus />
        <PasswordField
          value={password}
          onChange={setPassword}
          placeholder={`设置密码（${MIN_PASSWORD_LENGTH}-${MAX_PASSWORD_LENGTH} 位）`}
        />
        <PasswordField value={confirm} onChange={setConfirm} placeholder="确认密码" />
        {error && <p className="text-sm text-rose-500">{error}</p>}
        <SubmitButton loading={loading}>设置并进入</SubmitButton>
      </form>
    </AuthLayout>
  )
}

function AuthLayout({
  icon,
  title,
  subtitle,
  children,
}: {
  icon: ReactNode
  title: string
  subtitle: string
  children: ReactNode
}) {
  return (
    <div className="grid min-h-screen place-items-center bg-slate-50 px-4">
      <div className="w-full max-w-sm rounded-2xl border border-slate-200/70 bg-white p-8 shadow-sm shadow-slate-200/60">
        <div className="mb-6 flex flex-col items-center text-center">
          <div className="grid h-14 w-14 place-items-center rounded-2xl bg-indigo-50 text-indigo-500">
            {icon}
          </div>
          <h1 className="mt-4 text-lg font-bold text-slate-800">{title}</h1>
          <p className="mt-1 text-sm text-slate-400">{subtitle}</p>
        </div>
        {children}
      </div>
    </div>
  )
}

/**
 * 用户名输入框。
 *
 * 留空也能提交 —— 升级上来的老配置里没有用户名这个概念，
 * 后端会放行；强制要求填会让那批用户登不进去。placeholder 里说清楚默认值。
 */
function UsernameField({
  value,
  onChange,
  autoFocus,
}: {
  value: string
  onChange: (v: string) => void
  autoFocus?: boolean
}) {
  return (
    <div className="relative">
      <User className="pointer-events-none absolute left-3.5 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
      <input
        type="text"
        value={value}
        autoFocus={autoFocus}
        autoComplete="username"
        maxLength={32}
        onChange={(e) => onChange(e.target.value)}
        placeholder="用户名（留空用 admin）"
        className="w-full rounded-xl border border-slate-200 bg-slate-50 py-2.5 pl-10 pr-4 text-sm text-slate-800 outline-none transition focus:border-indigo-400 focus:bg-white focus:ring-2 focus:ring-indigo-100"
      />
    </div>
  )
}

function PasswordField({
  value,
  onChange,
  placeholder,
  autoFocus,
}: {
  value: string
  onChange: (v: string) => void
  placeholder: string
  autoFocus?: boolean
}) {
  return (
    <input
      type="password"
      value={value}
      autoFocus={autoFocus}
      // 只在**设置新密码**时限制输入长度；登录页也带这个上限是合理的 ——
      // 超过上限的密码当初就不可能设成功
      maxLength={MAX_PASSWORD_LENGTH}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      className="w-full rounded-xl border border-slate-200 bg-slate-50 px-4 py-2.5 text-sm text-slate-800 outline-none transition focus:border-indigo-400 focus:bg-white focus:ring-2 focus:ring-indigo-100"
    />
  )
}

function SubmitButton({ loading, children }: { loading: boolean; children: ReactNode }) {
  return (
    <button
      type="submit"
      disabled={loading}
      className="w-full rounded-xl bg-indigo-500 px-4 py-2.5 text-sm font-bold text-white transition hover:bg-indigo-600 disabled:cursor-not-allowed disabled:opacity-60"
    >
      {loading ? '处理中…' : children}
    </button>
  )
}
