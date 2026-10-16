/**
 * AccessControlEditor 访问控制（IP 门禁）编辑器 —— 反代站点共用这一份。
 *
 * 规则写法和后端一致（见 modules/proxy/model）：一行一条，支持
 *   - 单个 IP：192.168.1.5
 *   - CIDR：   10.0.0.0/8
 *   - **IPv6 同样支持**：2409:8a60::/32（穿透经常是 v6 出去的，
 *     反代也可能是 v6 进来，所以这里不做 v4/v6 区分，统统按字符串收）
 *
 * 附带一个「从最近访问的 IP 里挑」：IP 不用手打 —— 访问审计里抓到过哪些 IP，
 * 点一下就填进规则（加入白名单或黑名单都很方便）。
 * 审计没开/没数据时这个入口自动隐藏，不打扰。
 */
import { useEffect, useState } from 'react'
import { Plus, ShieldCheck, Trash2 } from 'lucide-react'
import * as api from '../lib/api'

export type AccessMode = '' | 'allow' | 'deny'

const MODE_HINT: Record<string, string> = {
  '': '谁都能访问（留空 = 不限制）',
  allow: '只有名单里的能访问（白名单）',
  deny: '名单里的不能访问，其它都放行（黑名单）',
}

export function AccessControlEditor({
  mode,
  rules,
  onChange,
}: {
  mode: AccessMode
  /** 规则文本（一行一条） */
  rules: string
  onChange: (next: { mode: AccessMode; rules: string }) => void
}) {
  const [recentIPs, setRecentIPs] = useState<string[]>([])
  const [showRecent, setShowRecent] = useState(false)

  // 只在用户展开"从最近访问里挑"时才拉，避免每次开表单都多一个请求
  useEffect(() => {
    if (!showRecent) return
    let alive = true
    api
      .getAuditIPs({ limit: 30 })
      .then((r) =>
        alive && setRecentIPs((r.list ?? []).map((x: { ip: string }) => x.ip).filter(Boolean)),
      )
      .catch(() => undefined)
    return () => {
      alive = false
    }
  }, [showRecent])

  const addIP = (ip: string) => {
    const cur = rules.split('\n').map((s) => s.trim()).filter(Boolean)
    if (cur.includes(ip)) return
    onChange({ mode: mode || 'allow', rules: [...cur, ip].join('\n') })
  }

  return (
    <div className="rounded-xl ring-1 ring-slate-200">
      <div className="flex items-center justify-between gap-2 px-3 pt-2.5">
        <div className="flex items-center gap-1.5 text-xs font-semibold text-slate-600">
          <ShieldCheck className="h-3.5 w-3.5" />
          访问控制（IP 门禁）
        </div>
        <button
          type="button"
          onClick={() => setShowRecent((v) => !v)}
          className="rounded px-1.5 py-0.5 text-[11px] text-blue-500 transition hover:bg-blue-50"
          title="从访问审计里抓到过的 IP 里挑，不用手打"
        >
          {showRecent ? '收起' : '从最近访问的 IP 里挑'}
        </button>
      </div>

      {/* 模式：三选一，和后端 access.mode 的语义一致 */}
      <div className="flex flex-wrap gap-1.5 px-3 pt-2">
        {(['', 'allow', 'deny'] as AccessMode[]).map((m) => (
          <button
            key={m || 'any'}
            type="button"
            onClick={() => onChange({ mode: m, rules })}
            className={`rounded-lg px-2 py-1 text-[11px] font-semibold transition ${
              mode === m
                ? 'bg-blue-50 text-blue-600 ring-1 ring-blue-200'
                : 'bg-slate-50 text-slate-500 ring-1 ring-slate-200 hover:bg-slate-100'
            }`}
          >
            {m === '' ? '不限制' : m === 'allow' ? '白名单' : '黑名单'}
          </button>
        ))}
      </div>

      <div className="px-3 pb-1 pt-1.5 text-[11px] text-slate-400">{MODE_HINT[mode] ?? MODE_HINT['']}</div>

      {mode !== '' && (
        <div className="px-3 pb-3">
          <textarea
            value={rules}
            onChange={(e) => onChange({ mode, rules: e.target.value })}
            rows={3}
            placeholder={'一行一条，例如：\n192.168.1.5\n10.0.0.0/8\n2409:8a60::/32'}
            className="w-full rounded-lg border border-slate-200 bg-white px-2.5 py-2 font-mono text-xs outline-none focus:border-blue-400"
          />
        </div>
      )}

      {/* 从审计里挑 IP：IPv4 / IPv6 都可能有，这里不区分，原样填进规则 */}
      {showRecent && (
        <div className="border-t border-slate-100 px-3 py-2.5">
          {recentIPs.length === 0 ? (
            <div className="text-[11px] text-slate-400">
              访问审计里还没有记录（有人访问过服务之后这里才有）
            </div>
          ) : (
            <div className="flex flex-wrap gap-1">
              {recentIPs.map((ip) => (
                <button
                  key={ip}
                  type="button"
                  onClick={() => addIP(ip)}
                  title={`把 ${ip} 加进名单`}
                  className="flex items-center gap-1 rounded bg-slate-50 px-1.5 py-0.5 font-mono text-[10px] text-slate-600 ring-1 ring-slate-200 transition hover:bg-blue-50 hover:text-blue-600 hover:ring-blue-300"
                >
                  <Plus className="h-2.5 w-2.5" />
                  {ip}
                </button>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

/** 清空按钮（给已经在别处渲染的规则列表用；这里主要是保持 API 完整） */
export function ClearRulesButton({ onClick }: { onClick: () => void }) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[11px] text-slate-400 transition hover:bg-rose-50 hover:text-rose-500"
    >
      <Trash2 className="h-3 w-3" />
      清空
    </button>
  )
}
