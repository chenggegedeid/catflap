/**
 * 分组解锁弹窗。
 *
 * 什么时候出现：点到一个 lock 分组，而这次会话还没解锁过。
 *
 * 三条体验上的讲究：
 * 1. 自动聚焦输入框 —— 弹出来还得先点一下输入框，是很烦的细节
 * 2. 失败说清还剩几次/等多久 —— "口令不对"四个字让人只能反复试
 * 3. Esc 关闭、Enter 提交
 */

import { useEffect, useRef, useState } from 'react'
import { Lock } from 'lucide-react'
import { unlockCategory } from '../../api/home'
import { setUnlockToken } from '../../api/unlock'
import type { Category } from '../../types'

export function UnlockModal({
  category,
  onClose,
  onUnlocked,
}: {
  category: Category
  onClose: () => void
  onUnlocked: () => void
}) {
  const [pw, setPw] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    inputRef.current?.focus()
  }, [])

  const submit = async () => {
    setErr('')
    if (!pw) {
      setErr('请输入口令')
      return
    }
    setBusy(true)
    try {
      const { data } = await unlockCategory(category.id, pw)
      // 存 sessionStorage：关标签就没，下次要重新输 —— 这是分组锁的语义
      if (data?.token) setUnlockToken(data.token)
      onUnlocked()
    } catch (e) {
      setErr(e instanceof Error ? e.message : '解锁失败')
      setPw('')
      inputRef.current?.focus()
    } finally {
      setBusy(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 grid grid-cols-1 place-items-center bg-black/45 px-4 backdrop-blur-sm"
      onKeyDown={(e) => {
        if (e.key === 'Escape') onClose()
      }}
    >
      <div
        role="dialog"
        aria-modal
        aria-label={`解锁分组「${category.name}」`}
        className="w-full max-w-sm rounded-2xl bg-white p-5 shadow-2xl"
      >
        <div className="mb-3 flex items-center gap-2 text-slate-800">
          <Lock className="h-4 w-4" />
          <h2 className="text-sm font-semibold">解锁「{category.name}」</h2>
        </div>

        <p className="mb-3 text-xs leading-relaxed text-slate-500">
          这个分组有自己的口令。解锁只在当前浏览器标签有效 —— 关掉标签或浏览器，下次要重新输。
        </p>

        <input
          ref={inputRef}
          type="password"
          value={pw}
          onChange={(e) => setPw(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') void submit()
          }}
          placeholder="分组口令"
          className="w-full rounded-xl border border-slate-200 px-3 py-2 text-sm outline-none transition focus:border-blue-400"
        />

        {err && <p className="mt-2 text-xs text-rose-600">{err}</p>}

        <div className="mt-4 flex justify-end gap-2">
          <button
            type="button"
            onClick={onClose}
            className="rounded-xl px-3 py-2 text-xs font-semibold text-slate-500 transition hover:bg-slate-100"
          >
            取消
          </button>
          <button
            type="button"
            onClick={() => void submit()}
            disabled={busy}
            className="rounded-xl bg-blue-500 px-4 py-2 text-xs font-bold text-white transition hover:bg-blue-600 disabled:opacity-60"
          >
            {busy ? '验证中…' : '解锁'}
          </button>
        </div>
      </div>
    </div>
  )
}
