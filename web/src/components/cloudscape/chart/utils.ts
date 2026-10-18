import type { ChartDataTypes, ChartDatum, ChartSeries } from './types'

export function getKeyValue(key: ChartDataTypes): number | string {
  return key instanceof Date ? key.getTime() : key
}

export function matchesX(a: ChartDataTypes, b: ChartDataTypes): boolean {
  if (a instanceof Date && b instanceof Date) return a.getTime() === b.getTime()
  return a === b
}

export function isYThreshold(series: ChartSeries): series is Extract<ChartSeries, { type: 'threshold'; y: number }> {
  return series.type === 'threshold' && 'y' in series && (series as { y?: number }).y !== undefined
}

export function isXThreshold(series: ChartSeries): series is Extract<ChartSeries, { type: 'threshold'; x: ChartDataTypes }> {
  return series.type === 'threshold' && 'x' in series && (series as { x?: ChartDataTypes }).x !== undefined
}

export function isDataSeries(series: ChartSeries): series is Extract<ChartSeries, { type: 'line' | 'bar' }> {
  return series.type === 'line' || series.type === 'bar'
}

/**
 * Cumulative values for stacked bars, keyed by x and then by series index.
 * Negative values are accumulated separately so positive and negative bars
 * stack away from the zero baseline (matching the official implementation).
 */
export function calculateStackedBarValues(dataBySeries: ChartDatum[][]): Map<number | string, Map<number, number>> {
  const negative = new Map<number | string, number>()
  const positive = new Map<number | string, number>()
  const values = new Map<number | string, Map<number, number>>()

  for (let seriesIndex = 0; seriesIndex < dataBySeries.length; seriesIndex++) {
    for (const datum of dataBySeries[seriesIndex]) {
      const key = getKeyValue(datum.x)
      if (datum.y < 0) {
        negative.set(key, (negative.get(key) ?? 0) + datum.y)
      } else {
        positive.set(key, (positive.get(key) ?? 0) + datum.y)
      }
      const cumulative = datum.y < 0 ? (negative.get(key) ?? 0) : (positive.get(key) ?? 0)
      const byIndex = values.get(key) ?? new Map<number, number>()
      byIndex.set(seriesIndex, cumulative)
      values.set(key, byIndex)
    }
  }

  return values
}

type RoundedSide = 'left' | 'right' | 'top' | 'bottom'

/**
 * Port of the official create-one-side-rounded-rect-path helper: a rectangle
 * where only the corners of one side are rounded (used for stacked bar ends).
 */
export function createOneSideRoundedRectPath(
  { x, y, width, height }: { x: number; y: number; width: number; height: number },
  radius: number,
  side: RoundedSide
): string {
  const coordinates = [
    { x, y },
    { x: x + width, y },
    { x: x + width, y: y + height },
    { x, y: y + height },
  ]
  const startIndex = { left: 2, right: 0, top: 3, bottom: 1 }[side]

  let path = ''
  for (let i = startIndex; i < startIndex + coordinates.length + 1; i++) {
    const start = coordinates[i % coordinates.length]
    const end = coordinates[(i + 1) % coordinates.length]
    const c =
      radius && i < startIndex + 3
        ? Math.min(radius / Math.hypot(end.x - start.x, end.y - start.y), 0.5)
        : 0

    if (i === startIndex) {
      path += `M${start.x * (1 - c) + end.x * c},${start.y * (1 - c) + end.y * c}`
    }
    if (i > startIndex) {
      path += `Q${start.x},${start.y} ${start.x * (1 - c) + end.x * c},${start.y * (1 - c) + end.y * c}`
    }
    if (i < startIndex + coordinates.length) {
      path += `L${start.x * c + end.x * (1 - c)},${start.y * c + end.y * (1 - c)}`
    }
  }
  return path + 'Z'
}

const TEXT_FONT = '12px "Open Sans", "Helvetica Neue", Roboto, Arial, sans-serif'
const textWidthCache = new Map<string, number>()
let measureContext: CanvasRenderingContext2D | null | undefined

/** Measures tick label width so the axis gutter matches the rendered text. */
export function measureText(text: string): number {
  if (typeof document === 'undefined') return text.length * 7
  if (measureContext === undefined) {
    measureContext = document.createElement('canvas').getContext('2d')
  }
  if (!measureContext) return text.length * 7

  const cacheKey = `${TEXT_FONT}|${text}`
  const cached = textWidthCache.get(cacheKey)
  if (cached !== undefined) return cached

  measureContext.font = TEXT_FONT
  const width = measureContext.measureText(text).width
  textWidthCache.set(cacheKey, width)
  return width
}

/** Same output as the official `String(value)` after trimming float noise. */
export function formatDefaultNumber(value: number): string {
  if (!isFinite(value)) return ''
  return String(Number(value.toPrecision(6)))
}

export interface TickLayoutItem {
  position: number
  width: number
}

function hasIntersection(a: TickLayoutItem, b: TickLayoutItem): boolean {
  const [left, right] = a.position < b.position ? [a, b] : [b, a]
  const leftEdge = left.position + left.width / 2 + 4
  const rightEdge = right.position - right.width / 2
  return leftEdge > rightEdge
}

/**
 * Official `getReducedTicks`: pick a uniform stride (1, 2, 3 ...) so the kept
 * labels never overlap, keeping the spacing even instead of greedily skipping.
 */
export function reduceTicks<T extends TickLayoutItem>(ticks: T[]): T[] {
  if (ticks.length <= 1) return ticks
  const findRatio = (ratio: number): number => {
    if (ratio >= ticks.length) return ratio
    for (let i = ratio; i < ticks.length; i += ratio) {
      if (hasIntersection(ticks[i - ratio], ticks[i])) return findRatio(ratio + 1)
    }
    return ratio
  }
  const ratio = findRatio(1)
  return ticks.filter((_, index) => index % ratio === 0)
}
