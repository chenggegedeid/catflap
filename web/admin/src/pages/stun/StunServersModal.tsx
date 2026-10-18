/**
 * STUN 服务器列表弹窗：编辑内置那一批、测速选最优。
 */
import { useState } from 'react'
import { AlertCircle, RotateCw, Server, X } from 'lucide-react'
import { modalBackdrop } from '../../components/modal'
import * as api from '../../lib/api'

export function StunServersModal({ initial, onClose, onSaved }: { initial: string[]; onClose: () => void; onSaved: (best: string) => void }) {
  const [text, setText] = useState(initial.join('\n'))
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const save = async () => {
    setSaving(true)
    setError('')
    try {
      const servers = text
        .split('\n')
        .map((s) => s.trim())
        .filter(Boolean)
      const result = await api.updateStunServers(servers)
      onSaved(result.data?.best ?? '')
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : '保存失败')
    } finally {
      setSaving(false)
    }
  }

  // 恢复内置列表：把后端返回的默认列表填回输入框，不直接保存。
  // 让用户先看一眼再决定 —— 一键覆盖掉他刚敲的内容会很烦人。
  const resetDefault = async () => {
    setSaving(true)
    setError('')
    try {
      const result = await api.resetStunServers()
      setText((result.data?.servers ?? []).join('\n'))
    } catch (e) {
      setError(e instanceof Error ? e.message : '恢复失败')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="flex max-h-[88vh] w-full max-w-2xl flex-col rounded-lg bg-surface shadow-2xl ring-1 ring-line">
        <div className="flex items-center justify-between gap-4 border-b border-line px-5 py-4">
          <div className="flex min-w-0 items-center gap-3">
            <span className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-blue-50 text-blue-600 ring-1 ring-blue-100">
              <Server className="h-4.5 w-4.5" />
            </span>
            <div className="min-w-0">
              <div className="text-base font-bold text-content">STUN 服务器</div>
              <div className="mt-0.5 text-xs text-muted">每行一个，host 或 host:port（默认 3478）</div>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            title="关闭"
            aria-label="关闭"
            className="grid h-8 w-8 place-items-center rounded-md text-muted transition hover:bg-surface-2 hover:text-content"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="overflow-y-auto p-5">
          <textarea
            value={text}
            onChange={(e) => setText(e.target.value)}
            spellCheck={false}
            className="h-72 w-full resize-y rounded-md border border-line bg-surface-2 p-3 font-mono text-xs leading-5 text-content outline-none transition focus:border-blue-300 focus:bg-surface focus:ring-2 focus:ring-blue-100"
            placeholder={'stun.moonlight-stream.org:3478\nstun.threema.ch:3478\n...'}
          />
          <div className="mt-2 space-y-1 text-xs leading-5 text-muted">
            <p>保存后会立即重测一遍延迟并选出「最优」。正在跑的打洞服务不受影响，下一次打洞或保活重连起用新列表。</p>
            <p>提示：部分服务商（如 sonetel / nextcloud）会在几秒内掐掉 TCP 连接，TCP 打洞会反复重启，建议从列表里去掉它们。</p>
            <p>
              想换成更新的一批？这里有一份持续校验过的公开清单，复制进来即可：
              <a
                href="https://github.com/pradt2/always-online-stun/blob/master/valid_hosts_tcp.txt"
                target="_blank"
                rel="noreferrer"
                className="ml-1 text-blue-500 underline hover:text-blue-600 dark:text-blue-400"
              >
                always-online-stun（TCP）
              </a>
            </p>
          </div>
          {error && (
            <div className="mt-3 flex items-start gap-2 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-xs leading-5 text-rose-700">
              <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
              <span className="break-words">{error}</span>
            </div>
          )}
        </div>
        <div className="flex flex-wrap items-center justify-between gap-3 border-t border-line px-5 py-3">
          <span className="text-xs text-muted">保存前会逐行校验并去重</span>
          <div className="flex items-center gap-2">
            {/* 恢复内置列表放在「取消」左边：它是个不常用但关键的回头路，
                排在次要位置，避免和「保存」抢注意力 */}
            <button
              type="button"
              onClick={resetDefault}
              disabled={saving}
              title="丢弃当前编辑，恢复成程序内置的默认列表并立即重测"
              className="rounded-md border border-line px-3 py-2 text-xs font-semibold text-muted transition hover:bg-surface-2 disabled:opacity-50"
            >
              恢复内置列表
            </button>
            <button
              type="button"
              onClick={onClose}
              className="rounded-md border border-line px-3 py-2 text-xs font-semibold text-content transition hover:bg-surface-2"
            >
              取消
            </button>
            <button
              type="button"
              onClick={save}
              disabled={saving}
              className="flex min-w-[88px] items-center justify-center gap-1.5 rounded-md bg-blue-600 px-3 py-2 text-xs font-semibold text-white shadow-sm transition hover:bg-blue-700 disabled:cursor-wait disabled:opacity-60"
            >
              <RotateCw className={`h-3.5 w-3.5 ${saving ? 'animate-spin' : ''}`} />
              {saving ? '重测中...' : '保存并重测'}
            </button>
          </div>
        </div>
      </div>
    </div>
  )
}

