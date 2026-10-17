/**
 * 图标选择器。
 *
 * 三个来源并排，不互斥（Sun-Panel 的做法）：
 *   1. 内置图标库 —— 分类陈列，点一下就选
 *   2. 从地址抓取 —— 用已填的地址，不用重输
 *   3. 上传本地图片
 *
 * 之所以把"库"放最前：抓 favicon 不是总能成功（内网服务、没 favicon、
 * 被墙），而 NAS / 路由器这类条目压根没有可抓的东西 —— 库才是主力路径。
 */

import { useMemo, useState } from 'react'
import { ICON_LIBRARY, libIconValue, type LibIcon } from '../../lib/iconLibrary'
import { AppIcon } from '../card/AppIcon'

export function IconPicker({
  value,
  onPick,
}: {
  value: string
  onPick: (iconValue: string) => void
}) {
  const [q, setQ] = useState('')

  const groups = useMemo(() => {
    const kw = q.trim().toLowerCase()
    if (!kw) return ICON_LIBRARY
    // 搜索：按名字或 key 过滤，空分组不显示
    return ICON_LIBRARY.map((g) => ({
      name: g.name,
      icons: g.icons.filter((i) => i.label.includes(kw) || i.key.includes(kw) || i.icon.toLowerCase().includes(kw)),
    })).filter((g) => g.icons.length > 0)
  }, [q])

  return (
    <div className="space-y-2">
      <input
        value={q}
        onChange={(e) => setQ(e.target.value)}
        placeholder="搜图标：NAS、下载、路由…"
        className="w-full rounded-lg border border-slate-200 px-2 py-1 text-xs focus:border-blue-400 focus:outline-none"
      />

      <div className="max-h-52 space-y-2.5 overflow-y-auto pr-0.5">
        {groups.map((g) => (
          <div key={g.name}>
            <p className="mb-1 text-[10px] font-semibold text-slate-400">{g.name}</p>
            <div className="grid grid-cols-6 gap-1.5">
              {g.icons.map((it) => (
                <IconCell key={it.key} item={it} selected={value === libIconValue(it.key)} onPick={onPick} />
              ))}
            </div>
          </div>
        ))}
        {groups.length === 0 && <p className="py-4 text-center text-[11px] text-slate-400">没有匹配的图标</p>}
      </div>
    </div>
  )
}

function IconCell({
  item,
  selected,
  onPick,
}: {
  item: LibIcon
  selected: boolean
  onPick: (v: string) => void
}) {
  return (
    <button
      type="button"
      onClick={() => onPick(libIconValue(item.key))}
      title={item.label}
      aria-label={item.label}
      aria-pressed={selected}
      className={`grid aspect-square place-items-center overflow-hidden rounded-[22%] transition ${
        selected ? 'ring-2 ring-blue-500 ring-offset-1' : 'hover:scale-105'
      }`}
    >
      <span className="h-full w-full overflow-hidden rounded-[22%]">
        <AppIcon name={item.label} icon={libIconValue(item.key)} />
      </span>
    </button>
  )
}
