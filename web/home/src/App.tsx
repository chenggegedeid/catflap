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

import { deleteApp, gotoLogin, moveApp, reorderApps, reorderCategories, saveDock } from './api/home'
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
import { useWheelPaging } from './components/layout/useWheelPaging'
import { HomeDnd, type DragInfo } from './components/dnd/HomeDnd'
import { Dock, useDockDroppable } from './components/dock/Dock'
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
  // 壁纸和标题不依赖配置：加载中、加载失败也要有个能看的样子。
  // 放在最前面 —— 模式判断（currentCatId 等）都依赖它
  const cfg = state.config
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
  // Dock 项的右键菜单（打开/复制/编辑/移除 —— 和卡片菜单同一套）
  const [dockMenu, setDockMenu] = useState<MenuState | null>(null)

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
  const cats = visibleCategories(state.config)
  const catIds = cats.map((c) => c.id)

  /**
   * 滚轮翻分组。
   *
   * 判据是"**当前这一组**是不是已经看到头了"，而不是"整页滚到头了" ——
   * 用整页判据的话，一屏能放下好几组时永远翻不动（页面还没到底），
   * 表现就是"滚轮没反应"。这正是上一版不生效的原因。
   *
   * 手机上不启用：触屏本来就能直接滑，再叠一层"滚轮翻页"只会打架。
   */
  // 翻页方向：给滑动动画用（往下翻 = 新内容从下滑入）
  const [pageDir, setPageDir] = useState<1 | -1>(1)
  // 正在拖拽的信息 —— 拖动时禁用滚轮翻页（拖到一半页面跳走是很糟的体验）
  const [dragInfo, setDragInfo] = useState<DragInfo | null>(null)

  /**
   * "当前在哪个分组"的唯一答案：点击态。
   *
   * **为什么没有"跟着滚动走"的模式了**：用户实测反馈，把所有分组堆在
   * 一页里（我之前理解的"流式"）根本不是他要的 —— 他要的是**以组为界面**，
   * 滚一下翻一组。那就只有一个真相：点击/翻页选中的那一组。
   * scrollspy、scrollIntoView 那一整套都删了。
   */
  const currentCatId = state.activeCategory

  const pageByWheel = useCallback(
    (dir: 1 | -1) => {
      /**
       * 翻页**跳过上锁且未解锁的分组**。
       * 用户明确要的：上锁的分组不参与翻页（翻到了也只是一块空态），
       * 想进就自己在左栏点它解锁。这样翻页永远落在"能看的内容"上。
       */
      const skippable = new Set(
        (cfg?.categories ?? [])
          .filter((c) => c.visibility === 'lock' && !c.unlocked)
          .map((c) => c.id),
      )
      const targets = catIds.filter((id) => !skippable.has(id))
      if (targets.length < 2) return
      const i = targets.indexOf(currentCatId)
      if (i < 0) return
      const next = Math.min(targets.length - 1, Math.max(0, i + dir))
      if (next === i) return
      const nextId = targets[next]
      setPageDir(dir)
      // 内容替换就是反馈，不需要滚动
      setPageDir(dir)
      setActiveCategory(nextId)
    },
    [catIds, currentCatId, cfg, setActiveCategory],
  )

  /**
   * 键盘翻分组：PageDown / PageUp。
   *
   * 为什么要有：滚轮翻页依赖 wheel 事件，而有些环境（远程桌面、
   * 某些触控板驱动、自动化工具）不产生 wheel —— 那时"翻页"就完全没路了。
   * 键盘是兜底，而且对键盘用户本来就该有。
   *
   * 焦点在输入框里时不响应，免得打断打字。
   */
  const onPageKey = useCallback(
    (e: KeyboardEvent) => {
      if (e.key !== 'PageDown' && e.key !== 'PageUp') return
      const t = e.target as HTMLElement | null
      if (t && (t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.isContentEditable)) return
      if (catIds.length < 2) return
      e.preventDefault()
      pageByWheel(e.key === 'PageDown' ? 1 : -1)
    },
    [catIds, pageByWheel],
  )

  useEffect(() => {
    window.addEventListener('keydown', onPageKey)
    return () => window.removeEventListener('keydown', onPageKey)
  }, [onPageKey])



  /** 分组拖动排序：按新顺序写回 order（1..N 连续） */
  const reorderCats = useCallback(
    (ids: string[]) => {
      const pos = new Map(ids.map((id, i) => [id, i + 1]))
      void mutate({
        apply: (cfg) => ({
          ...cfg,
          categories: cfg.categories.map((c) => (pos.has(c.id) ? { ...c, order: pos.get(c.id)! } : c)),
        }),
        request: () => reorderCategories(ids),
      })
    },
    [mutate],
  )

  /** 拖进托盘：追加到末尾（已在里面就不动，避免重复项） */
  const dropToDock = useCallback(
    (appId: string) => {
      const cur = state.config?.dock ?? []
      const ids = cur.map((a) => a.id)
      if (ids.includes(appId)) return
      const next = [...ids, appId]
      void mutate({
        apply: (cfg) => {
          const app = cfg.apps.find((a) => a.id === appId)
          if (!app) return cfg
          return { ...cfg, dock: [...(cfg.dock ?? []), app] }
        },
        request: () => saveDock(next),
        okMsg: '已添加到托盘',
      })
    },
    [state.config, mutate],
  )

  /**
   * 拖到别的分组按钮上 = 移动分组。
   *
   * 不弹确认框：拖拽是**可逆**的（再拖回去就行），
   * 而确认框会打断"拖放"这个连续动作，手感很糟。
   * 真正不可逆的删除才需要确认。
   */
  const moveToCategory = useCallback(
    (appId: string, categoryId: string) => {
      const app = state.config?.apps.find((a) => a.id === appId)
      if (!app || app.categoryId === categoryId) return
      const catName = state.config?.categories.find((c) => c.id === categoryId)?.name ?? ''
      void mutate({
        apply: (cfg) => ({
          ...cfg,
          apps: cfg.apps.map((a) => (a.id === appId ? { ...a, categoryId } : a)),
        }),
        request: () => moveApp(appId, categoryId),
        okMsg: `已移到「${catName}」`,
      })
    },
    [state.config, mutate],
  )

  const removeFromDock = useCallback(
    (appId: string) => {
      const ids = (state.config?.dock ?? []).map((a) => a.id).filter((id) => id !== appId)
      void mutate({
        apply: (cfg) => ({ ...cfg, dock: (cfg.dock ?? []).filter((a) => a.id !== appId) }),
        request: () => saveDock(ids),
        okMsg: '已从托盘移除',
      })
    },
    [state.config, mutate],
  )

  const { setNodeRef: dockRef, isOver } = useDockDroppable()

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

  // hover: none 表示触屏设备 —— 那里不要滚轮翻页（触屏直接滑就行）
  const isTouch = typeof window !== 'undefined' && window.matchMedia('(hover: none)').matches
  // 拖动中禁用翻页：拖拽本身就在用指针，滚轮这时翻页只会捣乱
  useWheelPaging(!isTouch && catIds.length > 1 && !dragInfo, () => null, pageByWheel, true)

  return (
    /**
     * 全页唯一的 DndContext。卡片网格 / 分组栏 / 底部 Dock 都要挂在它下面，
     * 否则跨区域拖不动（每个区域各建一个 DndContext = 三个互不相通的会话）。
     * 访客（enabled=false）时这里直接返回 children，不做任何拖拽包装。
     */
    <HomeDnd
      enabled={!!cfg?.authed}
      onReorderApps={onReorder}
      onReorderCategories={reorderCats}
      onDropToDock={dropToDock}
      onMoveToCategory={moveToCategory}
      onDragStateChange={setDragInfo}
    >
      {/* min-h-dvh：移动端的 100vh 是"地址栏收起时"的高度，用它做满屏会被挡住一截 */}
      <div className="min-h-dvh">
      <Background wallpaper={cfg?.wallpaper} />

      {/*
        安全区：左右给刘海/圆角让位；底部多留一截给 Dock 和 iPhone 横条。
        用 max() 是因为 env() 在不支持的浏览器里是无效值 —— 直接写 env() 会让
        整条 padding 声明被丢弃，等于没有边距。
      */}
      <div
        className="mx-auto flex min-h-dvh w-full max-w-[1600px] flex-col pt-4 sm:px-6 sm:pt-8"
        style={{
          paddingLeft: 'max(1rem, var(--safe-left))',
          paddingRight: 'max(1rem, var(--safe-right))',
          paddingBottom: 'calc(1.5rem + var(--safe-bottom) + 3.5rem)',
        }}
      >
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
         * 一屏一组：内容区只渲染当前分组，左栏点击/滚轮翻页切换。
         *
         * 手机上左栏变成**横向一排贴在顶部**（纵向排会吃掉太多宽度，
         * 卡片就挤得没法点了）；宽屏才是左侧竖栏。
         */}
        <div className="mt-5 flex flex-1 flex-col gap-3 sm:mt-8 sm:flex-row sm:gap-4">
          <div className="sticky top-0 z-20 -mx-1 bg-gradient-to-b from-slate-900/10 to-transparent px-1 py-1 backdrop-blur-sm sm:top-6 sm:mx-0 sm:bg-none sm:px-0 sm:py-0 sm:pt-1 sm:backdrop-blur-none">
            <CategoryRail
              categories={visibleCategories(cfg)}
              active={currentCatId}
              onSelect={(id) => {
                const cat = visibleCategories(cfg).find((c) => c.id === id)
                if (cat && cat.visibility === 'lock') {
                  setUnlocking(cat)
                  return
                }
                // 点击 = 直接切换内容（paged），即时反馈
                setActiveCategory(id)
              }}
              onAdd={() => setSettingsOpen(true)}
              canManage={!!cfg?.authed}
              onReorder={cfg?.authed ? reorderCats : undefined}
              hiddenCount={cfg?.hiddenCategoryCount}
            />
          </div>

          <main className="min-w-0 flex-1">
            {state.load === 'loading' && <GridSkeleton />}

            {state.load === 'error' && <ErrorState message={state.error} onRetry={onRetry} retrying={retrying} />}

            {state.load === 'ready' && cfg && (
              <Feed
                canManage={!!cfg.authed}
                activeId={currentCatId}
                pageDir={pageDir}
                onOpenMenu={(app, x, y) => setMenu({ app, x, y })}
                onAddApp={openAddApp}
                onBlankContextMenu={(x, y) => setBlankMenu({ x, y })}
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

      <Dock
        items={cfg?.dock ?? []}
        prefer={cfg?.networkPrefer ?? 'wanV4'}
        isOver={isOver}
        dropRef={dockRef}
        canManage={!!cfg?.authed}
        onOpenApp={(app) => setFormFor(app)}
        onOpenMenu={(app, x, y) => setDockMenu({ app, x, y })}
      />

      {dockMenu && (
        <ContextMenu
          menu={dockMenu}
          prefer={cfg?.networkPrefer ?? 'wanV4'}
          onClose={() => setDockMenu(null)}
          onDelete={removeFromDock}
          deleteLabel="从托盘移除"
          onEdit={setFormFor}
          onNotify={notify}
        />
      )}

      <Toasts toasts={toasts} onDismiss={dismissToast} />
    </div>
    </HomeDnd>
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
  activeId,
  pageDir,
  onOpenMenu,
  onAddApp,
  onBlankContextMenu,
}: {
  canManage: boolean
  activeId: string
  /** 翻页方向，决定滑动动画从哪边进来 */
  pageDir: 1 | -1
  onOpenMenu: (app: AppView, x: number, y: number) => void
  onAddApp: () => void
  onBlankContextMenu: (x: number, y: number) => void
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

  /**
   * paged 模式：只渲染当前分组。
   *
   * 为什么要有这个模式：stream 下"翻页"靠的是滚动，而**内容不足一屏时
   * 页面根本没有可滚的余地** —— 滚轮发了事件、scrollIntoView 也调了，
   * 但页面动不了，用户看到的就是"翻页没反应"。
   * paged 每次只显示一组，翻页就是真的翻（配滑动动画）。
   *
   * key 用 activeId：React 按 key 重建节点，动画每次都从头播；
   * 不换 key 的话同一组内翻页动画不会触发。
   */
  {
    const cat = cats.find((c) => c.id === activeId) ?? cats[0]
    const apps = cat ? visibleApps(cfg, cat.id) : []
    return (
      <div
        className="min-h-[60vh] pb-12"
        onContextMenu={
          canManage
            ? (e) => {
                e.preventDefault()
                onBlankContextMenu(e.clientX, e.clientY)
              }
            : undefined
        }
      >
        {cat && (
          <div key={cat.id} className={pageDir === 1 ? 'animate-page-next' : 'animate-page-prev'}>
            <h2 className="mb-2 px-1 text-[11px] font-medium tracking-wide text-white/55">{cat.name}</h2>
            {apps.length > 0 ? (
              <AppGrid apps={apps} prefer={cfg.networkPrefer} onOpenMenu={canManage ? onOpenMenu : undefined} />
            ) : (
              /** 空态必须是**一块看得见的区域**，不能是一行小字 ——
               *  否则点击分组后视口里啥也没变，用户以为没切换成功 */
              <EmptyState
                title={`「${cat.name}」还没有应用`}
                hint="右键空白处可以新建；也可以把别的分组的卡片拖过来。"
                action={canManage ? { label: '添加应用', onClick: onAddApp } : undefined}
              />
            )}
          </div>
        )}
      </div>
    )
  }


}
