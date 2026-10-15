import { useState } from 'react'
import { ArrowUpRight, Blocks, CircleCheck, Power, RotateCw, TriangleAlert } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import { disabledIn, refreshComponents, toggleComponent, useComponents } from '../lib/components'
import { pages, type PageKey, type PageMeta } from './registry'
import type { ComponentHealth, ComponentInfo } from '../types'

/**
 * 按组件名找出它对应的页面。功能管理页据此把「stun」这种机器名换成界面上的
 * 「STUN 内网穿透」，并给出「打开这一页」的入口。
 *
 * 从注册表推出来而不是再写一张表：那条对应关系本来就已经写在注册表里了
 * （PageMeta.component），再抄一遍就是第二个真相源，改了注册表忘了改这里
 * 就会出现「点进去是空白页」。
 *
 * # 为什么是函数，不是模块顶层的常量
 *
 * 这里有一条循环依赖：registry.tsx 引入本文件（把 ComponentManager 挂进
 * pages），本文件又从 registry.tsx 取 pages。谁先求值由打包顺序决定 ——
 * 一旦本文件先被求值，顶层就去 Object.keys(pages) 的话，pages 还是
 * undefined，Object.keys(undefined) 抛 TypeError，整个 bundle 停在这一行，
 * React 根本没挂载，后台就是一片白屏（控制台里只有一句
 * "Cannot convert undefined or null to object"，看不出跟页面有关）。
 *
 * 推迟到渲染时才读，那时注册表必然已经初始化完毕。页面只有十来个，
 * 每次渲染扫一遍的开销可以忽略，不值得为它引入缓存。
 */
function pageByComponent(name: string): { key: PageKey; meta: PageMeta } | undefined {
  for (const key of Object.keys(pages) as PageKey[]) {
    // 这里必须显式标注成 PageMeta：pages 用了 satisfies，每个值的类型都是它自己的
    // 字面量形状，没写 component 的那几条（dashboard、logs……）在联合类型上
    // 根本没有这个属性，直接取会报 TS2339。
    const meta: PageMeta = pages[key]
    if (meta.component === name) return { key, meta }
  }
  return undefined
}

type Tone = 'ok' | 'warn' | 'bad' | 'idle'

const toneClass: Record<Tone, string> = {
  ok: 'bg-emerald-50 text-emerald-600',
  warn: 'bg-amber-50 text-amber-600',
  bad: 'bg-rose-50 text-rose-600',
  idle: 'bg-slate-100 text-slate-500',
}

const HEALTH_LABEL: Record<string, string> = {
  healthy: '正常',
  degraded: '需要留意',
  down: '异常',
  unknown: '未知',
}

const healthTone = (h: ComponentHealth): Tone => {
  switch (h.status) {
    case 'healthy':
      return 'ok'
    case 'degraded':
      return 'warn'
    case 'down':
      return 'bad'
    default:
      return 'idle'
  }
}

/**
 * 功能管理 —— 列出后端注册的全部组件，并允许开关它们。
 *
 * 这一页的内容**完全由后端驱动**：新增一个组件（只加一个
 * modules/<x>/component.go）就会自动出现在列表里，前端一行都不用改。
 * 这正是 M3 要达成的效果，也是这一页存在的意义。
 */
export function ComponentManager() {
  const state = useComponents()
  const [busy, setBusy] = useState('')
  const [err, setErr] = useState('')

  const { loaded, error, list, health } = state
  const off = disabledIn(state)

  const onToggle = async (c: ComponentInfo, next: boolean) => {
    setBusy(c.name)
    setErr('')
    try {
      await toggleComponent(c.name, next)
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy('')
    }
  }

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader
          title="功能管理"
          action={
            <button
              type="button"
              onClick={() => {
                setErr('')
                void refreshComponents()
              }}
              className="flex items-center gap-1 rounded-lg border border-slate-200 px-2.5 py-1 text-xs font-semibold text-slate-600 transition hover:bg-slate-50"
            >
              <RotateCw className="h-3.5 w-3.5" /> 刷新
            </button>
          }
        />

        <p className="text-xs leading-relaxed text-slate-500">
          关闭一个功能后，它的接口会直接返回 404（不是 403 —— 不告诉未被授权的人
          「这里本来有个功能」），侧栏上对应的入口也会消失。改动立即生效，不用重启。
        </p>

        {off.length > 0 && (
          <div className="mt-3 rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700">
            当前已关闭：{off.join('、')}
          </div>
        )}

        {(err || error) && (
          <div className="mt-3 rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-600">
            {err || error}
          </div>
        )}
      </Card>

      <Card className="p-0">
        {!loaded ? (
          <div className="px-5 py-6 text-sm text-slate-400">读取中…</div>
        ) : list.length === 0 ? (
          <div className="px-5 py-6 text-sm text-slate-400">
            没有读到任何组件。若是接口报错，上面的提示里会有原因。
          </div>
        ) : (
          <ul className="divide-y divide-slate-100">
            {list.map((c) => {
              const linked = pageByComponent(c.name)
              const Icon = linked?.meta.icon ?? Blocks
              const h = health[c.name]
              const working = busy === c.name

              return (
                <li key={c.name} className="flex flex-wrap items-center gap-x-3 gap-y-2 px-4 py-3 sm:px-5">
                  <span className="grid h-9 w-9 shrink-0 place-items-center rounded-xl bg-slate-100 text-slate-500">
                    <Icon className="h-4.5 w-4.5" />
                  </span>

                  <div className="min-w-0 flex-1">
                    <div className="flex items-center gap-2">
                      <span className="truncate text-sm font-semibold text-slate-800">
                        {linked?.meta.label ?? c.name}
                      </span>
                      {/* 机器名照旧露出来：查日志、改 config/components.json 时用的是它 */}
                      <span className="shrink-0 font-mono text-[10px] text-slate-400">{c.name}</span>
                    </div>
                    <div className="mt-0.5 flex flex-wrap items-center gap-x-2 gap-y-1 text-[11px] text-slate-400">
                      {c.tasks && c.tasks.length > 0 && <span>定时任务：{c.tasks.join('、')}</span>}
                      {h?.message && <span className="text-amber-600">{h.message}</span>}
                    </div>
                  </div>

                  {/* 健康状态：后端没实现 HealthChecker 的组件不显示这一块 */}
                  {h && (
                    <span
                      className={`shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-bold ${toneClass[healthTone(h)]}`}
                      title={h.message || '一切正常'}
                    >
                      {h.status === 'healthy' ? (
                        <CircleCheck className="mr-0.5 inline h-3 w-3" />
                      ) : (
                        <TriangleAlert className="mr-0.5 inline h-3 w-3" />
                      )}
                      {HEALTH_LABEL[h.status] ?? h.status}
                    </span>
                  )}

                  {linked && c.enabled && (
                    <a
                      href={`#/${linked.key}`}
                      className="flex shrink-0 items-center gap-1 rounded-lg border border-slate-200 px-2.5 py-1 text-xs font-semibold text-slate-600 transition hover:bg-slate-50"
                    >
                      打开 <ArrowUpRight className="h-3.5 w-3.5" />
                    </a>
                  )}

                  {c.enableable ? (
                    <button
                      type="button"
                      role="switch"
                      aria-checked={c.enabled}
                      aria-label={`${c.enabled ? '关闭' : '开启'} ${c.name}`}
                      disabled={working}
                      onClick={() => void onToggle(c, !c.enabled)}
                      title={c.enabled ? '关闭这个功能' : '开启这个功能'}
                      className={`relative h-6 w-11 shrink-0 rounded-full transition disabled:opacity-50 ${
                        c.enabled ? 'bg-blue-500' : 'bg-slate-300'
                      }`}
                    >
                      <span
                        className={`absolute top-0.5 grid h-5 w-5 place-items-center rounded-full bg-white shadow transition-all ${
                          c.enabled ? 'left-[1.375rem]' : 'left-0.5'
                        }`}
                      >
                        <Power
                          className={`h-3 w-3 ${c.enabled ? 'text-blue-500' : 'text-slate-400'}`}
                        />
                      </span>
                    </button>
                  ) : (
                    // 不能关的功能说明白为什么，否则用户会去找那个不存在的开关
                    <span className="shrink-0 text-[11px] text-slate-400">不可关闭</span>
                  )}
                </li>
              )
            })}
          </ul>
        )}
      </Card>
    </div>
  )
}
