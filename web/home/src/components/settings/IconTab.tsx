/**
 * 图标管理。
 *
 * 一眼看过去哪些应用没图标、哪些图标加载不出来 —— 然后就地补。
 *
 * 补的方式两种：
 *   1. 上传（本地文件）
 *   2. 从网址抓（后端去取，避开浏览器跨域）
 *
 * 为什么值得单独一个标签：图标是导航页的脸面，但它恰恰是最容易坏的东西 ——
 * favicon 站点改版、外链挂掉、抓的时候被墙，都可能让一格变成白板。
 * 散在"编辑应用"里的话，得一个一个点进去才发现哪个坏了。
 */

import { useRef, useState } from 'react'
import { Upload } from 'lucide-react'
import { fetchIconFromURL, updateApp, uploadIcon } from '../../api/home'
import { useHome, visibleApps } from '../../state/HomeProvider'
import { AppIcon } from '../card/AppIcon'
import { IconPicker } from './IconPicker'
import type { AppView } from '../../types'

/** 抓 favicon 的候选地址：公网 v4 → 内网 → v6（应用里填了的才算） */
const fetchSourcesOf = (app: AppView): string[] =>
  [app.addresses.wanV4, app.addresses.lan, app.addresses.wanV6].filter(Boolean)

export function IconTab() {
  const { state, mutate, notify } = useHome()
  /**
   * 图标管理要的是**全部**应用，用 '*'。
   *
   * 这里踩过：之前传空串，而 visibleApps 的语义早就改了
   * （空串 = 空数组，因为首页不再有"全部"这一页），
   * 结果图标管理一片空白 —— 一个静默的回归，编译器不会报错。
   */
  const apps = visibleApps(state.config, '*')
  const [busyId, setBusyId] = useState('')
  const [url, setUrl] = useState('')
  // 哪个应用正在展开图标库
  const [libFor, setLibFor] = useState('')
  const fileRef = useRef<HTMLInputElement>(null)
  const uploadTarget = useRef<string>('')

  const missing = apps.filter((a) => !a.icon).length

  const setIcon = async (app: AppView, path: string) => {
    await mutate({
      apply: (cfg) => ({ ...cfg, apps: cfg.apps.map((x) => (x.id === app.id ? { ...x, icon: path } : x)) }),
      request: () => updateApp(app.id, { name: app.name, icon: path }),
      okMsg: '图标已更新',
    })
  }

  const onPickFile = (app: AppView) => {
    uploadTarget.current = app.id
    fileRef.current?.click()
  }

  const onFile = async (file: File) => {
    const id = uploadTarget.current
    const app = apps.find((a) => a.id === id)
    if (!app) return
    setBusyId(id)
    try {
      const { data } = await uploadIcon(file)
      if (data?.path) await setIcon(app, data.path)
    } catch (e) {
      notify('error', e instanceof Error ? e.message : '上传失败')
    } finally {
      setBusyId('')
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  /**
   * 抓取用**应用已填的地址**（公网 v4 → 内网 → v6 依次试）。
   * 之前要再手输一个地址 —— 地址就在应用里，重打一遍纯属多余。
   * 全都失败才提示上传。
   */
  const onFetchURL = async (app: AppView) => {
    const sources = url.trim() ? [url.trim()] : fetchSourcesOf(app)
    if (sources.length === 0) {
      notify('error', '这个应用还没有地址，先手填一个或上传图标')
      return
    }
    setBusyId(app.id)
    let ok = false
    for (const u of sources) {
      try {
        const { data } = await fetchIconFromURL(u)
        if (data?.path) {
          await setIcon(app, data.path)
          ok = true
          break
        }
      } catch {
        // 试下一个来源
      }
    }
    if (!ok) notify('error', '没抓到图标，可以试试上传本地图标')
    setBusyId('')
    setUrl('')
  }

  return (
    <div className="space-y-3">
      <div className="flex items-center justify-between">
        <p className="text-xs leading-relaxed text-slate-500">
          图标加载不出来时会自动退化成名字首字，但补上真图标更好认。
        </p>
        {missing > 0 && (
          <span className="shrink-0 rounded-full bg-amber-100 px-2 py-0.5 text-[11px] font-semibold text-amber-700">
            {missing} 个缺图标
          </span>
        )}
      </div>

      <input
        ref={fileRef}
        type="file"
        accept="image/*"
        className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0]
          if (f) void onFile(f)
        }}
      />

      <ul className="grid grid-cols-2 gap-2">
        {apps.map((a) => (
          <li key={a.id} className="rounded-xl border border-slate-200 p-2">
            <div className="flex items-center gap-2">
              <span className="h-9 w-9 shrink-0 overflow-hidden rounded-[22%] ring-1 ring-black/5">
                <AppIcon name={a.name} icon={a.icon} />
              </span>
              <div className="min-w-0 flex-1">
                <div className="truncate text-xs font-semibold text-slate-800">{a.name}</div>
                <div className="truncate text-[10px] text-slate-400">{a.icon || '无图标'}</div>
              </div>
            </div>

            <div className="mt-2 flex gap-1">
              <button
                type="button"
                disabled={busyId === a.id}
                onClick={() => onPickFile(a)}
                className="flex flex-1 items-center justify-center gap-1 rounded-lg bg-slate-100 py-1 text-[11px] font-semibold text-slate-600 transition hover:bg-slate-200 disabled:opacity-50"
              >
                <Upload className="h-3 w-3" />
                上传
              </button>
              <button
                type="button"
                onClick={() => void onFetchURL(a)}
                disabled={busyId === a.id}
                title={fetchSourcesOf(a)[0] ? `从 ${fetchSourcesOf(a)[0]} 抓取` : '先给应用填地址'}
                className="flex-1 rounded-lg bg-slate-100 py-1 text-[11px] font-semibold text-slate-600 transition hover:bg-slate-200 disabled:opacity-50"
              >
                {busyId === a.id ? '抓取中…' : '抓取图标'}
              </button>
              <button
                type="button"
                onClick={() => setLibFor(libFor === a.id ? '' : a.id)}
                className={`flex-1 rounded-lg py-1 text-[11px] font-semibold transition ${
                  libFor === a.id ? 'bg-blue-500 text-white' : 'bg-slate-100 text-slate-600 hover:bg-slate-200'
                }`}
              >
                图标库
              </button>
            </div>

            {libFor === a.id && (
              <div className="mt-1.5 rounded-lg border border-slate-200 p-1.5">
                <IconPicker
                  value={a.icon ?? ''}
                  onPick={(v) => {
                    void setIcon(a, v)
                    setLibFor('')
                  }}
                />
              </div>
            )}
          </li>
        ))}
      </ul>

      {apps.length === 0 && <p className="py-6 text-center text-xs text-slate-400">还没有应用</p>}
    </div>
  )
}
