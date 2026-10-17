/**
 * 右键菜单（照 Sun-Panel 的交互）。
 *
 * 结构：
 *   ┌ 打开默认地址   [打开] [复制] [新窗口]
 *   ├ 打开内网地址   [打开] [复制] [新窗口]
 *   ├ ─────────────
 *   └ 编辑 / 删除
 *
 * 三条规矩：
 *
 * 1. **没有的地址不显示那一组。** 没配内网就没有「打开内网地址」这一栏，
 *    而不是给一组灰按钮 —— 灰按钮只会让人以为"坏了"，不会让人理解为"没配"。
 *    访客被脱敏之后也自动走这条：他看不到内网栏，不需要额外逻辑。
 *
 * 2. 「默认地址」是按用户设置的网络偏好算出来的（可能是 v4/v6/内网），
 *    「内网地址」是**明确指定**走内网。两个都要：默认那个是日常要用的，
 *    内网那个是"我在家，想走局域网快点"。
 *
 * 3. 定位：先按鼠标位置渲染，渲染完量一次自己的高度，超出就往上翻 ——
 *    在屏幕底部弹出来的菜单会被裁掉一半。
 */

import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { Copy, ExternalLink, Pencil, SquareArrowOutUpRight, Trash2 } from 'lucide-react'
import { copyText, entriesOf, preferredEntry } from '../../lib/addresses'
import type { AppView, NetworkPrefer } from '../../types'

export interface MenuState {
  app: AppView
  x: number
  y: number
}

export function ContextMenu({
  menu,
  prefer,
  onClose,
  onDelete,
  onEdit,
  onNotify,
  deleteLabel = '删除',
}: {
  menu: MenuState
  prefer: NetworkPrefer
  onClose: () => void
  onDelete: (id: string) => void
  onEdit: (app: AppView) => void
  onNotify: (kind: 'ok' | 'error', text: string) => void
  /** 托盘里叫"从托盘移除"，卡片里叫"删除" —— 同一个动作的不同说法 */
  deleteLabel?: string
}) {
  const ref = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState({ left: menu.x, top: menu.y })
  /**
   * 菜单刚弹出时**先不接受点击**。
   *
   * 手机上长按会触发 contextmenu，浏览器在手指抬起时还会补一个 click，
   * 而菜单是**正好在手指位置**弹出的（top = 手指 Y），那个 click 就直接
   * 落进菜单第一项 —— 长按一下图标就被打开了/被删了，纯属误触。
   * 桌面端也有类似情况：右键后手抖点一下。
   *
   * 所以弹出后先屏蔽一小会儿，等手指抬完了再放行。
   */
  const [armed, setArmed] = useState(false)
  useEffect(() => {
    const t = window.setTimeout(() => setArmed(true), 320)
    return () => window.clearTimeout(t)
  }, [])

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

  const entries = entriesOf(menu.app.addresses)
  const defURL = preferredEntry(menu.app.addresses, prefer)
  const lanURL = menu.app.addresses.lan || ''
  const hasAny = entries.length > 0

  const doCopy = async (url: string, label: string) => {
    const ok = await copyText(url)
    onNotify(ok ? 'ok' : 'error', ok ? `已复制${label}` : '复制失败，请手动选择地址')
    onClose()
  }

  const doOpen = (url: string) => {
    window.open(url, '_blank', 'noopener,noreferrer')
    onClose()
  }

  return (
    <div
      ref={ref}
      tabIndex={-1}
      role="menu"
      aria-label={`应用「${menu.app.name}」的操作`}
      style={{ left: pos.left, top: pos.top }}
      className={`fixed z-50 w-64 overflow-hidden rounded-xl bg-slate-800/95 py-1.5 text-sm text-slate-100 shadow-2xl ring-1 ring-white/10 backdrop-blur transition-opacity focus:outline-none ${
        armed ? 'opacity-100' : 'pointer-events-none opacity-80'
      }`}
    >
      {hasAny && defURL ? (
        <>
          <AddressGroup
            title="打开默认地址"
            url={defURL}
            onOpen={() => doOpen(defURL)}
            onCopy={() => void doCopy(defURL, '默认地址')}
          />
          {/* 默认地址就是内网时不再重复一栏，否则同一个地址出现两次 */}
          {lanURL && lanURL !== defURL && (
            <AddressGroup title="打开内网地址" url={lanURL} onOpen={() => doOpen(lanURL)} onCopy={() => void doCopy(lanURL, '内网地址')} />
          )}
          <div className="my-1.5 border-t border-white/10" />
        </>
      ) : (
        <div className="px-3 py-1.5 text-xs text-white/50">还没配地址，先编辑填一个</div>
      )}

      <button
        type="button"
        role="menuitem"
        onClick={() => {
          onClose()
          onEdit(menu.app)
        }}
        className="flex w-full items-center gap-2.5 px-3 py-1.5 text-left transition hover:bg-white/10"
      >
        <Pencil className="h-3.5 w-3.5" />
        编辑
      </button>
      <button
        type="button"
        role="menuitem"
        onClick={() => {
          onClose()
          onDelete(menu.app.id)
        }}
        className="flex w-full items-center gap-2.5 px-3 py-1.5 text-left text-rose-300 transition hover:bg-rose-500/20"
      >
        <Trash2 className="h-3.5 w-3.5" />
        {deleteLabel}
      </button>
    </div>
  )
}

/** 一组地址：标题（点了直接打开）+ 两个动作按钮 */
function AddressGroup({ title, url, onOpen, onCopy }: { title: string; url: string; onOpen: () => void; onCopy: () => void }) {
  return (
    <div className="px-3 py-1.5">
      <button
        type="button"
        role="menuitem"
        onClick={onOpen}
        className="block w-full truncate text-left text-[13px] font-semibold text-white/90 transition hover:text-white"
        title={url}
      >
        {title}
      </button>
      <div className="mt-1.5 flex gap-1">
        <ActionBtn label="打开" icon={<ExternalLink className="h-3.5 w-3.5" />} onClick={onOpen} />
        <ActionBtn label="复制" icon={<Copy className="h-3.5 w-3.5" />} onClick={onCopy} />
        <ActionBtn
          label="新窗口打开"
          icon={<SquareArrowOutUpRight className="h-3.5 w-3.5" />}
          // 和「打开」同为新标签页：导航页点任何东西都不该把导航页自己顶掉，
          // 否则跳过去想回来还得按返回。两个按钮的区别留给将来做"同窗口"。
          onClick={onOpen}
        />
      </div>
    </div>
  )
}

function ActionBtn({ label, icon, onClick }: { label: string; icon: React.ReactNode; onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      title={label}
      aria-label={label}
      className="grid h-7 flex-1 place-items-center rounded-md bg-white/10 text-white/80 transition hover:bg-white/20 hover:text-white"
    >
      {icon}
    </button>
  )
}
