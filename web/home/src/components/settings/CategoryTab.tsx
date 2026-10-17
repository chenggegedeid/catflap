/**
 * 分组管理。
 *
 * 每个分组一行：名字（可直接改）、可见性、上下移、删除。
 * 设为「上锁」时展开一个口令输入 —— 不给口令就存不进去（后端也拦着），
 * 因为一个没有口令的锁分组是谁也进不去的死结。
 *
 * 可见性的说法尽量用大白话，不用 public/login/lock：
 *   所有人可见 / 登录后可见 / 需要口令
 */

import { useState } from 'react'
import { ChevronDown, ChevronUp, Lock, Plus, Trash2 } from 'lucide-react'
import { addCategory, deleteCategory, reorderCategories, updateCategory } from '../../api/home'
import { useHome, visibleCategories } from '../../state/HomeProvider'
import type { Category, Visibility } from '../../types'

const VIS_OPTIONS: { value: Visibility; label: string; hint: string }[] = [
  { value: 'public', label: '所有人可见', hint: '没登录的访客也能看到' },
  { value: 'login', label: '登录后可见', hint: '要登录导航页才看得到' },
  { value: 'lock', label: '需要口令', hint: '登录了也要再输这个分组的口令（换人用这台电脑也看不了）' },
]

export function CategoryTab() {
  const { state, mutate, notify } = useHome()
  const cats = visibleCategories(state.config)
  const [adding, setAdding] = useState(false)
  const [newName, setNewName] = useState('')
  const [pwFor, setPwFor] = useState<string>('')
  const [pw, setPw] = useState('')

  const doAdd = async () => {
    const name = newName.trim()
    if (!name) return
    const ok = await mutate({ request: () => addCategory(name), okMsg: `已新增分组「${name}」` })
    if (ok) {
      setNewName('')
      setAdding(false)
    }
  }

  const doRename = (c: Category, name: string) => {
    const n = name.trim()
    if (!n || n === c.name) return
    void mutate({ request: () => updateCategory(c.id, { name: n }) })
  }

  const doVisibility = async (c: Category, v: Visibility) => {
    if (v === 'lock' && !c.unlocked) {
      // 先要口令：没有口令的锁分组谁也进不去
      setPwFor(c.id)
      setPw('')
      return
    }
    void mutate({
      request: () => updateCategory(c.id, { visibility: v }),
      okMsg: v === 'public' ? '已设为所有人可见' : '已设为登录后可见',
    })
  }

  const confirmLock = async (c: Category) => {
    if (pw.trim().length < 4) {
      notify('error', '口令至少 4 位')
      return
    }
    const ok = await mutate({
      request: () => updateCategory(c.id, { visibility: 'lock', password: pw.trim() }),
      okMsg: `「${c.name}」已上锁`,
    })
    if (ok) {
      setPwFor('')
      setPw('')
    }
  }

  const doDelete = (c: Category) => {
    const n = state.config?.apps.filter((a) => a.categoryId === c.id).length ?? 0
    const msg = n > 0 ? `删除分组「${c.name}」？里面的 ${n} 个应用会变成未分组（不会删掉）。` : `删除分组「${c.name}」？`
    if (!confirm(msg)) return
    void mutate({ request: () => deleteCategory(c.id), okMsg: '已删除分组' })
  }

  const move = (idx: number, dir: -1 | 1) => {
    const next = [...cats]
    const target = idx + dir
    if (target < 0 || target >= next.length) return
    ;[next[idx], next[target]] = [next[target], next[idx]]
    void mutate({ request: () => reorderCategories(next.map((c) => c.id)) })
  }

  return (
    <div className="space-y-3">
      <p className="text-xs leading-relaxed text-slate-500">
        分组决定左侧那一列显示什么。名字点一下就能改，回车保存。
      </p>

      <ul className="space-y-2">
        {cats.map((c, i) => {
          const vis = (c.visibility ?? (c.requireAuth ? 'login' : 'public')) as Visibility
          const locked = vis === 'lock'
          return (
            <li key={c.id} className="rounded-xl border border-slate-200 px-3 py-2.5">
              <div className="flex items-center gap-2">
                <input
                  defaultValue={c.name}
                  onBlur={(e) => doRename(c, e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') (e.target as HTMLInputElement).blur()
                  }}
                  className="min-w-0 flex-1 rounded-lg border border-transparent px-1.5 py-1 text-sm font-semibold text-slate-800 transition hover:border-slate-200 focus:border-blue-400 focus:outline-none"
                />
                <button
                  type="button"
                  onClick={() => move(i, -1)}
                  disabled={i === 0}
                  aria-label="上移"
                  className="rounded p-1 text-slate-400 transition hover:bg-slate-100 disabled:opacity-30"
                >
                  <ChevronUp className="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  onClick={() => move(i, 1)}
                  disabled={i === cats.length - 1}
                  aria-label="下移"
                  className="rounded p-1 text-slate-400 transition hover:bg-slate-100 disabled:opacity-30"
                >
                  <ChevronDown className="h-3.5 w-3.5" />
                </button>
                <button
                  type="button"
                  onClick={() => doDelete(c)}
                  aria-label="删除分组"
                  className="rounded p-1 text-slate-400 transition hover:bg-rose-50 hover:text-rose-600"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>

              <div className="mt-2 flex flex-wrap items-center gap-1.5">
                {VIS_OPTIONS.map((o) => (
                  <button
                    key={o.value}
                    type="button"
                    title={o.hint}
                    onClick={() => void doVisibility(c, o.value)}
                    className={`flex items-center gap-1 rounded-lg px-2 py-1 text-[11px] font-semibold transition ${
                      vis === o.value ? 'bg-slate-900 text-white' : 'bg-slate-100 text-slate-600 hover:bg-slate-200'
                    }`}
                  >
                    {o.value === 'lock' && <Lock className="h-3 w-3" />}
                    {o.label}
                  </button>
                ))}
                {locked && (
                  <button
                    type="button"
                    onClick={() => {
                      setPwFor(c.id)
                      setPw('')
                    }}
                    className="text-[11px] font-semibold text-blue-600 underline-offset-2 hover:underline"
                  >
                    改口令
                  </button>
                )}
              </div>

              {pwFor === c.id && (
                <div className="mt-2 flex gap-1.5">
                  <input
                    autoFocus
                    type="password"
                    value={pw}
                    onChange={(e) => setPw(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') void confirmLock(c)
                      if (e.key === 'Escape') setPwFor('')
                    }}
                    placeholder="给这个分组设一个口令（至少 4 位）"
                    className="min-w-0 flex-1 rounded-lg border border-slate-200 px-2 py-1 text-xs focus:border-blue-400 focus:outline-none"
                  />
                  <button
                    type="button"
                    onClick={() => void confirmLock(c)}
                    className="rounded-lg bg-blue-500 px-2.5 py-1 text-[11px] font-bold text-white transition hover:bg-blue-600"
                  >
                    确定
                  </button>
                  <button
                    type="button"
                    onClick={() => setPwFor('')}
                    className="rounded-lg px-2 py-1 text-[11px] font-semibold text-slate-500 hover:bg-slate-100"
                  >
                    取消
                  </button>
                </div>
              )}
            </li>
          )
        })}
      </ul>

      {adding ? (
        <div className="flex gap-1.5">
          <input
            autoFocus
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') void doAdd()
              if (e.key === 'Escape') setAdding(false)
            }}
            placeholder="分组名，比如「工作」"
            className="min-w-0 flex-1 rounded-xl border border-slate-200 px-3 py-2 text-sm focus:border-blue-400 focus:outline-none"
          />
          <button
            type="button"
            onClick={() => void doAdd()}
            className="rounded-xl bg-blue-500 px-3 py-2 text-xs font-bold text-white transition hover:bg-blue-600"
          >
            添加
          </button>
        </div>
      ) : (
        <button
          type="button"
          onClick={() => setAdding(true)}
          className="flex w-full items-center justify-center gap-1.5 rounded-xl border border-dashed border-slate-300 py-2 text-xs font-semibold text-slate-500 transition hover:border-slate-400 hover:bg-slate-50"
        >
          <Plus className="h-3.5 w-3.5" />
          新增分组
        </button>
      )}
    </div>
  )
}
