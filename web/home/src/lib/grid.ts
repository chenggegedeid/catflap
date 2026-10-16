/**
 * 网格坐标与占用。
 *
 * 卡片可以跨格（1×1 / 1×2 / 2×1 / 2×2 / 2×4），所以位置不能再用「第几个」
 * 这种线性序号 —— 必须是二维坐标 + 占几格，并且要有一张占用表来回答
 * 「这块地方有没有人」。
 *
 * 尺寸一律写成 **宽×高**：
 *   1×2 = 宽 1 列、高 2 行（竖着的长条）
 *   2×1 = 宽 2 列、高 1 行（横着的扁条）
 *
 * ⚠️ 老版本存的是 pagedOrder（page*pageSize+slot），那是为固定 8×5 分页设计的，
 * 表达不了跨格。迁移时由后端换算成 row/col，见 modules/home 的迁移。
 */

/** 卡片尺寸，宽×高 */
export type CardSize = '1x1' | '1x2' | '2x1' | '2x2' | '2x4'

/** 占几格 */
export interface Span {
  /** 占几列 */
  w: number
  /** 占几行 */
  h: number
}

/** 一个卡片在网格里的位置 */
export interface GridPos extends Span {
  row: number
  col: number
}

/** 带 id 的位置，占用表用 */
export interface PlacedItem extends GridPos {
  id: string
}

/** 格子 key。用字符串是为了能直接进 Map/Set，也方便日志里看 */
export function cellKey(row: number, col: number): string {
  return `${row}:${col}`
}

const SIZE_TABLE: Record<CardSize, Span> = {
  '1x1': { w: 1, h: 1 },
  '1x2': { w: 1, h: 2 },
  '2x1': { w: 2, h: 1 },
  '2x2': { w: 2, h: 2 },
  '2x4': { w: 2, h: 4 },
}

export function parseSize(size: CardSize | string): Span {
  return SIZE_TABLE[size as CardSize] ?? SIZE_TABLE['1x1']
}

export const CARD_SIZES = Object.keys(SIZE_TABLE) as CardSize[]

/** 一个位置占住的所有格子 */
export function cellsOf(pos: GridPos): string[] {
  const out: string[] = []
  for (let r = pos.row; r < pos.row + pos.h; r++) {
    for (let c = pos.col; c < pos.col + pos.w; c++) {
      out.push(cellKey(r, c))
    }
  }
  return out
}

/** 两个位置是否重叠 */
export function overlaps(a: GridPos, b: GridPos): boolean {
  return (
    a.col < b.col + b.w &&
    b.col < a.col + a.w &&
    a.row < b.row + b.h &&
    b.row < a.row + a.h
  )
}

/** 位置是否越界（col+w 超出列数，或跑到负数） */
export function outOfBounds(pos: GridPos, cols: number): boolean {
  return pos.col < 0 || pos.row < 0 || pos.col + pos.w > cols
}

/**
 * 占用表：格子 → 卡片 id。
 *
 * 后放的会覆盖先放的 —— 所以调用方要保证 items 本身不重叠。
 * 重叠检测单独用 `findConflict`，别指望这里报错。
 */
export function buildOccupancy(items: PlacedItem[]): Map<string, string> {
  const occ = new Map<string, string>()
  for (const it of items) {
    for (const k of cellsOf(it)) {
      occ.set(k, it.id)
    }
  }
  return occ
}

/**
 * 这个位置能不能放（不越界、且除了 excludeId 之外没别人占）。
 *
 * excludeId 是给自己留的：移动一张卡时，它原来占的格子还在占用表里，
 * 不排除掉的话「原地放下」会被判成冲突。
 */
export function canPlace(
  pos: GridPos,
  occ: Map<string, string>,
  cols: number,
  excludeId?: string,
): boolean {
  if (outOfBounds(pos, cols)) return false
  for (const k of cellsOf(pos)) {
    const owner = occ.get(k)
    if (owner && owner !== excludeId) return false
  }
  return true
}

/** 找出和 pos 冲突的卡片 id（第一个就够用） */
export function findConflict(
  pos: GridPos,
  occ: Map<string, string>,
  excludeId?: string,
): string | null {
  for (const k of cellsOf(pos)) {
    const owner = occ.get(k)
    if (owner && owner !== excludeId) return owner
  }
  return null
}

/**
 * 从左上角开始扫，找一个能放下 span 的空位。
 *
 * 逐行、每行逐列地试，找到第一个能放下的就返回 —— 结果是确定的，
 * 不会「这次补在这儿、下次补在那儿」。
 *
 * maxRows 给 0 表示不限行（网格可以一直往下长）。
 */
export function findFreeSpot(
  span: Span,
  occ: Map<string, string>,
  cols: number,
  maxRows = 0,
): GridPos | null {
  const limit = maxRows > 0 ? maxRows : occ.size + span.h + cols
  for (let row = 0; row <= limit; row++) {
    for (let col = 0; col + span.w <= cols; col++) {
      const pos: GridPos = { row, col, ...span }
      if (canPlace(pos, occ, cols)) return pos
    }
  }
  return null
}

/**
 * 整理一遍所有卡片的位置。
 *
 * 处理三件事：没坐标的（补位）、越界的（拉回来）、互相重叠的（后到的让位）。
 *
 * **为什么要做这个**：配置可能来自老版本迁移、可能被人手工改坏、
 * 也可能换了窗口宽度导致列数变化。渲染前统一过一遍，
 * 界面上就不会出现「两张卡叠在一起」或者「有张卡在网格外面」。
 *
 * 顺序按 sortOrder 走，先来的先占好位置 —— 这样结果稳定可预期。
 */
export function normalize(
  items: { id: string; row?: number; col?: number; size?: CardSize | string }[],
  cols: number,
): PlacedItem[] {
  const occ = new Map<string, string>()
  const out: PlacedItem[] = []

  for (const it of items) {
    const span = parseSize(it.size ?? '1x1')
    // 先试它自己原来的位置
    let pos: GridPos | null = null
    if (typeof it.row === 'number' && typeof it.col === 'number') {
      const want: GridPos = { row: it.row, col: it.col, ...span }
      if (canPlace(want, occ, cols)) pos = want
    }
    // 原来那儿不行（没坐标 / 越界 / 被占）就找新位置
    if (!pos) pos = findFreeSpot(span, occ, cols)
    if (!pos) continue // 理论上不会发生（网格可无限往下长）

    for (const k of cellsOf(pos)) occ.set(k, it.id)
    out.push({ id: it.id, ...pos })
  }

  return out
}

/**
 * 把一张卡挪到目标位置，返回挪完之后所有卡片的新位置。
 *
 * 冲突策略：**把挡路的那个推到下一个空位**，而不是拒绝这次移动。
 *
 * 为什么是推走而不是拒绝 —— 拖到一半发现放不下、卡片弹回原处，
 * 用户只会觉得「这软件不好用」。推走的结果是「我想放这儿，它就让我放这儿」，
 * 被推的那张自动去别处，这才是人预期的。
 *
 * 只推一层：如果推它时又撞上别人，就递归（上限 8 层，防止死循环）。
 */
export function moveItem(
  items: PlacedItem[],
  id: string,
  target: { row: number; col: number },
  cols: number,
): PlacedItem[] {
  const moving = items.find((i) => i.id === id)
  if (!moving) return items

  const next = items.map((i) => ({ ...i }))
  const self = next.find((i) => i.id === id)!
  self.row = target.row
  self.col = target.col
  // 越界就夹回来，别让它跑到网格外
  if (self.col + self.w > cols) self.col = Math.max(0, cols - self.w)
  if (self.row < 0) self.row = 0
  if (self.col < 0) self.col = 0

  // 逐层把挡路的推开
  for (let depth = 0; depth < 8; depth++) {
    const occ = buildOccupancy(next.filter((i) => i.id !== id))
    const conflictId = findConflict(self, occ, id)
    if (!conflictId) break

    const victim = next.find((i) => i.id === conflictId)
    if (!victim) break

    const occForVictim = buildOccupancy(next.filter((i) => i.id !== conflictId))
    const spot = findFreeSpot({ w: victim.w, h: victim.h }, occForVictim, cols)
    if (!spot) break
    victim.row = spot.row
    victim.col = spot.col
  }

  return next
}

/** 网格需要多少行才能装下所有卡片（容器高度用） */
export function rowsOf(items: PlacedItem[]): number {
  let max = 0
  for (const it of items) max = Math.max(max, it.row + it.h)
  return max
}
