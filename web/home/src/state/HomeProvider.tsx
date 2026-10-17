/**
 * 导航页的唯一数据源。
 *
 * 为什么要有这一层，而不是各组件自己 useState：
 *
 * 1. **乐观更新只有一处可写**。拖拽、改尺寸、删除都是"先改本地、再发请求、
 *    失败回滚"。这三条散在组件里的话，迟早会有一个忘了回滚 —— 表现为
 *    "明明失败了，界面上却像是成功了，刷新才变回去"。
 * 2. **反馈统一**。loading / 成功 / 失败从这里统一抛出，toast 只认一种形状。
 * 3. 老版本是一个 `config` useState 贯穿 3000 行，任何小改动都是全量 setState。
 *
 * 变更一律走 action：这样一来"这次改动是什么"是可枚举的，
 * 将来要加撤销/重做、要打点，都只用改这里。
 */

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
  type ReactNode,
} from 'react'

import * as api from '../api/home'
import { clearToken } from '../api/token'
import type { AppView, Category, HomeConfig } from '../types'
import { resolveActive } from './view'

/* ============ 状态 ============ */

export type LoadState = 'loading' | 'ready' | 'error'

interface State {
  config: HomeConfig | null
  load: LoadState
  error: string
  /** 正在进行的写操作数量；>0 时显示"保存中"但**不锁界面** */
  pending: number
  /** 当前选中的分类；'' = 全部 */
  activeCategory: string
}

type Action =
  | { type: 'loaded'; config: HomeConfig }
  | { type: 'failed'; message: string }
  | { type: 'replaceConfig'; config: HomeConfig }
  | { type: 'pending'; delta: number }
  | { type: 'category'; id: string }

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case 'loaded':
      return {
        ...state,
        config: action.config,
        load: 'ready',
        error: '',
        // 选中的分类可能已经被删掉了 —— 落回**第一个**，不要落回空串
        // （空串以前表示"全部"，现在没这个概念了）
        activeCategory: resolveActive(action.config, state.activeCategory),
      }
    case 'failed':
      return { ...state, load: 'error', error: action.message }
    case 'replaceConfig':
      return { ...state, config: action.config }
    case 'pending':
      return { ...state, pending: Math.max(0, state.pending + action.delta) }
    case 'category':
      return { ...state, activeCategory: action.id }
  }
}

const initialState: State = {
  config: null,
  load: 'loading',
  error: '',
  pending: 0,
  activeCategory: '',
}

/* ============ Toast ============ */

export interface Toast {
  id: number
  kind: 'ok' | 'error'
  text: string
  /** 有重试按钮时挂在这里 */
  retry?: () => void
}

/* ============ Context ============ */

interface HomeStore {
  state: State
  /** 重新拉一次配置（写操作失败后回滚要用） */
  reload: () => Promise<void>
  setActiveCategory: (id: string) => void
  /**
   * 执行一个写操作。
   *
   * `apply` 是本地的乐观修改，`request` 是真的发请求。
   * 失败时：回滚到 request 之前的快照 + 弹一个带「重试」的 toast。
   *
   * **为什么默认不弹成功 toast**：动作本身已经给了反馈（卡片动了、开关翻了），
   * 再弹一次是噪音。需要说话的调用方自己传 okMsg。
   */
  mutate: (opts: {
    apply?: (cfg: HomeConfig) => HomeConfig
    request: () => Promise<unknown>
    okMsg?: string
    retry?: () => void
  }) => Promise<boolean>
  toasts: Toast[]
  dismissToast: (id: number) => void
  notify: (kind: 'ok' | 'error', text: string, retry?: () => void) => void
}

const HomeContext = createContext<HomeStore | null>(null)

export function HomeProvider({ children }: { children: ReactNode }) {
  const [state, dispatch] = useReducer(reducer, initialState)
  const [toasts, setToasts] = useState<Toast[]>([])
  const seq = useRef(0)
  // 卸载后不再 setState（离开页面时的请求返回会炸）
  const alive = useRef(true)
  // 快照：失败回滚要用。放在 ref 里是为了让 mutate 能读到"发起请求那一刻"的配置
  const snapshot = useRef<HomeConfig | null>(null)

  useEffect(() => {
    alive.current = true
    return () => {
      alive.current = false
    }
  }, [])

  const notify = useCallback((kind: 'ok' | 'error', text: string, retry?: () => void) => {
    const id = ++seq.current
    setToasts((prev) => [...prev, { id, kind, text, retry }])
    // 成功的自己消失；失败的留久一点，并且不自动消失（要留时间让人点重试）
    if (kind === 'ok') {
      setTimeout(() => setToasts((prev) => prev.filter((t) => t.id !== id)), 2200)
    }
  }, [])

  const dismissToast = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id))
  }, [])

  const reload = useCallback(async () => {
    try {
      const cfg = await api.getConfig()
      if (!alive.current) return
      snapshot.current = cfg
      dispatch({ type: 'loaded', config: cfg })
    } catch (e) {
      if (!alive.current) return
      dispatch({ type: 'failed', message: e instanceof Error ? e.message : '加载失败' })
    }
  }, [])

  useEffect(() => {
    void reload()
  }, [reload])

  const mutate = useCallback<HomeStore['mutate']>(
    async ({ apply, request, okMsg, retry }) => {
      const before = state.config
      if (apply && before) {
        const next = apply(before)
        snapshot.current = before
        if (!alive.current) return false
        dispatch({ type: 'replaceConfig', config: next })
      }

      dispatch({ type: 'pending', delta: 1 })
      try {
        await request()
        if (!alive.current) return false
        if (okMsg) notify('ok', okMsg)
        return true
      } catch (e) {
        if (!alive.current) return false
        // 回滚：把乐观修改撤掉。用 reload 而不是直接还原快照，
        // 是因为服务端可能已经部分生效了 —— 以服务端为准最稳
        if (before) {
          try {
            const fresh = await api.getConfig()
            if (alive.current) dispatch({ type: 'replaceConfig', config: fresh })
          } catch {
            if (alive.current && before) dispatch({ type: 'replaceConfig', config: before })
          }
        }
        notify('error', e instanceof Error ? e.message : '操作失败', retry)
        return false
      } finally {
        if (alive.current) dispatch({ type: 'pending', delta: -1 })
      }
    },
    [state.config, notify],
  )

  const setActiveCategory = useCallback((id: string) => {
    dispatch({ type: 'category', id })
  }, [])

  const value = useMemo<HomeStore>(
    () => ({ state, reload, setActiveCategory, mutate, toasts, dismissToast, notify }),
    [state, reload, setActiveCategory, mutate, toasts, dismissToast, notify],
  )

  return <HomeContext.Provider value={value}>{children}</HomeContext.Provider>
}

export function useHome(): HomeStore {
  const ctx = useContext(HomeContext)
  if (!ctx) throw new Error('useHome 必须在 HomeProvider 里用')
  return ctx
}

/* ============ 派生数据 ============ */

/**
 * 当前该显示哪些应用。
 *
 * 未登录时后端**已经**过滤掉了没权限的分组（服务端过滤，不是前端藏），
 * 所以这里只管按选中的分类筛，不做任何权限判断 ——
 * 前端藏东西是假安全，翻源码就能看到。
 */
/**
 * 当前分组里的应用。
 *
 * `activeCategory` 为空时**返回空数组**，不是"全部"。
 * 首页没有"全部"这一页 —— 具体理由见 state/view.ts 的注释。
 * 真要全量列表的地方（搜索、设置里的图标管理）显式传 '*'。
 */
export function visibleApps(config: HomeConfig | null, activeCategory: string): AppView[] {
  if (!config) return []
  let list: AppView[]
  if (activeCategory === '*') {
    list = config.apps
  } else if (!activeCategory) {
    list = []
  } else {
    list = config.apps.filter((a) => a.categoryId === activeCategory)
  }
  return [...list].sort((a, b) => a.order - b.order)
}

export function visibleCategories(config: HomeConfig | null): Category[] {
  if (!config) return []
  return [...config.categories].sort((a, b) => a.order - b.order)
}

/** 退出登录：清 token 再重载（配置里那些"要登录才给"的内容会消失） */
export function useLogout() {
  const { reload } = useHome()
  return useCallback(async () => {
    clearToken()
    await reload()
  }, [reload])
}
