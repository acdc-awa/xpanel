<script setup lang="ts">
import { inject, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import * as echarts from 'echarts'
import { CsDashboardWidget, CsSegmentedControl } from '@/components/cloudscape'
import type { SegmentedOption } from '@/components/cloudscape'
import { useThemeStore } from '@/stores/theme'
import type { DashboardContext } from '../types'
import type { WidgetLayoutItem } from '@/components/cloudscape'

defineProps<{
  layoutItem: WidgetLayoutItem
}>()

const ctx = inject<DashboardContext>('adminDashboardContext')!
const theme = useThemeStore()

const chartRef = ref<HTMLDivElement | null>(null)
let chart: echarts.ECharts | null = null
let ro: ResizeObserver | null = null

const trendOptions: SegmentedOption[] = [
  { id: 3, text: '3 天' },
  { id: 7, text: '7 天' },
  { id: 30, text: '30 天' },
]

function updateChart() {
  if (!chartRef.value) return
  if (!chart) {
    chart = echarts.init(chartRef.value)
  }

  const isMob = typeof window !== 'undefined' ? window.innerWidth <= 768 : false
  const isDark = theme.isDark

  const textColor = isDark ? '#9ba7b6' : '#656871'
  const titleColor = isDark ? '#f9fafb' : '#0f141a'
  const axisLineColor = isDark ? '#2a3441' : '#e9ebed'
  const splitLineColor = isDark ? '#232b37' : '#e9ebed'
  const tooltipBg = isDark ? '#161f2e' : '#ffffff'
  const tooltipBorder = isDark ? '#2a3441' : '#e9ebed'

  let dates: string[] = []
  let downData: number[] = []
  let upData: number[] = []

  if (ctx.dashData.value?.traffic_trend?.length) {
    const trend = ctx.dashData.value.traffic_trend
    dates = trend.map((t) => t.date.slice(5))
    downData = trend.map((t) => parseFloat((t.down_bytes / 1073741824).toFixed(2)))
    upData = trend.map((t) => parseFloat((t.up_bytes / 1073741824).toFixed(2)))
  }

  chart.setOption({
    animationDuration: 300,
    tooltip: {
      trigger: 'axis',
      axisPointer: { type: 'shadow' },
      backgroundColor: tooltipBg,
      borderColor: tooltipBorder,
      borderWidth: 1,
      textStyle: { color: titleColor, fontSize: 12, fontFamily: "'Open Sans', sans-serif" },
      formatter: (params: any) => {
        let res = `<div style="font-weight:700;margin-bottom:6px;color:${titleColor}">${params[0]?.axisValue} (UTC)</div>`
        let total = 0
        params.forEach((item: any) => {
          total += Number(item.value || 0)
          res += `<div style="display:flex;align-items:center;justify-content:space-between;gap:16px;margin:3px 0">
            <span style="display:flex;align-items:center;gap:6px">
              <span style="display:inline-block;width:8px;height:8px;border-radius:2px;background:${item.color};"></span>
              <span style="color:${textColor};font-size:11.5px">${item.seriesName}</span>
            </span>
            <b style="color:${titleColor};font-variant-numeric:tabular-nums">${Number(item.value).toFixed(2)} GB</b>
          </div>`
        })
        res += `<div style="border-top:1px dashed ${splitLineColor};margin-top:6px;padding-top:4px;display:flex;justify-content:space-between">
          <span style="color:${textColor};font-size:11.5px">合计用量</span>
          <b style="color:#006ce0;font-variant-numeric:tabular-nums">${total.toFixed(2)} GB</b>
        </div>`
        return res
      },
    },
    legend: {
      data: ['下行流量 (Ingress)', '上行流量 (Egress)'],
      top: 0,
      icon: 'roundRect',
      itemWidth: 12,
      itemHeight: 8,
      textStyle: { color: textColor, fontSize: 11.5, fontFamily: "'Open Sans', sans-serif" },
    },
    grid: {
      left: isMob ? 36 : 48,
      right: isMob ? 16 : 28,
      top: 42,
      bottom: isMob ? 28 : 34,
    },
    xAxis: {
      type: 'category',
      data: dates,
      axisLine: { lineStyle: { color: axisLineColor } },
      axisTick: { show: false },
      axisLabel: {
        color: textColor,
        fontSize: 11,
        fontFamily: "'Open Sans', sans-serif",
        hideOverlap: true,
      },
    },
    yAxis: {
      type: 'value',
      name: 'GB',
      nameTextStyle: { color: textColor, fontSize: 11, align: 'right', padding: [0, 4, 0, 0] },
      splitNumber: 3,
      splitLine: { lineStyle: { type: 'dashed', color: splitLineColor } },
      axisLabel: { color: textColor, fontSize: 11, fontFamily: "'Open Sans', sans-serif" },
    },
    series: [
      {
        name: '下行流量 (Ingress)',
        type: 'bar',
        stack: 'total',
        barMaxWidth: 20,
        itemStyle: { color: '#006ce0' },
        data: downData,
      },
      {
        name: '上行流量 (Egress)',
        type: 'bar',
        stack: 'total',
        barMaxWidth: 20,
        itemStyle: { color: '#8c4bff', borderRadius: [3, 3, 0, 0] },
        data: upData,
      },
    ],
  })
}

watch([() => ctx.dashData.value, () => ctx.trendRange.value], () => {
  nextTick(() => updateChart())
})

watch(() => theme.isDark, () => {
  nextTick(() => updateChart())
})

onMounted(() => {
  nextTick(() => {
    updateChart()
    if (chartRef.value) {
      ro = new ResizeObserver(() => {
        chart?.resize()
      })
      ro.observe(chartRef.value)
    }
  })
})

onUnmounted(() => {
  ro?.disconnect()
  ro = null
  chart?.dispose()
  chart = null
})
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
        @change="updateChart"
      />
    </template>
    <div class="chart-container">
      <div ref="chartRef" class="aws-echart"></div>
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

.aws-echart {
  width: 100%;
  height: 100%;
  min-height: 0;
  min-width: 0;
  flex: 1;
}
</style>
