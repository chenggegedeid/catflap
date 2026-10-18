import { useEffect, useState } from 'react'
import { X } from 'lucide-react'
import logo from '../assets/logo.png'
import { useNavGroups } from './nav'
import { getUpdateStatus, getVersion } from '../lib/api'
import type { PageKey } from '../types'

interface SidebarProps {
  active: PageKey
  onChange: (key: PageKey) => void
  /** 窄屏抽屉是否拉出来了；lg 以上忽略 */
  open: boolean
  onClose: () => void
}

export function Sidebar({ active, onChange, open, onClose }: SidebarProps) {
  const [version, setVersion] = useState('2.5.6')
  const [latest, setLatest] = useState('')
  // 导航来自页面注册表 + 组件开关：被「功能管理」关掉的功能不在这里出现
  const navGroups = useNavGroups()

  useEffect(() => {
    getVersion()
      .then((data) => {
        if (data?.version) setVersion(data.version)
      })
      .catch(() => {
        // 忽略错误，使用默认版本号
      })
    // 后端每天查一次 GitHub，这里只读结果，不会让每次打开页面都去请求 GitHub
    getUpdateStatus()
      .then((s) => setLatest(s.hasUpdate ? s.latest : ''))
      .catch(() => {})
  }, [])

  return (
    <aside
      className={`fixed inset-y-0 left-0 z-40 flex h-screen w-60 shrink-0 flex-col border-r border-line/70 bg-surface/95 backdrop-blur transition-transform duration-200 lg:static lg:z-auto lg:translate-x-0 lg:bg-surface/80 ${
        open ? 'translate-x-0 shadow-2xl' : '-translate-x-full'
      }`}
    >
      {/* Logo。导航页砍掉后，这里就是个纯标识，不再可点跳转 */}
      <div className="flex h-16 items-center gap-2 px-5">
        <div className="-mx-1.5 flex min-w-0 items-center gap-2 rounded-lg px-1.5 py-1">
          <img src={logo} alt="Catflap" className="h-9 w-9" />
          <span className="truncate text-lg font-bold tracking-tight text-content">catflap</span>
        </div>
        <button
          type="button"
          onClick={onClose}
          title="关闭菜单"
          className="ml-auto grid h-8 w-8 place-items-center rounded-lg text-muted transition hover:bg-surface-2 hover:text-content lg:hidden"
        >
          <X className="h-4 w-4" />
        </button>
      </div>

      <nav className="flex-1 overflow-y-auto px-3 pb-4">
        {navGroups.map((group, gi) => (
          <div key={gi} className="mb-3">
            {group.title && (
              <div className="px-3 pb-1 pt-3 text-[11px] font-semibold uppercase tracking-wider text-muted">
                {group.title}
              </div>
            )}
            {group.items.map((item) => {
              const Icon = item.icon
              const isActive = item.key === active
              return (
                <button
                  key={item.key}
                  type="button"
                  onClick={() => onChange(item.key)}
                  className={`group mb-0.5 flex w-full items-center gap-2.5 rounded-xl px-3 py-2 text-sm transition ${
                    isActive
                      ? 'bg-gradient-to-r from-blue-500 to-indigo-500 text-on-inverse shadow-md shadow-blue-500/25'
                      : 'text-content hover:bg-surface-2'
                  }`}
                >
                  <Icon
                    className={`h-4 w-4 ${isActive ? 'text-on-inverse' : 'text-muted group-hover:text-content'}`}
                  />
                  <span className="flex-1 text-left font-medium">{item.label}</span>
                  {item.badge && (
                    <span
                      className={`rounded-md px-1.5 py-0.5 text-[10px] font-bold ${
                        isActive
                          ? 'bg-surface/25 text-on-inverse'
                          : 'bg-blue-50 text-blue-600'
                      }`}
                    >
                      {item.badge}
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        ))}
      </nav>

      <div className="px-5 pb-4 text-[11px] text-muted">
        <a
          href="https://github.com/chenggegedeid/catflap"
          target="_blank"
          rel="noreferrer"
          title="GitHub 仓库"
          className="inline-flex items-center gap-1.5 transition hover:text-content"
        >
          <GithubMark className="h-3.5 w-3.5" />© 2026 catflap
        </a>
        <div className="flex items-center gap-1.5">
          v{version}
          {latest && (
            <button
              type="button"
              onClick={() => onChange('settings')}
              title="去系统设置里更新"
              className="rounded-md bg-rose-50 px-1.5 py-0.5 text-[10px] font-bold text-rose-500 transition hover:bg-rose-100"
            >
              新版本 v{latest}
            </button>
          )}
        </div>
      </div>
    </aside>
  )
}

// GithubMark GitHub 官方标志。lucide 1.x 去掉了所有品牌图标，只能自己放
function GithubMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true" className={className}>
      <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.013 8.013 0 0016 8c0-4.42-3.58-8-8-8z" />
    </svg>
  )
}
