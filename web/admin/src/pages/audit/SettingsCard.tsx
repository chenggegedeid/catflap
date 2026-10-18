import { useEffect, useState } from 'react'
import { Check } from 'lucide-react'
import { Card, CardHeader } from '../../components/Card'
import * as api from '../../lib/api'
import type { AuditConfigView } from '../../types'

export function SettingsCard({
  cfg,
  onSaved,
  onToast,
}: {
  cfg: AuditConfigView | null
  onSaved: () => void
  onToast: (m: string) => void
}) {
  const [enabled, setEnabled] = useState(cfg?.enabled ?? true)
  const [keepDays, setKeepDays] = useState(String(cfg?.keepDays ?? 30))
  const [maxFileMB, setMaxFileMB] = useState(String(cfg?.maxFileMB ?? 32))
  const [autoBlock, setAutoBlock] = useState(cfg?.autoBlock ?? false)
  const [threshold, setThreshold] = useState(String(cfg?.autoBlockThreshold ?? 500))
  const [minutes, setMinutes] = useState(String(cfg?.autoBlockMinutes ?? 60))
  const [busy, setBusy] = useState(false)

  // cfg 从后端回来时同步进表单（首屏是 null，拿到后要填上）
  useEffect(() => {
    if (!cfg) return
    setEnabled(cfg.enabled)
    setKeepDays(String(cfg.keepDays))
    setMaxFileMB(String(cfg.maxFileMB))
    setAutoBlock(cfg.autoBlock)
    setThreshold(String(cfg.autoBlockThreshold))
    setMinutes(String(cfg.autoBlockMinutes))
  }, [cfg])

  const save = async () => {
    setBusy(true)
    try {
      const res = await api.saveAuditSettings({
        enabled,
        keepDays: Number(keepDays) || 30,
        maxFileMB: Number(maxFileMB) || 32,
        autoBlock,
        autoBlockThreshold: Number(threshold) || 500,
        autoBlockMinutes: Number(minutes) || 60,
        // 白名单已移到下面的「IP 名单」区块，这里原样带回，别把它清空
        whitelist: cfg?.whitelist ?? [],
        trustedProxies: cfg?.trustedProxies ?? [],
      })
      onToast(res.msg || '已保存')
      onSaved()
    } catch (e) {
      onToast('保存失败：' + (e instanceof Error ? e.message : ''))
    } finally {
      setBusy(false)
    }
  }

  const cls = 'w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400'

  return (
    <Card className="!p-0">
      <div className="px-5 pt-4">
        <CardHeader title="审计设置" />
      </div>
      <div className="space-y-3 px-5 pb-5">
        {/*
          审计总开关。

          为什么把它从「一个不起眼的小复选框」提成开关、放在最上面：
          它不是细节选项，而是**整个访问审计功能的开关** —— 关掉之后不再记录
          任何访问，统计、趋势、IP 列表、攻击类型全都停在原地。
          用户要一眼找到它，也要一眼看出它的分量。
        */}
        <div
          className={`flex items-start gap-3 rounded-xl px-3 py-3 ring-1 transition ${
            enabled ? 'bg-emerald-50/60 ring-emerald-200/70' : 'bg-surface-2 ring-line'
          }`}
        >
          <button
            type="button"
            role="switch"
            aria-checked={enabled}
            aria-label="记录访问（访问审计总开关）"
            onClick={() => setEnabled((v) => !v)}
            className={`mt-0.5 flex h-6 w-11 shrink-0 items-center rounded-full px-0.5 transition ${
              enabled ? 'bg-emerald-500' : 'bg-fill'
            }`}
          >
            <span
              className={`h-5 w-5 rounded-full bg-surface shadow transition-transform ${
                enabled ? 'translate-x-5' : 'translate-x-0'
              }`}
            />
          </button>
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-sm font-semibold text-content">记录访问</span>
              <span
                className={`rounded-md px-1.5 py-0.5 text-[10px] font-semibold ${
                  enabled ? 'bg-emerald-100 text-emerald-700' : 'bg-fill text-content'
                }`}
              >
                {enabled ? '审计已开启' : '审计已关闭'}
              </span>
            </div>
            <div className="mt-1 text-[11px] leading-relaxed text-muted">
              {enabled
                ? '正在记录每一次访问：统计、趋势、IP 列表都基于它。关掉之后不再产生新记录。'
                : '已停止记录新访问。已有的记录仍然保留、仍然可以查看和封禁，但统计不再更新。'}
              <br />
              <span className="text-muted">
                关掉它等于关掉整个访问审计功能 —— 这是这一页的总开关。
              </span>
            </div>
          </div>
        </div>

        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-muted">明细保留（天）</div>
            <input value={keepDays} onChange={(e) => setKeepDays(e.target.value)} className={cls} />
          </label>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-muted">单天文件上限（MB）</div>
            <input value={maxFileMB} onChange={(e) => setMaxFileMB(e.target.value)} className={cls} />
            <div className="mt-1 text-[11px] text-muted">超了停写，防磁盘被刷满</div>
          </label>
        </div>

        <div className="rounded-xl bg-surface-2 p-3">
          <label className="flex cursor-pointer items-center gap-2 text-xs text-content">
            <input type="checkbox" checked={autoBlock} onChange={(e) => setAutoBlock(e.target.checked)} />
            <span className="font-semibold">自动拉黑可疑 IP</span>
          </label>
          <div className="mt-1 text-[11px] leading-relaxed text-amber-600">
            容易误伤：你自己调试时刷出几百个 404 也会被算成可疑。开启前请把常用地址
            填进白名单（白名单里的地址永不封禁）。
          </div>
          {autoBlock && (
            <div className="mt-2 grid grid-cols-1 sm:grid-cols-2 gap-3">
              <label className="block">
                <div className="mb-1 text-xs font-semibold text-muted">异常数门槛</div>
                <input value={threshold} onChange={(e) => setThreshold(e.target.value)} className={cls} />
              </label>
              <label className="block">
                <div className="mb-1 text-xs font-semibold text-muted">封禁时长（分钟，0=永久）</div>
                <input value={minutes} onChange={(e) => setMinutes(e.target.value)} className={cls} />
              </label>
            </div>
          )}
        </div>

        <div className="flex justify-end">
          <button
            onClick={save}
            disabled={busy}
            className="flex items-center gap-1 rounded-xl bg-blue-500 px-4 py-2 text-xs font-semibold text-white hover:bg-blue-600 disabled:opacity-50"
          >
            <Check className="h-3 w-3" /> {busy ? '保存中…' : '保存设置'}
          </button>
        </div>
      </div>
    </Card>
  )
}

/** 拉黑弹窗：选时长 + 填原因 */
