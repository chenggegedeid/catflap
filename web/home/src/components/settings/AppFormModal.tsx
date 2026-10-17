/**
 * 应用表单。
 *
 * 一个弹窗管两件事：新增、编辑。字段完全一致，没必要拆成两个组件 ——
 * 拆了就会慢慢长出"编辑页有但新增页没有"的差异，那种差异通常只是忘了同步。
 *
 * 图标三种来源，优先级从上到下：
 *   1. 上传本地图片
 *   2. 从站点首页抓取 favicon（后端去抓，绕过浏览器跨域限制）
 *   3. 手填路径（给已经知道文件在哪的人留个口子）
 * 三个入口是并列的、不是互斥的 —— 用户试了抓取失败，还能直接上传，
 * 不用先关掉一个对话框再打开另一个。
 */

import { useRef, useState } from 'react'
import { Check, Upload, X } from 'lucide-react'
import { addApp, fetchIconFromURL, updateApp, uploadIcon } from '../../api/home'
import { DEFAULT_PRESETS } from '../../lib/icons'
import { AppIcon } from '../card/AppIcon'
import type { AppView, Category } from '../../types'

export function AppFormModal({
  editing,
  categories,
  onClose,
  onSaved,
  onNotify,
}: {
  /** 空 = 新增 */
  editing: AppView | null
  categories: Category[]
  onClose: () => void
  onSaved: () => void
  onNotify: (kind: 'ok' | 'error', text: string) => void
}) {
  const [name, setName] = useState(editing?.name ?? '')
  const [icon, setIcon] = useState(editing?.icon ?? '')
  const [color, setColor] = useState(editing?.color ?? DEFAULT_PRESETS[0].color)
  const [categoryId, setCategoryId] = useState(editing?.categoryId ?? categories[0]?.id ?? '')
  const [wanV4, setWanV4] = useState(editing?.addresses.wanV4 ?? '')
  const [wanV6, setWanV6] = useState(editing?.addresses.wanV6 ?? '')
  const [lan, setLan] = useState(editing?.addresses.lan ?? '')

  const [busy, setBusy] = useState(false)
  const [fetchUrl, setFetchUrl] = useState('')
  const fileRef = useRef<HTMLInputElement>(null)

  const submit = async () => {
    const n = name.trim()
    if (!n) {
      onNotify('error', '名称不能为空')
      return
    }
    setBusy(true)
    try {
      const payload = {
        name: n,
        icon: icon.trim(),
        color,
        categoryId,
        addresses: {
          wanV4: wanV4.trim(),
          wanV6: wanV6.trim(),
          lan: lan.trim(),
        },
      }
      if (editing) await updateApp(editing.id, payload)
      else await addApp(payload)
      onNotify('ok', editing ? '已保存' : '已添加')
      onSaved()
      onClose()
    } catch (e) {
      onNotify('error', e instanceof Error ? e.message : '保存失败')
    } finally {
      setBusy(false)
    }
  }

  const onUpload = async (file: File) => {
    setBusy(true)
    try {
      const { data } = await uploadIcon(file)
      if (data?.path) setIcon(data.path)
    } catch (e) {
      onNotify('error', e instanceof Error ? e.message : '上传失败')
    } finally {
      setBusy(false)
      if (fileRef.current) fileRef.current.value = ''
    }
  }

  /**
   * 抓取的地址来源：**优先用已填的地址**（公网 v4 / 内网 / v6），
   * 都没填才让用户手输。
   *
   * 之前是必须再输入一遍站点地址 —— 地址明明就填在下面，让人重打一遍
   * 纯属多余。多地址时依次试，第一个成功就用。
   */
  const fetchSources = [wanV4.trim(), lan.trim(), wanV6.trim()].filter(Boolean)
  const canAutoFetch = fetchSources.length > 0

  const tryFetch = async (urls: string[]): Promise<boolean> => {
    for (const u of urls) {
      try {
        const { data } = await fetchIconFromURL(u)
        if (data?.path) {
          setIcon(data.path)
          return true
        }
      } catch {
        // 试下一个来源
      }
    }
    return false
  }

  const onFetch = async () => {
    const sources = canAutoFetch ? fetchSources : fetchUrl.trim() ? [fetchUrl.trim()] : []
    if (sources.length === 0) {
      onNotify('error', '先在下面填一个地址，或者手动上传图标')
      return
    }
    setBusy(true)
    try {
      const ok = await tryFetch(sources)
      if (!ok) onNotify('error', '没抓到图标，可以手动上传')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 grid grid-cols-1 place-items-center bg-black/45 px-4 py-6 backdrop-blur-sm"
      onMouseDown={onClose}
      onKeyDown={(e) => {
        if (e.key === 'Escape') onClose()
      }}
    >
      <div
        role="dialog"
        aria-modal
        aria-label={editing ? `编辑应用「${editing.name}」` : '新增应用'}
        onMouseDown={(e) => e.stopPropagation()}
        className="flex max-h-full w-full max-w-md flex-col overflow-hidden rounded-2xl bg-white shadow-2xl"
      >
        <div className="flex items-center justify-between border-b border-slate-100 px-5 py-3.5">
          <h2 className="text-sm font-semibold text-slate-800">{editing ? '编辑应用' : '新增应用'}</h2>
          <button
            type="button"
            onClick={onClose}
            aria-label="关闭"
            className="rounded-lg p-1 text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto px-5 py-4">
          <label className="block">
            <span className={labelCls}>名称</span>
            <input
              autoFocus
              value={name}
              onChange={(e) => setName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') void submit()
              }}
              placeholder="比如「家庭相册」"
              className={fieldCls}
            />
          </label>

          <label className="block">
            <span className={labelCls}>分组</span>
            <select value={categoryId} onChange={(e) => setCategoryId(e.target.value)} className={fieldCls}>
              {categories.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
          </label>

          <div>
            <span className={labelCls}>颜色</span>
            <div className="flex flex-wrap gap-1.5">
              {DEFAULT_PRESETS.map((p) => (
                <button
                  key={p.id}
                  type="button"
                  onClick={() => setColor(p.color)}
                  title={p.name}
                  aria-label={p.name}
                  aria-pressed={color === p.color}
                  className={`h-7 w-7 rounded-lg bg-gradient-to-br ${p.color} transition ${
                    color === p.color
                      ? 'ring-2 ring-slate-900 ring-offset-2'
                      : 'opacity-70 hover:opacity-100'
                  }`}
                >
                  {color === p.color && <Check className="h-3.5 w-3.5 mx-auto text-white" />}
                </button>
              ))}
            </div>
          </div>

          {/* 图标：三种来源并列，不互斥 */}
          <div>
            <span className={labelCls}>图标</span>
            <div className="flex items-center gap-3">
              <span className="h-12 w-12 shrink-0 overflow-hidden rounded-[22%] shadow-sm ring-1 ring-black/5">
                <AppIcon name={name || '?'} icon={icon} />
              </span>
              <div className="flex flex-wrap gap-1.5">
                <button
                  type="button"
                  onClick={() => fileRef.current?.click()}
                  disabled={busy}
                  className="flex items-center gap-1 rounded-lg bg-slate-100 px-2.5 py-1.5 text-[11px] font-semibold text-slate-600 transition hover:bg-slate-200 disabled:opacity-50"
                >
                  <Upload className="h-3 w-3" />
                  上传
                </button>
                <button
                  type="button"
                  onClick={() => void onFetch()}
                  disabled={busy || (!canAutoFetch && !fetchUrl.trim())}
                  title={canAutoFetch ? `从 ${fetchSources[0]} 抓取` : '先在下面填地址，或手动上传'}
                  className="rounded-lg bg-slate-100 px-2.5 py-1.5 text-[11px] font-semibold text-slate-600 transition hover:bg-slate-200 disabled:opacity-40"
                >
                  抓取
                </button>
              </div>
            </div>

            <input
              ref={fileRef}
              type="file"
              accept="image/*"
              className="hidden"
              onChange={(e) => {
                const f = e.target.files?.[0]
                if (f) void onUpload(f)
              }}
            />

            {/*
              手输地址只在**没有任何已填地址**时出现 —— 有地址还让人重打一遍
              纯属多余（用户实测反馈：抓取应该用地址栏里的地址）。
            */}
            {!canAutoFetch && (
              <div className="mt-1.5 flex gap-1.5">
                <input
                  value={fetchUrl}
                  onChange={(e) => setFetchUrl(e.target.value)}
                  placeholder="站点首页地址，用来抓 favicon"
                  className={`${fieldCls} text-xs`}
                />
              </div>
            )}

            <p className="mt-1.5 text-[11px] text-slate-400">
              {canAutoFetch
                ? '会从上面填的地址抓取图标'
                : '留空就用名字首字当图标，不会白板。'}
            </p>
          </div>

          <fieldset>
            <legend className={labelCls}>地址</legend>
            <p className="mb-2 text-[11px] leading-relaxed text-slate-400">
              只在需要的时候填。访客看不到内网地址。
            </p>
            <div className="space-y-1.5">
              <AddressRow label="公网 v4" value={wanV4} onChange={setWanV4} placeholder="https://xxx.example.com" />
              <AddressRow label="公网 v6" value={wanV6} onChange={setWanV6} placeholder="https://[2001:db8::1]:8080" />
              <AddressRow label="内网" value={lan} onChange={setLan} placeholder="http://192.168.1.10:8080" />
            </div>
          </fieldset>
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-slate-100 px-5 py-3">
          <button
            type="button"
            onClick={onClose}
            className="rounded-xl px-3 py-2 text-xs font-semibold text-slate-500 transition hover:bg-slate-100"
          >
            取消
          </button>
          <button
            type="button"
            onClick={() => void submit()}
            disabled={busy || !name.trim()}
            className="rounded-xl bg-blue-500 px-4 py-2 text-xs font-bold text-white transition hover:bg-blue-600 disabled:opacity-50"
          >
            {busy ? '保存中…' : '保存'}
          </button>
        </div>
      </div>
    </div>
  )
}

const labelCls = 'mb-1 block text-[11px] font-semibold text-slate-500'
const fieldCls =
  'w-full rounded-xl border border-slate-200 px-3 py-2 text-sm text-slate-800 transition focus:border-blue-400 focus:outline-none'

function AddressRow({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string
  value: string
  onChange: (v: string) => void
  placeholder: string
}) {
  return (
    <div className="flex items-center gap-2">
      <span className="w-16 shrink-0 text-[11px] text-slate-500">{label}</span>
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="min-w-0 flex-1 rounded-lg border border-slate-200 px-2 py-1 font-mono text-[11px] transition focus:border-blue-400 focus:outline-none"
      />
    </div>
  )
}
