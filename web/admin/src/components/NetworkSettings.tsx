/**
 * 出口网络设置：挑哪张网卡出去、用什么 DNS 解析 STUN 服务器。
 *
 * 原来挂在「系统设置」页，现在挪到**设备页** —— 因为这两件事都是"出口"相关：
 * 设备页管内网有哪些机器、哪条线路出去；网络设置管 Catflap 自己从哪张卡出去。
 * 放在一起，排查"为什么打洞拿到的是代理 IP"时不用两处跑。
 *
 * **默认折叠**：设备加到几十台时页面本身就很长了，这张卡再展开会把设备列表
 * 推到很下面。默认只占一行标题，点开才展开。
 */
import { useCallback, useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { ChevronDown, Network, Save } from 'lucide-react'
import { Card, CardHeader } from './Card'
import { ModeSwitch } from './ModeSwitch'
import { getStunNetwork, updateStunNetwork } from '../lib/api'
import type { NetworkMode, StunNetwork } from '../types'

const fieldCls =
  'w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none transition focus:border-blue-400 disabled:bg-surface-2 disabled:text-muted'
const labelCls = 'mb-1 text-xs font-semibold text-muted'

export function NetworkSettings() {
  // 默认收起：设备多了这张卡不该霸占首屏
  const [open, setOpen] = useState(false)
  const [net, setNet] = useState<StunNetwork | null>(null)
  const [ifaceMode, setIfaceMode] = useState<NetworkMode>('')
  const [iface, setIface] = useState('')
  const [dnsMode, setDnsMode] = useState<NetworkMode>('')
  const [dns, setDns] = useState('')
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState('')
  const [done, setDone] = useState('')

  const apply = useCallback((n: StunNetwork) => {
    setNet(n)
    setIfaceMode(n.config.ifaceMode)
    setIface(n.config.iface)
    setDnsMode(n.config.dnsMode)
    setDns(n.config.dns.join('\n'))
  }, [])

  useEffect(() => {
    getStunNetwork()
      .then(apply)
      .catch((e) => setErr(e instanceof Error ? e.message : '读取网络设置失败'))
  }, [apply])

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setErr('')
    setDone('')
    if (ifaceMode === 'custom' && !iface) {
      setErr('选一张网卡')
      return
    }
    const dnsList = dns
      .split(/[\s,，]+/)
      .map((s) => s.trim())
      .filter(Boolean)
    if (dnsMode === 'custom' && dnsList.length === 0) {
      setErr('至少填一个 DNS 地址')
      return
    }
    setLoading(true)
    try {
      const { msg } = await updateStunNetwork({ ifaceMode, iface, dnsMode, dns: dnsList })
      setDone(msg)
      apply(await getStunNetwork())
    } catch (e) {
      setErr(e instanceof Error ? e.message : '保存失败')
    } finally {
      setLoading(false)
    }
  }

  if (!net) {
    return (
      <Card>
        <CardHeader
          title="出口网络"
          action={
            <button type="button" onClick={() => setOpen(true)} className="text-[11px] font-semibold text-blue-600">
              展开
            </button>
          }
        />
        <div className="py-4 text-center text-xs text-muted">{err || '正在加载…'}</div>
      </Card>
    )
  }

  const autoIface = net.ifaces.find((i) => i.auto)
  const pickedIface = net.ifaces.find((i) => i.name === iface)

  return (
    <Card>
      {/*
        折叠开关放在标题右侧：设备多了页面本来就长，这张卡默认收起，
        只占一行；要改出口网卡/DNS 时点一下展开。
      */}
      <div className="px-5 pt-4">
        <CardHeader
          title="出口网络"
          action={
            <button
              type="button"
              onClick={() => setOpen((v) => !v)}
              aria-expanded={open}
              aria-label={open ? '收起网络设置' : '展开网络设置'}
              className="flex items-center gap-1 text-[11px] font-semibold text-blue-600 transition hover:text-blue-700"
            >
              {open ? '收起' : '展开'}
              <ChevronDown className={`h-3.5 w-3.5 transition-transform ${open ? 'rotate-180' : ''}`} />
            </button>
          }
        />
      </div>

      {!open ? (
        // 收起时也给一句当前出口，不用展开就能知道现在从哪张卡出去
        <div className="px-5 pb-5">
          <div className="flex flex-wrap items-center gap-x-1.5 text-[11px] text-muted">
            <Network className="h-3.5 w-3.5 shrink-0 text-muted" />
            现在从
            <span className="font-mono text-content">
              {net.current.localIP ? `${net.current.name || '系统路由'}（${net.current.localIP}）` : '——'}
            </span>
            出去
          </div>
        </div>
      ) : (
        <div className="px-5 pb-5">

      <div className="mb-4 rounded-xl bg-surface-2 px-3 py-2.5 text-[11px] leading-relaxed text-muted ring-1 ring-line/70">
        只管打洞这一块：查公网 IP、STUN、UPnP、NAT 检测。DDNS、证书、Webhook 照常走系统网络。
        <br />
        开着 Clash / sing-box 的 TUN 时，不指定出口的连接会被代理接走，查到的就是代理节点的 IP。保持「自动」就能绕开。
      </div>

      <form onSubmit={submit} className="max-w-xl space-y-5">
        {/* 出口网卡 */}
        <div>
          <div className={labelCls}>出口网卡</div>
          <ModeSwitch
            value={ifaceMode}
            onChange={setIfaceMode}
            options={[
              { value: '', label: '自动' },
              { value: 'system', label: '跟随系统' },
              { value: 'custom', label: '指定网卡' },
            ]}
          />
          <div className="mt-2 text-[11px] leading-relaxed text-muted">
            {ifaceMode === '' &&
              (autoIface ? (
                <>
                  挑关掉代理后系统本来会走的那张物理网卡，换了网络自动跟着换。现在会选{' '}
                  <span className="font-mono text-content">{autoIface.name}</span>（{autoIface.localIP}）。
                </>
              ) : (
                <span className="text-rose-500">挑关掉代理后系统本来会走的那张物理网卡。现在一张能用的都没有。</span>
              ))}
            {ifaceMode === 'system' && (
              <span className="text-amber-600">
                不绑网卡，按系统路由走。开着代理 TUN 时打洞会从代理出去，公网 IP 也会变成代理节点的。
              </span>
            )}
            {ifaceMode === 'custom' && '两条宽带时用来选线路。这张卡断了打洞会停下来等它，不会自己换到别的卡。'}
          </div>

          {ifaceMode === 'custom' && (
            <div className="mt-2 space-y-1.5">
              {net.ifaces.map((i) => (
                <label
                  key={i.name}
                  className={`flex cursor-pointer flex-wrap items-center gap-x-3 gap-y-1 rounded-xl border px-3 py-2 text-xs transition ${
                    iface === i.name ? 'border-blue-300 bg-blue-50/60' : 'border-line hover:border-line'
                  }`}
                >
                  <input
                    type="radio"
                    name="iface"
                    checked={iface === i.name}
                    onChange={() => setIface(i.name)}
                    className="accent-blue-500"
                  />
                  <span className="font-semibold text-content">{i.name}</span>
                  <span className="font-mono text-muted">{i.localIP}</span>
                  {i.gateway && <span className="text-muted">网关 {i.gateway}</span>}
                  {i.auto && <span className="rounded bg-emerald-50 px-1.5 py-0.5 text-emerald-600">自动会选</span>}
                  {i.tunnel && <span className="rounded bg-amber-50 px-1.5 py-0.5 text-amber-600">隧道</span>}
                </label>
              ))}
              {/* 存的网卡现在不在列表里（拔了 / 没连上），也得让人看见选的是哪张 */}
              {iface && !pickedIface && (
                <div className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-600">
                  之前选的 {iface} 现在没连上
                </div>
              )}
            </div>
          )}
        </div>

        {/* DNS */}
        <div>
          <div className={labelCls}>解析 STUN 服务器用的 DNS</div>
          <ModeSwitch
            value={dnsMode}
            onChange={setDnsMode}
            options={[
              { value: '', label: '内置' },
              { value: 'system', label: '跟随系统' },
              { value: 'custom', label: '自定义' },
            ]}
          />
          <div className="mt-2 text-[11px] leading-relaxed text-muted">
            {dnsMode === '' && (
              <>
                从出口网卡直接问 <span className="font-mono text-content">{net.defaultDns.join('、')}</span>
                ，绕开代理的 fake-ip。
              </>
            )}
            {dnsMode === 'system' && (
              <span className="text-amber-600">代理开着 fake-ip 时，STUN 服务器会被解析成 198.18.x.x，从物理网卡连不上。</span>
            )}
            {dnsMode === 'custom' && '一行一个，只认 IPv4，可以带端口，比如路由器的 192.168.1.1 或 192.168.1.1:5353。'}
          </div>
          {dnsMode === 'custom' && (
            <textarea
              value={dns}
              onChange={(e) => setDns(e.target.value)}
              rows={3}
              placeholder={net.defaultDns.join('\n')}
              className={`${fieldCls} mt-2 font-mono`}
            />
          )}
        </div>

        <div className="flex flex-wrap items-center gap-x-1.5 text-[11px] text-muted">
          <Network className="h-3.5 w-3.5 text-muted" />
          现在从
          <span className="font-mono text-content">
            {net.current.localIP ? `${net.current.name || '系统路由'}（${net.current.localIP}）` : '——'}
          </span>
          出去
        </div>

        {err && <div className="rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>}
        {done && <div className="rounded-lg bg-emerald-50 px-3 py-2 text-xs text-emerald-600">{done}</div>}

        <div className="flex flex-wrap items-center gap-3">
          <button
            type="submit"
            disabled={loading}
            className="flex items-center gap-1.5 rounded-xl bg-blue-500 px-4 py-2 text-sm font-bold text-white transition hover:bg-blue-600 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <Save className="h-4 w-4" />
            {loading ? '保存中…' : '保存'}
          </button>
          <span className="text-[11px] text-muted">保存后立刻生效，出口变了会把所有打洞服务重启一遍。</span>
        </div>
      </form>
        </div>
      )}
    </Card>
  )
}
