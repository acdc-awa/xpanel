// Chart palette from the pinned Cloudscape theme. Values are CSS custom-property
// references so light/dark tokens resolve at paint time.
import type { ChartSeries } from './types'

const PALETTE_KEYS = [
  '1-r41m2k',
  '2-37jc27',
  '3-2qk36g',
  '4-greu4f',
  '5-5cbtgt',
  '6-wr9hab',
  '7-m8ft2u',
  '8-f7ufk6',
  '9-sdg2ga',
  '10-yfb2h7',
  '11-b38mwa',
  '12-6ey9tm',
  '13-m3rl66',
  '14-7ospze',
  '15-fpd5zx',
  '16-3vux0m',
  '17-zb9wnt',
  '18-qyjp0b',
  '19-v16eb8',
  '20-ws3klu',
  '21-4m5ha5',
  '22-njdznc',
  '23-g4kffj',
  '24-fh9v3z',
  '25-6ba6en',
  '26-w7fzwx',
  '27-qrw50i',
  '28-x3v5x7',
  '29-drg4rc',
  '30-8j9ihk',
]

const FALLBACKS = [
  '#688ae8',
  '#c33d69',
  '#2ea597',
  '#8456ce',
  '#e07941',
  '#3759ce',
  '#962249',
  '#096f64',
  '#6237a7',
  '#a84401',
  '#273ea5',
  '#780d35',
  '#03524a',
  '#4a238b',
  '#7e3103',
  '#1b2b88',
  '#ce567c',
  '#003e38',
  '#9469d6',
  '#602400',
  '#4066df',
  '#a32952',
  '#0d7d70',
  '#6b40b2',
  '#bc4d01',
  '#2c46b1',
  '#81143b',
  '#045b52',
  '#512994',
  '#8a3603',
]

export const categoryPalette = PALETTE_KEYS.map(
  (key, index) => `var(--color-charts-palette-categorical-${key}, ${FALLBACKS[index]})`
)

export const THRESHOLD_NEUTRAL = 'var(--color-charts-threshold-neutral-iz3odf, #656871)'

/** Mirrors the official create-category-color-scale: thresholds never consume a palette slot. */
export function createCategoryColorScale(series: readonly ChartSeries[]): string[] {
  const colors: string[] = []
  let categoryIndex = 0
  for (const item of series) {
    if (item.color) {
      colors.push(item.color)
      if (item.type !== 'threshold') categoryIndex++
      continue
    }
    if (item.type === 'threshold') {
      colors.push(THRESHOLD_NEUTRAL)
    } else {
      colors.push(categoryPalette[categoryIndex % categoryPalette.length])
      categoryIndex++
    }
  }
  return colors
}
