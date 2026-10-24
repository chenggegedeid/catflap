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
import { ScheduleBackup } from '../components/ScheduleBackup'
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
  // 恢复成功但尚未重启的模块数。> 0 时顶部要挂醒目提示 ——
  // 见 doRestore 里的说明：不重启界面就看不到恢复结果。
  const [restoredCount, setRestoredCount] = useState(0)

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
      // 恢复只写磁盘：运行中的进程仍用内存里的旧配置，不重启界面上看不出变化，
      // 会被当成"恢复坏了"。所以必须把这一步讲清楚。
      setRestoredCount(r.restoredModules.length)
      say(`已把 ${r.restoredModules.length} 个模块的配置写入磁盘`)
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
      {/*
        恢复成功但还没重启的醒目提示。
        为什么要挂在页面顶部而不是只弹个 toast：
        恢复只写磁盘，运行中的进程仍用内存里的旧配置 ——
        界面上看着毫无变化，用户会以为"恢复坏了"。
        不重启的话，之后在界面上改任何配置，模块还会把内存里的旧状态
        写回磁盘，把刚恢复的内容盖掉。所以必须说清楚、并且要看得见。
      */}
      {restoredCount > 0 && (
        <div className="rounded-xl bg-amber-50 px-4 py-3 text-xs leading-relaxed text-amber-800 dark:bg-amber-900/30 dark:text-amber-200">
          <div className="font-semibold">
            已把 {restoredCount} 个模块的配置写入磁盘，需要重启 Catflap 才能生效
          </div>
          <div className="mt-1">
            在命令行执行：<code className="rounded bg-black/10 px-1 font-mono dark:bg-white/10">sudo systemctl restart catflap</code>
          </div>
          <div className="mt-1">
            <strong>重启前请不要在本界面修改任何配置</strong>，否则运行中的旧配置会写回磁盘，把刚恢复的内容覆盖掉。
          </div>
          <button
            type="button"
            onClick={() => setRestoredCount(0)}
            className="mt-2 text-[11px] font-semibold underline underline-offset-2"
          >
            已重启，知道了
          </button>
        </div>
      )}

      {/* 定时备份放在最上面：多数人真正需要的是它自己会存，手动导出是补充 */}
      <ScheduleBackup />

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

          {/* 资产（图标/壁纸）已随导航页废弃，后端返回空数组时这一块不渲染 */}
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
                  {(manifest.modules ?? []).map((m) => {
                    // 空配置识别：一个模块的配置文件如果只有一两百字节，
                    // 基本就是 {} 或 {"records":null} 这种空壳。
                    //
                    // 为什么必须标出来：恢复是**覆盖式**的，勾了一个空模块
                    // 就等于用空配置把现有配置盖掉 —— 用户看到的是
                    // "恢复成功了，但我的 DDNS/反代没了"，最难排查的那种。
                    // 宁可在勾选前就警告，也别让人恢复完才发现。
                    const empty = m.exists && m.size > 0 && m.size < 300
                    const picked = restorePicked[m.name]
                    return (
                      <button
                        key={m.name}
                        type="button"
                        onClick={() =>
                          setRestorePicked((p) => ({ ...p, [m.name]: !p[m.name] }))
                        }
                        disabled={!m.exists}
                        title={
                          !m.exists
                            ? '包里没有这个模块的内容'
                            : empty
                              ? `包里这个模块只有 ${m.size} 字节，很可能是空配置——勾选会把现有的覆盖成空的`
                              : `包内 ${m.size} 字节`
                        }
                        className={`flex items-center gap-1.5 rounded-lg px-2.5 py-1.5 text-xs font-semibold transition disabled:opacity-40 ${
                          picked
                            ? empty
                              ? 'bg-amber-50 text-amber-700 dark:bg-amber-900/40 dark:text-amber-300'
                              : 'bg-rose-50 text-rose-600 dark:bg-rose-900/40 dark:text-rose-300'
                            : empty
                              ? 'bg-amber-50/60 text-amber-600 dark:bg-amber-900/20 dark:text-amber-400'
                              : 'bg-surface-2 text-muted hover:bg-fill'
                        }`}
                      >
                        {picked && <Check className="h-3 w-3" />}
                        {m.label || m.name}
                        {empty && <span className="text-[10px] font-normal">（可能是空的）</span>}
                      </button>
                    )
                  })}
                </div>
              </div>

              <div className="mb-3 rounded-lg bg-surface-2 px-3 py-2 text-[11px] leading-relaxed text-muted">
                恢复前会自动给现有配置打快照，中途失败会自动回滚。
                <br />
                注意：证书分两处——<strong>配置在 config/，证书实体文件在 data/cert/</strong>。
                手动搬配置时只拷 config 的话，程序找不到证书会自己签一张顶上。
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
