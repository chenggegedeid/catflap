export function Metric({ label, value, tone }: { label: string; value: string; tone?: 'danger' }) {
  return (
    <div className="rounded-xl bg-surface-2 px-4 py-3">
      <div className="text-xs text-muted">{label}</div>
      <div className={`mt-0.5 text-xl font-semibold tabular-nums ${tone === 'danger' ? 'text-rose-600' : 'text-content'}`}>
        {value}
      </div>
    </div>
  )
}

/**
 * 登录尝试。
 *
 * 单独一块而不是混进上面的访问来源：那是"谁访问了我的服务"，这是
 * "谁在试着进我的后台"。两者的处置方式也不同 —— 后者要的是"看有没有人在
 * 试密码、试的是哪些用户名"，而不是按 IP 统计流量。
 *
 * 界面上刻意显示**用户名**：看到一堆 admin / root / test 就知道有人在字典
 * 攻击，这比只给一个 IP 有用得多。
 */
