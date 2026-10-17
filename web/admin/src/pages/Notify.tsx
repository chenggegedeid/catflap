/**
 * 通知中心。
 *
 * 三层：渠道（怎么发）→ 规则（什么事件发）→ 引用（各服务选渠道）。
 * 这一页管前两层，第三层在各自的服务页里选。
 *
 * 关键设计：**渠道是全局的**。你的 PushPlus token、群机器人地址在这里配一次，
 * 所有服务和规则都引用它 —— 改 token 只改一次，停用也只停一次。
 */
import { useCallback, useEffect, useMemo, useState } from 'react'
import { Bell, Pencil, Plus, Send, Trash2, TriangleAlert } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import * as api from '../lib/api'
import type { NotifyChannel, NotifyConfigView, NotifyEventMeta, NotifyRule, NotifyChannelType } from '../types'

// 各渠道要填哪些字段。顺序就是表单里的顺序。
const channelFields: Record<NotifyChannelType, { key: string; label: string; placeholder: string; secret?: boolean; textarea?: boolean }[]> = {
  webhook: [
    { key: 'url', label: '回调地址', placeholder: 'https://example.com/hook', secret: true },
    { key: 'method', label: '请求方法', placeholder: 'POST（留空为 GET）' },
    { key: 'headers', label: '请求头', placeholder: 'Key: Value，多行', textarea: true },
    { key: 'body', label: '请求体', placeholder: '支持 #{title} #{content} #{time} 占位符', textarea: true },
  ],
  dingtalk: [{ key: 'url', label: '机器人 Webhook', placeholder: 'https://oapi.dingtalk.com/robot/send?access_token=...', secret: true }],
  wecom: [{ key: 'url', label: '机器人 Webhook', placeholder: 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=...', secret: true }],
  feishu: [{ key: 'url', label: '机器人 Webhook', placeholder: 'https://open.feishu.cn/open-apis/bot/v2/hook/...', secret: true }],
  bark: [
    { key: 'server', label: '服务器', placeholder: 'https://api.day.app（留空用官方）' },
    { key: 'key', label: '推送 Key', placeholder: '你的 Bark Key', secret: true },
  ],
  serverchan: [{ key: 'key', label: 'SendKey', placeholder: 'SCT...', secret: true }],
  pushplus: [{ key: 'token', label: 'Token', placeholder: '你的 PushPlus token', secret: true }],
}

const channelTypeLabels: Record<NotifyChannelType, string> = {
  webhook: '通用 Webhook',
  dingtalk: '钉钉群机器人',
  wecom: '企业微信群机器人',
  feishu: '飞书群机器人',
  bark: 'Bark（iOS）',
  serverchan: 'Server 酱',
  pushplus: 'PushPlus',
}

export function Notify() {
  const [cfg, setCfg] = useState<NotifyConfigView | null>(null)
  const [err, setErr] = useState('')
  const [toast, setToast] = useState('')

  const [channelModal, setChannelModal] = useState<{ open: boolean; initial?: NotifyChannel }>({ open: false })
  const [ruleModal, setRuleModal] = useState<{ open: boolean; initial?: NotifyRule }>({ open: false })
  const [testing, setTesting] = useState('')

  const refresh = useCallback(async () => {
    try {
      setCfg(await api.getNotifyConfig())
    // 兼容老版本：serviceRefs 可能没有
    if (!cfg?.serviceRefs) {
      try {
        const refs = await api.getNotifyServiceRefs()
        setCfg((c) => (c ? { ...c, serviceRefs: refs } : c))
      } catch {
        // 老后端没这个接口也无妨
      }
    }
      setErr('')
    } catch (e) {
      setErr(e instanceof Error ? e.message : '加载失败')
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const say = (m: string) => {
    setToast(m)
    window.setTimeout(() => setToast(''), 2600)
  }

  const channels = useMemo(() => cfg?.channels ?? [], [cfg])
  const rules = useMemo(() => cfg?.rules ?? [], [cfg])
  const events = useMemo(() => cfg?.events ?? [], [cfg])

  const channelName = (id: string) => channels.find((c) => c.id === id)?.name ?? `（已删除 ${id}）`

  const testChannel = async (ch: NotifyChannel) => {
    setTesting(ch.id)
    try {
      await api.testNotifyChannel({ id: ch.id })
      say(`已发送到「${ch.name}」，请查看是否收到`)
    } catch (e) {
      say('发送失败：' + (e instanceof Error ? e.message : ''))
    } finally {
      setTesting('')
    }
  }

  const removeChannel = async (ch: NotifyChannel) => {
    if (ch.usedByRules.length > 0) {
      const ok = window.confirm(
        `「${ch.name}」正被 ${ch.usedByRules.length} 条规则引用：\n${ch.usedByRules.join('、')}\n\n` +
          `删掉之后这些规则就发不出去了。确认删除？`,
      )
      if (!ok) return
    } else if (!window.confirm(`确认删除渠道「${ch.name}」？`)) {
      return
    }
    await api.deleteNotifyChannel(ch.id)
    say('已删除')
    await refresh()
  }

  const removeRule = async (r: NotifyRule) => {
    if (!window.confirm(`确认删除规则「${r.name}」？`)) return
    await api.deleteNotifyRule(r.id)
    say('已删除')
    await refresh()
  }

  if (err) {
    return (
      <Card>
        <div className="text-sm text-rose-600">{err}</div>
      </Card>
    )
  }

  return (
    <div className="space-y-5">
      {/* 说明：这个设计为什么要全局 */}
      <Card className="!py-3.5">
        <div className="flex items-start gap-3">
          <Bell className="mt-0.5 h-4 w-4 shrink-0 text-blue-500" />
          <div className="text-[12px] leading-relaxed text-muted">
            渠道在这里配**一次**，之后在穿透服务、证书、域名解析那儿按名字选用 ——
            改 token / 换机器人 / 临时停用都只改这一处，所有引用它的地方立刻跟着变。
            {cfg?.migrated && (
              <div className="mt-1 text-muted">
                已有 {channels.length} 个渠道（其中一部分是从原有服务的 Webhook 配置迁移来的，
                内容相同的已自动合并成一个）。
              </div>
            )}
          </div>
        </div>
      </Card>

      {/* 渠道 */}
      <Card className="!p-0">
        <div className="px-5 pt-4">
          <CardHeader
            title="通知渠道"
            action={
              <button
                onClick={() => setChannelModal({ open: true })}
                className="flex items-center gap-1 rounded-lg bg-blue-500 px-2.5 py-1 text-xs font-semibold text-white hover:bg-blue-600"
              >
                <Plus className="h-3 w-3" /> 添加渠道
              </button>
            }
          />
        </div>
        <div className="px-5 pb-5">
          {channels.length === 0 ? (
            <div className="rounded-xl bg-surface-2 px-4 py-6 text-center text-xs text-muted">
              还没有渠道。加一个之后，各服务就能选用它来发通知。
            </div>
          ) : (
            <div className="space-y-2">
              {channels.map((ch) => (
                <div
                  key={ch.id}
                  className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-xl border border-line/70 px-3 py-2.5"
                >
                  <span className={`h-2 w-2 shrink-0 rounded-full ${ch.enabled ? 'bg-emerald-500' : 'bg-fill'}`} />
                  <div className="min-w-0">
                    <div className="text-sm font-semibold text-content">{ch.name}</div>
                    <div className="text-[11px] text-muted">
                      {channelTypeLabels[ch.type] ?? ch.type}
                      {!ch.enabled && ' · 已停用'}
                      {ch.usedByRules.length > 0 && ` · 被 ${ch.usedByRules.length} 条规则使用`}
                      {ch.migratedFrom && ' · 由服务配置迁移'}
                    </div>
                  </div>
                  <div className="ml-auto flex items-center gap-1">
                    <button
                      onClick={() => testChannel(ch)}
                      disabled={testing === ch.id}
                      title="发一条测试消息"
                      className="flex items-center gap-1 rounded-lg px-2 py-1 text-xs font-semibold text-muted hover:bg-surface-2 disabled:opacity-50"
                    >
                      <Send className={`h-3 w-3 ${testing === ch.id ? 'animate-pulse' : ''}`} /> 测试
                    </button>
                    <button
                      onClick={() => setChannelModal({ open: true, initial: ch })}
                      className="grid h-7 w-7 place-items-center rounded-lg text-muted hover:bg-surface-2"
                      title="编辑"
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </button>
                    <button
                      onClick={() => removeChannel(ch)}
                      className="grid h-7 w-7 place-items-center rounded-lg text-muted hover:bg-rose-100 hover:text-rose-600"
                      title="删除"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
      </Card>

      {/* 启用通知的服务 —— 通知页要能看到"我配的渠道都被谁用了" */}
      <Card className="!p-0">
        <div className="px-5 pt-4">
          <CardHeader
            title="启用通知的服务"
            action={
              <a
                href="#/stun"
                className="text-xs text-blue-500 transition hover:text-blue-600"
              >
                去配置服务 ↗
              </a>
            }
          />
        </div>
        <div className="px-5 pb-5">
          {channels.length === 0 ? (
            <div className="rounded-xl bg-surface-2 px-4 py-6 text-center text-xs text-muted">
              先在通知页添加渠道，再去穿透页开启服务通知。
            </div>
          ) : !cfg?.serviceRefs || cfg.serviceRefs.length === 0 ? (
            <div className="rounded-xl bg-surface-2 px-4 py-6 text-center text-xs text-muted">
              还没有任何穿透服务。在穿透页添加服务 → 选几个通知渠道 → 这里就会出现。
            </div>
          ) : (
            <div className="space-y-1.5">
              {cfg.serviceRefs.map((r) => {
                // 用 channelIds 的渠道名拼成"发到哪些"
                const chs = (r.channelIds || []).map((id) => channels.find((c) => c.id === id)?.name ?? id)
                return (
                  <div
                    key={`${r.deviceId}-${r.serviceId}`}
                    className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-xl border border-line/70 px-3 py-2 text-xs"
                  >
                    <div className="min-w-0 flex-1">
                      <div className="font-semibold text-content">{r.serviceName || `服务 #${r.serviceId}`}</div>
                      <div className="text-[11px] text-muted">设备 #{r.deviceId}</div>
                    </div>
                    {chs.length > 0 ? (
                      <div className="text-[11px] text-muted">发到：{chs.join('、')}</div>
                    ) : (
                      <div className="text-[11px] text-muted">这个服务没选渠道</div>
                    )}
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </Card>

      {/* 规则 */}
      <Card className="!p-0">
        <div className="px-5 pt-4">
          <CardHeader
            title="通知规则"
            action={
              <button
                onClick={() => setRuleModal({ open: true })}
                disabled={channels.length === 0}
                className="flex items-center gap-1 rounded-lg bg-blue-500 px-2.5 py-1 text-xs font-semibold text-white hover:bg-blue-600 disabled:opacity-40"
              >
                <Plus className="h-3 w-3" /> 添加规则
              </button>
            }
          />
        </div>
        <div className="px-5 pb-5">
          {channels.length === 0 ? (
            <div className="rounded-xl bg-surface-2 px-4 py-6 text-center text-xs text-muted">
              先添加渠道，再来配「什么时候通知你」。
            </div>
          ) : rules.length === 0 ? (
            <div className="rounded-xl bg-surface-2 px-4 py-6 text-center text-xs text-muted">
              还没有规则 —— 没配规则的渠道不会收到任何通知。
            </div>
          ) : (
            <div className="space-y-2">
              {rules.map((r) => {
                const meta = events.find((e) => e.type === r.event)
                return (
                  <div key={r.id} className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-xl border border-line/70 px-3 py-2.5">
                    <span className={`h-2 w-2 shrink-0 rounded-full ${r.enabled ? 'bg-emerald-500' : 'bg-fill'}`} />
                    <div className="min-w-0">
                      <div className="text-sm font-semibold text-content">
                        {meta?.title ?? r.event}
                        <span className="ml-2 text-[11px] font-normal text-muted">{meta?.group}</span>
                      </div>
                      <div className="text-[11px] text-muted">
                        发给：{r.channelIds.map(channelName).join('、') || '（无）'}
                      </div>
                    </div>
                    <div className="ml-auto flex items-center gap-1">
                      <button
                        onClick={() => setRuleModal({ open: true, initial: r })}
                        className="grid h-7 w-7 place-items-center rounded-lg text-muted hover:bg-surface-2"
                        title="编辑"
                      >
                        <Pencil className="h-3.5 w-3.5" />
                      </button>
                      <button
                        onClick={() => removeRule(r)}
                        className="grid h-7 w-7 place-items-center rounded-lg text-muted hover:bg-rose-100 hover:text-rose-600"
                        title="删除"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </Card>

      {channelModal.open && (
        <ChannelModal
          initial={channelModal.initial}
          onCancel={() => setChannelModal({ open: false })}
          onSubmit={async (body) => {
            await api.saveNotifyChannel(body)
            setChannelModal({ open: false })
            say('已保存')
            await refresh()
          }}
        />
      )}

      {ruleModal.open && (
        <RuleModal
          initial={ruleModal.initial}
          channels={channels}
          events={events}
          onCancel={() => setRuleModal({ open: false })}
          onSubmit={async (body) => {
            await api.saveNotifyRule(body)
            setRuleModal({ open: false })
            say('已保存')
            await refresh()
          }}
        />
      )}

      {toast && (
        <div className="fixed bottom-6 left-1/2 z-50 -translate-x-1/2 rounded-xl bg-inverse px-4 py-2 text-xs font-medium text-on-inverse shadow-lg">
          {toast}
        </div>
      )}
    </div>
  )
}

function ChannelModal({
  initial,
  onCancel,
  onSubmit,
}: {
  initial?: NotifyChannel
  onCancel: () => void
  onSubmit: (body: api.NotifyChannelSavePayload) => Promise<void>
}) {
  const [name, setName] = useState(initial?.name ?? '')
  const [type, setType] = useState<NotifyChannelType>(initial?.type ?? 'pushplus')
  const [enabled, setEnabled] = useState(initial?.enabled ?? true)
  const [config, setConfig] = useState<Record<string, string>>(() => {
    const c: Record<string, string> = {}
    for (const f of channelFields[initial?.type ?? 'pushplus']) c[f.key] = initial?.config?.[f.key] ?? ''
    return c
  })
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const fields = channelFields[type] ?? []

  const switchType = (t: NotifyChannelType) => {
    setType(t)
    // 换类型时把字段重置成新类型的空值（旧的键对新类型没意义）。
    // 已有的掩码值不能带过去 —— 那会变成"另一个渠道的密钥"。
    const c: Record<string, string> = {}
    for (const f of channelFields[t]) c[f.key] = ''
    setConfig(c)
  }

  const submit = async () => {
    if (!name.trim()) {
      setErr('渠道名称不能为空')
      return
    }
    setBusy(true)
    setErr('')
    try {
      await onSubmit({ id: initial?.id ?? '', name: name.trim(), type, enabled, config })
    } catch (e) {
      setErr(e instanceof Error ? e.message : '保存失败')
      setBusy(false)
    }
  }

  return (
    <Modal title={initial ? '编辑渠道' : '添加渠道'} onCancel={onCancel} onSubmit={submit} busy={busy} submitText="保存">
      <label className="block">
        <div className={labelCls}>渠道名称</div>
        <input
          autoFocus
          value={name}
          onChange={(e) => setName(e.target.value)}
          placeholder="如：我的 PushPlus / 家庭群"
          className={inputCls}
        />
        <div className="mt-1 text-[11px] text-muted">各服务的下拉里显示的就是这个名字</div>
      </label>

      <label className="block">
        <div className={labelCls}>类型</div>
        <select value={type} onChange={(e) => switchType(e.target.value as NotifyChannelType)} className={inputCls}>
          {(Object.keys(channelTypeLabels) as NotifyChannelType[]).map((t) => (
            <option key={t} value={t}>
              {channelTypeLabels[t]}
            </option>
          ))}
        </select>
      </label>

      {fields.map((f) => (
        <label key={f.key} className="block">
          <div className={labelCls}>{f.label}</div>
          {f.textarea ? (
            <textarea
              value={config[f.key] ?? ''}
              onChange={(e) => setConfig((p) => ({ ...p, [f.key]: e.target.value }))}
              placeholder={f.placeholder}
              rows={3}
              className={`${inputCls} font-mono`}
            />
          ) : (
            <input
              type={f.secret ? 'password' : 'text'}
              value={config[f.key] ?? ''}
              onChange={(e) => setConfig((p) => ({ ...p, [f.key]: e.target.value }))}
              placeholder={f.placeholder}
              className={`${inputCls} font-mono`}
            />
          )}
        </label>
      ))}

      {initial && (
        <div className="rounded-xl bg-surface-2 px-3 py-2 text-[11px] leading-relaxed text-muted">
          显示为 <span className="font-mono">••••••</span> 的字段表示已保存过，留着不动就是不改。
        </div>
      )}

      <label className="flex cursor-pointer items-center gap-2 text-xs text-content">
        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
        启用（关掉后所有引用它的地方立刻停发）
      </label>

      {err && <div className="rounded-xl bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>}
    </Modal>
  )
}

function RuleModal({
  initial,
  channels,
  events,
  onCancel,
  onSubmit,
}: {
  initial?: NotifyRule
  channels: NotifyChannel[]
  events: NotifyEventMeta[]
  onCancel: () => void
  onSubmit: (body: api.NotifyRuleSavePayload) => Promise<void>
}) {
  const [event, setEvent] = useState(initial?.event ?? events[0]?.type ?? '')
  const [picked, setPicked] = useState<string[]>(initial?.channelIds ?? [])
  const [enabled, setEnabled] = useState(initial?.enabled ?? true)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  // 按分组归拢事件，选择列表更好看
  const groups = useMemo(() => {
    const m = new Map<string, NotifyEventMeta[]>()
    for (const e of events) {
      const list = m.get(e.group) ?? []
      list.push(e)
      m.set(e.group, list)
    }
    return [...m.entries()]
  }, [events])

  const toggle = (id: string) => {
    setPicked((p) => (p.includes(id) ? p.filter((x) => x !== id) : [...p, id]))
  }

  const submit = async () => {
    if (picked.length === 0) {
      setErr('至少选一个渠道，否则这条规则不会发任何东西')
      return
    }
    setBusy(true)
    setErr('')
    try {
      await onSubmit({ id: initial?.id ?? '', name: initial?.name ?? '', event, channelIds: picked, enabled })
    } catch (e) {
      setErr(e instanceof Error ? e.message : '保存失败')
      setBusy(false)
    }
  }

  return (
    <Modal title={initial ? '编辑规则' : '添加规则'} onCancel={onCancel} onSubmit={submit} busy={busy} submitText="保存">
      <label className="block">
        <div className={labelCls}>什么时候通知</div>
        <select value={event} onChange={(e) => setEvent(e.target.value as typeof event)} className={inputCls}>
          {groups.map(([group, list]) => (
            <optgroup key={group} label={group}>
              {list.map((e) => (
                <option key={e.type} value={e.type}>
                  {e.title}
                </option>
              ))}
            </optgroup>
          ))}
        </select>
      </label>

      <div>
        <div className={labelCls}>发给哪些渠道</div>
        <div className="space-y-1.5 rounded-xl border border-line p-2.5">
          {channels.map((ch) => (
            <label key={ch.id} className="flex cursor-pointer items-center gap-2 text-xs">
              <input type="checkbox" checked={picked.includes(ch.id)} onChange={() => toggle(ch.id)} />
              <span className={ch.enabled ? 'text-content' : 'text-muted'}>
                {ch.name}
                {!ch.enabled && '（已停用，勾了也不会发）'}
              </span>
            </label>
          ))}
        </div>
        <div className="mt-1 text-[11px] text-muted">
          同一事件在冷却窗口内只发一次，不会刷屏。渠道在「通知渠道」里统一管理。
        </div>
      </div>

      <label className="flex cursor-pointer items-center gap-2 text-xs text-content">
        <input type="checkbox" checked={enabled} onChange={(e) => setEnabled(e.target.checked)} />
        启用这条规则
      </label>

      {err && (
        <div className="flex items-start gap-2 rounded-xl bg-rose-50 px-3 py-2 text-xs text-rose-600">
          <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
          {err}
        </div>
      )}
    </Modal>
  )
}

function Modal({
  title,
  children,
  onCancel,
  onSubmit,
  busy,
  submitText,
}: {
  title: string
  children: React.ReactNode
  onCancel: () => void
  onSubmit: () => void
  busy: boolean
  submitText: string
}) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 dark:bg-black/70">
      <div className="max-h-[88vh] w-full max-w-lg overflow-y-auto rounded-2xl bg-surface p-5 shadow-2xl">
        <div className="mb-4 text-base font-bold text-content">{title}</div>
        <div className="space-y-3">{children}</div>
        <div className="mt-5 flex justify-end gap-2">
          <button onClick={onCancel} className="rounded-xl px-4 py-2 text-sm text-muted hover:bg-surface-2">
            取消
          </button>
          <button
            onClick={onSubmit}
            disabled={busy}
            className="rounded-xl bg-blue-500 px-4 py-2 text-sm font-semibold text-white hover:bg-blue-600 disabled:opacity-50"
          >
            {busy ? '保存中...' : submitText}
          </button>
        </div>
      </div>
    </div>
  )
}

const labelCls = 'mb-1 text-xs font-semibold text-muted'
const inputCls =
  'w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400'
