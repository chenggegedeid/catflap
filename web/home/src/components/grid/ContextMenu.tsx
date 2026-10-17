/**
 * 右键菜单。
 *
 * 三项：删除 / 批量删除 / 修改大小（1×1 ~ 2×4，参照极光Tab）。
 *
 * 定位有个容易翻车的点：**菜单打开后才知道自己多高**，
 * 在屏幕底部弹出来会被裁掉一半。所以先按鼠标位置渲染，
 * 渲染完量一次自己的高度，超出就把整个菜单往上翻。
 *
 * 点外面 / Esc 关闭。关闭前把焦点还给触发它的卡片 ——
 * 否则键盘用户的焦点掉到 body 上，Tab 要从头再走一遍。
 */

import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Trash2, Rows3 } from 'lucide-react'
import { CARD_SIZES, parseSize } from '../../lib/grid'
import type { AppView } from '../../types'

export interface MenuState {
  app: AppView
  x: number
  y: number
}

export function ContextMenu({
  menu,
  onClose,
  onDelete,
  onBatchDelete,
  onResize,
}: {
  menu: MenuState
  onClose: () => void
  onDelete: (id: string) => void
  onBatchDelete: (id: string) => void
  onResize: (id: string, size: string) => void
}) {
  const ref = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState({ left: menu.x, top: menu.y })

  // 渲染完量一次：超出屏幕底部就往上翻
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const h = el.offsetHeight
    const w = el.offsetWidth
    let top = menu.y
    let left = menu.x
    if (top + h > window.innerHeight - 8) top = Math.max(8, window.innerHeight - h - 8)
    if (left + w > window.innerWidth - 8) left = Math.max(8, window.innerWidth - w - 8)
    setPos({ left, top })
    el.focus()
  }, [menu.x, menu.y])

  useEffect(() => {
    const onDown = (e: MouseEvent) => {
      // 点在菜单里面不算"外面"
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

  const current = parseSize(menu.app.size)

  return (
    <div
      ref={ref}
      // tabIndex 让它能接收焦点（Esc 监听在 window 上，但焦点在菜单里语义才对）
      tabIndex={-1}
      role="menu"
      aria-label={`应用「${menu.app.name}」的操作`}
      style={{ left: pos.left, top: pos.top }}
      className="fixed z-50 w-40 overflow-hidden rounded-xl bg-slate-900/95 py-1 text-sm text-white shadow-xl ring-1 ring-white/10 backdrop-blur focus:outline-none"
    >
      <MenuItem
        icon={<Trash2 className="h-3.5 w-3.5" />}
        label="删除"
        onClick={() => {
          onClose()
          onDelete(menu.app.id)
        }}
      />
      <MenuItem
        icon={<Rows3 className="h-3.5 w-3.5" />}
        label="批量删除"
        onClick={() => {
          onClose()
          onBatchDelete(menu.app.id)
        }}
      />

      <div className="my-1 border-t border-white/10" />

      <div className="px-3 pb-1 pt-0.5 text-[11px] text-white/50">修改大小</div>
      <div className="grid grid-cols-5 gap-1 px-2 pb-1.5">
        {CARD_SIZES.map((s) => {
          const { w, h } = parseSize(s)
          const active = w === current.w && h === current.h
          return (
            <button
              key={s}
              type="button"
              role="menuitemradio"
              aria-checked={active}
              onClick={() => {
                onClose()
                onResize(menu.app.id, s)
              }}
              className={`rounded-lg py-1 text-[11px] font-semibold transition ${
                active ? 'bg-blue-500 text-white' : 'bg-white/10 text-white/80 hover:bg-white/20'
              }`}
            >
              {s}
            </button>
          )
        })}
      </div>
    </div>
  )
}

function MenuItem({ icon, label, onClick }: { icon: React.ReactNode; label: string; onClick: () => void }) {
  return (
    <button
      type="button"
      role="menuitem"
      onClick={onClick}
      className="flex w-full items-center gap-2 px-3 py-1.5 text-left transition hover:bg-white/10"
    >
      {icon}
      {label}
    </button>
  )
}
