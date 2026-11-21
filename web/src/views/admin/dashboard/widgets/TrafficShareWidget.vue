<script setup lang="ts">
import { inject, nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import * as echarts from 'echarts'
import { CsDashboardWidget, CsSegmentedControl } from '@/components/cloudscape'
import { rankPeriodOptions } from '../rank-period'
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
let lastNarrow: boolean | null = null

const CHART_PALETTE_TOKENS: Array<[string, string]> = [
  ['--color-text-link-default-qabnuk', '#006ce0'],
  ['--color-charts-palette-categorical-3-2qk36g', '#2ea597'],
  ['--color-charts-palette-categorical-5-5cbtgt', '#e07941'],
  ['--color-charts-palette-categorical-1-r41m2k', '#688ae8'],
  ['--color-text-status-success-1i2jf8', '#1d8102'],
  ['--color-charts-palette-categorical-2-37jc27', '#c33d69'],
  ['--color-text-body-secondary-66ygbb', '#656871'],
  ['--color-text-status-info-55w81a', '#00a1c9'],
]
const CHART_FONT_FAMILY =
  "-apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'PingFang SC', 'Hiragino Sans GB', 'Microsoft YaHei', sans-serif"

function readToken(el: HTMLElement, token: string, fallbackTokenOrColor: string): string {
  const style = getComputedStyle(el)
  const val = style.getPropertyValue(token).trim()
  if (val) return val
  if (fallbackTokenOrColor.startsWith('--')) {
    return style.getPropertyValue(fallbackTokenOrColor).trim()
  }
  return fallbackTokenOrColor
}

function escapeHtml(raw: string): string {
  return String(raw)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
}

function updateChart() {
  if (!chartRef.value) return
  if (!chart) {
    chart = echarts.init(chartRef.value)
  }

  const hostEl = document.body || chartRef.value
  const textColor = readToken(hostEl, '--color-text-body-secondary-66ygbb', '--x-text-2')
  const titleColor = readToken(hostEl, '--color-text-body-default-b2us04', '--x-text')
  const accentColor = readToken(hostEl, '--color-text-link-default-qabnuk', '--x-primary')
  const tooltipBg = readToken(hostEl, '--color-background-container-content-mjfil9', '--x-card')
  const tooltipBorder = readToken(hostEl, '--color-border-divider-default-diwd4w', '--x-border')
  const donutCenterBg = tooltipBg
  const chartPalette = CHART_PALETTE_TOKENS.map(([token, fallback]) => readToken(hostEl, token, fallback))

  let pieData: { name: string; value: number; percent?: number }[] = []
  if (ctx.dashData.value?.server_rank?.length) {
    pieData = ctx.dashData.value.server_rank.map((s) => {
      const base = s.name || `服务器#${s.server_id}`
      return {
        name: s.deleted ? `${base}（已删除）` : base,
        value: parseFloat((s.total_bytes / 1073741824).toFixed(2)),
        percent: s.percent,
      }
    })
  }

  const isNarrow = chartRef.value.clientWidth < 420
  lastNarrow = isNarrow

  chart.setOption({
    animationDuration: 300,
    color: chartPalette,
    tooltip: {
      trigger: 'item',
      backgroundColor: tooltipBg,
      borderColor: tooltipBorder,
      borderWidth: 1,
      textStyle: { color: titleColor, fontSize: 12, fontFamily: CHART_FONT_FAMILY },
      formatter: (params: any) => {
        const item = params.data
        const pct = item.percent !== undefined ? item.percent.toFixed(1) : params.percent
        return `<div style="font-weight:700;margin-bottom:4px;color:${titleColor}">${escapeHtml(params.name)}</div>
        <div style="display:flex;align-items:center;justify-content:space-between;gap:16px">
          <span style="color:${textColor};font-size:12px">承载用量</span>
          <b style="color:${titleColor};font-variant-numeric:tabular-nums">${params.value} GB</b>
        </div>
        <div style="display:flex;align-items:center;justify-content:space-between;gap:16px;margin-top:2px">
          <span style="color:${textColor};font-size:12px">全网流量占比</span>
          <b style="color:${accentColor};font-variant-numeric:tabular-nums">${pct}%</b>
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
          textStyle: { color: textColor, fontSize: 12, fontFamily: CHART_FONT_FAMILY },
        }
      : {
          orient: 'vertical',
          right: '8%',
          top: 'middle',
          itemWidth: 9,
          itemHeight: 9,
          textStyle: { color: textColor, fontSize: 12, fontFamily: CHART_FONT_FAMILY },
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
  }, true)
}

watch([() => ctx.dashData.value, () => ctx.rankPeriod.value], () => {
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
        if (!chartRef.value) return
        const narrow = chartRef.value.clientWidth < 420
        if (lastNarrow !== null && narrow !== lastNarrow) {
          updateChart()
        }
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
