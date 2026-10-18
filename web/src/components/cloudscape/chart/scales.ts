import type { ChartDataTypes } from './types'

/**
 * Categorical band scale matching d3-scale's scaleBand with the padding values
 * Cloudscape uses (inner 0.2 / outer 0.05 for charts with bars, inner 0.7 / outer 0
 * for line-only charts).
 */
export interface BandScale {
  domain: ChartDataTypes[]
  position: (value: ChartDataTypes) => number
  bandwidth: () => number
  step: number
}

export function createBandScale(
  domain: ChartDataTypes[],
  rangeStart: number,
  rangeEnd: number,
  innerPadding: number,
  outerPadding: number
): BandScale {
  const span = rangeEnd - rangeStart
  const step =
    domain.length > 0 ? span / Math.max(1e-9, domain.length - innerPadding + 2 * outerPadding) : 0
  const bandwidth = Math.max(0, step * (1 - innerPadding))
  const position = (value: ChartDataTypes) => {
    const index = domain.findIndex((d) => (d instanceof Date && value instanceof Date ? d.getTime() === value.getTime() : d === value))
    if (index < 0) return NaN
    return rangeStart + step * (outerPadding + index)
  }
  return { domain, position, bandwidth: () => bandwidth, step }
}

export interface LinearScale {
  min: number
  max: number
  ticks: number[]
  scale: (value: number) => number
}

/** Rounds the raw step to the nearest 1/2/5 x 10^n bucket, like d3's nice(). */
function niceStep(span: number, tickCount: number): number {
  const rough = span / Math.max(1, tickCount)
  if (!isFinite(rough) || rough <= 0) return 1
  const magnitude = Math.pow(10, Math.floor(Math.log10(rough)))
  const normalized = rough / magnitude
  const multiplier = normalized <= 1 ? 1 : normalized <= 2 ? 2 : normalized <= 5 ? 5 : 10
  return multiplier * magnitude
}

export function createLinearScale(
  domain: [number, number],
  rangeStart: number,
  rangeEnd: number,
  tickCount: number
): LinearScale {
  let [min, max] = domain
  if (!isFinite(min) || !isFinite(max)) {
    min = 0
    max = 1
  }
  if (min === max) {
    if (min === 0) {
      max = 1
    } else {
      const delta = Math.abs(min) * 0.1
      min -= delta
      max += delta
    }
  }

  const step = niceStep(max - min, tickCount)
  const niceMin = Math.floor(min / step) * step
  const niceMax = Math.ceil(max / step) * step
  const decimals = Math.max(0, -Math.floor(Math.log10(step)) + 2)

  const ticks: number[] = []
  for (let value = niceMin; value <= niceMax + step / 2; value += step) {
    ticks.push(Number(value.toFixed(decimals)))
  }

  const scale = (value: number) => {
    if (niceMax === niceMin) return rangeStart
    return rangeStart + ((value - niceMin) / (niceMax - niceMin)) * (rangeEnd - rangeStart)
  }

  return { min: niceMin, max: niceMax, ticks, scale }
}
