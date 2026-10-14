import { useEffect, useState } from 'react'
import { Clock, Plus, Play, Trash2 } from 'lucide-react'

import { Card } from '../components/Card'
import { modalBackdrop } from '../components/modal'
import * as api from '../lib/api'

/**
 * 计划任务。
 *
 * 页面上刻意把「手动跑一次」放在显眼位置：定时任务的排查难点是
 * 「不知道它到底会不会跑」，有个立刻执行的按钮，用户就不用干等到点。
 * 手动和定时走的是后端同一条路径（task.RunNow），所以不存在
 * 「手动能跑、定时没反应」这种两套逻辑不一致的情况。
 */
export function Task() {
  const [tasks, setTasks] = useState<api.TaskItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  const load = async () => {
    setLoading(true)
    setError('')
    try {
      const res = await api.getTasks()
      setTasks(res.tasks ?? [])
    } catch (e) {
      setError(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const run = async (t: api.TaskItem) => {
    setNotice('')
    setError('')
    try {
      const r = await api.runTask(t.id)
      const res = r.data
      if (res.ok) {
        setNotice(`「${t.name}」已执行完成（${res.lastRun ?? ''}）`)
      } else {
        setError(`「${t.name}」执行失败：${res.error}`)
      }
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : '执行失败')
    }
  }

  const remove = async (t: api.TaskItem) => {
    if (!confirm(`确定删除任务「${t.name}」？`)) return
    try {
      await api.deleteTask(t.id)
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : '删除失败')
    }
  }

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-lg font-bold text-slate-800">计划任务</h2>
        <p className="mt-1 text-xs text-slate-500">
          定时做一件事：同步 DDNS 解析记录，或唤醒一台机器。调度精度是 30 秒。
        </p>
      </div>

      {error && (
        <div className="rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">
          {error}
        </div>
      )}
      {notice && (
        <div className="rounded-md border border-blue-200 bg-blue-50 px-3 py-2 text-xs text-blue-700">
          {notice}
        </div>
      )}

      <Card>
        {loading ? (
          <div className="px-4 py-8 text-center text-xs text-slate-400">加载中…</div>
        ) : tasks.length === 0 ? (
          <div className="px-4 py-8 text-center text-xs text-slate-400">
            还没有计划任务。点下面「新建任务」添加一个。
          </div>
        ) : (
          <table className="w-full text-sm">
            <thead>
              <tr className="border-b border-slate-100 text-left text-xs text-slate-500">
                <th className="px-4 py-2 font-semibold">名称</th>
                <th className="px-4 py-2 font-semibold">动作</th>
                <th className="px-4 py-2 font-semibold">周期</th>
                <th className="px-4 py-2 font-semibold">上次执行</th>
                <th className="px-4 py-2 text-right font-semibold">操作</th>
              </tr>
            </thead>
            <tbody>
              {tasks.map((t) => (
                <tr key={t.id} className="border-b border-slate-50 last:border-0">
                  <td className="px-4 py-2.5">
                    <div className="flex items-center gap-2">
                      <span className="font-medium text-slate-700">{t.name}</span>
                      {!t.enabled && (
                        <span className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-500">
                          已停用
                        </span>
                      )}
                    </div>
                    {t.lastError && (
                      <div className="mt-0.5 text-xs text-rose-600">{t.lastError}</div>
                    )}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-slate-600">
                    {t.action === 'ddns' ? '同步 DDNS' : t.action === 'wol' ? '唤醒机器' : t.action}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-slate-600">
                    {t.kind === 'interval'
                      ? `每 ${t.intervalMin} 分钟`
                      : `每天 ${String(t.hour).padStart(2, '0')}:${String(t.minute).padStart(2, '0')}`}
                    {t.nextRun && (
                      <div className="mt-0.5 text-slate-400">下次 {t.nextRun}</div>
                    )}
                  </td>
                  <td className="px-4 py-2.5 text-xs text-slate-600">
                    {t.lastRun ? (
                      <>
                        <div>{t.lastRun}</div>
                        <div className={t.lastResult === '成功' ? 'text-emerald-600' : 'text-rose-600'}>
                          {t.lastResult}
                        </div>
                      </>
                    ) : (
                      <span className="text-slate-400">未执行</span>
                    )}
                  </td>
                  <td className="px-4 py-2.5">
                    <div className="flex items-center justify-end gap-2">
                      <button
                        type="button"
                        onClick={() => void run(t)}
                        className="flex items-center gap-1 rounded-md border border-slate-200 px-2.5 py-1.5 text-xs font-semibold text-slate-600 transition hover:bg-slate-50"
                        title="立刻执行一次"
                      >
                        <Play className="h-3.5 w-3.5" />
                        执行
                      </button>
                      <button
                        type="button"
                        onClick={() => void remove(t)}
                        className="rounded-md border border-slate-200 px-2 py-1.5 text-xs text-slate-500 transition hover:border-rose-200 hover:text-rose-600"
                        title="删除"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </Card>

      <div className="flex justify-end">
        <button type="button" onClick={() => setNotice('')} className="hidden" />
        <NewTaskModal onSaved={load} />
      </div>
    </div>
  )
}

/** 新建任务。只做新增：改周期不如删掉重加来得快，少维护一套编辑状态。 */
function NewTaskModal({ onSaved }: { onSaved: () => void }) {
  const [open, setOpen] = useState(false)
  const [name, setName] = useState('')
  const [kind, setKind] = useState<'interval' | 'daily'>('interval')
  const [intervalMin, setIntervalMin] = useState('60')
  const [timeOfDay, setTimeOfDay] = useState('03:00')
  const [action, setAction] = useState<'ddns' | 'wol'>('ddns')
  const [target, setTarget] = useState('')
  const [wolTargets, setWolTargets] = useState<api.WolTarget[]>([])
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const openModal = async () => {
    setOpen(true)
    setError('')
    try {
      const res = await api.getWolTargets()
      setWolTargets(res.targets ?? [])
    } catch {
      // 拿不到机器列表也不影响建 DDNS 任务，静默即可
    }
  }

  const submit = async () => {
    setSaving(true)
    setError('')
    try {
      const [h, m] = timeOfDay.split(':').map((s) => parseInt(s, 10))
      await api.addTask({
        id: '',
        name,
        enabled: true,
        kind,
        intervalMin: parseInt(intervalMin, 10) || 60,
        hour: Number.isFinite(h) ? h : 3,
        minute: Number.isFinite(m) ? m : 0,
        action,
        target: action === 'wol' ? target : '',
      })
      setName('')
      setOpen(false)
      onSaved()
    } catch (e) {
      setError(e instanceof Error ? e.message : '添加失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <button
        type="button"
        onClick={() => void openModal()}
        className="flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-2 text-xs font-semibold text-white shadow-sm transition hover:bg-blue-700"
      >
        <Plus className="h-4 w-4" />
        新建任务
      </button>

      {open && (
        <div
          className={modalBackdrop}
          onMouseDown={(e) => {
            if (e.target === e.currentTarget) setOpen(false)
          }}
        >
          <div className="w-full max-w-lg rounded-lg bg-white p-5 shadow-2xl ring-1 ring-slate-200">
            <div className="text-base font-bold text-slate-800">新建计划任务</div>

            <div className="mt-4 space-y-3">
              <label className="block">
                <span className="text-xs font-semibold text-slate-600">名称</span>
                <input
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  placeholder="每天凌晨同步解析"
                  className="mt-1 w-full rounded-md border border-slate-200 px-3 py-2 text-sm outline-none focus:border-blue-400"
                />
              </label>

              <div>
                <span className="text-xs font-semibold text-slate-600">周期</span>
                <div className="mt-1 flex gap-2">
                  {(['interval', 'daily'] as const).map((k) => (
                    <button
                      key={k}
                      type="button"
                      onClick={() => setKind(k)}
                      className={`rounded-md border px-3 py-1.5 text-xs font-semibold transition ${
                        kind === k
                          ? 'border-blue-400 bg-blue-50 text-blue-700'
                          : 'border-slate-200 text-slate-600 hover:bg-slate-50'
                      }`}
                    >
                      {k === 'interval' ? '每隔' : '每天'}
                    </button>
                  ))}
                </div>
                {kind === 'interval' ? (
                  <input
                    value={intervalMin}
                    onChange={(e) => setIntervalMin(e.target.value)}
                    placeholder="60"
                    className="mt-2 w-full rounded-md border border-slate-200 px-3 py-2 text-sm outline-none focus:border-blue-400"
                  />
                ) : (
                  <input
                    type="time"
                    value={timeOfDay}
                    onChange={(e) => setTimeOfDay(e.target.value)}
                    className="mt-2 w-full rounded-md border border-slate-200 px-3 py-2 text-sm outline-none focus:border-blue-400"
                  />
                )}
                {kind === 'daily' && (
                  <span className="mt-1 block text-xs text-slate-400">
                    当天这个时刻已经过了就顺延到第二天，不会补跑
                  </span>
                )}
              </div>

              <div>
                <span className="text-xs font-semibold text-slate-600">做什么</span>
                <div className="mt-1 flex gap-2">
                  {(['ddns', 'wol'] as const).map((a) => (
                    <button
                      key={a}
                      type="button"
                      onClick={() => setAction(a)}
                      className={`rounded-md border px-3 py-1.5 text-xs font-semibold transition ${
                        action === a
                          ? 'border-blue-400 bg-blue-50 text-blue-700'
                          : 'border-slate-200 text-slate-600 hover:bg-slate-50'
                      }`}
                    >
                      {a === 'ddns' ? '同步 DDNS' : '唤醒机器'}
                    </button>
                  ))}
                </div>
                {action === 'wol' && (
                  <select
                    value={target}
                    onChange={(e) => setTarget(e.target.value)}
                    className="mt-2 w-full rounded-md border border-slate-200 px-3 py-2 text-sm outline-none focus:border-blue-400"
                  >
                    <option value="">选择一台机器…</option>
                    {wolTargets.map((t) => (
                      <option key={t.id} value={t.id}>
                        {t.name}
                      </option>
                    ))}
                  </select>
                )}
                {action === 'wol' && wolTargets.length === 0 && (
                  <span className="mt-1 block text-xs text-amber-600">
                    还没有登记任何机器，先去「网络唤醒」页面添加
                  </span>
                )}
              </div>
            </div>

            {error && (
              <div className="mt-3 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">
                {error}
              </div>
            )}

            <div className="mt-5 flex items-center justify-end gap-2">
              <button
                type="button"
                onClick={() => setOpen(false)}
                className="rounded-md border border-slate-200 px-3 py-2 text-xs font-semibold text-slate-600 transition hover:bg-slate-50"
              >
                取消
              </button>
              <button
                type="button"
                onClick={() => void submit()}
                disabled={saving || !name || (action === 'wol' && !target)}
                className="flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-2 text-xs font-semibold text-white shadow-sm transition hover:bg-blue-700 disabled:opacity-60"
              >
                <Clock className="h-3.5 w-3.5" />
                {saving ? '保存中...' : '保存'}
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  )
}
