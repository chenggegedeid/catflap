import { useCallback, useEffect, useState } from 'react'
import { CalendarClock, Play, Save } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import {
  type AutoBackupFile,
  type BackupSchedule,
  getBackupSchedule,
  listAutoBackups,
  runBackupNow,
  saveBackupSchedule,
} from '../lib/api'

/**
 * 定时备份卡片。
 *
 * 备份**存本机**的 `data/backup/`，不做自动上传 ——
 * 上传要么让用户配对象存储密钥（备份功能本身反而变重），
 * 要么发到某个不该假设的地方。存本机后想同步到哪儿，
 * 用户自己用 syncthing / rclone 都行，界面上把目录说清楚即可。
 */
export function ScheduleBackup() {
  const [cfg, setCfg] = useState<BackupSchedule | null>(null)
  const [files, setFiles] = useState<AutoBackupFile[]>([])
  const [busy, setBusy] = useState('')
  const [err, setErr] = useState('')
  const [done, setDone] = useState('')

  const load = useCallback(async () => {
    try {
      const [c, f] = await Promise.all([getBackupSchedule(), listAutoBackups()])
      setCfg(c)
      setFiles(f)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '读取定时备份设置失败')
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const save = async () => {
    if (!cfg) return
    setBusy('save')
    setErr('')
    setDone('')
    try {
      const { msg } = await saveBackupSchedule(cfg)
      setDone(msg || '已保存')
      await load()
    } catch (e) {
      setErr(e instanceof Error ? e.message : '保存失败')
    } finally {
      setBusy('')
    }
  }

  const runNow = async () => {
    setBusy('run')
    setErr('')
    setDone('')
    try {
      const { msg } = await runBackupNow()
      setDone(msg || '已生成一份备份')
      await load()
    } catch (e) {
      setErr(e instanceof Error ? e.message : '执行失败')
    } finally {
      setBusy('')
    }
  }

  if (!cfg) {
    return (
      <Card>
        <CardHeader title="定时备份" />
        <div className="py-4 text-center text-xs text-muted">{err || '正在加载…'}</div>
      </Card>
    )
  }

  return (
    <Card className="!p-0">
      <div className="px-5 pt-4">
        <CardHeader
          title="定时备份"
          action={
            <span className="text-[11px] text-muted">存到本机 data/backup/</span>
          }
        />
      </div>
      <div className="px-5 pb-5">
        {/* 开关 */}
        <label className="mb-4 flex cursor-pointer items-start gap-2.5">
          <input
            type="checkbox"
            checked={cfg.enabled}
            onChange={(e) => setCfg({ ...cfg, enabled: e.target.checked })}
            className="mt-0.5 accent-blue-500"
          />
          <span>
            <span className="text-sm font-semibold text-content">开启定时备份</span>
            <span className="mt-0.5 block text-[11px] leading-relaxed text-muted">
              按下面的间隔自动打包一份配置。默认每天一次、留最近 7 份。
            </span>
          </span>
        </label>

        {cfg.enabled && (
          <div className="mb-4 grid gap-3 sm:grid-cols-2">
            <label className="block">
              <div className={labelCls}>间隔（小时）</div>
              <input
                type="number"
                min={1}
                max={720}
                value={cfg.intervalHours}
                onChange={(e) => setCfg({ ...cfg, intervalHours: Number(e.target.value) })}
                className={fieldCls}
              />
              <div className="mt-1 text-[11px] text-muted">24 = 每天一次</div>
            </label>
            <label className="block">
              <div className={labelCls}>保留最近几份</div>
              <input
                type="number"
                min={1}
                max={100}
                value={cfg.keepCount}
                onChange={(e) => setCfg({ ...cfg, keepCount: Number(e.target.value) })}
                className={fieldCls}
              />
              <div className="mt-1 text-[11px] text-muted">超出的自动删掉</div>
            </label>
          </div>
        )}

        {/* 上次执行 */}
        {cfg.lastRunAt && (
          <div className="mb-4 flex flex-wrap items-center gap-1.5 rounded-lg bg-surface-2 px-3 py-2 text-[11px] text-muted">
            <CalendarClock className="h-3.5 w-3.5 shrink-0" />
            上次：{new Date(cfg.lastRunAt).toLocaleString('zh-CN')}
            {cfg.lastResult && <span className="text-content">· {cfg.lastResult}</span>}
          </div>
        )}

        {/* 已生成的文件 */}
        {files.length > 0 && (
          <div className="mb-4">
            <div className={labelCls}>已生成 {files.length} 份</div>
            <div className="max-h-40 space-y-1 overflow-y-auto">
              {files.map((f) => (
                <div
                  key={f.name}
                  className="flex flex-wrap items-center gap-x-2 gap-y-0.5 rounded-lg bg-surface-2 px-2.5 py-1.5 text-[11px]"
                >
                  <span className="min-w-0 flex-1 break-all font-mono text-content">{f.name}</span>
                  <span className="shrink-0 text-muted">{formatSize(f.size)}</span>
                </div>
              ))}
            </div>
          </div>
        )}

        {err && (
          <div className="mb-3 rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>
        )}
        {done && (
          <div className="mb-3 rounded-lg bg-emerald-50 px-3 py-2 text-xs text-emerald-600">
            {done}
          </div>
        )}

        <div className="flex flex-wrap items-center gap-2">
          <button
            type="button"
            onClick={() => void save()}
            disabled={busy !== ''}
            className="flex items-center gap-1.5 rounded-lg bg-blue-500 px-3 py-2 text-xs font-semibold text-white transition hover:bg-blue-600 disabled:opacity-50"
          >
            <Save className="h-3.5 w-3.5" />
            {busy === 'save' ? '保存中…' : '保存设置'}
          </button>
          <button
            type="button"
            onClick={() => void runNow()}
            disabled={busy !== ''}
            className="flex items-center gap-1.5 rounded-lg px-3 py-2 text-xs font-semibold text-content ring-1 ring-line transition hover:bg-surface-2 disabled:opacity-50"
          >
            <Play className="h-3.5 w-3.5" />
            {busy === 'run' ? '生成中…' : '立即备份一次'}
          </button>
          <span className="text-[11px] text-muted">
            改完设置立即生效，不用重启
          </span>
        </div>
      </div>
    </Card>
  )
}

const labelCls = 'mb-1 text-xs font-semibold text-muted'
const fieldCls =
  'w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none transition focus:border-blue-400'

function formatSize(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(1)} MB`
}
