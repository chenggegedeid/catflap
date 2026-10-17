import { useEffect, useState } from 'react'
import {
  AlertTriangle,
  Check,
  Download,
  FileUp,
  RotateCw,
  ShieldAlert,
  Upload,
} from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import {
  type BackupItem,
  type BackupManifest,
  exportBackup,
  getBackupModules,
  importBackup,
  previewBackup,
} from '../lib/api'

/**
 * 备份与恢复。
 *
 * 后端的导出/导入/预览本来就有（含快照与失败回滚），这里补齐的是界面。
 *
 * 刻意做成两步：**先预览再恢复**。导入是覆盖性操作，让人先看清包里有什么、
 * 再用勾选决定范围，比"传上去直接全覆盖"稳妥。
 */
export function Backup() {
  const [items, setItems] = useState<{ modules: BackupItem[]; assets: BackupItem[] } | null>(null)
  const [err, setErr] = useState('')
  const [toast, setToast] = useState('')

  // 导出时勾了哪些。空 = 全部（后端语义）
  const [picked, setPicked] = useState<Record<string, boolean>>({})
  const [exporting, setExporting] = useState(false)

  // 恢复：选中的文件 + 它的清单 + 要恢复哪些
  const [file, setFile] = useState<File | null>(null)
  const [manifest, setManifest] = useState<BackupManifest | null>(null)
  const [restorePicked, setRestorePicked] = useState<Record<string, boolean>>({})
  const [previewing, setPreviewing] = useState(false)
  const [restoring, setRestoring] = useState(false)

  const say = (m: string) => {
    setToast(m)
    window.setTimeout(() => setToast(''), 3200)
  }

  useEffect(() => {
    getBackupModules()
      .then((d) => {
        setItems(d)
        // 默认全勾上存在的项 —— 备份通常是"全都要"
        const init: Record<string, boolean> = {}
        d.modules.forEach((m) => (init[m.name] = m.exists))
        d.assets.forEach((a) => (init[a.name] = a.exists))
        setPicked(init)
      })
      .catch((e) => setErr(e instanceof Error ? e.message : '加载模块清单失败'))
  }, [])

  const toggle = (name: string) =>
    setPicked((p) => ({ ...p, [name]: !p[name] }))

  const selectedModules = () =>
    (items?.modules ?? []).filter((m) => picked[m.name]).map((m) => m.name)
  const selectedAssets = () =>
    (items?.assets ?? []).filter((a) => picked[a.name]).map((a) => a.name)

  const doExport = async () => {
    setExporting(true)
    try {
      const name = await exportBackup(selectedModules(), selectedAssets())
      say(`已导出 ${name}`)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '导出失败')
    } finally {
      setExporting(false)
    }
  }

  // 选了文件就立刻预览，让用户先看清再决定
  const onPickFile = async (f: File | null) => {
    setFile(f)
    setManifest(null)
    setErr('')
    if (!f) return
    setPreviewing(true)
    try {
      const mf = await previewBackup(f)
      setManifest(mf)
      const init: Record<string, boolean> = {}
      // 只勾包里**确实有内容**的模块，免得勾了恢复不出东西
      ;(mf.modules ?? []).forEach((m) => (init[m.name] = m.exists))
      setRestorePicked(init)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '读不了这个备份包')
    } finally {
      setPreviewing(false)
    }
  }

  const doRestore = async () => {
    if (!file) return
    // 覆盖性操作，说清楚再动手
    const mods = (manifest?.modules ?? [])
      .filter((m) => restorePicked[m.name])
      .map((m) => m.name)
    if (mods.length === 0) {
      setErr('至少勾一个模块')
      return
    }
    if (!window.confirm(`将用备份包覆盖 ${mods.length} 个模块的现有配置，确定继续？`)) return

    setRestoring(true)
    try {
      const r = await importBackup(file, mods)
      say(`已恢复 ${r.restoredModules.length} 个模块`)
      setFile(null)
      setManifest(null)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '恢复失败')
    } finally {
      setRestoring(false)
    }
  }

  if (err && !items) {
    return (
      <Card>
        <div className="text-sm text-rose-600">{err}</div>
      </Card>
    )
  }

  return (
    <div className="space-y-4">
      {/* 创建备份 */}
      <Card className="!p-0">
        <div className="px-5 pt-4">
          <CardHeader title="创建备份" />
        </div>
        <div className="px-5 pb-5">
          <div className="mb-3 flex flex-wrap gap-2">
            {(items?.modules ?? []).map((m) => (
              <button
                key={m.name}
                type="button"
                onClick={() => toggle(m.name)}
                disabled={!m.exists}
                title={m.exists ? undefined : '这个模块现在没有配置文件'}
                className={`flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-semibold transition disabled:opacity-40 ${
                  picked[m.name]
                    ? m.sensitive
                      ? 'bg-amber-100 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
                      : 'bg-blue-50 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
                    : 'bg-surface-2 text-muted hover:bg-fill'
                }`}
              >
                {picked[m.name] && <Check className="h-3 w-3" />}
                {m.label}
                {m.sensitive && <ShieldAlert className="h-3 w-3" />}
              </button>
            ))}
          </div>

          {(items?.assets ?? []).length > 0 && (
            <div className="mb-3">
              <div className="mb-1.5 text-xs font-semibold text-muted">上传的资产</div>
              <div className="flex flex-wrap gap-2">
                {(items?.assets ?? []).map((a) => (
                  <button
                    key={a.name}
                    type="button"
                    onClick={() => toggle(a.name)}
                    disabled={!a.exists}
                    className={`flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-semibold transition disabled:opacity-40 ${
                      picked[a.name]
                        ? 'bg-blue-50 text-blue-600 dark:bg-blue-900/40 dark:text-blue-300'
                        : 'bg-surface-2 text-muted hover:bg-fill'
                    }`}
                  >
                    {picked[a.name] && <Check className="h-3 w-3" />}
                    {a.label}
                  </button>
                ))}
              </div>
            </div>
          )}

          <div className="mb-3 flex items-start gap-2 rounded-lg bg-amber-50 px-3 py-2 text-[11px] leading-relaxed text-amber-700 ring-1 ring-amber-200 dark:bg-amber-900/20 dark:text-amber-300 dark:ring-amber-800">
            <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <span>
              带盾牌图标的模块含凭据（服务商 token、密钥、口令哈希），备份包里是**明文**。
              妥善保管，别随手发给别人。
            </span>
          </div>

          <button
            type="button"
            onClick={() => void doExport()}
            disabled={exporting || selectedModules().length === 0}
            className="flex items-center gap-1.5 rounded-lg bg-blue-500 px-3 py-2 text-xs font-semibold text-white transition hover:bg-blue-600 disabled:opacity-50"
          >
            {exporting ? (
              <RotateCw className="h-3.5 w-3.5 animate-spin" />
            ) : (
              <Download className="h-3.5 w-3.5" />
            )}
            {exporting ? '打包中…' : '下载备份包'}
          </button>
        </div>
      </Card>

      {/* 恢复 */}
      <Card className="!p-0">
        <div className="px-5 pt-4">
          <CardHeader title="恢复" />
        </div>
        <div className="px-5 pb-5">
          <label className="mb-3 flex cursor-pointer items-center gap-2 rounded-lg border border-dashed border-line px-3 py-3 text-xs text-muted transition hover:bg-surface-2">
            <FileUp className="h-4 w-4 shrink-0" />
            <span className="min-w-0 flex-1 truncate">
              {file ? file.name : '选择一个备份包（.zip）'}
            </span>
            <input
              type="file"
              accept=".zip"
              className="hidden"
              onChange={(e) => void onPickFile(e.target.files?.[0] ?? null)}
            />
          </label>

          {previewing && <div className="mb-3 text-xs text-muted">正在读取备份包…</div>}

          {manifest && (
            <>
              <div className="mb-3 rounded-lg bg-surface-2 px-3 py-2 text-[11px] leading-relaxed">
                <div>
                  打这个包的是 <span className="font-semibold">v{manifest.appVersion ?? '?'}</span>
                  {manifest.createdAt && (
                    <span className="text-muted">
                      {' '}
                      · {new Date(manifest.createdAt).toLocaleString('zh-CN')}
                    </span>
                  )}
                </div>
                {manifest.sensitive && (
                  <div className="mt-1 text-amber-600 dark:text-amber-400">
                    包内含凭据模块，恢复后会覆盖现有的对应配置
                  </div>
                )}
              </div>

              <div className="mb-3">
                <div className="mb-1.5 text-xs font-semibold text-muted">
                  要恢复哪些模块（被覆盖的是现有配置）
                </div>
                <div className="flex flex-wrap gap-2">
                  {(manifest.modules ?? []).map((m) => (
                    <button
                      key={m.name}
                      type="button"
                      onClick={() =>
                        setRestorePicked((p) => ({ ...p, [m.name]: !p[m.name] }))
                      }
                      disabled={!m.exists}
                      title={m.exists ? undefined : '包里没有这个模块的内容'}
                      className={`flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-semibold transition disabled:opacity-40 ${
                        restorePicked[m.name]
                          ? 'bg-rose-50 text-rose-600 dark:bg-rose-900/40 dark:text-rose-300'
                          : 'bg-surface-2 text-muted hover:bg-fill'
                      }`}
                    >
                      {restorePicked[m.name] && <Check className="h-3 w-3" />}
                      {m.label || m.name}
                    </button>
                  ))}
                </div>
              </div>

              <div className="mb-3 rounded-lg bg-surface-2 px-3 py-2 text-[11px] leading-relaxed text-muted">
                恢复前会自动给现有配置打快照，中途失败会自动回滚。
              </div>

              <button
                type="button"
                onClick={() => void doRestore()}
                disabled={restoring}
                className="flex items-center gap-1.5 rounded-lg bg-rose-500 px-3 py-2 text-xs font-semibold text-white transition hover:bg-rose-600 disabled:opacity-50"
              >
                {restoring ? (
                  <RotateCw className="h-3.5 w-3.5 animate-spin" />
                ) : (
                  <Upload className="h-3.5 w-3.5" />
                )}
                {restoring ? '恢复中…' : '确认恢复'}
              </button>
            </>
          )}
        </div>
      </Card>

      {err && (
        <div className="rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-700 ring-1 ring-rose-200 dark:bg-rose-900/20 dark:text-rose-300 dark:ring-rose-800">
          {err}
        </div>
      )}

      {toast && (
        <div className="fixed bottom-6 left-1/2 z-50 -translate-x-1/2 rounded-xl bg-inverse px-4 py-2 text-xs font-medium text-on-inverse shadow-lg">
          {toast}
        </div>
      )}
    </div>
  )
}
