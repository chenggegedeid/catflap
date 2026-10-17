/**
 * 右上角入口区。
 *
 * 两件事必须分清（用户明确要求）：
 *   - **导航页设置**（齿轮）：分组管理、图标管理、壁纸、网络偏好。
 *     这些是"这个页面长什么样、怎么分组"的设置，就在这一页里改，
 *     不该跳去后台 —— 跳过去改一个分组名再跳回来，思路断了两次。
 *   - **管理后台**（另一个按钮）：反代、穿透、审计那些真正重的东西。
 *
 * 未登录时不显示这两颗，只留「登录」—— 访客不需要知道管理入口在哪。
 */

import { LayoutDashboard, Loader2, LogIn, LogOut, Settings } from 'lucide-react'
import { logout as apiLogout } from '../../api/home'
import { clearToken } from '../../api/token'
import { clearUnlockToken } from '../../api/unlock'

export function TopRightActions({
  authed,
  pending,
  onLogin,
  onOpenSettings,
  onReload,
}: {
  authed: boolean
  pending: boolean
  onLogin: () => void
  onOpenSettings?: () => void
  onReload?: () => void
}) {
  const onLogout = async () => {
    try {
      await apiLogout()
    } catch {
      // 服务端登不登得掉无所谓，本地凭据一定要清 ——
      // 不清的话页面还以为自己有权限，刷新才"发现"退出了
    }
    clearToken()
    // 解锁 token 也要清：换了人用这台机器，不该继承上一个人的解锁状态
    clearUnlockToken()
    onReload?.()
    if (!onReload) window.location.reload()
  }

  return (
    <div className="mb-2 flex items-center justify-end gap-2">
      {/* 有写操作在跑时给个安静的指示 —— 不弹 toast，动作本身已经有反馈了 */}
      {pending && (
        <span className="flex items-center gap-1 text-[11px] text-white/70">
          <Loader2 className="h-3 w-3 animate-spin" />
          保存中
        </span>
      )}

      {authed ? (
        <>
          <button
            type="button"
            onClick={onOpenSettings}
            title="导航页设置：分组、图标、壁纸"
            className="flex items-center gap-1.5 rounded-xl bg-white/25 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-white/40"
          >
            <Settings className="h-3.5 w-3.5" />
            设置
          </button>
          <a
            href="/catflap/"
            title="反代 / 穿透 / 审计等"
            className="flex items-center gap-1.5 rounded-xl bg-white/25 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-white/40"
          >
            <LayoutDashboard className="h-3.5 w-3.5" />
            管理后台
          </a>
          <button
            type="button"
            onClick={onLogout}
            className="flex items-center gap-1.5 rounded-xl bg-white/25 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-white/40"
          >
            <LogOut className="h-3.5 w-3.5" />
            退出
          </button>
        </>
      ) : (
        <button
          type="button"
          onClick={onLogin}
          className="flex items-center gap-1.5 rounded-xl bg-white/25 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-white/40"
        >
          <LogIn className="h-3.5 w-3.5" />
          登录
        </button>
      )}
    </div>
  )
}
