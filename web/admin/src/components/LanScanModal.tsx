/**
 * 局域网扫描弹窗。
 *
 * **穿透和反向代理共用这一个**。「这台机器开着哪些端口」只有一个答案，
 * 两套实现迟早会在探测手法上分叉（超时、并发、要不要试 HTTP）——
 * 而后端本来就是同一套（core/lanscan），前端没理由各写一遍。
 *
 * 两边业务差异靠 props 注入，不在这组件里判断：
 *   - 穿透：点端口 → 建一个「服务」；要标出"这个端口已建过服务"
 *   - 反代：点端口 → 填站点的 backend；不关心服务
 *
 * 所以这里只接收"这个 IP 上哪些端口已被占用"这样的**现成数据**，
 * 而不是自己去读设备列表 —— 那样就把 stun 的业务知识塞进共用组件了。
 */
import { useState } from 'react'
import { LoaderCircle, Radar, Server, X } from 'lucide-react'
import { modalBackdrop } from './modal'
import * as api from '../lib/api'
import type { LanHost, LanPort, LanSubnet } from '../types'

const EMPTY_TAKEN = new Set<string>()
const EMPTY_ADDED = new Map<string, string>()

export function LanScanModal({
  subnets,
  onCancel,
  onPick,
  onPickPort,
  takenPorts,
  addedHosts,
}: {
  subnets: LanSubnet[]
  onCancel: () => void
  /** 点整台机器（不指定端口时用） */
  onPick: (host: LanHost) => void
  /** 点了机器上的某个端口 */
  onPickPort: (host: LanHost, port: LanPort) => void
  /** 已被占用的「IP:端口」，界面上会灰掉。不关心就省略 */
  takenPorts?: Set<string>
  /** 已添加过的机器：IP → 显示名，界面上标「已加入」。不关心就省略 */
  addedHosts?: Map<string, string>
}) {
  const [cidr, setCidr] = useState(subnets[0]?.cidr ?? '')
  const [hosts, setHosts] = useState<LanHost[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  // 占用情况与已添加列表由调用方给（只有外面才知道那是"服务"还是"站点 backend"），
  // 这里只做展示。默认空集合，让不关心的调用方少写两行。
  const taken = takenPorts ?? EMPTY_TAKEN
  const added = addedHosts ?? EMPTY_ADDED

  const run = async () => {
    setBusy(true)
    setErr('')
    // 上一轮的结果先清掉。换个网段再扫的时候，旧的那几台还挂在那儿，
    // 看着就跟这一轮扫出来的一样
    setHosts(null)
    try {
      setHosts(await api.scanLan(cidr))
    } catch (e) {
      setHosts(null)
      setErr(e instanceof Error ? e.message : '扫描失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="max-h-full w-full max-w-lg overflow-y-auto rounded-2xl bg-surface p-6 text-content shadow-2xl ring-1 ring-line">
        <div className="mb-4 flex items-center justify-between">
          <div className="text-base font-bold text-content">扫描局域网</div>
          <button
            type="button"
            onClick={onCancel}
            className="grid h-7 w-7 place-items-center rounded-full text-muted transition hover:bg-surface-2 hover:text-content"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {subnets.length === 0 ? (
          <div className="rounded-xl bg-amber-50 px-3 py-2.5 text-xs leading-5 text-amber-700">
            没找到本机接着的内网网段。手动填 IP 添加设备吧。
          </div>
        ) : (
          <>
            <div className="flex items-end gap-2">
              <label className="min-w-0 flex-1">
                <div className="mb-1 text-xs font-semibold text-muted">扫哪一段</div>
                <select
                  value={cidr}
                  onChange={(e) => setCidr(e.target.value)}
                  className="w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none focus:border-blue-400"
                >
                  {subnets.map((s) => (
                    <option key={s.cidr} value={s.cidr}>
                      {s.cidr} · {s.iface}（本机 {s.localIP}）
                    </option>
                  ))}
                </select>
              </label>
              <button
                type="button"
                onClick={run}
                disabled={busy}
                className="flex shrink-0 items-center gap-1.5 rounded-xl bg-blue-500 px-4 py-2 text-sm font-semibold text-white shadow-md shadow-blue-500/20 transition hover:bg-blue-600 disabled:opacity-50"
              >
                {busy ? (
                  <LoaderCircle className="h-4 w-4 animate-spin" />
                ) : (
                  <Radar className="h-4 w-4" />
                )}
                {busy ? '扫描中' : '开始扫描'}
              </button>
            </div>

            {/*
              扫不到的是防火墙设成「什么都不回」的机器，Windows 默认就这样。
              不说这句的话，用户会以为那台设备根本不在网里，转头去查网线。
            */}
            <div className="mt-2 text-[11px] leading-5 text-muted">
              两三秒出结果。防火墙拦得严的机器（Windows 默认就是）扫不出来，这种只能自己填 IP。
            </div>

            {err && <div className="mt-3 text-xs text-rose-500">扫不了：{err}</div>}

            {hosts && (
              <div className="mt-4">
                <div className="mb-2 flex items-baseline gap-2">
                  <span className="text-xs font-semibold text-muted">找到 {hosts.length} 台</span>
                  {/* 端口能点这件事看不出来，得说一句 */}
                  <span className="text-[11px] text-muted">已加过的设备，点端口直接建服务</span>
                </div>
                {hosts.length === 0 ? (
                  <div className="rounded-xl bg-surface-2 px-3 py-4 text-center text-xs text-muted">
                    一台都没扫到。换个网段试试，或者手动填 IP
                  </div>
                ) : (
                  <ul className="space-y-1.5">
                    {hosts.map((h) => {
                      const name = added.get(h.ip)
                      return (
                        <li
                          key={h.ip}
                          className="rounded-xl border border-line bg-surface-2/60 px-3 py-2"
                        >
                          <div className="flex items-center gap-2">
                            <Server className="h-4 w-4 shrink-0 text-muted" />
                            <span className="truncate font-mono text-sm font-semibold text-content">
                              {h.ip}
                            </span>
                            {h.name && (
                              <span className="truncate text-xs text-muted">{h.name}</span>
                            )}
                            {h.self && (
                              <span className="shrink-0 rounded bg-fill px-1.5 py-0.5 text-[10px] font-semibold text-content">
                                本机
                              </span>
                            )}
                            <div className="flex-1" />
                            {name ? (
                              <span className="shrink-0 truncate text-[11px] text-emerald-600">
                                已添加「{name}」
                              </span>
                            ) : (
                              <button
                                type="button"
                                onClick={() => onPick(h)}
                                className="shrink-0 rounded-md bg-surface px-2 py-0.5 text-[11px] font-semibold text-blue-600 ring-1 ring-blue-200 transition hover:bg-blue-50"
                              >
                                添加
                              </button>
                            )}
                          </div>
                          {h.ports.length > 0 && (
                            <div className="mt-1.5 flex flex-wrap gap-1">
                              {h.ports.map((p) => {
                                const isTaken = taken.has(`${h.ip}:${p.port}`)
                                // 已被占用的端口灰掉不给点：同一个端口建两份，
                                // 会各自去打一个洞，对外两个地址指向同一个服务
                                if (isTaken) {
                                  return (
                                    <span
                                      key={p.port}
                                      title="这个端口已经用过了"
                                      className="rounded bg-emerald-50 px-1.5 py-0.5 text-[10px] text-emerald-600 ring-1 ring-emerald-200"
                                    >
                                      ✓ {p.port}
                                      {p.name && <span className="opacity-70"> {p.name}</span>}
                                    </span>
                                  )
                                }
                                return (
                                  <button
                                    key={p.port}
                                    type="button"
                                    onClick={() => onPickPort(h, p)}
                                    title={`用 ${h.ip}:${p.port}`}
                                    className="rounded bg-surface px-1.5 py-0.5 text-[10px] text-content ring-1 ring-line transition hover:bg-blue-50 hover:text-blue-600 hover:ring-blue-300"
                                  >
                                    {p.port}
                                    {p.name && <span className="opacity-70"> {p.name}</span>}
                                  </button>
                                )
                              })}
                            </div>
                          )}
                        </li>
                      )
                    })}
                  </ul>
                )}
              </div>
            )}
          </>
        )}

        <div className="mt-5 flex justify-end">
          <button
            type="button"
            onClick={onCancel}
            className="rounded-xl bg-surface-2 px-4 py-2 text-sm font-semibold text-content transition hover:bg-fill"
          >
            关闭
          </button>
        </div>
      </div>
    </div>
  )
}
