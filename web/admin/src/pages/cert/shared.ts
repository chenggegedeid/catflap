import { useCallback, useRef, useState } from 'react'
import {
  CircleCheck, CircleSlash, Clock, FileKey, FolderOpen, KeyRound,
  RefreshCw, ShieldCheck, TriangleAlert,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import type { AcmeProvider, CertSource, Certificate } from '../../types'

export interface SourceMeta {
  value: CertSource
  label: string
  hint: string
  icon: LucideIcon
  /** 走 ACME 自动签发 */
  acme?: boolean
}

export const SOURCES: SourceMeta[] = [
  {
    value: 'acme-dns01',
    label: '自动签发 · DNS-01',
    hint: '用 DDNS 里已配好的服务商写 TXT 记录验证。支持通配符证书，不需要任何入站端口，CGNAT / 大内网也能用。',
    icon: ShieldCheck,
    acme: true,
  },
  {
    value: 'acme-http01',
    label: '自动签发 · HTTP-01',
    hint: '由 CA 回连你的 80 端口验证。不支持通配符域名。',
    icon: ShieldCheck,
    acme: true,
  },
  {
    value: 'upload',
    label: '手动上传',
    hint: '直接粘贴证书和私钥的 PEM 文本。到期需要自己重新上传，系统不会自动续期。',
    icon: FileKey,
  },
  {
    value: 'path',
    label: '读取本机文件',
    hint: '指向本机上已有的 PEM 文件（如 certbot / acme.sh 的输出目录）。文件变动后自动热重载，无需重启。',
    icon: FolderOpen,
  },
  {
    value: 'self-signed',
    label: '自签（本机现签）',
    hint: '不用域名、不用联网、不用等，点一下就有。浏览器会拦一道「不受信任」，要手动点继续——想要地址栏干净只能用自动签发或上传真证书。',
    icon: KeyRound,
  },
]

/** 自签证书换不来什么，得当面说清楚，别让人配完了才发现 */
export const SELF_SIGNED_NOTE =
  '自签证书没有任何机构背书，浏览器一定会拦一道「您的连接不是私密连接」，点「高级 → 继续前往」才能进。它解决的只是「HTTPS 必须有证书」这一条，加密是真的，身份不是。自己内网用、或者外面还有一层（Cloudflare 等）在管证书时够用；要给别人访问、要地址栏干净，用 DNS-01 自动签发。'

export const sourceMetaMap = new Map(SOURCES.map((s) => [s.value, s]))
export const sourceLabel = (s: CertSource) => sourceMetaMap.get(s)?.label ?? s

/** HTTP-01 的端口是协议写死的，改不了。这个提示必须常驻，不能只在报错后才说。 */
export const HTTP01_WARNING =
  'HTTP-01 需要公网 80 端口可入站，且端口由协议规定不可更改。CGNAT / 大内网环境用不了，请改用 DNS-01。'

export const DIRECTORIES = [
  { value: '', label: "Let's Encrypt 正式环境" },
  {
    value: 'https://acme-staging-v02.api.letsencrypt.org/directory',
    // 原来只写「测试环境（staging）」，加上「配不通时先拿它试」是因为
    // 用户看到「浏览器不信任」就会以为这东西没用，转头去正式环境硬试，
    // 然后被锁一小时——而这恰恰是 staging 存在的理由
    label: "Let's Encrypt 测试环境（staging，配不通时先拿它试）",
  },
  { value: 'custom', label: '自定义 ACME 服务（ZeroSSL 等）' },
]

// ===================== 小工具 =====================

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
    window.setTimeout(() => setToasts((p) => p.filter((t) => t.id !== id)), 2600)
  }, [])
  return { toasts, show }
}

export function fmtDate(s?: string): string {
  if (!s) return '--'
  const d = new Date(s)
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return '--'
  return d.toLocaleDateString()
}

/** 域名输入：按空白或逗号分隔，顺便去重 */
export function parseDomains(raw: string): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const part of raw.split(/[\s,，]+/)) {
    const d = part.trim().toLowerCase().replace(/\.$/, '')
    if (!d || seen.has(d)) continue
    seen.add(d)
    out.push(d)
  }
  return out
}

export interface CertStatus {
  label: string
  tone: string
  icon: LucideIcon
}

export function certStatus(c: Certificate): CertStatus {
  if (!c.enabled) return { label: '已禁用', tone: 'bg-surface-2 text-muted', icon: CircleSlash }
  if (c.expired) return { label: '已过期', tone: 'bg-rose-50 text-rose-500', icon: TriangleAlert }
  if (c.lastError) return { label: '有错误', tone: 'bg-rose-50 text-rose-500', icon: TriangleAlert }
  if (!c.loaded) {
    // ACME 证书新建后还没签下来、自签的还没生成，都属于正常的中间态，别吓唬用户
    const pending =
      c.source === 'acme-dns01' || c.source === 'acme-http01' || c.source === 'self-signed'
    return pending
      ? { label: '等待签发', tone: 'bg-amber-50 text-amber-600', icon: RefreshCw }
      : { label: '未加载', tone: 'bg-amber-50 text-amber-600', icon: Clock }
  }
  if (c.daysLeft <= 7) return { label: '即将到期', tone: 'bg-amber-50 text-amber-600', icon: Clock }
  return { label: '生效中', tone: 'bg-emerald-50 text-emerald-600', icon: CircleCheck }
}

// ===================== 新建 / 编辑弹窗 =====================

export interface CertFormState {
  name: string
  source: CertSource
  domainsRaw: string
  enabled: boolean
  isDefault: boolean
  certFile: string
  keyFile: string
  directoryChoice: string
  directoryCustom: string
  email: string
  providerId: string
  httpPort: string
  eabKeyId: string
  eabHmac: string
  renewDays: string
}

export function initialForm(initial?: Certificate, providers: AcmeProvider[] = []): CertFormState {
  if (!initial) {
    const firstUsable = providers.find((p) => p.supportsDns01)
    return {
      name: '',
      source: 'acme-dns01',
      domainsRaw: '',
      enabled: true,
      isDefault: false,
      certFile: '',
      keyFile: '',
      directoryChoice: '',
      directoryCustom: '',
      email: '',
      providerId: firstUsable ? String(firstUsable.id) : '',
      httpPort: '80',
      eabKeyId: '',
      eabHmac: '',
      renewDays: '30',
    }
  }
  const known = DIRECTORIES.some((d) => d.value === initial.acme.directory)
  return {
    name: initial.name,
    source: initial.source,
    domainsRaw: initial.domains.join('\n'),
    enabled: initial.enabled,
    isDefault: initial.isDefault,
    certFile: initial.certFile,
    keyFile: initial.keyFile,
    directoryChoice: known ? initial.acme.directory : initial.acme.directory ? 'custom' : '',
    directoryCustom: known ? '' : initial.acme.directory,
    email: initial.acme.email,
    providerId: initial.acme.providerId ? String(initial.acme.providerId) : '',
    httpPort: String(initial.acme.httpPort || 80),
    eabKeyId: initial.acme.eabKeyId,
    eabHmac: '',
    renewDays: String(initial.acme.renewDays || 30),
  }
}

