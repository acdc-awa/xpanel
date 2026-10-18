// Types mirroring @cloudscape-design/components@3.0.1401 chart interfaces.
export type ChartDataTypes = number | string | Date

export interface ChartDatum {
  x: ChartDataTypes
  y: number
}

interface SeriesBase {
  title: string
  /** Explicit color; when omitted the categorical palette assigns one. */
  color?: string
  /** Formats the value shown in the detail popover. */
  valueFormatter?: (value: number, x: ChartDataTypes) => string
}

export interface BarSeries extends SeriesBase {
  type: 'bar'
  data: ChartDatum[]
}

export interface LineSeries extends SeriesBase {
  type: 'line'
  data: ChartDatum[]
  /** Renders the line with the official threshold dash pattern (used for peak/envelope lines). */
  dashed?: boolean
}

export interface YThresholdSeries extends SeriesBase {
  type: 'threshold'
  y: number
}

export interface XThresholdSeries extends SeriesBase {
  type: 'threshold'
  x: ChartDataTypes
}

export type ChartSeries = BarSeries | LineSeries | YThresholdSeries | XThresholdSeries

export interface ChartProps {
  series?: ChartSeries[]
  /** Plot height in px (ignored when `fitHeight` is set). */
  height?: number
  /** Fill the container height instead of using `height`. */
  fitHeight?: boolean
  /** Stack bars that share the same x value. */
  stackedBars?: boolean
  xScaleType?: 'categorical' | 'linear'
  xTickFormatter?: (x: ChartDataTypes) => string
  yTickFormatter?: (y: number) => string
  xTitle?: string
  yTitle?: string
  legendTitle?: string
  hideLegend?: boolean
  emptyText?: string
  ariaLabel?: string
  loading?: boolean
  /** Emphasizes the zero baseline (official `emphasizeBaselineAxis`). */
  emphasizeBaselineAxis?: boolean
}
