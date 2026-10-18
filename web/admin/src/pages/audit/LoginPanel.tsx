import { useCallback, useEffect, useState } from 'react'
import { Card, CardHeader } from '../../components/Card'
import { Pager } from '../../components/Pager'
import * as api from '../../lib/api'
import { getPageLimit } from '../../lib/prefs'
import type { LoginEvent, LoginSummary } from '../../types'
import { applyLimit } from './shared'

export function LoginPanel() {
  const [data, setData] = useState<{
    list: LoginEvent[]
    day: string
    days: string[]
    total: number
    summary: LoginSummary
  } | null>(null)

  // 按哪天看。空 = 后端给最近有记录的那天
  const [day, setDay] = useState('')
  const [page, setPage] = useState(1)
  const [limit, setLimit] = useState(getPageLimit)

  const load = useCallback(async () => {
    try {
      setData(await api.getAuditLogins({ day, limit, page }))
    } catch {
      // 没有记录 / 接口不可用都不该影响这一页其它块
    }
  }, [day, page, limit])

  useEffect(() => {
    void load()
  }, [load])

  // 换日期就回到第一页
  useEffect(() => {
    setPage(1)
  }, [day, limit])

  const s = data?.summary
  const failed = s?.failed ?? 0
  const total = s?.total ?? 0
  const days = data?.days ?? []

  return (
    <Card className="!p-0">
      <div className="px-5 pt-4">
        <CardHeader
          title={
            <span>
              登录尝试
              {data?.day && <span className="ml-1 font-normal text-muted">{data.day}</span>}
            </span>
          }
          action={
            /* 按日期看：登录事件本来就按天存文件（login-<日期>.jsonl），
               所以这里是挑文件，不是过滤 —— 选哪天就只看哪天 */
            <select
              value={day}
              onChange={(e) => setDay(e.target.value)}
              className="rounded-lg border border-line bg-surface px-2 py-1 text-xs outline-none focus:border-blue-400"
              title="选择查看哪一天的登录记录"
            >
              <option value="">最近一天</option>
              {days.map((d) => (
                <option key={d} value={d}>
                  {d}
                </option>
              ))}
            </select>
          }
        />
      </div>
      <div className="px-5 pb-5">
        {!s || total === 0 ? (
          <div className="rounded-xl bg-surface-2 px-4 py-6 text-center text-xs text-muted">
            还没有登录记录
          </div>
        ) : (
          <>
            <div className="mb-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
              <span className="text-muted">
                共 <span className="font-semibold text-content">{total}</span> 次
              </span>
              {failed > 0 && (
                <span className="text-rose-600">
                  失败 <span className="font-semibold">{failed}</span>
                </span>
              )}
              {s.succeeded > 0 && (
                <span className="text-emerald-600">
                  成功 <span className="font-semibold">{s.succeeded}</span>
                </span>
              )}
              {failed > 0 && (
                <span className="text-[11px] text-muted">
                  来自 {s.ips.length} 个地址
                  {s.usernames.length > 1 && ` · 试过 ${s.usernames.length} 个用户名`}
                </span>
              )}
            </div>

            {/* 失败明显偏多时提示一句 —— 这是用户真正需要被提醒的信号 */}
            {failed >= 10 && (
              <div className="mb-3 rounded-xl bg-rose-50 px-3 py-2 text-[11px] leading-relaxed text-rose-700 ring-1 ring-rose-200">
                有 {failed} 次登录失败。如果是你不小心输错，忽略即可；
                如果来自不认识的地址，说明有人在试你的密码 —— 检查上面的来源，必要时拉黑。
              </div>
            )}

            <div className="max-h-64 space-y-0.5 overflow-y-auto rounded-lg bg-surface-2/60 p-2">
              {(data?.list ?? []).map((e, i) => (
                <div key={i} className="flex flex-wrap items-center gap-x-3 gap-y-0.5 px-1 py-1 text-[11px]">
                  <span className="font-mono text-muted">{e.time.slice(5, 19).replace('T', ' ')}</span>
                  <span className={`shrink-0 rounded px-1 py-0.5 text-[10px] font-bold ${
                    e.success ? 'bg-emerald-50 text-emerald-600' : 'bg-rose-50 text-rose-600'
                  }`}>
                    {e.success ? '成功' : '失败'}
                  </span>
                  <span className="font-mono text-content">{e.ip}</span>
                  <span className="text-muted">
                    用户名 <span className="font-mono text-content">{e.username || '（空）'}</span>
                  </span>
                  {!e.success && e.reason && (
                    <span className="text-[10px] text-muted">{e.reason}</span>
                  )}
                </div>
              ))}

              {/* 分页：和访问来源、黑名单同一个组件 */}
              <Pager
                total={data?.total ?? 0}
                page={page}
                limit={limit}
                onPage={setPage}
                onLimit={applyLimit(setLimit)}
                unit="次"
              />
            </div>
          </>
        )}
      </div>
    </Card>
  )
}
