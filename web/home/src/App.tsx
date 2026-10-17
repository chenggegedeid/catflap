/**
 * 导航主页 —— 装配层。
 *
 * 这个文件只做三件事：拉状态、摆区块、处理加载/失败。
 * 具体怎么渲染交给 components/ 下面各自的模块。
 *
 * （旧版本是一个 2954 行的 App.tsx，把所有组件、表单、拖拽都塞在里面。
 *   那份文件已被 components/ 下的模块取代。）
 */

import { useCallback, useEffect, useState } from 'react'

import './App.css'

import { deleteApp, gotoLogin, reorderApps } from './api/home'
import { Background } from './components/layout/Background'
import { CategoryRail } from './components/layout/CategoryRail'
import { Header } from './components/layout/Header'
import { EmptyState, ErrorState, GridSkeleton } from './components/feedback/States'
import { Toasts } from './components/feedback/Toasts'
import { TopRightActions } from './components/chrome/TopRightActions'
import { AppGrid } from './components/grid/AppGrid'
import { ContextMenu, type MenuState } from './components/grid/ContextMenu'
import { UnlockModal } from './components/grid/UnlockModal'
import { AppFormModal } from './components/settings/AppFormModal'
import { BlankContextMenu, type BlankMenuState } from './components/grid/BlankContextMenu'
import { Section, useScrollSpy } from './components/grid/SectionedFeed'
import { SearchBar } from './components/search/SearchBar'
import { SettingsPanel } from './components/settings/SettingsPanel'
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
  // 右键菜单
  const [menu, setMenu] = useState<MenuState | null>(null)
  // 设置面板（分组 / 图标 / 外观）
  const [settingsOpen, setSettingsOpen] = useState(false)
  // 需要解锁的分组（点了 lock 分组但这次会话还没验过）
  const [unlocking, setUnlocking] = useState<Category | null>(null)
  // 表单：null = 关着；'new' = 新增；AppView = 编辑它
  const [formFor, setFormFor] = useState<AppView | 'new' | null>(null)
  // 空白处右键的菜单（新建应用 / 新建分组 / 设置）
  const [blankMenu, setBlankMenu] = useState<BlankMenuState | null>(null)

  const openAddApp = useCallback(() => setFormFor('new'), [])

  /**
   * 拖完落位。
   *
   * 本地先按新顺序改 `order`（乐观更新，卡片立刻停在放下的位置），
   * 再整串发给后端。失败时 mutate 会自动重新拉配置 —— 卡片回到原位，
   * 这比"卡在错的位置但没人知道"好得多。
   *
   * 注意 order 要重排成 1..N 连续的：只给被拖的两张换位置的话，
   * 别处的空洞会留着，下次插入算 max+1 时就可能出现重复。
   */
  const onReorder = useCallback(
    (ids: string[]) => {
      const pos = new Map(ids.map((id, i) => [id, i + 1]))
      void mutate({
        apply: (cfg) => ({
          ...cfg,
          apps: cfg.apps.map((a) => (pos.has(a.id) ? { ...a, order: pos.get(a.id)! } : a)),
        }),
        request: () => reorderApps(ids),
      })
    },
    [mutate],
  )

  /**
   * 左栏高亮哪一组 —— 以**滚到哪儿**为准，不是以"点了哪个"为准。
   *
   * 手动点左栏时会临时把它设成点的那组（否则滚过去的动画期间高亮会一直跳），
   * 滚完由 scrollspy 接管。
   */
  const [spyActive, setSpyActive] = useState('')
  const cats = visibleCategories(state.config)
  const catIds = cats.map((c) => c.id)
  const { registerRef } = useScrollSpy(catIds, setSpyActive)

  // 配置到位后，第一屏默认落在第一个分组
  useEffect(() => {
    if (catIds.length > 0 && !catIds.includes(spyActive)) setSpyActive(catIds[0])
  }, [catIds, spyActive])

  /** 点左栏：平滑滚到那一组 */
  const scrollToCategory = useCallback((id: string) => {
    const el = document.getElementById(`cat-${id}`)
    if (!el) return
    // 用 scrollIntoView 而不是自己算 scrollTop：它能正确处理
    // 图片没加载完、字体替换导致的布局偏移
    el.scrollIntoView({ behavior: 'smooth', block: 'start' })
    setSpyActive(id)
  }, [])

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

  // 壁纸和标题不依赖配置：加载中、加载失败也要有个能看的样子
  const cfg = state.config

  return (
    <div className="min-h-screen">
      <Background wallpaper={cfg?.wallpaper} />

      <div className="mx-auto flex min-h-screen w-full max-w-[1600px] flex-col px-4 pb-10 pt-8 sm:px-6">
        <TopRightActions
          authed={!!cfg?.authed}
          pending={state.pending > 0}
          onLogin={gotoLogin}
          onOpenSettings={() => setSettingsOpen(true)}
          onReload={() => void reload()}
        />

        <div className="flex flex-col items-center gap-6">
          <Header title={cfg?.title ?? 'Catflap'} showTime={cfg?.showTime ?? true} />
          {/* 搜索框要等配置到位（要拿引擎和应用列表），加载中不渲染 */}
          {cfg && <SearchBar config={cfg} />}
        </div>

        {/**
         * 左栏固定在视口左侧，内容区是一条滚动的分组流。
         *
         * 以前是"点分组 → 只显示那一组"，现在改成所有分组连着排、滚到哪
         * 左栏就高亮哪 —— 这是极光那种上下翻的手感。改动原因：
         * 一次只显示一组时，短分组下面全是空白，而且跨组找东西要点两次。
         */}
        <div className="mt-8 flex flex-1 gap-3 sm:gap-4">
          <div className="sticky top-6 self-start pt-1">
            <CategoryRail
              categories={visibleCategories(cfg)}
              active={spyActive}
              onSelect={(id) => {
                const cat = visibleCategories(cfg).find((c) => c.id === id)
                if (cat && cat.visibility === 'lock') {
                  setUnlocking(cat)
                  return
                }
                setActiveCategory(id)
                scrollToCategory(id)
              }}
              onAdd={() => setSettingsOpen(true)}
              canManage={!!cfg?.authed}
              hiddenCount={cfg?.hiddenCategoryCount}
            />
          </div>

          <main className="min-w-0 flex-1">
            {state.load === 'loading' && <GridSkeleton />}

            {state.load === 'error' && <ErrorState message={state.error} onRetry={onRetry} retrying={retrying} />}

            {state.load === 'ready' && cfg && (
              <Feed
                canManage={!!cfg.authed}
                onOpenMenu={(app, x, y) => setMenu({ app, x, y })}
                onAddApp={openAddApp}
                onBlankContextMenu={(x, y) => setBlankMenu({ x, y })}
                onReorder={onReorder}
                registerRef={registerRef}
              />
            )}
          </main>
        </div>
      </div>


      {blankMenu && (
        <BlankContextMenu
          menu={blankMenu}
          onClose={() => setBlankMenu(null)}
          onNewApp={openAddApp}
          onNewCategory={() => {
            setSettingsOpen(true)
            notify('ok', '在「分组管理」里新增分组')
          }}
          onOpenSettings={() => setSettingsOpen(true)}
        />
      )}

      {menu && (
        <ContextMenu
          menu={menu}
          prefer={cfg?.networkPrefer ?? 'wanV4'}
          onClose={() => setMenu(null)}
          onDelete={deleteOne}
          onEdit={setFormFor}
          onNotify={notify}
        />
      )}

      {formFor !== null && (
        <AppFormModal
          editing={formFor === 'new' ? null : formFor}
          categories={state.config?.categories ?? []}
          onClose={() => setFormFor(null)}
          onSaved={() => void reload()}
          onNotify={notify}
        />
      )}

      {settingsOpen && <SettingsPanel onClose={() => setSettingsOpen(false)} />}

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
 * 分组流：所有分组连着排，滚到哪高亮哪。
 *
 * 和旧的 GridArea 的区别：不再"只渲染选中的那一个"。
 * 每个分组一节，空分组给一句轻提示（不是整块空态卡片 —— 那条流里
 * 出现一个大空框反而更碍眼）。
 */
function Feed({
  canManage,
  onOpenMenu,
  onAddApp,
  onBlankContextMenu,
  onReorder,
  registerRef,
}: {
  canManage: boolean
  onOpenMenu: (app: AppView, x: number, y: number) => void
  onAddApp: () => void
  onBlankContextMenu: (x: number, y: number) => void
  onReorder: (ids: string[]) => void
  registerRef: (id: string, el: HTMLElement | null) => void
}) {
  const { state } = useHome()
  const cfg = state.config!
  const cats = visibleCategories(cfg)

  if (cats.length === 0) {
    return (
      <EmptyState
        title="还没有分组"
        hint="分组决定左边那一列显示什么。先建一个，再把应用放进去。"
        action={canManage ? { label: '新建分组', onClick: onAddApp } : undefined}
      />
    )
  }

  return (
    /**
     * 右键挂在整条流上（不只是网格）：分组之间的间距、流末尾的空白也算"空白处"。
     * 网格自己的 onContextMenu 已去掉 —— 两层都接的话会弹两次菜单。
     */
    <div
      className="min-h-[60vh] space-y-3 pb-12"
      onContextMenu={
        canManage
          ? (e) => {
              e.preventDefault()
              onBlankContextMenu(e.clientX, e.clientY)
            }
          : undefined
      }
    >
      {cats.map((cat) => {
        const apps = visibleApps(cfg, cat.id)
        /**
         * 空分组**不渲染任何东西**。
         *
         * 以前这里会写「「私密」还锁着，点左栏解锁」—— 但左栏那个锁图标
         * 已经说得很清楚了，同一句话在内容区再写一遍，既占地方又打断流式节奏
         * （几个空分组连着排，整条流看起来断断续续的）。
         *
         * 空分组仍然留在左栏里：它是"点过去解锁"的入口，不能没有。
         */
        if (apps.length === 0) {
          return <Section key={cat.id} id={cat.id} title="" registerRef={registerRef}><span /></Section>
        }
        return (
          <Section key={cat.id} id={cat.id} title={cat.name} registerRef={registerRef}>
            <AppGrid
              apps={apps}
              prefer={cfg.networkPrefer}
              onOpenMenu={canManage ? onOpenMenu : undefined}
              onReorder={canManage ? onReorder : undefined}
            />
          </Section>
        )
      })}
    </div>
  )
}
