import { useEffect, useState } from 'react'
import { Pager } from '../../components/Pager'
import * as api from '../../lib/api'
import { getPageLimit } from '../../lib/prefs'
import type { AuditConfigView } from '../../types'
import { IPCard } from './IPCard'
import { applyLimit, blockUntil } from './shared'

export function IPLists({
  cfg,
  onSaved,
  onToast,
}: {
  cfg: AuditConfigView | null
  onSaved: () => void
  onToast: (m: string) => void
}) {
  /*
    两个名单的交互刻意做成**一模一样**：填进去回车就存，没有「保存」按钮。
    用户分不出（也不该分）哪个需要手动保存 —— 加一条就生效一条。

    区别只在后端怎么存：
      - 黑名单每条是一次独立封禁（有自己的原因与到期时间），增删走
        Block / Unblock 单条接口；不做整份覆盖，否则会把自动拉黑的条目一起抹掉。
      - 白名单是一份配置数组，只能整份覆盖 —— 所以每次增删都用**新数组**整体保存。
  */

  // 黑名单就是**整张封禁表**：手动加的、自动拉黑的，都在这里。
  // 以前还单独摆一张「封禁列表」，其实读的是同一份 Blocks ——
  // 两块并列会让人以为有两套名单。现在合并，并标出每条的来源与原因。
  const blocks = cfg?.blocks ?? []

  // 黑名单分页：自动拉黑开着的时候条目会一直涨，
  // 不分页的话这一块会把整页撑得老长（和访问来源一个道理）
  const [blackPage, setBlackPage] = useState(1)
  const [blackLimit, setBlackLimit] = useState(getPageLimit)
  useEffect(() => {
    setBlackPage(1)
  }, [blackLimit, blocks.length])
  const blackStart = (blackPage - 1) * blackLimit
  const blackSlice = blocks.slice(blackStart, blackStart + blackLimit)

  const addBlack = async (line: string): Promise<boolean> => {
    try {
      const r = await api.blockIP({ ip: line, reason: '手动添加黑名单', minutes: 0 })
      onToast(r.msg || `已封禁 ${line}`)
      await onSaved()
      return true
    } catch (e) {
      onToast('添加失败：' + (e instanceof Error ? e.message : ''))
      return false
    }
  }

  const removeBlack = async (ip: string): Promise<void> => {
    try {
      const r = await api.unblockIP(ip)
      onToast(r.msg || `已解除 ${ip}`)
      await onSaved()
    } catch (e) {
      onToast('移除失败：' + (e instanceof Error ? e.message : ''))
    }
  }

  // 白名单：拿当前名单算出新数组，整份保存
  const commitWhite = async (next: string[]): Promise<boolean> => {
    try {
      const res = await api.saveAuditWhitelist({
        whitelist: next,
        autoBlock: cfg?.autoBlock ?? false,
        autoBlockThreshold: cfg?.autoBlockThreshold ?? 500,
        autoBlockMinutes: cfg?.autoBlockMinutes ?? 60,
        trustedProxies: cfg?.trustedProxies ?? [],
      })
      onToast(res.msg || '白名单已保存')
      await onSaved()
      return true
    } catch (e) {
      onToast('保存失败：' + (e instanceof Error ? e.message : ''))
      return false
    }
  }

  const addWhite = async (line: string): Promise<boolean> => {
    const cur = cfg?.whitelist ?? []
    if (cur.includes(line)) {
      onToast('已经在名单里了')
      return false
    }
    return commitWhite([...cur, line])
  }

  const removeWhite = async (ip: string): Promise<void> => {
    await commitWhite((cfg?.whitelist ?? []).filter((x) => x !== ip))
  }

  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <IPCard
        title="黑名单（全局拦截）"
        hint="这些来源会被直接拦下，对全部站点生效。支持网段，例如 10.0.0.0/8、2409:8a60::/32。
              手动加的和自动拉黑的都在这里 —— 它就是整张封禁表，没有第二份。"
        entries={blackSlice.map((b) => ({
          value: b.ip,
          // 老数据里可能没写原因，给个兜底 —— 用户要靠这一行判断"为什么封的"
          reason: b.reason || (b.auto ? '自动拉黑' : '手动添加'),
          auto: b.auto,
          until: blockUntil(b.expiresAt),
        }))}
        onAdd={addBlack}
        onRemove={removeBlack}
        tone="danger"
        footer={
          <Pager
            total={blocks.length}
            page={blackPage}
            limit={blackLimit}
            onPage={setBlackPage}
            onLimit={applyLimit(setBlackLimit)}
            unit="条"
          />
        }
      />
      <IPCard
        title="白名单（永不封禁）"
        hint="这些来源永不被封，自动拉黑也会绕开。一般填你自己的出口 IP，别把自己关在门外。"
        entries={(cfg?.whitelist ?? []).map((w) => ({ value: w }))}
        onAdd={addWhite}
        onRemove={removeWhite}
        tone="ok"
      />
    </div>
  )
}

/**
 * 封禁的到期时间怎么显示。
 *
 * Go 的零值时间是 0001-01-01，序列化过来是 "0001-01-01T00:00:00Z" ——
 * 字符串**非空**，所以只判空是不够的，界面上会出现「至 0001/1/1」。
 * 零值表示"没设到期"，也就是永久。
 */
