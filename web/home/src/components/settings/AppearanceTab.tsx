/**
 * 外观：壁纸 + 网络偏好。
 *
 * 壁纸三种来源，切换时**不清空另外两种的配置** ——
 * 切到 Bing 再切回自定义，刚才上传的那张还在（后端把 custom 单独存）。
 * 这是老版本就做对的一点，这里保持。
 */

import { useState } from 'react'
import { Check, Upload } from 'lucide-react'
import { updateNetworkPrefer, updateWallpaper, uploadWallpaper } from '../../api/home'
import { PRESET_WALLPAPERS } from '../../lib/wallpaperLibrary'
import { useHome } from '../../state/HomeProvider'
import type { NetworkPrefer, Wallpaper, WallpaperMode } from '../../types'

const MODES: { value: WallpaperMode; label: string; hint: string }[] = [
  { value: 'preset', label: '壁纸库', hint: '内置的渐变主题，点一下就换' },
  { value: 'bing', label: '必应每日图', hint: '每天换一张，需要能访问必应' },
  { value: 'custom', label: '自己上传', hint: '选一张本地图片' },
]

const PREFERS: { value: NetworkPrefer; label: string; hint: string }[] = [
  { value: 'wanV4', label: '公网 v4', hint: '点卡片优先走 IPv4 公网地址' },
  { value: 'wanV6', label: '公网 v6', hint: '有 IPv6 的话更快' },
  { value: 'lan', label: '内网', hint: '在家优先走局域网，最快' },
]

export function AppearanceTab() {
  const { state, mutate, notify } = useHome()
  const cfg = state.config
  const wp: Wallpaper = cfg?.wallpaper ?? { mode: 'default', resolution: '1080', blur: 0, custom: '', css: '' }
  const [uploading, setUploading] = useState(false)

  const save = (patch: Partial<Wallpaper>) => {
    void mutate({
      apply: (c) => ({ ...c, wallpaper: { ...c.wallpaper, ...patch } }),
      request: () => updateWallpaper({ ...wp, ...patch }),
    })
  }

  const onUpload = async (file: File) => {
    setUploading(true)
    try {
      const { data } = await uploadWallpaper(file)
      if (data?.path) {
        save({ custom: data.path, mode: 'custom' })
        notify('ok', '壁纸已更新')
      }
    } catch (e) {
      notify('error', e instanceof Error ? e.message : '上传失败')
    } finally {
      setUploading(false)
    }
  }

  return (
    <div className="space-y-5">
      <section>
        <h3 className="mb-2 text-xs font-semibold text-slate-700">壁纸</h3>
        <div className="space-y-1.5">
          {MODES.map((m) => (
            <button
              key={m.value}
              type="button"
              onClick={() => save({ mode: m.value })}
              className={`flex w-full items-start gap-2.5 rounded-xl border px-3 py-2 text-left transition ${
                wp.mode === m.value ? 'border-blue-400 bg-blue-50/60' : 'border-slate-200 hover:bg-slate-50'
              }`}
            >
              <span
                className={`mt-0.5 grid h-4 w-4 shrink-0 place-items-center rounded-full border ${
                  wp.mode === m.value ? 'border-blue-500 bg-blue-500 text-white' : 'border-slate-300'
                }`}
              >
                {wp.mode === m.value && <Check className="h-2.5 w-2.5" />}
              </span>
              <span className="min-w-0">
                <span className="block text-xs font-semibold text-slate-800">{m.label}</span>
                <span className="block text-[11px] text-slate-500">{m.hint}</span>
              </span>
            </button>
          ))}
        </div>

        {/*
          壁纸库：点缩略图即换（保存动作在点击里完成，和图标库一致 ——
          选了就生效，不用再点保存）。
        */}
        {wp.mode === 'preset' && (
          <div className="mt-2 grid grid-cols-3 gap-1.5">
            {PRESET_WALLPAPERS.map((p) => (
              <button
                key={p.key}
                type="button"
                title={p.name}
                onClick={() => save({ mode: 'preset', css: p.css })}
                className={`group relative h-16 overflow-hidden rounded-xl ring-offset-2 transition ${
                  wp.css === p.css ? 'ring-2 ring-blue-500' : 'ring-1 ring-slate-200 hover:ring-slate-400'
                }`}
                style={{ background: p.preview }}
              >
                <span className="absolute inset-x-0 bottom-0 bg-black/35 py-0.5 text-[10px] font-semibold text-white">
                  {p.name}
                </span>
              </button>
            ))}
          </div>
        )}

        {wp.mode === 'custom' && (
          <div className="mt-2 flex items-center gap-2">
            <label className="flex cursor-pointer items-center gap-1.5 rounded-lg bg-slate-100 px-3 py-1.5 text-[11px] font-semibold text-slate-600 transition hover:bg-slate-200">
              <Upload className="h-3 w-3" />
              {uploading ? '上传中…' : wp.custom ? '换一张' : '选择图片'}
              <input
                type="file"
                accept="image/*"
                className="hidden"
                onChange={(e) => {
                  const f = e.target.files?.[0]
                  if (f) void onUpload(f)
                }}
              />
            </label>
            {wp.custom && <span className="truncate text-[10px] text-slate-400">{wp.custom}</span>}
          </div>
        )}

        <label className="mt-3 block">
          <span className="mb-1 flex items-center justify-between text-[11px] text-slate-600">
            模糊程度
            <span className="tabular-nums text-slate-400">{wp.blur}px</span>
          </span>
          <input
            type="range"
            min={0}
            max={20}
            value={wp.blur}
            onChange={(e) => save({ blur: Number(e.target.value) })}
            className="w-full accent-blue-500"
          />
        </label>
      </section>

      <section>
        <h3 className="mb-2 text-xs font-semibold text-slate-700">点卡片时优先走哪个地址</h3>
        <div className="grid grid-cols-3 gap-1.5">
          {PREFERS.map((p) => (
            <button
              key={p.value}
              type="button"
              title={p.hint}
              onClick={() =>
                void mutate({
                  apply: (c) => ({ ...c, networkPrefer: p.value }),
                  request: () => updateNetworkPrefer(p.value),
                })
              }
              className={`rounded-xl border py-2 text-[11px] font-semibold transition ${
                cfg?.networkPrefer === p.value
                  ? 'border-blue-400 bg-blue-50 text-blue-700'
                  : 'border-slate-200 text-slate-600 hover:bg-slate-50'
              }`}
            >
              {p.label}
            </button>
          ))}
        </div>
        <p className="mt-1.5 text-[11px] leading-relaxed text-slate-400">
          想要的地址没配时，会自动退到其它可用的那个。右键菜单里永远能看到全部入口。
        </p>
      </section>
    </div>
  )
}
