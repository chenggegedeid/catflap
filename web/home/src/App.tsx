/**
 * 导航主页 —— 装配层。
 *
 * 这个文件只做三件事：拉状态、摆区块、处理加载/失败。
 * 具体怎么渲染交给 components/ 下面各自的模块。
 *
 * （旧版本是一个 2954 行的 App.tsx，把所有组件、表单、拖拽都塞在里面。
 *   那份文件已被 components/ 下的模块取代。）
 */

import { useCallback, useState } from 'react'

import './App.css'

import { deleteApp, resizeApp, gotoLogin } from './api/home'
import { Background } from './components/layout/Background'
import { CategoryRail } from './components/layout/CategoryRail'
import { Header } from './components/layout/Header'
import { EmptyState, ErrorState, GridSkeleton } from './components/feedback/States'
import { Toasts } from './components/feedback/Toasts'
import { TopRightActions } from './components/chrome/TopRightActions'
import { AppGrid } from './components/grid/AppGrid'
import { ContextMenu, type MenuState } from './components/grid/ContextMenu'
import { UnlockModal } from './components/grid/UnlockModal'
import { SearchBar } from './components/search/SearchBar'
import { HomeProvider, useHome, visibleApps, visibleCategories } from './state/HomeProvider'
import type { AppView, Category } from './types'

export default function App() {
  return (
    <HomeProvider>
      <HomePage />
    </HomeProvider>
  )
}

function HomePage() {
  const { state, reload, setActiveCategory, mutate, notify, toasts, dismissToast } = useHome()
  const [retrying, setRetrying] = useState(false)
  // 右键菜单 & 批量删除
  const [menu, setMenu] = useState<MenuState | null>(null)
  const [selection, setSelection] = useState<Set<string> | null>(null)
  // 需要解锁的分组（点了 lock 分组但这次会话还没验过）
  const [unlocking, setUnlocking] = useState<Category | null>(null)

  const onRetry = useCallback(async () => {
    setRetrying(true)
    await reload()
    setRetrying(false)
  }, [reload])

  const deleteOne = useCallback(
    (id: string) => {
      const name = state.config?.apps.find((a) => a.id === id)?.name ?? ''
      if (!confirm(`删除「${name}」？只从导航页移除，不会动服务本身。`)) return
      void mutate({
        apply: (cfg) => ({ ...cfg, apps: cfg.apps.filter((a) => a.id !== id) }),
        request: () => deleteApp(id),
        okMsg: '已删除',
        retry: () => deleteOne(id),
      })
    },
    [state.config, mutate],
  )

  const startBatch = useCallback((id: string) => {
    setSelection(new Set([id]))
  }, [])

  const toggleSelect = useCallback((id: string) => {
    setSelection((prev) => {
      const next = new Set(prev ?? [])
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const confirmBatch = useCallback(() => {
    if (!selection || selection.size === 0) return
    if (!confirm(`删除选中的 ${selection.size} 个应用？只从导航页移除，不会动服务本身。`)) return
    const ids = [...selection]
    void mutate({
      apply: (cfg) => ({ ...cfg, apps: cfg.apps.filter((a) => !selection.has(a.id)) }),
      request: async () => {
        for (const id of ids) await deleteApp(id)
      },
      okMsg: `已删除 ${ids.length} 个`,
      retry: () => confirmBatch(),
    })
    setSelection(null)
  }, [selection, mutate])

  const doResize = useCallback(
    (id: string, size: string) => {
      void mutate({
        apply: (cfg) => ({ ...cfg, apps: cfg.apps.map((a) => (a.id === id ? { ...a, size } : a)) }),
        request: () => resizeApp(id, size as AppView['size'] as never),
        retry: () => doResize(id, size),
      })
    },
    [mutate],
  )

  const onMenuResize = useCallback(
    (id: string, size: string) => {
      void doResize(id, size)
      notify('ok', '大小已调整')
    },
    [doResize, notify],
  )

  // 壁纸和标题不依赖配置：加载中、加载失败也要有个能看的样子
  const cfg = state.config

  return (
    <div className="min-h-screen">
      <Background wallpaper={cfg?.wallpaper} />

      <div className="mx-auto flex min-h-screen w-full max-w-[1600px] flex-col px-4 pb-10 pt-8 sm:px-6">
        <TopRightActions authed={!!cfg?.authed} pending={state.pending > 0} onLogin={gotoLogin} />

        <div className="flex flex-col items-center gap-6">
          <Header title={cfg?.title ?? 'Catflap'} showTime={cfg?.showTime ?? true} />
          {/* 搜索框要等配置到位（要拿引擎和应用列表），加载中不渲染 */}
          {cfg && <SearchBar config={cfg} />}
        </div>

        <div className="mt-8 flex flex-1 gap-4 sm:gap-6">
          <CategoryRail
            categories={visibleCategories(cfg)}
            active={state.activeCategory}
            onSelect={(id) => {
              const cat = visibleCategories(cfg).find((c) => c.id === id)
              if (cat && cat.visibility === 'lock') {
                setUnlocking(cat)
                return
              }
              setActiveCategory(id)
            }}
            canManage={!!cfg?.authed}
            hiddenCount={cfg?.hiddenCategoryCount}
          />

          <main className="min-w-0 flex-1">
            {state.load === 'loading' && <GridSkeleton />}

            {state.load === 'error' && <ErrorState message={state.error} onRetry={onRetry} retrying={retrying} />}

            {state.load === 'ready' && cfg && (
              <GridArea
                categoryId={state.activeCategory}
                canManage={!!cfg.authed}
                onOpenMenu={(app, x, y) => setMenu({ app, x, y })}
                selection={selection}
                onToggleSelect={toggleSelect}
              />
            )}
          </main>
        </div>
      </div>

      {/* 批量删除的操作条 */}
      {selection && selection.size > 0 && (
        <div className="fixed bottom-4 left-1/2 z-40 flex -translate-x-1/2 items-center gap-2 rounded-xl bg-slate-900/90 px-3 py-2 text-sm text-white shadow-lg backdrop-blur">
          <span>已选 {selection.size} 个</span>
          <button
            type="button"
            onClick={confirmBatch}
            className="rounded-lg bg-rose-500 px-3 py-1 text-xs font-semibold transition hover:bg-rose-600"
          >
            删除所选
          </button>
          <button
            type="button"
            onClick={() => setSelection(null)}
            className="rounded-lg bg-white/10 px-3 py-1 text-xs font-semibold transition hover:bg-white/20"
          >
            取消
          </button>
        </div>
      )}

      {menu && (
        <ContextMenu
          menu={menu}
          onClose={() => setMenu(null)}
          onDelete={deleteOne}
          onBatchDelete={startBatch}
          onResize={onMenuResize}
        />
      )}

      {unlocking && (
        <UnlockModal
          category={unlocking}
          onClose={() => setUnlocking(null)}
          onUnlocked={() => {
            // 解锁成功：选中它并重新拉配置（服务端这时才会下发它的内容）
            setActiveCategory(unlocking.id)
            setUnlocking(null)
            void reload()
          }}
        />
      )}

      <Toasts toasts={toasts} onDismiss={dismissToast} />
    </div>
  )
}

/**
 * 网格区。
 *
 * 空态分两种，文案不同 —— 「还没加任何应用」和「这个分类是空的」是两回事，
 * 给用户的话也不一样。
 */
function GridArea({
  categoryId,
  canManage,
  onOpenMenu,
  selection,
  onToggleSelect,
}: {
  categoryId: string
  canManage: boolean
  onOpenMenu: (app: AppView, x: number, y: number) => void
  selection: Set<string> | null
  onToggleSelect: (id: string) => void
}) {
  const { state } = useHome()
  const cfg = state.config!
  const apps = visibleApps(cfg, categoryId)
  const hasAny = cfg.apps.length > 0

  if (apps.length === 0) {
    return (
      <EmptyState
        title={hasAny ? '这个分类还是空的' : '还没有任何应用'}
        hint={
          hasAny
            ? '切到别的分类看看，或者把应用移进来。'
            : '在反代或穿透里把服务勾上「显示到主页」，它就会自己出现在这里。'
        }
        action={canManage ? { label: '添加应用', onClick: () => undefined } : undefined}
      />
    )
  }

  return (
    <AppGrid
      apps={apps}
      prefer={cfg.networkPrefer}
      onOpenMenu={canManage ? onOpenMenu : undefined}
      selection={canManage && selection ? selection : undefined}
      onToggleSelect={canManage ? onToggleSelect : undefined}
    />
  )
}
