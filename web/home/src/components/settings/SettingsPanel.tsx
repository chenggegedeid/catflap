/**
 * 导航页设置面板。
 *
 * 三个标签：
 *   分组管理  分组的新增/改名/排序/可见性/口令
 *   图标管理  批量看所有应用的图标，缺的能补
 *   外观      壁纸、网络偏好
 *
 * 为什么不做成后台的一个页面：这些是"这一页怎么摆"的事，
 * 在同一个页面里改完立刻能看到结果。跳到后台改、再跳回来，
 * 一方面思路断了两次，另一方面后台那套表单是为了"配置"设计的，
 * 不是为了"摆弄这一页给你看"。
 */

import { useState } from 'react'
import { Image, LayoutGrid, Palette, X } from 'lucide-react'
import { AppearanceTab } from './AppearanceTab'
import { CategoryTab } from './CategoryTab'
import { IconTab } from './IconTab'

type Tab = 'category' | 'icon' | 'appearance'

const TABS: { id: Tab; label: string; icon: React.ReactNode }[] = [
  { id: 'category', label: '分组管理', icon: <LayoutGrid className="h-3.5 w-3.5" /> },
  { id: 'icon', label: '图标管理', icon: <Image className="h-3.5 w-3.5" /> },
  { id: 'appearance', label: '外观', icon: <Palette className="h-3.5 w-3.5" /> },
]

export function SettingsPanel({ onClose }: { onClose: () => void }) {
  const [tab, setTab] = useState<Tab>('category')

  return (
    <div className="fixed inset-0 z-50 flex justify-end bg-black/40 backdrop-blur-sm" onMouseDown={onClose}>
      {/* 点面板内部不该关掉 —— 所以 onMouseDown 到这一层就停 */}
      <div
        role="dialog"
        aria-modal
        aria-label="导航页设置"
        onMouseDown={(e) => e.stopPropagation()}
        onKeyDown={(e) => {
          if (e.key === 'Escape') onClose()
        }}
        className="flex h-full w-full max-w-lg flex-col bg-white shadow-2xl"
      >
        <div className="flex items-center justify-between border-b border-slate-100 px-5 py-3.5">
          <h2 className="text-sm font-semibold text-slate-800">导航页设置</h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="关闭"
            className="rounded-lg p-1 text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="flex gap-1 border-b border-slate-100 px-3 py-2">
          {TABS.map((t) => (
            <button
              key={t.id}
              type="button"
              role="tab"
              aria-selected={tab === t.id}
              onClick={() => setTab(t.id)}
              className={`flex items-center gap-1.5 rounded-lg px-3 py-1.5 text-xs font-semibold transition ${
                tab === t.id ? 'bg-slate-900 text-white' : 'text-slate-500 hover:bg-slate-100'
              }`}
            >
              {t.icon}
              {t.label}
            </button>
          ))}
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
          {tab === 'category' && <CategoryTab />}
          {tab === 'icon' && <IconTab />}
          {tab === 'appearance' && <AppearanceTab />}
        </div>
      </div>
    </div>
  )
}
