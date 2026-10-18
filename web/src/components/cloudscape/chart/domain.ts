import type { ChartDataTypes, ChartSeries } from './types'
import { isDataSeries, isXThreshold, isYThreshold, matchesX } from './utils'

/** Domain of x values in first-seen order (official computeDomainX for categorical scales). */
export function computeDomainX(series: readonly ChartSeries[]): ChartDataTypes[] {
  const domain: ChartDataTypes[] = []
  const push = (value: ChartDataTypes) => {
    if (!domain.some((existing) => matchesX(existing, value))) domain.push(value)
  }
  for (const item of series) {
    if (isDataSeries(item)) {
      item.data.forEach((datum) => push(datum.x))
    } else if (isXThreshold(item)) {
      push(item.x)
    }
  }
  return domain
}

/** Y domain always includes zero; stacked bars contribute their cumulative sums. */
export function computeDomainY(series: readonly ChartSeries[], stackedBars: boolean): [number, number] {
  let min = 0
  let max = 0
  const include = (value: number) => {
    if (!isFinite(value)) return
    if (value < min) min = value
    if (value > max) max = value
  }

  if (stackedBars) {
    const positive = new Map<number | string, number>()
    const negative = new Map<number | string, number>()
    for (const item of series) {
      if (item.type !== 'bar') continue
      for (const datum of item.data) {
        const key = datum.x instanceof Date ? datum.x.getTime() : datum.x
        const bucket = datum.y < 0 ? negative : positive
        bucket.set(key, (bucket.get(key) ?? 0) + datum.y)
      }
    }
    positive.forEach(include)
    negative.forEach(include)
  }

  for (const item of series) {
    if (isYThreshold(item)) {
      include(item.y)
    } else if (isDataSeries(item)) {
      if (item.type === 'bar' && stackedBars) continue
      item.data.forEach((datum) => include(datum.y))
    }
  }

  return [min, max]
}
