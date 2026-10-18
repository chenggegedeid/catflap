/**
 * ModeSwitch 三选一的分段按钮。
 *
 * 设置页与「出口网络」组件都要用，所以抽出来 —— 免得两边各存一份，
 * 改样式的时候只改了一处、另一处慢慢长得不一样。
 */
// ModeSwitch 三选一的分段按钮段按钮
export function ModeSwitch<T extends string>({
  value,
  options,
  onChange,
}: {
  value: T
  options: { value: T; label: string }[]
  onChange: (v: T) => void
}) {
  return (
    <div className="inline-flex flex-wrap gap-1 rounded-xl bg-surface-2 p-1">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          onClick={() => onChange(o.value)}
          className={`rounded-lg px-3 py-1.5 text-xs font-semibold transition ${
            value === o.value ? 'bg-surface text-blue-600 shadow-sm' : 'text-muted hover:text-content'
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

