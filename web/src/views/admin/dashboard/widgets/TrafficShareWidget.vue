<script setup lang="ts">
import { computed, inject, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
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

const rankPeriodOptions: SegmentedOption[] = [
  { id: 'today', text: '今日' },
  { id: '7d', text: '7 天' },
  { id: '30d', text: '30 天' },
]

const RANK_PERIOD_LABELS: Record<string, string> = { today: '今日', '7d': '近 7 天', '30d': '近 30 天', month: '近 30 天' }
const rankLabel = computed(() => ctx.dashData.value?.rank_label || RANK_PERIOD_LABELS[ctx.rankPeriod.value] || '今日')

const MOCK_SERVER_BREAKDOWN = [
  { name: 'hk-edge-01 (CN2 GIA)', value: 482.5, percent: 36.5 },
  { name: 'tokyo-bgp-02 (SoftBank)', value: 356.2, percent: 27.0 },
  { name: 'sg-direct-01 (SingTel)', value: 242.0, percent: 18.3 },
  { name: 'us-west-01 (Silicon Valley)', value: 154.8, percent: 11.7 },
  { name: 'de-frankfurt-01 (Core)', value: 86.5, percent: 6.5 },
]

const awsChartPalette = ['#006ce0', '#8c4bff', '#00a1c9', '#ec7211', '#1d8102', '#656871', '#d13212', '#2489c5']

function updateChart() {
  if (!chartRef.value) return
  if (!chart) {
    chart = echarts.init(chartRef.value)
  }

  const isDark = theme.isDark
  const textColor = isDark ? '#9ba7b6' : '#656871'
  const titleColor = isDark ? '#f9fafb' : '#0f141a'
  const tooltipBg = isDark ? '#161f2e' : '#ffffff'
  const tooltipBorder = isDark ? '#2a3441' : '#e9ebed'
  const donutCenterBg = isDark ? '#161f2e' : '#ffffff'

  let pieData: { name: string; value: number; percent?: number }[] = []
  if (ctx.useMockData.value) {
    pieData = MOCK_SERVER_BREAKDOWN.map((m) => ({
      name: m.name,
      value: m.value,
      percent: m.percent,
    }))
  } else if (ctx.dashData.value?.server_rank?.length) {
    pieData = ctx.dashData.value.server_rank.map((s) => ({
      name: s.name || `节点#${s.server_id}`,
      value: parseFloat((s.total_bytes / 1073741824).toFixed(2)),
      percent: s.percent,
    }))
  }

  const isNarrow = chartRef.value.clientWidth < 420

  chart.setOption({
    animationDuration: 300,
    color: awsChartPalette,
    tooltip: {
      trigger: 'item',
      backgroundColor: tooltipBg,
      borderColor: tooltipBorder,
      borderWidth: 1,
      textStyle: { color: titleColor, fontSize: 12, fontFamily: "'Open Sans', sans-serif" },
      formatter: (params: any) => {
        const item = params.data
        const pct = item.percent !== undefined ? item.percent.toFixed(1) : params.percent
        return `<div style="font-weight:700;margin-bottom:4px;color:${titleColor}">${params.name}</div>
        <div style="display:flex;align-items:center;justify-content:space-between;gap:16px">
          <span style="color:${textColor};font-size:11.5px">承载用量</span>
          <b style="color:${titleColor};font-variant-numeric:tabular-nums">${params.value} GB</b>
        </div>
        <div style="display:flex;align-items:center;justify-content:space-between;gap:16px;margin-top:2px">
          <span style="color:${textColor};font-size:11.5px">全网流量占比</span>
          <b style="color:#006ce0;font-variant-numeric:tabular-nums">${pct}%</b>
        </div>`
      },
    },
    legend: isNarrow
      ? {
          orient: 'horizontal',
          bottom: 4,
          left: 'center',
          itemWidth: 8,
          itemHeight: 8,
          textStyle: { color: textColor, fontSize: 10.5, fontFamily: "'Open Sans', sans-serif" },
        }
      : {
          orient: 'vertical',
          right: '8%',
          top: 'middle',
          itemWidth: 9,
          itemHeight: 9,
          textStyle: { color: textColor, fontSize: 11, fontFamily: "'Open Sans', sans-serif" },
          formatter: (name: string) => {
            return name.length > 12 ? name.slice(0, 11) + '…' : name
          },
        },
    series: [
      {
        name: '服务器流量',
        type: 'pie',
        radius: isNarrow ? ['46%', '72%'] : ['58%', '84%'],
        center: isNarrow ? ['50%', '38%'] : ['34%', '52%'],
        avoidLabelOverlap: false,
        itemStyle: {
          borderRadius: 3,
          borderColor: donutCenterBg,
          borderWidth: 2,
        },
        label: { show: false },
        labelLine: { show: false },
        emphasis: {
          scale: true,
          scaleSize: 5,
        },
        data: pieData.length ? pieData : [{ name: '暂无流量', value: 0 }],
      },
    ],
  })
}

watch([() => ctx.dashData.value, () => ctx.useMockData.value, () => ctx.rankPeriod.value], () => {
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
    id="traffic-share"
    title="服务器流量分布"
    :badge="rankLabel"
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
        v-model="ctx.rankPeriod.value"
        :options="rankPeriodOptions"
        @change="ctx.refreshDashboard"
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
