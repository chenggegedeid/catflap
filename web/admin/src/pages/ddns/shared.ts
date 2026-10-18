import { useCallback, useRef, useState } from 'react'
import {
  CircleCheck, RefreshCw, TriangleAlert,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type {
  DdnsProvider, DdnsProviderTypeSpec, DdnsRecordStatus, DnsRecordType, NetInterface,
} from '../../types'

export const statusInfo: Record<DdnsRecordStatus, { label: string; tone: string; icon: LucideIcon }> = {
  success: { label: '正常', tone: 'bg-emerald-50 text-emerald-600', icon: CircleCheck },
  pending: { label: '等待同步', tone: 'bg-amber-50 text-amber-600', icon: RefreshCw },
  skipped: { label: '无变化', tone: 'bg-surface-2 text-muted', icon: CircleCheck },
  failed: { label: '同步失败', tone: 'bg-rose-50 text-rose-500', icon: TriangleAlert },
}

export const ipSourceLabel: Record<string, string> = {
  stun: 'STUN 公网',
  web: 'Web 查询',
  dns: 'DNS 解析',
  interface: '本地网卡',
  custom: '自定义',
}

// 每种来源「来源参数」那一栏的说明。
// 原来只对 web/dns 写了 placeholder，其余来源那个框是灰的、没人知道该不该填，
// 现在每种来源自己说清楚：填什么、必不必填。
export const ipSourceArgMeta: Record<string, { label: string; placeholder: string; required: boolean }> = {
  stun: { label: '来源参数（当前来源无需填写）', placeholder: '', required: false },
  web: { label: '查询地址', placeholder: '如 https://ip.sb（留空用内置源）', required: false },
  dns: { label: '要跟随的域名', placeholder: '如 example.com', required: true },
  interface: { label: '网卡', placeholder: '留空自动挑一块', required: false },
  custom: { label: 'IP 地址', placeholder: '如 192.0.2.1', required: true },
}

// ifaceOptionText 下拉里一块网卡显示成什么样：名字 + 当前记录类型用得上的地址。
// 光有名字认不出哪块是连外网那块，Windows 上一堆 vEthernet 尤其分不清。
export function ifaceOptionText(n: NetInterface, recordType: DnsRecordType) {
  // 双栈两个都要，于是两边地址都列出来
  const addrs = recordType === 'A' ? n.ipv4 : recordType === 'AAAA' ? n.ipv6 : [...n.ipv4, ...n.ipv6]
  if (addrs.length === 0) {
    return `${n.name}（没有${recordType === 'A' ? ' IPv4' : recordType === 'AAAA' ? ' IPv6' : '可用'}地址）`
  }
  const shown = addrs.slice(0, 2).join('、')
  return `${n.name} — ${shown}${addrs.length > 2 ? ' 等' : ''}${n.hasPublic ? '' : '（内网地址）'}`
}

export function ifaceHasType(n: NetInterface, recordType: DnsRecordType) {
  if (recordType === 'A') return n.ipv4.length > 0
  if (recordType === 'AAAA') return n.ipv6.length > 0
  // 双栈：两种地址都得有才谈得上"同时同步"
  return n.ipv4.length > 0 && n.ipv6.length > 0
}

// ===================== 服务商字段表 =====================
// 这张表**不在这里**：后端 /api/ddns/provider/types 按 core/provider 注册表返回，
// 前端只负责渲染。新增一家服务商 = 后端加一个 register_*.go，本文件零改动。
//
// 以前这里手维护着一张 PROVIDER_TYPES，和后端的 switch、ProviderCredentialKeys
// 各说各话，加一家要改四处，漏一处就是「能选但保存不了」或者「选项里根本没有它」。

// ProviderTypeSpec / 能力常量与 loadProviderTypes 见 lib/providerTypes.ts
export type ProviderTypeSpec = DdnsProviderTypeSpec

export interface Toast {
  id: number
  text: string
}

export function useToast() {
  const [toasts, setToasts] = useState<Toast[]>([])
  const idRef = useRef(0)
  const show = useCallback((text: string) => {
    const id = ++idRef.current
    setToasts((p) => [...p, { id, text }])
    window.setTimeout(() => setToasts((p) => p.filter((t) => t.id !== id)), 2200)
  }, [])
  return { toasts, show }
}

export function fmtTime(s?: string): string {
  if (!s) return '--'
  const d = new Date(s)
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return '--'
  return d.toLocaleString()
}

// ===================== 服务商弹窗 =====================

export interface ProviderFormState {
  name: string
  type: DdnsProvider['type']
  credential: Record<string, string>
}

export interface RecordFormState {
  enabled: boolean
  providerId: string
  name: string
  domain: string
  subDomain: string
  recordType: DnsRecordType
  ipSource: 'stun' | 'web' | 'dns' | 'interface' | 'custom'
  ipSourceArg: string
  ttl: string
  proxied: boolean
}

export const intervalOptions = [
  { value: 60, label: '1 分钟' },
  { value: 300, label: '5 分钟' },
  { value: 600, label: '10 分钟' },
  { value: 1800, label: '30 分钟' },
  { value: 3600, label: '1 小时' },
]

