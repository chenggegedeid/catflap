import { useSyncExternalStore } from 'react'
import { getComponents, setComponentEnabled } from './api'
import type { ComponentHealth, ComponentInfo } from '../types'

/**
 * 后端组件（功能）的开关与健康状态，全应用共享一份。
 *
 * 为什么要有这个模块：侧栏要隐藏「已经被关掉的功能」的入口，功能管理页要能改
 * 开关，两处必须看同一份数据 —— 各拉各的就会出现「管理页里关了，侧栏还挂着」
 * 这种要等刷新才自洽的状态。
 *
 * 用 useSyncExternalStore 而不是 Context：
 *   - 这份状态是**模块级单例**，本来就不该随组件树增删（Sidebar 在手机上会被
 *     卸掉重建，Context 得提到最外层才行）；
 *   - 它天生「外部可变」，正是 useSyncExternalStore 的适用场景。
 */

export interface ComponentsState {
  /** 已经拿到过后端的答复（成功或失败都算），失败时 list 可能仍是上一次的好数据 */
  loaded: boolean
  /** 最近一次拉取失败的原因，空串表示没失败 */
  error: string
  list: ComponentInfo[]
  health: Record<string, ComponentHealth>
}

const EMPTY: ComponentsState = { loaded: false, error: '', list: [], health: {} }

// state 只在真正变化时整体替换。useSyncExternalStore 要求 getSnapshot 在
// 没有变化时返回**同一个引用**，就地改字段会让 React 认不出来、或反过来死循环。
let state = EMPTY

const listeners = new Set<() => void>()
let inflight: Promise<void> | null = null

function emit() {
  for (const fn of listeners) fn()
}

function subscribe(fn: () => void): () => void {
  listeners.add(fn)
  // 第一个订阅者出现时自动拉一次，调用方不用记得「先 await 一下再渲染」
  if (listeners.size === 1 && !state.loaded) void refreshComponents()
  return () => {
    listeners.delete(fn)
  }
}

const getSnapshot = () => state

/** 拉一次组件列表。并发调用只会真发一次请求，返回的是同一个 Promise */
export function refreshComponents(): Promise<void> {
  if (inflight) return inflight

  inflight = (async () => {
    try {
      const data = await getComponents()
      state = {
        loaded: true,
        error: '',
        list: data.components ?? [],
        health: data.health ?? {},
      }
    } catch (e) {
      // 保留上一次的 list：接口偶发失败时，界面不该突然「所有页面都消失」。
      // 没拿到过数据时 list 为空，enabledIn 对空表一律放行（见下面的说明）。
      state = { ...state, loaded: true, error: e instanceof Error ? e.message : String(e) }
    } finally {
      inflight = null
      emit()
    }
  })()

  return inflight
}

/**
 * 开关一个组件。
 *
 * 成功后本地立刻改状态、不等下一次拉取：用户点完开关要马上看到侧栏变化，
 * 让界面等一轮网络往返才响应会显得点了没用。
 */
export async function toggleComponent(name: string, enabled: boolean): Promise<void> {
  await setComponentEnabled(name, enabled)

  state = {
    ...state,
    list: state.list.map((c) => (c.name === name ? { ...c, enabled } : c)),
  }
  emit()
}

/**
 * 某个组件当前是否启用。
 *
 * **拿不准就放行**是这里刻意的取舍，两种情况都返回 true：
 *   - name 为空 —— 这个页面不归任何组件管（例如仪表盘本身）；
 *   - 列表里找不到它 —— 还没有数据、拉取失败、或后端版本比前端旧。
 *
 * 反过来做的代价是「信息不全的时候先把功能藏起来」：用户看到的是一片缺了入口
 * 的侧栏，却没有任何提示，只能猜是不是装坏了。而放行的代价只是点进去接口 404，
 * 那是个能看懂、也会被日志记下来的现象。
 */
export function enabledIn(s: ComponentsState, name?: string): boolean {
  if (!name) return true
  const c = s.list.find((x) => x.name === name)
  if (!c) return true
  return c.enabled
}

/** 被关掉的组件名（有序）。界面提示用，例如「这台机器改过什么」 */
export function disabledIn(s: ComponentsState): string[] {
  return s.list
    .filter((c) => !c.enabled)
    .map((c) => c.name)
    .sort()
}

/** 订阅组件状态。任何用到它的组件都会在数据变化时自动重渲染 */
export function useComponents(): ComponentsState {
  return useSyncExternalStore(subscribe, getSnapshot)
}
