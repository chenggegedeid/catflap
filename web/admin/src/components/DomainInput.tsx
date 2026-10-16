/**
 * DomainInput 域名输入：**子域手填 + 主域下拉（候选来自 DDNS 解析记录）**。
 *
 * 为什么做这个：原来三个地方都要手打完整域名 —— 打错一个字母整条规则就
 * 解析不到。而 DDNS 里本来就有"你能用哪些主域"这个信息（没加解析记录的
 * 主域填了也解析不到），所以主域应该**选**而不是**填**，用户只敲子域。
 *
 * 行为：
 *   - zones 非空：输入框只敲子域（nas），右边下拉选主域（1235889.xyz），
 *     拼出 nas.1235889.xyz 往外报。
 *   - zones 为空（还没配 DDNS）：退化成普通完整域名输入框，行为和以前一样
 *     —— 不逼用户先去配 DDNS 才能加服务。
 *
 * 受控组件：内部不存状态，sub/zone 每次从 value 拆出来（找到 value 以之
 * 结尾的最长候选主域），改哪半就拼回完整域名往 onChange 报。
 */
import { useMemo } from 'react'

/** 把完整域名拆成 子域 + 主域：按候选主域从长到短找能匹配的后缀 */
export function splitDomain(
  value: string,
  zones: string[],
): { sub: string; zone: string } {
  const v = value.trim().toLowerCase()
  const sorted = [...zones].sort((a, b) => b.length - a.length)
  for (const z of sorted) {
    if (z && v.toLowerCase().endsWith('.' + z.toLowerCase())) {
      return { sub: v.slice(0, v.length - z.length - 1), zone: z }
    }
    // 正好等于某个主域 → 子域为空，主域就是它
    if (z && v.toLowerCase() === z.toLowerCase()) {
      return { sub: '', zone: z }
    }
  }
  // 没匹配到任何候选主域：整段当子域，主域默认第一个（或者空 = 自由输入）
  return { sub: v, zone: zones[0] ?? '' }
}

export function DomainInput({
  value,
  zones,
  onChange,
  placeholder,
  className = '',
}: {
  value: string
  /** 候选主域（来自 DDNS 解析记录，去重）。空数组 = 退化成普通输入框 */
  zones: string[]
  onChange: (full: string) => void
  placeholder?: string
  className?: string
}) {
  const { sub, zone } = useMemo(() => splitDomain(value, zones), [value, zones])
  const box =
    'w-full min-w-0 bg-transparent px-3 py-2 font-mono text-sm outline-none'

  // 没有候选主域：普通输入框（老行为，不逼人先配 DDNS）
  if (zones.length === 0) {
    return (
      <div className={`flex items-center rounded-xl border border-slate-200 bg-white focus-within:border-blue-400 ${className}`}>
        <input
          value={value}
          onChange={(e) => onChange(e.target.value)}
          placeholder={placeholder || 'nas.example.com'}
          className={box}
        />
      </div>
    )
  }

  const emit = (newSub: string, newZone: string) => {
    const s = newSub.trim().toLowerCase()
    onChange(newZone ? (s ? `${s}.${newZone}` : newZone) : s)
  }

  return (
    <div
      className={`flex items-stretch overflow-hidden rounded-xl border border-slate-200 bg-white focus-within:border-blue-400 ${className}`}
    >
      <input
        value={sub}
        onChange={(e) => emit(e.target.value, zone)}
        placeholder={placeholder || '子域，如 nas'}
        className={box + ' flex-1'}
      />
      <span className="grid place-items-center px-1 font-mono text-sm text-slate-300">.</span>
      <select
        value={zone}
        onChange={(e) => emit(sub, e.target.value)}
        title="主域名（来自 DDNS 解析记录）"
        className="max-w-[45%] shrink-0 border-l border-slate-100 bg-slate-50 px-2 font-mono text-sm text-slate-600 outline-none"
      >
        {zones.map((z) => (
          <option key={z} value={z}>
            {z}
          </option>
        ))}
        {!zones.includes(zone) && zone && <option value={zone}>{zone}</option>}
      </select>
    </div>
  )
}
