import { useCallback, useEffect, useRef, useState, type FormEvent } from 'react'
import { ExternalLink, KeyRound, RefreshCw, Save, Upload } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import { ModeSwitch } from '../components/ModeSwitch'
import {
  MAX_PASSWORD_LENGTH,
  MIN_PASSWORD_LENGTH,
  cancelUpdate,
  changePassword,
  checkUpdate,
  getProxyConfig,
  getUpdateStatus,
  installUpdate,
  saveErrorPage,
  saveExposeAdmin,
  setToken,
  setUpdateConfig,
  setUsername as saveUsername,
  uploadUpdate,
} from '../lib/api'
import type { UpdateSource, UpdateStatus } from '../types'

const fieldCls =
  'w-full rounded-xl border border-line bg-surface px-3 py-2 text-sm outline-none transition focus:border-blue-400 disabled:bg-surface-2 disabled:text-muted'
const labelCls = 'mb-1 text-xs font-semibold text-muted'

export function Settings() {
  return (
    <div className="space-y-4">
      <SoftwareUpdate />
      {/*
        这两张并排：改密码的表单本来就窄（max-w-sm），
        一个人占一整行右边空一大片，看着像页面没排完。
        窄屏（手机）自动回到上下堆叠。
      */}
      <div className="grid gap-4 lg:grid-cols-2">
        <ChangePassword />
        <ErrorPageSettings />
      </div>
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
      // 顶栏头像旁边显示的是它。必须写回去，否则改了用户名界面纹丝不动
      saveUsername(res.username)
      setOldPassword('')
      setNewPassword('')
      setConfirm('')
      setUsername('')
      setDone(
        '改好了，右上角的用户名已经跟着变。其他设备上原来登着的都掉线了，要用新的用户名和密码重新登录',
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

      <div className="mb-4 rounded-xl bg-surface-2 px-3 py-2.5 text-[11px] leading-relaxed text-muted ring-1 ring-line/70">
        登录要同时给用户名和密码。连续错 5 次会把来源 IP 封禁 1 小时（阈值和时长可在后台配置里改），
        **这个锁是按来源 IP 算的** —— 别人一直试只会把他自己挡住，你从别的地方照样能进来。
        被封的来源在「访问审计」页能手动解封；把你的出口 IP 填进「永不封禁」名单就永远不会被误封。
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

/**
 * 错误页设置 —— 外网访客看到 404 / 502 时是什么样子。
 *
 * 默认是「不透露」：不列已配置的域名、不说后端地址、不出现产品名。
 * 以前那个页面会把域名清单直接打给任何带着任意 Host 头来的请求，
 * 等于替人做了一遍子域名枚举 —— 这一页就是把那个口子关掉、并留一个
 * 愿意自己写页面时的出口。
 */
function ErrorPageSettings() {
  const [mode, setMode] = useState('')
  const [html, setHtml] = useState('')
  const [savedMode, setSavedMode] = useState('')
  const [savedHtml, setSavedHtml] = useState('')
  // 远程管理开关（默认关，打开后站点后端才允许填后台端口）
  const [exposeAdmin, setExposeAdmin] = useState(false)
  const [loading, setLoading] = useState(false)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  // 两块状态分开显示，跟着各自的操作走：
  //   - 改「错误页」要点保存 → 提示显示在保存按钮下面
  //   - 拨「远程管理」开关是**即时生效、没有保存按钮** → 提示要贴在那个开关下面，
  //     否则用户看到开关下方一片绿，会以为那是「还没保存」的意思，跑去按钮下面找
  const [done, setDone] = useState('') // 错误页保存成功
  const [exposeDone, setExposeDone] = useState('') // 远程管理拨动结果

  useEffect(() => {
    getProxyConfig()
      .then((cfg) => {
        const p = cfg.errorPage || { mode: '' }
        setMode(p.mode || '')
        setHtml(p.customHtml || '')
        setSavedMode(p.mode || '')
        setSavedHtml(p.customHtml || '')
        setExposeAdmin(!!cfg.exposeAdmin)
      })
      .catch((e) => setErr(e instanceof Error ? e.message : '读取失败'))
      .finally(() => setLoading(false))
  }, [])

  // 开远程管理是**危险操作**，单独一把处理，不跟错误页共用保存按钮 ——
  // 免得改个壁纸顺手把后台挂出去
  const toggleExpose = async () => {
    const next = !exposeAdmin
    if (
      next &&
      !confirm(
        '打开之后，站点后端就可以填 Catflap 自己的后台端口了 —— 那等于把登录页挂到公网上，' +
          '任何人都能来试密码。\n\n' +
          '打开前请确认：审计页「永不封禁」名单里已经填了你自己的出口 IP。' +
          '否则别人故意输错几次密码，就能把你关在门外。\n\n' +
          '确定要打开吗？',
      )
    ) {
      return
    }
    setBusy(true)
    setErr('')
    setExposeDone('')
    try {
      const { msg } = await saveExposeAdmin(next)
      setExposeAdmin(next)
      // 后端回的「已保存」换成说清楚结果的话：这个开关没有保存按钮，
      // 提示得让用户知道「已经生效了」，而不是「存好了等你下次再点保存」。
      setExposeDone(msg && !/已保存/.test(msg) ? msg : next ? '已打开，立即生效' : '已关闭，立即生效')
    } catch (e) {
      setErr(e instanceof Error ? e.message : '保存失败')
    } finally {
      setBusy(false)
    }
  }

  const dirty = mode !== savedMode || (mode === 'custom' && html !== savedHtml)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setErr('')
    setDone('')
    if (mode === 'custom' && !html.trim()) {
      setErr('选了自定义就要填页面内容')
      return
    }
    setBusy(true)
    try {
      const { msg } = await saveErrorPage({ mode, customHtml: html })
      // 切回默认时后端会把内容清掉，界面跟着清，不然框里留着旧内容、下次切回去
      // 会突然冒出来
      const keptHtml = mode === 'custom' ? html : ''
      setSavedMode(mode)
      setSavedHtml(keptHtml)
      setHtml(keptHtml)
      setDone(msg || '已保存')
    } catch (e) {
      setErr(e instanceof Error ? e.message : '保存失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Card>
      <CardHeader title="外网访问" />

      <div className="mb-4 rounded-xl bg-surface-2 px-3 py-2.5 text-[11px] leading-relaxed text-muted ring-1 ring-line/70">
        别人访问到不存在的域名、或者后端连不上时看到的那页。
        <br />
        <span className="font-semibold text-content">默认那页什么都不透露</span>
        ：不列你配了哪些域名、不说后端地址在哪、也不写这是什么软件 —— 谁都能请求到它，
        写多了等于替人枚举一遍站点。真要查为什么连不上，去「运行日志」看，那里每条都记着。
      </div>

      {loading ? (
        <div className="py-6 text-center text-sm text-muted">正在加载…</div>
      ) : (
        <>
          <form onSubmit={submit} className="space-y-3">
            <div>
              <div className={labelCls}>显示方式</div>
              <ModeSwitch
                value={mode}
                onChange={setMode}
                options={[
                  { value: '', label: '默认（不透露）' },
                  { value: 'custom', label: '自定义 HTML' },
                ]}
              />
              <div className="mt-2 text-[11px] leading-relaxed text-muted">
                {mode === 'custom'
                  ? '整页 HTML，支持两个占位符：{{status}}（404 / 502）、{{message}}（一句说明）。想伪装成什么站点都行。'
                  : '一张中性卡片，只说「没有找到」或「服务暂时不可用」，看不出是谁家的服务。'}
              </div>
            </div>

            {mode === 'custom' && (
              <label className="block">
                <div className={labelCls}>页面内容（HTML）</div>
                <textarea
                  value={html}
                  onChange={(e) => setHtml(e.target.value)}
                  rows={7}
                  spellCheck={false}
                  placeholder={`<!DOCTYPE html>
<html><body style="font-family:system-ui;text-align:center;padding:4rem">
  <h1>{{status}}</h1>
  <p>{{message}}</p>
</body></html>`}
                  className={`${fieldCls} font-mono text-xs`}
                />
              </label>
            )}

            {err && (
              <div className="rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>
            )}
            {done && (
              <div className="rounded-lg bg-emerald-50 px-3 py-2 text-xs text-emerald-600">
                {done}
              </div>
            )}

            <button
              type="submit"
              disabled={busy || !dirty}
              className="flex items-center gap-1.5 rounded-xl bg-blue-500 px-4 py-2 text-sm font-bold text-white transition hover:bg-blue-600 disabled:cursor-not-allowed disabled:opacity-60"
            >
              <Save className="h-4 w-4" />
              {busy ? '保存中…' : '保存'}
            </button>
          </form>

          {/*
            远程管理**不在上面那个 form 里**。它是拨一下就生效的开关，和
            「改完再点保存」是两种交互；放在 form 里会让用户以为必须点一下
            下面那个「保存」才作数（或者反过来，以为保存按钮也管它）。
            和错误页放同一张卡，是因为它们回答的是同一个问题：对外暴露什么。
          */}
          <div className="mt-4 border-t border-line pt-3">
            <div className="flex items-start gap-3">
              <button
                type="button"
                role="switch"
                aria-checked={exposeAdmin}
                aria-label="允许通过反代暴露管理后台"
                onClick={toggleExpose}
                disabled={busy}
                className={`mt-0.5 flex h-6 w-11 shrink-0 items-center rounded-full px-0.5 transition ${
                  exposeAdmin ? 'bg-blue-500' : 'bg-fill'
                } ${busy ? 'opacity-60' : ''}`}
              >
                <span
                  className={`h-5 w-5 rounded-full bg-surface shadow transition-transform ${
                    exposeAdmin ? 'translate-x-5' : 'translate-x-0'
                  }`}
                />
              </button>
              <div className="min-w-0">
                <div className="flex flex-wrap items-center gap-2">
                  <span className="text-sm font-semibold text-content">
                    允许通过反代暴露管理后台
                  </span>
                  {/* 明说是即时生效的，省得用户去找保存按钮 */}
                  <span className="rounded-md bg-surface-2 px-1.5 py-0.5 text-[10px] font-semibold text-muted">
                    拨动即生效
                  </span>
                </div>
                <div className="mt-1 text-[11px] leading-relaxed text-muted">
                  关着的时候，站点后端不能填 Catflap 自己的后台端口 —— 那是防手滑用的。
                  打开后才能用「反代 → 后台」这条路从外面管理。
                  <br />
                  <span className={exposeAdmin ? 'text-amber-600' : 'text-muted'}>
                    {exposeAdmin
                      ? '现在是开着的：登录页对对应站点可见。请确认审计页「永不封禁」名单里有你自己的出口 IP。'
                      : '打开后，登录失败 5 次会自动封禁来源（阈值和时长可在后台配置里改）。'}
                  </span>
                </div>
                {exposeDone && (
                  <div className="mt-2 rounded-lg bg-emerald-50 px-3 py-2 text-xs text-emerald-600">
                    {exposeDone}
                  </div>
                )}
              </div>
            </div>
          </div>
        </>
      )}
    </Card>
  )
}



const sleep = (ms: number) => new Promise((r) => setTimeout(r, ms))

const formatMB = (n: number) => (n / 1024 / 1024).toFixed(1)

function SoftwareUpdate() {
  const [st, setSt] = useState<UpdateStatus | null>(null)
  const [busy, setBusy] = useState<'' | 'check' | 'install' | 'upload' | 'config' | 'cancel'>('')
  const [err, setErr] = useState('')
  const [done, setDone] = useState('')
  const [source, setSource] = useState<UpdateSource>('')
  const [custom, setCustom] = useState('')
  const [proxy, setProxy] = useState('')
  const fileRef = useRef<HTMLInputElement>(null)
  // 离开设置页后停掉轮询
  const alive = useRef(true)

  const apply = useCallback((s: UpdateStatus) => {
    setSt(s)
    setSource(s.config.source)
    setCustom(s.config.custom)
    setProxy(s.config.proxy ?? '')
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
  //
  // **必须把 proxy 一起带上**：后端 SetConfig 是整份覆盖，
  // 少传一个字段就等于把它清空 —— 改下载源会把代理悄悄抹掉。
  const saveSource = async (next: UpdateSource, nextCustom: string) => {
    setBusy('config')
    setErr('')
    setDone('')
    try {
      apply(await setUpdateConfig({ source: next, custom: nextCustom, proxy }))
      setDone('下载源已保存')
    } catch (e) {
      setErr(e instanceof Error ? e.message : '保存失败')
    } finally {
      setBusy('')
    }
  }

  // saveProxy 同理，source / custom 也要原样带回去
  const saveProxy = async () => {
    setBusy('config')
    setErr('')
    setDone('')
    try {
      apply(await setUpdateConfig({ source, custom, proxy: proxy.trim() }))
      setDone('代理设置已保存')
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

  // cancel 取消正在进行的更新，或者清掉一个卡住的任务。
  //
  // 下载慢、源不通、或者上一次卡在"正在重启"没走完的时候用它，
  // 之后就能重新点更新 / 手动上传了。
  const cancel = async () => {
    if (!confirm('取消这次更新？\n\n已经下了一半会丢掉，之后可以重新点更新或手动上传。')) return
    setBusy('cancel')
    setErr('')
    setDone('')
    try {
      apply(await cancelUpdate())
      setDone('已取消')
    } catch (e) {
      setErr(e instanceof Error ? e.message : '取消失败')
    } finally {
      setBusy('')
    }
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
        <div className="py-6 text-center text-sm text-muted">{err || '正在加载…'}</div>
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
              className="flex items-center gap-1.5 rounded-xl px-3 py-1.5 text-xs font-semibold text-muted ring-1 ring-line transition hover:bg-surface-2"
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
          <span className="text-muted">当前</span>
          <span className="font-mono font-semibold text-content">v{st.current}</span>
          {st.hasUpdate ? (
            <span className="rounded-md bg-rose-50 px-2 py-0.5 text-xs font-bold text-rose-500">
              有新版本 v{st.latest}
            </span>
          ) : (
            st.latest && <span className="text-xs text-emerald-600">已是最新</span>
          )}
          <span className="text-[11px] text-muted">
            {st.checkedAt ? `上次检查 ${new Date(st.checkedAt).toLocaleString()}` : '每天自动检查一次'}
          </span>
        </div>

        {st.checkError && (
          <div className="rounded-lg bg-amber-50 px-3 py-2 text-xs text-amber-700">上次检查失败：{st.checkError}</div>
        )}

        {/*
          进度条按**服务端返回的 job.state** 显示，不再看本地的 busy。

          以前是 busy === 'install' 才显示：busy 是组件内的 state，
          一刷新页面就变回空，明明后端还在下载，界面上却什么都不剩 ——
          用户以为更新没了，再点一次还被回「已经在更新了」。
          现在刷新后从接口重新拿到 job，进度照常接着走。
        */}
        {(job.state === 'downloading' || job.state === 'restarting' || job.state === 'failed') && (
          <div className="space-y-1">
            <div className="h-2 overflow-hidden rounded-full bg-surface-2">
              <div
                className={`h-full transition-all ${
                  job.state === 'failed' ? 'bg-rose-500' : 'bg-blue-500'
                }`}
                style={{ width: `${job.state === 'restarting' ? 100 : percent}%` }}
              />
            </div>
            <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
              <span className="text-[11px] text-muted">
                {job.state === 'restarting'
                  ? '下载完成，正在重启，起来后页面会自动刷新…'
                  : job.state === 'failed'
                    ? `更新失败：${job.error || '未知原因'}`
                    : `正在从 ${job.source || '…'} 下载 ${formatMB(job.done)} / ${formatMB(job.total)} MB（${percent}%）`}
              </span>
              {/* 取消 / 清掉卡住的任务。下载慢或者卡住时点它就能重新来过 */}
              <button
                type="button"
                onClick={cancel}
                disabled={busy === 'cancel'}
                className="text-[11px] font-semibold text-rose-500 underline-offset-2 transition hover:underline disabled:opacity-50"
              >
                {busy === 'cancel' ? '正在取消…' : '取消'}
              </button>
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
              <div className="text-[11px] text-muted">点「检查更新」确认后会自动下载并重启。</div>
            )}
            {st.notes && (
              <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-words rounded-xl bg-surface-2 px-3 py-2.5 font-sans text-[11px] leading-relaxed text-content ring-1 ring-line/70">
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
          <div className="mt-2 text-[11px] leading-relaxed text-muted">
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
                className="flex shrink-0 items-center gap-1.5 rounded-xl px-3 py-2 text-xs font-semibold text-content ring-1 ring-line transition hover:bg-surface-2 disabled:opacity-50"
              >
                <Save className="h-3.5 w-3.5" />
                保存
              </button>
            </div>
          )}
        </div>

        {/* 更新代理 */}
        <div>
          <div className={labelCls}>更新代理（可选）</div>
          <div className="mt-2 flex max-w-xl gap-2">
            <input
              value={proxy}
              onChange={(e) => setProxy(e.target.value)}
              placeholder="http://127.0.0.1:7890 或 socks5://127.0.0.1:1080"
              spellCheck={false}
              autoComplete="off"
              className={`${fieldCls} font-mono`}
            />
            <button
              type="button"
              onClick={saveProxy}
              disabled={busy !== '' || proxy.trim() === (st?.config.proxy ?? '')}
              className="flex shrink-0 items-center gap-1.5 rounded-xl px-3 py-2 text-xs font-semibold text-content ring-1 ring-line transition hover:bg-surface-2 disabled:opacity-50"
            >
              <Save className="h-3.5 w-3.5" />
              保存
            </button>
          </div>
          <div className="mt-2 text-[11px] leading-relaxed text-muted">
            查版本和下载更新包都会走这个代理。需要认证的写法：
            <code className="mx-1 rounded bg-surface-2 px-1 font-mono">http://用户名:密码@127.0.0.1:7890</code>
            <br />
            留空表示听环境变量的（HTTP_PROXY / HTTPS_PROXY）。
            <strong className="font-semibold"> 只对更新生效</strong>
            —— 反代和穿透的目标是内网地址，不会走它。
          </div>
        </div>

        {/* 手动上传 */}
        <div>
          <div className={labelCls}>手动上传更新</div>
          {st.uploadReason ? (
            <div className="text-[11px] text-muted">{st.uploadReason}</div>
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
                className="flex items-center gap-1.5 rounded-xl px-3 py-2 text-xs font-semibold text-content ring-1 ring-line transition hover:bg-surface-2 disabled:opacity-60"
              >
                <Upload className="h-3.5 w-3.5" />
                {busy === 'upload' ? '处理中…' : '选择程序文件'}
              </button>
              <span className="text-[11px] leading-relaxed text-muted">
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
