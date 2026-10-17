import { useEffect, useRef, useState } from 'react'
import { LogOut, Menu, UserRound } from 'lucide-react'
import { ThemeToggle } from '../components/ThemeToggle'
import type { PageKey } from '../types'
import { clearToken, getUsername, isDesktop, setUsername, USERNAME_EVENT } from '../lib/api'
import { findPage } from '../pages/registry'

interface TopBarProps {
  active: PageKey
  /** 窄屏上拉出侧边栏抽屉 */
  onMenu: () => void
}

export function TopBar({ active, onMenu }: TopBarProps) {
  // 标题与副标题都来自页面注册表，这里不再单独维护一份
  const page = findPage(active)

  // 用户名。改了它会派发 USERNAME_EVENT（见 lib/api.ts），这里跟着重读：
  // 不订阅的话要等下次整页刷新才看得见新名字。
  //
  // 以前这里是写死的 "admin" 加一个 "A" 头像 —— 改了用户名界面纹丝不动，
  // 用户会以为没改成功。
  const [name, setName] = useState(() => getUsername())
  useEffect(() => {
    const onChanged = () => setName(getUsername())
    window.addEventListener(USERNAME_EVENT, onChanged)
    return () => window.removeEventListener(USERNAME_EVENT, onChanged)
  }, [])

  // 桌面版靠 sessionStorage 里的 desktop secret 鉴权，清 token 也退不出去，就别给这个入口
  const canLogout = !isDesktop()
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!menuOpen) return
    const onDown = (e: MouseEvent) => {
      if (!menuRef.current?.contains(e.target as Node)) setMenuOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setMenuOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [menuOpen])

  // 整页重载，顺带把 SSE 连接和各页缓存的配置一起丢掉
  const logout = () => {
    clearToken()
    // 用户名一起清掉：下一个人在这台机器上登录时，顶栏不该先闪一下别人的名字
    setUsername('')
    window.location.reload()
  }

  // 头像里的那个字母。老配置（没有用户名）或还没读到时就留空，不硬塞一个 "A"。
  const initial = name.slice(0, 1).toUpperCase()

  return (
    <header className="flex h-16 items-center gap-2 border-b border-line bg-surface/80 px-4 backdrop-blur sm:gap-4 sm:px-6 lg:px-8">
      {/* 汉堡：只有窄屏才有，侧边栏这时候是抽屉 */}
      <button
        type="button"
        onClick={onMenu}
        title="菜单"
        className="-ml-1 grid h-9 w-9 shrink-0 place-items-center rounded-lg text-muted transition hover:bg-surface-2 lg:hidden"
      >
        <Menu className="h-5 w-5" />
      </button>

      <div className="min-w-0 flex-1">
        <div className="truncate text-base font-bold text-content">{page?.label ?? ''}</div>
        {/* 副标题在手机上占不下，藏起来 */}
        {page?.subtitle && (
          <div className="mt-0.5 hidden truncate text-xs text-muted sm:block">
            {page.subtitle}
          </div>
        )}
      </div>

      {/* 主题切换放在头像**前面**：两者都是"全局的、跟当前页面无关"的操作，
          排在一起比一个在左一个在右好找 */}
      <div className="flex shrink-0 items-center gap-1.5">
        <ThemeToggle />

        <div className="relative" ref={menuRef}>
          <button
            type="button"
            onClick={canLogout ? () => setMenuOpen((v) => !v) : undefined}
            aria-haspopup={canLogout || undefined}
            aria-expanded={canLogout ? menuOpen : undefined}
            className={`flex shrink-0 items-center gap-2 rounded-full bg-surface p-1 ring-1 transition ${
              menuOpen ? 'ring-line bg-surface-2' : 'ring-line'
            } ${canLogout ? 'hover:bg-surface-2' : 'cursor-default'}`}
          >
          <span className="grid h-7 w-7 place-items-center rounded-full bg-inverse text-xs font-bold text-on-inverse">
            {initial || <UserRound className="h-3.5 w-3.5" />}
          </span>
          {/* 手机上只留头像，名字藏掉 */}
          <span className="hidden max-w-[10rem] truncate pr-2 text-sm font-semibold text-content sm:inline">
            {name || '未登录'}
          </span>
        </button>

        {menuOpen && (
          <div className="absolute right-0 top-full z-20 mt-2 w-36 overflow-hidden rounded-xl border border-line bg-surface py-1 shadow-lg shadow-slate-900/5 dark:shadow-black/40">
            <button
              type="button"
              onClick={logout}
              className="flex w-full items-center gap-2 px-3 py-2 text-sm text-content transition hover:bg-surface-2"
            >
              <LogOut className="h-4 w-4 text-muted" />
              退出登录
            </button>
          </div>
        )}
        </div>
      </div>
    </header>
  )
}
