<script setup lang="ts">
// Vue port of the official Cloudscape LineChart / BarChart entry components.
import { computed } from 'vue'
import ChartPlot from './ChartPlot.vue'
import type { ChartDataTypes, ChartProps, ChartSeries } from './types'

const props = withDefaults(
  defineProps<{
    /** Line or threshold series. */
    series?: Extract<ChartSeries, { type: 'line' | 'threshold' }>[]
    height?: number
    fitHeight?: boolean
    xTickFormatter?: (x: ChartDataTypes) => string
    yTickFormatter?: (y: number) => string
    xTitle?: string
    yTitle?: string
    legendTitle?: string
    hideLegend?: boolean
    emptyText?: string
    ariaLabel?: string
    loading?: boolean
  }>(),
  {
    series: () => [],
    height: 300,
    fitHeight: false,
    xTickFormatter: undefined,
    yTickFormatter: undefined,
    xTitle: undefined,
    yTitle: undefined,
    legendTitle: undefined,
    hideLegend: false,
    emptyText: '暂无数据',
    ariaLabel: '折线图',
    loading: false,
  }
)

const plotProps = computed<ChartProps>(() => ({
  series: props.series,
  height: props.height,
  fitHeight: props.fitHeight,
  xScaleType: 'categorical',
  xTickFormatter: props.xTickFormatter,
  yTickFormatter: props.yTickFormatter,
  xTitle: props.xTitle,
  yTitle: props.yTitle,
  legendTitle: props.legendTitle,
  hideLegend: props.hideLegend,
  emptyText: props.emptyText,
  ariaLabel: props.ariaLabel,
  loading: props.loading,
  emphasizeBaselineAxis: true,
}))
</script>

<template>
  <ChartPlot v-bind="plotProps">
    <template v-if="$slots['popover-footer']" #popover-footer="slotProps">
      <slot name="popover-footer" v-bind="slotProps" />
    </template>
  </ChartPlot>
</template>
