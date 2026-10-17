import type { ReactNode } from 'react'
import PageBoundary from '../components/PageBoundary'
import type { LucideIcon } from 'lucide-react'
import {
  Archive,
  ArrowLeftRight,
  Cloud,
  Bell,
  Blocks,
  ShieldAlert,
  CalendarClock,
  FileText,
  Globe,
  LayoutDashboard,
  Power,
  Repeat,
  Server,
  Settings as SettingsIcon,
  ShieldCheck,
  Waves,
} from 'lucide-react'

import { Audit } from './Audit'
import { Backup } from './Backup'
import { Cert } from './Cert'
import { ComponentManager } from './Components'
import { Dashboard } from './Dashboard'
import { Ddns } from './Ddns'
import { Logs } from './Logs'
import { Notify } from './Notify'
import { Devices } from './Devices'
import { Forwards } from './Forwards'
import { ReverseProxy } from './ReverseProxy'
import { Settings } from './Settings'
import { Stun } from './Stun'
import { Task } from './Task'
import { Tunnel } from './Tunnel'
import { Wol } from './Wol'

/**
 * 后台页面注册表 —— **唯一**描述「后台有哪些页面」的地方。
 *
 * # ⚠️ 别在页面模块顶层读本文件导出的东西
 *
 * 本文件引入每个页面组件，某个页面组件反过来又引入本文件 —— 这是一条
 * **循环依赖**。谁先求值由打包顺序决定，而打包顺序会随「加一个页面」
 * 这种看似无关的改动变化。
 *
 * 于是：**在页面模块的顶层（模块作用域）读 `pages` / `pageGroups`，读到的
 * 很可能是 undefined**。打包后所有模块被拼进同一个作用域，ESM 原本会报的
 * TDZ 错误会退化成 `Object.keys(undefined)` 抛
 * `TypeError: Cannot convert undefined or null to object` —— 异常发生在 bundle
 * 顶层，React 根本没挂载，整个后台就是一片白屏，控制台里只剩这么一句，
 * 看不出跟页面有关。v1.0.0 就栽在这里（pages/Components.tsx 顶层建了
 * 组件名→页面的反向索引）。
 *
 * 规则很简单：**要读可以，放进函数里，等渲染时再读**。那时所有模块都求值
 * 完毕了。`layout/nav.ts` 就是这么做的（读取都在 navGroupsFor 里面）。
 *
 * # 这个文件要解决的问题
 *
 * 改造前加一个页面要同时改三处：types.ts 的 PageKey 联合、layout/nav.ts 的列表、
 * App.tsx 的 switch。三处里最危险的是 App.tsx 那个 switch —— 漏一个 case 不会
 * 报错，页面会静默渲染成「该模块尚未实现」的占位符，看着像是功能没做完。
 * 这正是 M3 要消掉的那一类问题。
 *
 * 现在：**加一个页面 = 在下面 `pages` 里加一条 + 在 `pageGroups` 里挂到某个分组**。
 * 两处都由编译器把关：
 *   - PageKey 由 `pages` 的键**推导**出来，不再是手写的联合类型；
 *   - 每条记录都必须是合法的 PageMeta（`satisfies`）；
 *   - 漏挂分组的页面会被 GroupedPagesComplete 那个断言挡下来（见文件末尾）。
 *
 * # 为什么不做懒加载
 *
 * 方案原文提过「懒加载组件」。这里刻意没做：整个后台编译出来是一份几百 KB 的
 * JS，而页面之间的切换频率很高，**把一次性的下载换成一串 Suspense 闪烁**
 * 并不划算。等体积真的成为问题时再按页拆，那时 `element` 换成 `lazy(...)`
 * 加一层 Suspense 即可，注册表这层结构不用动。
 */
export interface PageMeta {
  /** 侧栏与顶栏显示的标题 */
  label: string
  icon: LucideIcon
  /** 顶栏标题下面那行小字，可空 */
  subtitle?: string
  /**
   * 侧栏标签，例如 NEW。
   *
   * 目前没有页面在用了：给功能挂「NEW」的时期已经过去，几个小标签反而让侧栏
   * 显得吵。保留这个字段是为了将来真需要（比如「测试中」）时不至于又要改三处 ——
   * Sidebar 与 layout/nav.ts 的透传逻辑都还在。
   */
  badge?: string
  /**
   * 归属的后端组件名（core/component 注册表里的 Name）。
   *
   * 该组件在「功能管理」里被关掉时，本页从侧栏消失、直接敲 URL 也会回落到
   * 仪表盘 —— 关掉一个功能的接口却还留着入口，用户点进去只会看到一片报错。
   *
   * 留空 = 这个页面不归任何组件管，永远显示。
   */
  component?: string
  /** 页面主体 */
  element: ReactNode
}

/**
 * 全部页面。键就是 hash 路由里的 key（`#/<key>`）。
 *
 * `satisfies` 而不是 `: Record<string, PageMeta>`：前者会保留键的字面量类型，
 * PageKey 才能从它推导出来；后者会把键宽化成 string，联合类型就丢了。
 */
export const pages = {
  dashboard: {
    label: '仪表盘',
    icon: LayoutDashboard,
    subtitle: '一站式管理网络、域名、证书与服务，让一切安全、稳定、简单',
    element: <PageBoundary key="dashboard"><Dashboard /></PageBoundary>,
  },

  devices: {
    label: '设备',
    icon: Server,
    subtitle: '内网里的机器，穿透 / 反代 / 端口转发共用这一份',
    // 刻意不绑定组件：设备表是 proxy 与 stun 共用的，绑死任何一个都会出现
    // 「只关了反向代理，设备页也跟着没了」——而穿透还在用它。
    // 关掉其中一侧后，设备页里那一侧的操作会 404，这是能接受的代价。
    element: <PageBoundary key="devices"><Devices /></PageBoundary>,
  },

  forwards: {
    label: '端口转发',
    icon: ArrowLeftRight,
    subtitle: '端口对端口的转发，不看域名，适合非 HTTP 服务',
    component: 'proxy',
    element: <PageBoundary key="forwards"><Forwards /></PageBoundary>,
  },

  stun: {
    label: 'STUN 内网穿透',
    icon: Waves,
    subtitle: '基于 STUN 的内网穿透与连接质量检测',
    component: 'stun',
    element: <PageBoundary key="stun"><Stun /></PageBoundary>,
  },
  ddns: {
    label: 'DDNS 域名解析',
    icon: Globe,
    subtitle: '动态域名解析与多服务商同步',
    component: 'ddns',
    element: <PageBoundary key="ddns"><Ddns /></PageBoundary>,
  },
  'reverse-proxy': {
    label: '反向代理',
    icon: Repeat,
    component: 'proxy',
    element: <PageBoundary key="reverse-proxy"><ReverseProxy /></PageBoundary>,
  },
  cert: {
    label: '证书管理',
    icon: ShieldCheck,
    component: 'cert',
    element: <PageBoundary key="cert"><Cert /></PageBoundary>,
  },
  wol: {
    label: '网络唤醒',
    icon: Power,
    subtitle: '发魔术包把局域网里的机器叫醒',
    component: 'wol',
    element: <PageBoundary key="wol"><Wol /></PageBoundary>,
  },
  task: {
    label: '计划任务',
    icon: CalendarClock,
    subtitle: '定时同步 DDNS 解析记录或唤醒机器',
    component: 'task',
    element: <PageBoundary key="task"><Task /></PageBoundary>,
  },

  components: {
    label: '功能管理',
    icon: Blocks,
    subtitle: '查看已安装的功能、开关它们，并留意它们的健康状态',
    // 刻意不绑定任何组件：这一页是用来**把功能重新打开**的入口。
    // 万一哪天它自己被关掉，用户就只能去改 config/components.json 了。
    element: <ComponentManager />,
  },
  backup: {
    label: '备份与恢复',
    icon: Archive,
    subtitle: '把配置打包带走，换机器或重装后一键还原',
    element: <PageBoundary key="backup"><Backup /></PageBoundary>,
  },

  settings: {
    label: '系统设置',
    icon: SettingsIcon,
    element: <PageBoundary key="settings"><Settings /></PageBoundary>,
  },
  // 这里读的是 logs/ 下的运行日志，不是「谁在什么时候改了什么」的审计流水，别叫审计
  logs: {
    label: '运行日志',
    icon: FileText,
    element: <PageBoundary key="logs"><Logs /></PageBoundary>,
  },
  audit: {
    label: '访问审计',
    icon: ShieldAlert,
    subtitle: '谁在访问、谁在扫我',
    component: 'audit',
    element: <PageBoundary key="audit"><Audit /></PageBoundary>,
  },
  tunnel: {
    label: 'Tunnel',
    icon: Cloud,
    subtitle: 'CGNAT 下不带端口的最干净出口',
    component: 'tunnel',
    element: <PageBoundary key="tunnel"><Tunnel /></PageBoundary>,
  },
  notify: {
    label: '通知',
    icon: Bell,
    subtitle: '渠道全局配一次，各服务按名字选用',
    component: 'notify',
    element: <PageBoundary key="notify"><Notify /></PageBoundary>,
  },
} satisfies Record<string, PageMeta>

/** 页面标识。由 pages 的键推导，新增页面时自动跟着变 */
export type PageKey = keyof typeof pages

export interface NavGroupDef {
  /** 分组标题；不写就归到最上面那个无标题分组 */
  title?: string
  keys: readonly PageKey[]
}

/**
 * 侧栏的分组与顺序。
 *
 * 和 `pages` 分开而不是给每条加一个 order 字段：顺序是**分组的事**，
 * 把「网络与连接」这组整体挪到「系统与管理」后面，改一处就行；
 * 写成每条一个 order 数字的话，插一个页面就得回头调一串数字。
 */
export const pageGroups = [
  { keys: ['dashboard'] },
  { title: '网络与连接', keys: ['devices', 'forwards', 'stun', 'ddns', 'reverse-proxy', 'tunnel', 'cert', 'wol'] },
  { title: '自动化', keys: ['task'] },
  { title: '系统与管理', keys: ['components', 'settings', 'backup', 'logs', 'audit', 'notify'] },
] as const satisfies readonly NavGroupDef[]

/**
 * 编译期静态断言：每个页面都必须被 pageGroups 收进某个分组。
 *
 * 漏收的后果是这个页面**只存在于地址栏里** —— 直接敲 `#/xxx` 能打开，但侧栏上
 * 没有任何入口，用户不知道它在那儿；而且不会有任何运行时报错。所以交给编译器
 * 看着：`Exclude` 的结果不是 never 时，下面那行会因为不满足 `T extends never`
 * 而编译失败，报错信息里带着漏掉的页面名。
 *
 * 写成**导出的类型别名**而不是局部 const，是为了避开 noUnusedLocals —— 它的
 * 作用在编译期就已经完成，运行时不存在任何对应物。
 */
type RequiresNever<T extends never> = T
export type GroupedPagesComplete = RequiresNever<
  Exclude<PageKey, (typeof pageGroups)[number]['keys'][number]>
>

/** 默认落地页。hash 为空或不认识时用它 */
export const DEFAULT_PAGE: PageKey = 'dashboard'

export interface ResolvedPage extends PageMeta {
  key: PageKey
}

/**
 * 判断一个字符串是不是已知的页面 key。
 *
 * 用 hasOwnProperty 而不是 `key in pages`：否则 `#/toString`、`#/constructor`
 * 这类来自原型链的名字会被判成合法页面。
 */
export function isPageKey(v: string): v is PageKey {
  return Object.prototype.hasOwnProperty.call(pages, v)
}

/** 按 key 取页面；不认识的 key 返回 undefined（调用方决定回落到哪一页） */
export function findPage(key: string): ResolvedPage | undefined {
  if (!isPageKey(key)) return undefined
  return { key, ...pages[key] }
}
