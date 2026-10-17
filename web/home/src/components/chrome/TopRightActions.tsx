/**
 * 右上角入口区。
 *
 * 未登录只剩一颗「登录」；登录后才出现后台、退出。
 * 访客不该一眼就看到管理入口在哪 —— 所以全部收在角落里，不在中间摆按钮。
 *
 * 这些是**展示层**的判断，真正的权限在服务端：没权限的分组根本不会下发到
 * 这个页面（见 get_home_config 的过滤），所以这里就算被绕过也拿不到东西。
 */

import { LayoutDashboard, Loader2, LogIn, LogOut } from 'lucide-react'
import { logout as apiLogout } from '../../api/home'
import { clearToken } from '../../api/token'

export function TopRightActions({
  authed,
  pending,
  onLogin,
  onReload,
}: {
  authed: boolean
  pending: boolean
  onLogin: () => void
  onReload?: () => void
}) {
  const onLogout = async () => {
    try {
      await apiLogout()
    } catch {
      // 服务端登不登得掉无所谓，本地 token 一定要清 ——
      // 不清的话页面还以为自己有权限，刷新才"发现"退出了
    }
    clearToken()
    onReload?.()
    // 没给 onReload 就整页刷新，效果一样
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
          <a
            href="/catflap/"
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
