/**
 * Cloudflare Tunnel 面板。
 *
 * 为什么值得单独一块：没有 IPv4 公网（运营商 CGNAT）时，打洞端口是 NAT 给的、
 * URL 必须带端口；而 Tunnel 是 cloudflared 主动往外连 CF，入口在 CF 的 443，
 * 于是地址干净（不带端口）、不用打洞、不用 DDNS、路由器也不用放行。
 *
 * 这里只做「托管用户自己装的 cloudflared」：填路径 + token，Catflap 拉起、
 * 看住、停掉。token 是隧道凭证，回显用掩码，留空提交=不改动。
 */
import { useCallback, useEffect, useState } from 'react'
import { Cloud, RefreshCw } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import * as api from '../lib/api'
import type { TunnelConfigView } from '../types'

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e))

export function Tunnel() {
  const [cfg, setCfg] = useState<TunnelConfigView | null>(null)
  const [loadErr, setLoadErr] = useState('')
  const [binaryPath, setBinaryPath] = useState('')
  const [token, setToken] = useState('')
  const [extraArgs, setExtraArgs] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const refresh = useCallback(async () => {
    try {
      const data = await api.getTunnelConfig()
      setCfg(data)
      setBinaryPath(data.binaryPath ?? '')
      setExtraArgs(data.extraArgs ?? '')
      setLoadErr('')
    } catch (e) {
      setLoadErr(e instanceof Error ? e.message : '加载失败')
    }
  }, [])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const tunnel = cfg?.state

  const save = async (enabled: boolean) => {
    setErr('')
    setBusy(true)
    try {
      await api.saveTunnel({
        enabled,
        binaryPath: binaryPath.trim(),
        // 掩码/空串都表示"不改动"，后端会跳过
        token,
        clearToken: false,
        extraArgs: extraArgs.trim(),
      })
      setToken('')
      await refresh()
    } catch (e) {
      setErr(errText(e))
    } finally {
      setBusy(false)
    }
  }

  const restart = async () => {
    setErr('')
    setBusy(true)
    try {
      await api.restartTunnel()
      await refresh()
    } catch (e) {
      setErr(errText(e))
    } finally {
      setBusy(false)
    }
  }

  // 首屏还没拿到配置：给个加载态，别让下面那一堆 tunnel.xxx 空指针
  if (!tunnel) {
    return (
      <Card>
        <div className="text-sm text-slate-400">{loadErr || '正在加载隧道状态…'}</div>
      </Card>
    )
  }

  const on = tunnel.running

  return (
    <Card className="!py-3.5">
      <CardHeader
        title="Cloudflare Tunnel"
        action={
          <div className="flex items-center gap-2">
            {tunnel.tokenSet && (
              <button
                onClick={restart}
                disabled={busy}
                title="重启隧道进程"
                className="flex items-center gap-1 rounded-lg px-2 py-1 text-xs font-semibold text-slate-500 hover:bg-slate-100 disabled:opacity-50"
              >
                <RefreshCw className={`h-3 w-3 ${busy ? 'animate-spin' : ''}`} /> 重启
              </button>
            )}
            {on ? (
              <button
                onClick={() => save(false)}
                disabled={busy}
                className="rounded-lg px-2.5 py-1 text-xs font-semibold text-slate-500 hover:bg-slate-100 disabled:opacity-50"
              >
                停止
              </button>
            ) : (
              <button
                onClick={() => save(true)}
                disabled={busy}
                className="rounded-lg bg-blue-500 px-2.5 py-1 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 hover:bg-blue-600 disabled:opacity-50"
              >
                启动
              </button>
            )}
          </div>
        }
      />

      <div className="px-1 pb-1 text-[11px] leading-relaxed text-slate-400">
        CGNAT 下 URL 不带端口的最干净方案：cloudflared 主动外连 Cloudflare，不用打洞、不用 DDNS、路由器不用放行
      </div>

      <div className="flex flex-wrap items-center gap-x-3 gap-y-2 px-1">
        <span className={`h-2 w-2 shrink-0 rounded-full ${on ? 'bg-emerald-500' : 'bg-slate-300'}`} />
        <span className="text-sm font-semibold text-slate-700">{on ? '隧道运行中' : '未运行'}</span>
        {tunnel.tokenSet ? (
          <span className="rounded-md bg-slate-100 px-1.5 py-0.5 font-mono text-[11px] text-slate-500">token 已配置</span>
        ) : (
          <span className="rounded-md bg-amber-50 px-1.5 py-0.5 text-[11px] text-amber-600">还没填 token</span>
        )}
        {tunnel.message && <span className="text-[11px] text-slate-400">{tunnel.message}</span>}
      </div>

      <div className="mt-3 space-y-3 rounded-xl bg-slate-50 p-3">
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">cloudflared 路径（留空 = 在 PATH 里找）</div>
            <input
              value={binaryPath}
              onChange={(e) => setBinaryPath(e.target.value)}
              placeholder="/usr/local/bin/cloudflared"
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 font-mono text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">
              隧道 token{tunnel.tokenSet && '（留空 = 不改动）'}
            </div>
            <input
              type="password"
              value={token}
              onChange={(e) => setToken(e.target.value)}
              placeholder={tunnel.tokenSet ? '••••••••••••' : '在 Cloudflare Zero Trust → Tunnels 创建后粘贴'}
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 font-mono text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">额外参数（可选）</div>
            <input
              value={extraArgs}
              onChange={(e) => setExtraArgs(e.target.value)}
              placeholder="如 --protocol quic"
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 font-mono text-sm outline-none focus:border-blue-400"
            />
          </label>
          <div className="rounded-xl bg-blue-50/70 px-3 py-2 text-[11px] leading-relaxed text-slate-600 ring-1 ring-blue-200/70">
            还需要两步（在 Cloudflare 后台做）：① Zero Trust → Tunnels 里创建隧道、
            把内网服务（如 <span className="font-mono">http://localhost:8443</span>）映射到
            你的域名；② 域名的解析交给这条隧道（CF 会自动建 CNAME）。隧道里没写的域名
            不会生效。
          </div>
          {err && <div className="rounded-xl bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>}
          <div className="flex justify-end gap-2">
            <button
              onClick={() => save(tunnel.running || tunnel.tokenSet)}
              disabled={busy}
              className="rounded-xl bg-blue-500 px-3 py-1.5 text-xs font-semibold text-white hover:bg-blue-600 disabled:opacity-50"
            >
              {busy ? '保存中...' : '保存设置'}
            </button>
          </div>
        <div className="flex items-center gap-1.5 px-1 text-[11px] text-slate-300">
          <Cloud className="h-3 w-3" /> token 是隧道的全部凭证，按密码对待：面板上只显示是否已配置
        </div>
      </div>
    </Card>
  )
}
