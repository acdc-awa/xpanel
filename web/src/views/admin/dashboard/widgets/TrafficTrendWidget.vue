<script setup lang="ts">
import { computed, inject } from 'vue'
import { CsBarChart, CsDashboardWidget, CsSegmentedControl } from '@/components/cloudscape'
import type { BarSeries, SegmentedOption } from '@/components/cloudscape'
import type { DashboardContext } from '../types'
import type { WidgetLayoutItem } from '@/components/cloudscape'

defineProps<{
  layoutItem: WidgetLayoutItem
}>()

const ctx = inject<DashboardContext>('adminDashboardContext')!

const trendOptions: SegmentedOption[] = [
  { id: 3, text: '3 天' },
  { id: 7, text: '7 天' },
  { id: 30, text: '30 天' },
]

const GB = 1024 * 1024 * 1024

function toGb(bytes: number): number {
  return Number((bytes / GB).toFixed(2))
}

const series = computed<BarSeries[]>(() => {
  const trend = ctx.dashData.value?.traffic_trend || []
  return [
    {
      type: 'bar',
      title: '下行流量 (Ingress)',
      data: trend.map((t) => ({ x: t.date.slice(5), y: toGb(t.down_bytes) })),
      valueFormatter: (value) => `${value.toFixed(2)} GB`,
    },
    {
      type: 'bar',
      title: '上行流量 (Egress)',
      data: trend.map((t) => ({ x: t.date.slice(5), y: toGb(t.up_bytes) })),
      valueFormatter: (value) => `${value.toFixed(2)} GB`,
    },
  ]
})

const formatAxisValue = (value: number) => (value >= 1000 ? `${(value / 1000).toFixed(1)}k` : String(value))
</script>

<template>
  <CsDashboardWidget
    id="traffic-trend"
    title="吞吐趋势"
    :x="layoutItem.x"
    :y="layoutItem.y"
    :w="layoutItem.w"
    :h="layoutItem.h"
    :min-w="layoutItem.minW"
    :min-h="layoutItem.minH"
    no-padding
    @refresh="ctx.refreshDashboard"
  >
    <template #actions>
      <CsSegmentedControl
        v-model="ctx.trendRange.value"
        :options="trendOptions"
        @change="ctx.refreshDashboard"
      />
    </template>
    <div class="chart-container">
      <CsBarChart
        :series="series"
        stacked-bars
        fit-height
        y-title="GB"
        :y-tick-formatter="formatAxisValue"
        aria-label="吞吐趋势"
        empty-text="暂无流量数据"
      />
    </div>
  </CsDashboardWidget>
</template>

<style scoped lang="scss">
.chart-container {
  width: 100%;
  height: 100%;
  min-height: 0;
  min-width: 0;
  flex: 1;
  display: flex;
  overflow: hidden;
  box-sizing: border-box;
  padding: 0 12px 12px;
}
</style>
