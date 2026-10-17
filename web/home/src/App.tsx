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

import { gotoLogin } from './api/home'
import { Background } from './components/layout/Background'
import { CategoryRail } from './components/layout/CategoryRail'
import { Header } from './components/layout/Header'
import { EmptyState, ErrorState, GridSkeleton } from './components/feedback/States'
import { Toasts } from './components/feedback/Toasts'
import { TopRightActions } from './components/chrome/TopRightActions'
import { AppGrid } from './components/grid/AppGrid'
import { HomeProvider, useHome, visibleApps, visibleCategories } from './state/HomeProvider'

export default function App() {
  return (
    <HomeProvider>
      <HomePage />
    </HomeProvider>
  )
}

function HomePage() {
  const { state, reload, setActiveCategory, toasts, dismissToast } = useHome()
  const [retrying, setRetrying] = useState(false)

  const onRetry = useCallback(async () => {
    setRetrying(true)
    await reload()
    setRetrying(false)
  }, [reload])

  // 壁纸和标题不依赖配置：加载中、加载失败也要有个能看的样子
  const cfg = state.config

  return (
    <div className="min-h-screen">
      <Background wallpaper={cfg?.wallpaper} />

      <div className="mx-auto flex min-h-screen w-full max-w-[1600px] flex-col px-4 pb-10 pt-8 sm:px-6">
        <TopRightActions authed={!!cfg?.authed} pending={state.pending > 0} onLogin={gotoLogin} />

        <div className="flex flex-col items-center gap-6">
          <Header title={cfg?.title ?? 'Catflap'} showTime={cfg?.showTime ?? true} />
        </div>

        <div className="mt-8 flex flex-1 gap-4 sm:gap-6">
          <CategoryRail
            categories={visibleCategories(cfg)}
            active={state.activeCategory}
            onSelect={setActiveCategory}
            canManage={!!cfg?.authed}
            hiddenCount={cfg?.hiddenCategoryCount}
          />

          <main className="min-w-0 flex-1">
            {state.load === 'loading' && <GridSkeleton />}

            {state.load === 'error' && <ErrorState message={state.error} onRetry={onRetry} retrying={retrying} />}

            {state.load === 'ready' && cfg && <GridArea categoryId={state.activeCategory} />}
          </main>
        </div>
      </div>

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
function GridArea({ categoryId }: { categoryId: string }) {
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
      />
    )
  }

  return <AppGrid apps={apps} />
}
