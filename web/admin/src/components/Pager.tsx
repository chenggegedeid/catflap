/**
 * Pager 列表底部分页控件。
 *
 * 访问来源、黑名单、登录尝试三处共用 —— 分页的交互必须一致，
 * 否则用户在一个列表学会的操作到另一个就不适用了。
 */
export function Pager({
  total,
  page,
  limit,
  onPage,
  onLimit,
  unit = '条',
}: {
  total: number
  page: number
  limit: number
  onPage: (p: number) => void
  onLimit: (n: number) => void
  /** 计数单位：「条」/「个 IP」之类 */
  unit?: string
}) {
  const pageCount = Math.max(1, Math.ceil(total / limit))

  // 只有一页时整条不显示。
  // 原因：以前总显示、但按钮是 disabled，用户点了没反应只会以为坏了
  // （实际就是数据量不够分出一页）。
  if (pageCount <= 1 && total <= limit) return null

  return (
    <div className="mt-3 flex flex-wrap items-center gap-2 border-t border-line pt-3 text-xs text-muted">
      <span>
        共 {total} {unit}，第 {page} / {pageCount} 页
      </span>
      <div className="ml-auto flex flex-wrap items-center gap-1">
        <button
          onClick={() => onPage(Math.max(1, page - 1))}
          disabled={page <= 1}
          className="rounded-lg border border-line px-2 py-1 font-semibold transition hover:bg-surface-2 disabled:opacity-40"
        >
          上一页
        </button>
        {/* 只给首尾和当前页附近的按钮，几十页时不会排成一条长龙 */}
        {pageNumbers(page, pageCount).map((p, i) =>
          p === 0 ? (
            <span key={`gap-${i}`} className="px-1 text-muted">
              …
            </span>
          ) : (
            <button
              key={p}
              onClick={() => onPage(p)}
              className={`min-w-7 rounded-lg px-2 py-1 font-semibold transition ${
                p === page
                  ? 'bg-blue-500 text-white'
                  : 'border border-line text-content hover:bg-surface-2'
              }`}
            >
              {p}
            </button>
          ),
        )}
        <button
          onClick={() => onPage(Math.min(pageCount, page + 1))}
          disabled={page >= pageCount}
          className="rounded-lg border border-line px-2 py-1 font-semibold transition hover:bg-surface-2 disabled:opacity-40"
        >
          下一页
        </button>
        <select
          value={limit}
          onChange={(e) => onLimit(Number(e.target.value))}
          className="ml-1 rounded-lg border border-line bg-surface px-2 py-1 text-xs outline-none focus:border-blue-400"
        >
          <option value={10}>每页 10</option>
          <option value={20}>每页 20</option>
          <option value={50}>每页 50</option>
          <option value={100}>每页 100</option>
        </select>
      </div>
    </div>
  )
}

/**
 * 页码列表：首尾 + 当前页前后各一个，中间用 0 表示省略号。
 *
 * 几十页时把所有页码都排出来会撑爆那一行，用户也找不到要点的那个。
 */
export function pageNumbers(current: number, count: number): number[] {
  if (count <= 7) return Array.from({ length: count }, (_, i) => i + 1)
  const out: number[] = [1]
  const start = Math.max(2, current - 1)
  const end = Math.min(count - 1, current + 1)
  if (start > 2) out.push(0)
  for (let p = start; p <= end; p++) out.push(p)
  if (end < count - 1) out.push(0)
  out.push(count)
  return out
}
