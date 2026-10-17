/**
 * 空白处右键的菜单。
 *
 * 为什么要有这个：以前"新增应用"是网格末尾一张虚线卡 —— 它常驻在那儿，
 * 每次都占一格，看着像"这里有个东西还没加载完"。而且卡片少了的时候，
 * 一个空荡荡的加号框比什么都显眼。
 *
 * 改成右键空白处新建，好处有三：
 *   1. 界面上没有多余的东西（不新建的时候它不存在）
 *   2. 顺手把浏览器自己的右键菜单挡掉了 —— 那菜单在这里一项有用的都没有
 *   3. 符合这类页面的惯例（Sun-Panel、极光都是右键）
 *
 * 访客不会看到这个菜单：调用方只在登录时才传 onBlankContextMenu。
 */

import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { FolderPlus, Plus, Settings } from 'lucide-react'

export interface BlankMenuState {
  x: number
  y: number
}

export function BlankContextMenu({
  menu,
  onClose,
  onNewApp,
  onNewCategory,
  onOpenSettings,
}: {
  menu: BlankMenuState
  onClose: () => void
  onNewApp: () => void
  onNewCategory: () => void
  onOpenSettings: () => void
}) {
  const ref = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState({ left: menu.x, top: menu.y })

  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const { offsetHeight: h, offsetWidth: w } = el
    let top = menu.y
    let left = menu.x
    if (top + h > window.innerHeight - 8) top = Math.max(8, window.innerHeight - h - 8)
    if (left + w > window.innerWidth - 8) left = Math.max(8, window.innerWidth - w - 8)
    setPos({ left, top })
    el.focus()
  }, [menu.x, menu.y])

  useEffect(() => {
    const onDown = (e: MouseEvent) => {
      if (ref.current?.contains(e.target as Node)) return
      onClose()
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        e.stopPropagation()
        onClose()
      }
    }
    window.addEventListener('mousedown', onDown)
    window.addEventListener('keydown', onKey, true)
    return () => {
      window.removeEventListener('mousedown', onDown)
      window.removeEventListener('keydown', onKey, true)
    }
  }, [onClose])

  const pick = (fn: () => void) => () => {
    onClose()
    fn()
  }

  return (
    <div
      ref={ref}
      tabIndex={-1}
      role="menu"
      aria-label="新建"
      style={{ left: pos.left, top: pos.top }}
      className="fixed z-50 w-44 overflow-hidden rounded-xl bg-slate-800/95 py-1.5 text-sm text-slate-100 shadow-2xl ring-1 ring-white/10 backdrop-blur focus:outline-none"
    >
      <Item icon={<Plus className="h-3.5 w-3.5" />} label="新建应用" onClick={pick(onNewApp)} />
      <Item icon={<FolderPlus className="h-3.5 w-3.5" />} label="新建分组" onClick={pick(onNewCategory)} />
      <div className="my-1.5 border-t border-white/10" />
      <Item icon={<Settings className="h-3.5 w-3.5" />} label="导航页设置" onClick={pick(onOpenSettings)} />
    </div>
  )
}

function Item({ icon, label, onClick }: { icon: React.ReactNode; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      role="menuitem"
      onClick={onClick}
      className="flex w-full items-center gap-2.5 px-3 py-1.5 text-left transition hover:bg-white/10"
    >
      {icon}
      {label}
    </button>
  )
}
