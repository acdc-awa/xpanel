import { GRID, snapToGrid } from './model'

export interface Rect {
  x: number
  y: number
  w: number
  h: number
}

export interface Size {
  w: number
  h: number
}

/** 两个矩形是否相交（padding 为额外安全边距）。 */
export function rectsOverlap(a: Rect, b: Rect, padding = 0): boolean {
  return !(
    a.x + a.w + padding <= b.x ||
    b.x + b.w + padding <= a.x ||
    a.y + a.h + padding <= b.y ||
    b.y + b.h + padding <= a.y
  )
}

/**
 * 从 anchor 出发找最近的空位。
 *
 * 候选点取「锚点 + 每个障碍物的四条边外沿」——轴对齐矩形在固定尺寸下的最优空位
 * 一定落在这些边界上，比按步长铺网格扫描更准，候选数也只有 O(n²)。
 * 返回的左上角坐标始终吸附到 GRID。
 */
export function findFreeSlot(opts: {
  anchor: { x: number; y: number }
  size: Size
  occupied: Rect[]
  padding?: number
  grid?: number
}): { x: number; y: number } {
  const { anchor, size, occupied } = opts
  const padding = opts.padding ?? 24
  const grid = opts.grid ?? GRID
  const snap = (v: number) => Math.round(v / grid) * grid
  const base = { x: snap(anchor.x), y: snap(anchor.y) }

  const xs = new Set<number>([base.x])
  const ys = new Set<number>([base.y])
  for (const r of occupied) {
    xs.add(snap(r.x + r.w + padding))
    xs.add(snap(r.x - padding - size.w))
    ys.add(snap(r.y + r.h + padding))
    ys.add(snap(r.y - padding - size.h))
  }

  let best: { x: number; y: number } | null = null
  let bestScore = Number.POSITIVE_INFINITY
  for (const x of xs) {
    for (const y of ys) {
      const rect: Rect = { x, y, w: size.w, h: size.h }
      if (occupied.some((r) => rectsOverlap(rect, r, padding))) continue
      // 距离优先；同距离时优先下方、再优先右侧，保证「新卡片顺着内容往下长」
      const dx = x - base.x
      const dy = y - base.y
      const score = dx * dx + dy * dy * 1.15
      if (score < bestScore - 0.001 || (Math.abs(score - bestScore) <= 0.001 && y > (best?.y ?? -Infinity))) {
        bestScore = score
        best = { x, y }
      }
    }
  }
  if (best) return best

  // 所有候选都冲突（极端情况）：放到已占用区域下方
  const bottom = occupied.reduce((m, r) => Math.max(m, r.y + r.h), base.y)
  return { x: snapToGrid(base.x), y: snapToGrid(bottom + padding) }
}
