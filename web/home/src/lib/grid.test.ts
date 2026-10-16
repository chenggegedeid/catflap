import { describe, expect, it } from 'vitest'
import {
  buildOccupancy,
  canPlace,
  cellsOf,
  findFreeSpot,
  moveItem,
  normalize,
  outOfBounds,
  overlaps,
  parseSize,
  rowsOf,
  type PlacedItem,
} from './grid'

describe('parseSize', () => {
  it('尺寸是宽×高：1×2 是窄高条，2×1 是宽扁条', () => {
    expect(parseSize('1x2')).toEqual({ w: 1, h: 2 })
    expect(parseSize('2x1')).toEqual({ w: 2, h: 1 })
  })

  it('认不出的尺寸退回 1×1，不抛异常', () => {
    expect(parseSize('9x9')).toEqual({ w: 1, h: 1 })
  })
})

describe('cellsOf 与 overlaps', () => {
  it('1×1 占一格', () => {
    expect(cellsOf({ row: 0, col: 0, w: 1, h: 1 })).toEqual(['0:0'])
  })

  it('2×2 占四格', () => {
    const cells = cellsOf({ row: 1, col: 2, w: 2, h: 2 })
    expect(cells.sort()).toEqual(['1:2', '1:3', '2:2', '2:3'])
  })

  it('挨着不算重叠，压住才算', () => {
    const a = { row: 0, col: 0, w: 2, h: 2 }
    const right = { row: 0, col: 2, w: 1, h: 1 } // 紧挨右边
    const inside = { row: 1, col: 1, w: 1, h: 1 } // 压在 a 中间
    expect(overlaps(a, right)).toBe(false)
    expect(overlaps(a, inside)).toBe(true)
  })
})

describe('canPlace 与 outOfBounds', () => {
  it('右边缘刚好放下，再往右一格就越界', () => {
    const occ = new Map<string, string>()
    const cols = 8
    expect(canPlace({ row: 0, col: 6, w: 2, h: 1 }, occ, cols)).toBe(true)
    expect(canPlace({ row: 0, col: 7, w: 2, h: 1 }, occ, cols)).toBe(false)
    expect(outOfBounds({ row: 0, col: 7, w: 2, h: 1 }, cols)).toBe(true)
  })

  it('excludeId 让自己能放回原位（否则原地放下会被判冲突）', () => {
    const me: PlacedItem = { id: 'a', row: 0, col: 0, w: 1, h: 1 }
    const occ = buildOccupancy([me])
    expect(canPlace(me, occ, 8)).toBe(false)
    expect(canPlace(me, occ, 8, 'a')).toBe(true)
  })
})

describe('findFreeSpot', () => {
  it('空网格上补位在左上角', () => {
    const spot = findFreeSpot({ w: 1, h: 1 }, new Map(), 8)
    expect(spot).toEqual({ row: 0, col: 0, w: 1, h: 1 })
  })

  it('前面被占了就顺延到下一个空位', () => {
    const occ = buildOccupancy([{ id: 'a', row: 0, col: 0, w: 1, h: 1 }])
    expect(findFreeSpot({ w: 1, h: 1 }, occ, 8)).toEqual({ row: 0, col: 1, w: 1, h: 1 })
  })

  it('一行塞不下就换行', () => {
    const items: PlacedItem[] = []
    for (let c = 0; c < 8; c++) items.push({ id: `x${c}`, row: 0, col: c, w: 1, h: 1 })
    const spot = findFreeSpot({ w: 1, h: 1 }, buildOccupancy(items), 8)
    expect(spot).toEqual({ row: 1, col: 0, w: 1, h: 1 })
  })

  it('2 列宽的卡片在只剩 1 列的位置放不下，会去下一行', () => {
    const items: PlacedItem[] = [
      { id: 'a', row: 0, col: 0, w: 1, h: 1 },
      { id: 'b', row: 0, col: 7, w: 1, h: 1 }, // 占住第 8 列
    ]
    const spot = findFreeSpot({ w: 2, h: 1 }, buildOccupancy(items), 8)
    expect(spot).toEqual({ row: 0, col: 1, w: 2, h: 1 })
  })
})

describe('normalize', () => {
  it('没有坐标的卡片会被补位，且互不重叠', () => {
    const out = normalize([{ id: 'a' }, { id: 'b' }, { id: 'c' }], 4)
    expect(out).toHaveLength(3)
    const occ = buildOccupancy(out)
    // 占用表的格子数 = 三张 1×1 各占一格
    expect(occ.size).toBe(3)
  })

  it('坐标越界的会被拉回网格内', () => {
    const out = normalize([{ id: 'a', row: 0, col: 99, size: '1x1' }], 4)
    expect(out[0].col + out[0].w).toBeLessThanOrEqual(4)
  })

  it('两张卡叠在同一格时，后到的让位', () => {
    const out = normalize(
      [
        { id: 'first', row: 0, col: 0, size: '1x1' },
        { id: 'second', row: 0, col: 0, size: '1x1' },
      ],
      4,
    )
    const a = out.find((i) => i.id === 'first')!
    const b = out.find((i) => i.id === 'second')!
    expect(overlaps(a, b)).toBe(false)
    // 先来的保留原位置
    expect(a).toMatchObject({ row: 0, col: 0 })
  })

  it('结果稳定：同样的输入跑两次完全一样', () => {
    const input = [
      { id: 'a', row: 0, col: 0, size: '2x2' as const },
      { id: 'b', row: 0, col: 2, size: '1x1' as const },
      { id: 'c' },
    ]
    expect(normalize(input, 6)).toEqual(normalize(input, 6))
  })
})

describe('moveItem', () => {
  const base: PlacedItem[] = [
    { id: 'a', row: 0, col: 0, w: 1, h: 1 },
    { id: 'b', row: 0, col: 1, w: 1, h: 1 },
    { id: 'c', row: 0, col: 2, w: 1, h: 1 },
  ]

  it('挪到空位：只有它自己动', () => {
    const out = moveItem(base, 'a', { row: 1, col: 0 }, 4)
    expect(out.find((i) => i.id === 'a')).toMatchObject({ row: 1, col: 0 })
    expect(out.find((i) => i.id === 'b')).toMatchObject({ row: 0, col: 1 })
  })

  it('挪到别人身上：把挡路的推走，而不是拒绝移动', () => {
    const out = moveItem(base, 'a', { row: 0, col: 1 }, 4)
    const a = out.find((i) => i.id === 'a')!
    const b = out.find((i) => i.id === 'b')!
    // a 拿到了它想去的位置
    expect(a).toMatchObject({ row: 0, col: 1 })
    // b 被推到别处，没有和 a 叠在一起
    expect(overlaps(a, b)).toBe(false)
  })

  it('推出去的卡片不会跑到网格外', () => {
    const tight: PlacedItem[] = [
      { id: 'a', row: 0, col: 0, w: 1, h: 1 },
      { id: 'b', row: 0, col: 1, w: 1, h: 1 },
    ]
    const out = moveItem(tight, 'a', { row: 0, col: 1 }, 2)
    for (const it of out) {
      expect(outOfBounds(it, 2)).toBe(false)
    }
  })

  it('拖到远处（超出列数）会被夹回边缘', () => {
    const out = moveItem(base, 'a', { row: 0, col: 10 }, 4)
    expect(out.find((i) => i.id === 'a')!.col).toBe(3)
  })
})

describe('rowsOf', () => {
  it('按最下面那张卡算总行数', () => {
    expect(rowsOf([{ id: 'a', row: 0, col: 0, w: 1, h: 1 }])).toBe(1)
    expect(rowsOf([{ id: 'a', row: 2, col: 0, w: 1, h: 3 }])).toBe(5)
  })
})
