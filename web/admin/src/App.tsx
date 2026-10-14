import { useEffect, useState } from 'react'
import { AppShell } from './layout/AppShell'
import { Login, Setup } from './pages/Auth'
import { DEFAULT_PAGE, findPage, isPageKey, pages, type ResolvedPage } from './pages/registry'
import { enabledIn, useComponents } from './lib/components'
import { getAuthStatus, getToken, isDesktop } from './lib/api'
import type { PageKey } from './types'

function readPageFromHash(): PageKey {
  const h = window.location.hash.replace(/^#\/?/, '')
  return isPageKey(h) ? h : DEFAULT_PAGE
}

type AuthState = 'loading' | 'setup' | 'login' | 'authed'

function App() {
  const [page, setPage] = useState<PageKey>(() => readPageFromHash())
  const [auth, setAuth] = useState<AuthState>('loading')

  // 启动：查初始化状态，决定进引导 / 登录 / 后台
  useEffect(() => {
    let alive = true
    getAuthStatus()
      .then((s) => {
        if (!alive) return
        if (!s.initialized) setAuth('setup')
        else if (isDesktop() || getToken()) setAuth('authed')
        else setAuth('login')
      })
      .catch(() => {
        if (alive) setAuth('login')
      })
    return () => {
      alive = false
    }
  }, [])

  useEffect(() => {
    const onHash = () => setPage(readPageFromHash())
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [])

  const navigate = (key: PageKey) => {
    if (window.location.hash !== `#/${key}`) {
      window.location.hash = `#/${key}`
    }
    setPage(key)
  }

  if (auth === 'loading') {
    return <div className="grid min-h-screen place-items-center bg-slate-50 text-sm text-slate-400">加载中…</div>
  }
  if (auth === 'setup') {
    return <Setup onSuccess={() => setAuth('authed')} />
  }
  if (auth === 'login') {
    return <Login onSuccess={() => setAuth('authed')} />
  }

  // 登录后才需要组件状态（功能开关决定显示哪些页面）。放在这里而不是
  // 顶层的 App 里：useComponents 首次挂载就会拉 /api/system/components，
  // 那是个需要登录的接口，登录页上拉它只会白得一个 401。
  return <AdminApp page={page} onNavigate={navigate} />
}

/**
 * 登录后的后台壳。页面本体一律从注册表取（pages/registry.tsx）——
 * 这里不再有「每个页面一个 case」的 switch：漏写一个 case 不会报错，
 * 只会静默渲染成占位符，那正是这次改造要消掉的问题。
 */
function AdminApp({ page, onNavigate }: { page: PageKey; onNavigate: (k: PageKey) => void }) {
  const comps = useComponents()

  // 显式标注 ResolvedPage：?? 右侧展开的是 pages[dashboard] 的字面量类型，
  // 没有 component 这个可选属性，不标注会 TS2339。
  const wanted: ResolvedPage = findPage(page) ?? { key: DEFAULT_PAGE, ...pages[DEFAULT_PAGE] }

  // 页面归属的组件被「功能管理」关掉时，回落到仪表盘：
  // 入口从侧栏消失之后，直接敲 URL 也不该进得去（里面只会是一页报错）。
  const usable = enabledIn(comps, wanted.component)
  const active: PageKey = usable ? wanted.key : DEFAULT_PAGE

  useEffect(() => {
    // 地址栏也带回去，否则刷新 / 分享链接又会落回一个已关闭的页面
    if (!usable && window.location.hash !== `#/${active}`) {
      window.location.hash = `#/${active}`
    }
  }, [usable, active])

  return (
    <AppShell active={active} onChange={onNavigate}>
      {usable ? wanted.element : pages[DEFAULT_PAGE].element}
    </AppShell>
  )
}

export default App
