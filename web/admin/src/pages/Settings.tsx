import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { ExternalLink, KeyRound, Network, RefreshCw, Save, Upload } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import {
  MAX_PASSWORD_LENGTH,
  MIN_PASSWORD_LENGTH,
  changePassword,
  checkUpdate,
  getStunNetwork,
  getUpdateStatus,
  installUpdate,
  setToken,
  setUpdateConfig,
  updateStunNetwork,
  uploadUpdate,
} from '../lib/api'
import type { NetworkMode, StunNetwork, UpdateSource, UpdateStatus } from '../types'

const fieldCls =
  'w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none transition focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400'
const labelCls = 'mb-1 text-xs font-semibold text-slate-500'

export function Settings() {
  return (
    <div className="space-y-4">
      <SoftwareUpdate />
      <ChangePassword />
      <NetworkSettings />
    </div>
  )
}

function ChangePassword() {
  const [username, setUsername] = useState('')
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState('')
  const [done, setDone] = useState('')

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setDone('')
    if (!oldPassword) {
      setErr('请输入当前密码')
      return
    }
    if (newPassword.length < MIN_PASSWORD_LENGTH) {
      setErr(`新密码至少 ${MIN_PASSWORD_LENGTH} 位`)
      return
    }
    if (newPassword.length > MAX_PASSWORD_LENGTH) {
      setErr(`新密码最多 ${MAX_PASSWORD_LENGTH} 位`)
      return
    }
    if (newPassword === oldPassword) {
      setErr('新密码和当前密码一样')
      return
    }
    if (newPassword !== confirm) {
      setErr('两次输入的新密码不一致')
      return
    }
    setLoading(true)
    setErr('')
    try {
      // 后端改完密码会换掉签名密钥，旧 token 立刻作废，
      // 顺手回一个新的——不存下来这一页下一个请求就变成未登录了
      const res = await changePassword(oldPassword, newPassword, username)
      setToken(res.token)
      setOldPassword('')
      setNewPassword('')
      setConfirm('')
      setUsername('')
      setDone(
        '改好了。其他设备上原来登着的都掉线了，要用新的用户名和密码重新登录',
      )
    } catch (e) {
      setErr(e instanceof Error ? e.message : '修改失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Card>
      <CardHeader title="修改密码" />

      <div className="mb-4 rounded-xl bg-slate-50 px-3 py-2.5 text-[11px] leading-relaxed text-slate-500 ring-1 ring-slate-200/70">
        登录要同时给用户名和密码。连续错 13 次会锁 3 分钟，**这个锁是按来源 IP
        算的** —— 别人一直试只会把他自己挡住，你从别的地方照样能进来。
        <br />
        用户名不区分大小写，登录时怎么敲都认。
      </div>

      <form onSubmit={submit} className="max-w-sm space-y-3">
        <label className="block">
          <div className={labelCls}>新的用户名（留空就不改）</div>
          <input
            type="text"
            autoComplete="username"
            maxLength={32}
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="不改就留空"
            className={fieldCls}
          />
        </label>
        <label className="block">
          <div className={labelCls}>当前密码</div>
          <input
            type="password"
            autoComplete="current-password"
            value={oldPassword}
            onChange={(e) => setOldPassword(e.target.value)}
            className={fieldCls}
          />
        </label>
        <label className="block">
          <div className={labelCls}>新密码</div>
          <input
            type="password"
            autoComplete="new-password"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
            placeholder={`至少 ${MIN_PASSWORD_LENGTH} 位`}
            className={fieldCls}
          />
        </label>
        <label className="block">
          <div className={labelCls}>确认新密码</div>
          <input
            type="password"
            autoComplete="new-password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            className={fieldCls}
          />
        </label>

        {err && <div className="rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>}
        {done && (
          <div className="rounded-lg bg-emerald-50 px-3 py-2 text-xs text-emerald-600">{done}</div>
        )}

        <button
          type="submit"
          disabled={loading}
          className="flex items-center gap-1.5 rounded-xl bg-blue-500 px-4 py-2 text-sm font-bold text-white transition hover:bg-blue-600 disabled:cursor-not-allowed disabled:opacity-60"
        >
          <KeyRound className="h-4 w-4" />
          {loading ? '处理中…' : '保存新密码'}
        </button>
      </form>
    </Card>
  )
}

// ModeSwitch 三选一的分段按钮
function ModeSwitch<T extends string>({
  value,
  options,
  onChange,
}: {
  value: T
  options: { value: T; label: string }[]
  onChange: (v: T) => void
}) {
  return (
    <div className="inline-flex flex-wrap gap-1 rounded-xl bg-slate-100 p-1">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          onClick={() => onChange(o.value)}
          className={`rounded-lg px-3 py-1.5 text-xs font-semibold transition ${
            value === o.value ? 'bg-white text-blue-600 shadow-sm' : 'text-slate-500 hover:text-slate-700'
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

function NetworkSettings() {
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
        <CardHeader title="网络" />
        <div className="py-6 text-center text-sm text-slate-400">{err || '正在加载…'}</div>
      </Card>
    )
  }

  const autoIface = net.ifaces.find((i) => i.auto)
  const pickedIface = net.ifaces.find((i) => i.name === iface)

  return (
    <Card>
      <CardHeader title="网络" />

      <div className="mb-4 rounded-xl bg-slate-50 px-3 py-2.5 text-[11px] leading-relaxed text-slate-500 ring-1 ring-slate-200/70">
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
          <div className="mt-2 text-[11px] leading-relaxed text-slate-500">
            {ifaceMode === '' &&
              (autoIface ? (
                <>
                  挑关掉代理后系统本来会走的那张物理网卡，换了网络自动跟着换。现在会选{' '}
                  <span className="font-mono text-slate-700">{autoIface.name}</span>（{autoIface.localIP}）。
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
                    iface === i.name ? 'border-blue-300 bg-blue-50/60' : 'border-slate-200 hover:border-slate-300'
                  }`}
                >
                  <input
                    type="radio"
                    name="iface"
                    checked={iface === i.name}
                    onChange={() => setIface(i.name)}
                    className="accent-blue-500"
                  />
                  <span className="font-semibold text-slate-700">{i.name}</span>
                  <span className="font-mono text-slate-500">{i.localIP}</span>
                  {i.gateway && <span className="text-slate-400">网关 {i.gateway}</span>}
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
          <div className="mt-2 text-[11px] leading-relaxed text-slate-500">
            {dnsMode === '' && (
              <>
                从出口网卡直接问 <span className="font-mono text-slate-700">{net.defaultDns.join('、')}</span>
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

        <div className="flex flex-wrap items-center gap-x-1.5 text-[11px] text-slate-500">
          <Network className="h-3.5 w-3.5 text-slate-400" />
          现在从
          <span className="font-mono text-slate-700">
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
          <span className="text-[11px] text-slate-400">保存后立刻生效，出口变了会把所有打洞服务重启一遍。</span>
        </div>
      </form>
    </Card>
  )
}


const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

const formatMB = (n: number) => (n / 1024 / 1024).toFixed(1)

function SoftwareUpdate() {
  const [st, setSt] = useState<UpdateStatus | null>(null)
  const [busy, setBusy] = useState<'' | 'check' | 'install' | 'upload' | 'config'>('')
  const [err, setErr] = useState('')
  const [done, setDone] = useState('')
  const [source, setSource] = useState<UpdateSource>('')
  const [custom, setCustom] = useState('')
  const fileRef = useRef<HTMLInputElement>(null)
  // 离开设置页后停掉轮询
  const alive = useRef(true)

  const apply = useCallback((s: UpdateStatus) => {
    setSt(s)
    setSource(s.config.source)
    setCustom(s.config.custom)
  }, [])

  useEffect(() => {
    alive.current = true
    getUpdateStatus()
      .then(apply)
      .catch((e) => setErr(e instanceof Error ? e.message : '读取更新状态失败'))
    return () => {
      alive.current = false
    }
  }, [apply])

  // saveSource 内置和官方点了就存；自定义要等填好地址点保存
  const saveSource = async (next: UpdateSource, nextCustom: string) => {
    setBusy('config')
    setErr('')
    setDone('')
    try {
      apply(await setUpdateConfig({ source: next, custom: nextCustom }))
      setDone('下载源已保存')
    } catch (e) {
      setErr(e instanceof Error ? e.message : '保存失败')
    } finally {
      setBusy('')
    }
  }

  // waitRestart 等后端换成新进程再刷新页面。看启动时间而不是版本号：手动上传可能装回同一个版本
  const waitRestart = async (startedAt: string) => {
    for (let i = 0; i < 60; i++) {
      await sleep(2000)
      if (!alive.current) return
      try {
        const s = await getUpdateStatus()
        if (s.startedAt !== startedAt) {
          location.reload()
          return
        }
      } catch {
        // 还没起来，接着等
      }
    }
    throw new Error('等了两分钟还没起来，去「运行日志」或服务器上看看')
  }

  // install 下载（后端自己挑加速源）→ 替换 → 重启 → 刷新页面
  const install = async (s: UpdateStatus) => {
    setBusy('install')
    try {
      await installUpdate()
      // 下载阶段：盯进度，直到后端说要重启了
      for (;;) {
        await sleep(1000)
        if (!alive.current) return
        let now: UpdateStatus
        try {
          now = await getUpdateStatus()
        } catch {
          break // 连不上了，多半已经在重启
        }
        setSt(now)
        if (now.job.state === 'failed') throw new Error(now.job.error || '更新失败')
        if (now.job.state === 'restarting') break
      }
      await waitRestart(s.startedAt)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '更新失败')
      setBusy('')
    }
  }

  // check 查一次；有新版本、能一键更新就问一句，点确定直接开始
  const check = async () => {
    setBusy('check')
    setErr('')
    setDone('')
    let s: UpdateStatus
    try {
      s = await checkUpdate()
      setSt(s)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '检查失败')
      setBusy('')
      return
    }
    if (!s.hasUpdate) {
      setDone('已经是最新版本')
      setBusy('')
      return
    }
    if (s.manualReason) {
      setBusy('')
      return // 卡片上会显示原因和 Releases 入口
    }
    if (!confirm(`发现新版本 v${s.latest}，现在更新？\n\n会自动下载、替换并重启，所有打洞服务会断开几秒，配置不动。`)) {
      setBusy('')
      return
    }
    await install(s)
  }

  const upload = async (file: File) => {
    if (!st) return
    if (!confirm(`用 ${file.name} 替换现在的程序？\n\n换完会自动重启，所有打洞服务会断开几秒。`)) return
    setBusy('upload')
    setErr('')
    setDone('')
    try {
      await uploadUpdate(file)
      setDone('已换上，正在重启，起来后页面会自动刷新…')
      await waitRestart(st.startedAt)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '上传失败')
      setBusy('')
    }
  }

  if (!st) {
    return (
      <Card>
        <CardHeader title="软件更新" />
        <div className="py-6 text-center text-sm text-slate-400">{err || '正在加载…'}</div>
      </Card>
    )
  }

  const job = st.job
  const percent = job.total > 0 ? Math.min(100, Math.floor((job.done / job.total) * 100)) : 0

  return (
    <Card>
      <CardHeader
        title="软件更新"
        action={
          <div className="flex flex-wrap items-center gap-2">
            <a
              href={st.releaseUrl}
              target="_blank"
              rel="noreferrer"
              className="flex items-center gap-1.5 rounded-xl px-3 py-1.5 text-xs font-semibold text-slate-500 ring-1 ring-slate-200 transition hover:bg-slate-50"
            >
              <ExternalLink className="h-3.5 w-3.5" />
              Releases
            </a>
            <button
              type="button"
              onClick={check}
              disabled={busy !== ''}
              className="flex items-center gap-1.5 rounded-xl bg-blue-500 px-3 py-1.5 text-xs font-bold text-white transition hover:bg-blue-600 disabled:cursor-not-allowed disabled:opacity-60"
            >
              <RefreshCw className={`h-3.5 w-3.5 ${busy === 'check' ? 'animate-spin' : ''}`} />
              检查更新
            </button>
          </div>
        }
      />

      <div className="space-y-4">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
          <span className="text-slate-500">当前</span>
          <span className="font-mono font-semibold text-slate-700">v{st.current}</span>
          {st.hasUpdate ? (
            <span className="rounded-md bg-rose-50 px-2 py-0.5 text-xs font-bold text-rose-500">
              有新版本 v{st.latest}
            </span>
          ) : (
            st.latest && <span className="text-xs text-emerald-600">已是最新</span>
          )}
          <span className="text-[11px] text-slate-400">
            {st.checkedAt ? `上次检查 ${new Date(st.checkedAt).toLocaleString()}` : '每天自动检查一次'}
          </span>
        </div>

        {st.checkError && (
          <div className="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700">上次检查失败：{st.checkError}</div>
        )}

        {busy === 'install' && (
          <div className="space-y-1">
            <div className="h-2 overflow-hidden rounded-full bg-slate-100">
              <div
                className="h-full bg-blue-500 transition-all"
                style={{ width: `${job.state === 'restarting' ? 100 : percent}%` }}
              />
            </div>
            <div className="text-[11px] text-slate-500">
              {job.state === 'restarting'
                ? '下载完成，正在重启，起来后页面会自动刷新…'
                : `正在从 ${job.source || '…'} 下载 ${formatMB(job.done)} / ${formatMB(job.total)} MB（${percent}%）`}
            </div>
          </div>
        )}

        {st.hasUpdate && (
          <>
            {st.manualReason && (
              <div className="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700">
                {st.manualReason}。点右上角 Releases 下载，再在下面上传。
              </div>
            )}
            {!st.manualReason && busy === '' && (
              <div className="text-[11px] text-slate-500">点「检查更新」确认后会自动下载并重启。</div>
            )}
            {st.notes && (
              <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-xl bg-slate-50 px-3 py-2.5 font-sans text-[11px] leading-relaxed text-slate-600 ring-1 ring-slate-200/70">
                {st.notes}
              </pre>
            )}
          </>
        )}

        {/* 下载源 */}
        <div>
          <div className={labelCls}>下载源</div>
          <ModeSwitch
            value={source}
            onChange={(v) => {
              setSource(v)
              if (v !== 'custom') saveSource(v, custom)
            }}
            options={[
              { value: '', label: '内置加速' },
              { value: 'official', label: 'GitHub 官方' },
              { value: 'custom', label: '自定义' },
            ]}
          />
          <div className="mt-2 text-[11px] leading-relaxed text-slate-500">
            {source === '' && '依次试 ghfast.top、gh-proxy.com、gh.llkk.cc，都不通再直连 GitHub。这几个是第三方公益镜像，全失效了就换「自定义」。'}
            {source === 'official' && '只直连 GitHub。开着代理或者在海外用这个。'}
            {source === 'custom' && '填一个 GitHub 加速前缀，下载时拼在 GitHub 地址前面。'}
            {' '}不管走哪个，下载完都用 GitHub 给的 sha256 校验，镜像改不了文件。
          </div>
          {source === 'custom' && (
            <div className="mt-2 flex max-w-xl gap-2">
              <input
                value={custom}
                onChange={(e) => setCustom(e.target.value)}
                placeholder="https://ghproxy.example.com/"
                className={`${fieldCls} font-mono`}
              />
              <button
                type="button"
                onClick={() => saveSource('custom', custom.trim())}
                disabled={
                  busy !== '' ||
                  !custom.trim() ||
                  (st.config.source === 'custom' && custom.trim() === st.config.custom)
                }
                className="flex shrink-0 items-center gap-1.5 rounded-xl px-3 py-2 text-xs font-semibold text-slate-600 ring-1 ring-slate-200 transition hover:bg-slate-50 disabled:opacity-50"
              >
                <Save className="h-3.5 w-3.5" />
                保存
              </button>
            </div>
          )}
        </div>

        {/* 手动上传 */}
        <div>
          <div className={labelCls}>手动上传更新</div>
          {st.uploadReason ? (
            <div className="text-[11px] text-slate-500">{st.uploadReason}</div>
          ) : (
            <div className="flex flex-wrap items-center gap-3">
              <input
                ref={fileRef}
                type="file"
                className="hidden"
                onChange={(e) => {
                  const f = e.target.files?.[0]
                  e.target.value = '' // 同一个文件再选一次也要触发
                  if (f) upload(f)
                }}
              />
              <button
                type="button"
                onClick={() => fileRef.current?.click()}
                disabled={busy !== ''}
                className="flex items-center gap-1.5 rounded-xl px-3 py-2 text-xs font-semibold text-slate-600 ring-1 ring-slate-200 transition hover:bg-slate-50 disabled:opacity-60"
              >
                <Upload className="h-3.5 w-3.5" />
                {busy === 'upload' ? '处理中…' : '选择程序文件'}
              </button>
              <span className="text-[11px] leading-relaxed text-slate-500">
                连不上 GitHub 或想退回老版本时用。只认这台机器能跑的 Catflap
                {st.asset && (
                  <>
                    （<span className="font-mono">{st.asset}</span>）
                  </>
                )}
                ，传错了不会换。
              </span>
            </div>
          )}
        </div>

        {err && <div className="rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>}
        {done && <div className="rounded-lg bg-emerald-50 px-3 py-2 text-xs text-emerald-600">{done}</div>}
      </div>
    </Card>
  )
}
