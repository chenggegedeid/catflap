/**
 * ServiceCard 穿透 / 反向代理 / 端口转发**共用的一张服务卡**。
 *
 * 为什么要抽这一个：三个页面原本各画各的卡片（穿透是详情表、反代是域名卡、
 * 转发干脆是列表行），同一件事三套写法 —— 改一处样式要动三个文件，
 * 而且界面长得不一样，用户得在每个页面重新认一次。
 *
 * 现在统一成同一副骨架：
 *
 *   ● 服务名                    [图标行…]
 *     第二行：地址 / 目标（页面自己给）
 *     第三行：端口 + 小标签（页面自己给）
 *
 * **只管骨架，不管业务**：具体显示什么地址、图标行里放哪些按钮，
 * 全部由页面通过 props 传进来 —— 这个组件不认识"穿透""站点""转发"任何一个概念
 * （和 LanScanModal 同一个规矩：共用组件不判断业务）。
 *
 * 这样每个功能想加自己的小功能（比如穿透的日志、转发的协议标），
 * 传个图标或标签就行，不会互相污染。
 */
import type { ReactNode } from 'react'
import { BellRing } from 'lucide-react'

/** 服务状态：绿点=在跑，灰点=已停用，红点=出错 */
export type ServiceState = 'running' | 'off' | 'failed'

const stateDot: Record<ServiceState, string> = {
  running: 'bg-emerald-500',
  off: 'bg-slate-300',
  failed: 'bg-rose-400',
}

/** 图标按钮的统一尺寸 —— 三个页面的操作按钮都是这个方块，视觉才齐 */
export const iconBtn =
  'grid h-6 w-6 shrink-0 place-items-center rounded-md transition'

export function ServiceCard({
  name,
  state = 'running',
  stateHint,
  /** 通知已启用（显示在卡片头部） */
  notifyActive = false,
  /** 配置的通知渠道数 */
  notifyCount = 0,
  subtitle,
  meta,
  icons,
  dim = false,
  dragging = false,
  dropTarget = false,
  draggable = true,
  onDragStart,
  onDragEnd,
  onDragOver,
  onDrop,
}: {
  /** 服务名（主标题）。留空显示"未命名服务" */
  name?: string
  state?: ServiceState
  /** 鼠标悬停状态点时显示（放"穿透成功""最近错误"这类说明） */
  stateHint?: string
  /** 通知已启用（显示在卡片头部） */
  notifyActive?: boolean
  /** 配置的通知渠道数 */
  notifyCount?: number
  /** 第二行：外部地址 / 后端地址 / 转发目标 */
  subtitle?: ReactNode
  /** 第三行：端口 + 特性小标签 */
  meta?: ReactNode
  /** 图标行：启停、主页、编辑、日志、副本、删除……由页面决定放哪些 */
  icons?: ReactNode
  /** 卡片整体压暗（比如被拖动时） */
  dim?: boolean
  dragging?: boolean
  dropTarget?: boolean
  draggable?: boolean
  onDragStart?: () => void
  onDragEnd?: () => void
  onDragOver?: (e: React.DragEvent) => void
  onDrop?: (e: React.DragEvent) => void
}) {
  const off = state === 'off'
  return (
    <div
      draggable={draggable}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
      onDragOver={onDragOver}
      onDrop={onDrop}
      className={`flex cursor-grab flex-col rounded-xl border p-2.5 transition hover:border-blue-200 hover:shadow-md ${
        dropTarget ? 'ring-2 ring-blue-300' : ''
      } ${dragging ? 'opacity-40' : ''} ${
        off ? 'border-slate-200/80 bg-slate-50/60' : 'border-slate-200/80'
      } ${dim ? 'opacity-60' : ''}`}
    >
      {/* 行1：状态点 + 名字 + 图标行 */}
      <div className="flex items-center gap-1">
        <span
          title={stateHint}
          className={`h-2 w-2 shrink-0 rounded-full ${stateDot[state]}`}
        />
        <span
          className={`min-w-0 flex-1 truncate text-[13px] font-bold ${
            off ? 'text-slate-400' : 'text-slate-800'
          }`}
        >
          {name || '未命名服务'}
        </span>
        {/* 通知徽标：开了通知的服务一眼能认出来。
            之前这两个 prop 只声明了没渲染，界面上根本看不到。 */}
        {notifyActive && (
          <span
            title={`已启用通知，发往 ${notifyCount ?? 0} 个渠道`}
            className="flex shrink-0 items-center gap-0.5 rounded bg-emerald-50 px-1 py-px text-[10px] font-semibold text-emerald-600"
          >
            <BellRing size={10} />
            通知{notifyCount && notifyCount > 1 ? ` ${notifyCount}` : ''}
          </span>
        )}
        {icons}
      </div>

      {/* 行2：地址 / 目标 */}
      {subtitle && <div className="mt-1.5 min-w-0 text-xs">{subtitle}</div>}

      {/* 行3：端口 + 标签 */}
      {meta && <div className="mt-1 flex flex-wrap items-center gap-1.5">{meta}</div>}
    </div>
  )
}

/** 特性小标签：HTTPS / UPnP / Webhook / 协议 —— 三个页面都用它，样式才一致 */
export function CardBadge({
  children,
  tone = 'slate',
  title,
}: {
  children: ReactNode
  tone?: 'slate' | 'violet' | 'teal' | 'amber' | 'emerald' | 'rose' | 'blue'
  title?: string
}) {
  const tones: Record<string, string> = {
    slate: 'bg-slate-100 text-slate-500',
    violet: 'bg-violet-50 text-violet-600',
    teal: 'bg-teal-50 text-teal-600',
    amber: 'bg-amber-50 text-amber-600',
    emerald: 'bg-emerald-50 text-emerald-600',
    rose: 'bg-rose-50 text-rose-500',
    blue: 'bg-blue-50 text-blue-600',
  }
  return (
    <span
      title={title}
      className={`rounded px-1 py-px text-[10px] font-bold ${tones[tone]}`}
    >
      {children}
    </span>
  )
}
