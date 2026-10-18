<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { ElMessage } from 'element-plus'
import { getServerMetrics } from '@/api/admin'
import { errMsg } from '@/api/http'
import { formatAxisTime } from '@/utils/timezone'
import type { ServerMetricsData } from '@/api/types'
import { CsButton, CsLineChart, CsSegmentedControl } from '@/components/cloudscape'
import type { LineSeries, SegmentedOption } from '@/components/cloudscape'

const props = defineProps<{
  modelValue: boolean
  serverId: number
  serverName: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
}>()

const range = ref<'1h' | '6h' | '24h' | '7d' | '30d'>('1h')
const loading = ref(false)
const metricsData = ref<ServerMetricsData | null>(null)
const isMobile = ref(typeof window !== 'undefined' ? window.innerWidth <= 768 : false)

/** 各档时间跨度（小时），用于坐标轴按跨度切换日期/时刻格式。 */
const RANGE_SPAN_HOURS: Record<string, number> = { '1h': 1, '6h': 6, '24h': 24, '7d': 168, '30d': 720 }

let refreshTimer: ReturnType<typeof setInterval> | null = null

/**
 * 坐标轴标签。优先用后端返回的原始 UTC 时间戳按「显示时区」渲染；
 * 旧后端无该字段时回退服务端格式化串。
 */
const axisLabels = computed<string[]>(() => {
  const data = metricsData.value
  if (!data) return []
  const spanHours = RANGE_SPAN_HOURS[range.value] ?? 1
  const raw = data.timestamps_iso || []
  return raw.length ? raw.map((t) => formatAxisTime(t, spanHours)) : data.timestamps || []
})

/**
 * 峰值曲线用虚线（官方 threshold 样式）：峰值不在两点间插值，
 * 画成平滑曲线会低于真实峰值，把包络线变成假的。
 */
function peakSeries(
  title: string,
  color: string,
  values: number[] | undefined,
  format: (value: number) => string
): LineSeries {
  return {
    type: 'line',
    title,
    color,
    dashed: true,
    data: axisLabels.value.map((label, index) => ({ x: label, y: Number(values?.[index] ?? 0) })),
    valueFormatter: format,
  }
}

function lineSeries(
  title: string,
  color: string,
  values: number[] | undefined,
  format: (value: number) => string
): LineSeries {
  return {
    type: 'line',
    title,
    color,
    data: axisLabels.value.map((label, index) => ({ x: label, y: Number(values?.[index] ?? 0) })),
    valueFormatter: format,
  }
}

const PALETTE = {
  blue: 'var(--color-charts-palette-categorical-1-r41m2k, #688ae8)',
  pink: 'var(--color-charts-palette-categorical-2-37jc27, #c33d69)',
  green: 'var(--color-charts-palette-categorical-3-2qk36g, #2ea597)',
  purple: 'var(--color-charts-palette-categorical-4-greu4f, #8456ce)',
  orange: 'var(--color-charts-palette-categorical-5-5cbtgt, #e07941)',
}

const mbps = (value: number) => `${value.toFixed(2)} Mbps`
const percent = (value: number) => `${value.toFixed(1)}%`
const people = (value: number) => `${value.toFixed(0)} 人`

const netSeries = computed<LineSeries[]>(() => {
  const data = metricsData.value
  return [
    lineSeries('下行均值 (Rx)', PALETTE.blue, data?.rx_mbps, mbps),
    peakSeries('下行峰值 (Rx)', PALETTE.blue, data?.rx_mbps_max, mbps),
    lineSeries('上行均值 (Tx)', PALETTE.purple, data?.tx_mbps, mbps),
    peakSeries('上行峰值 (Tx)', PALETTE.purple, data?.tx_mbps_max, mbps),
  ]
})

const cpuSeries = computed<LineSeries[]>(() => {
  const data = metricsData.value
  return [
    lineSeries('CPU 均值', PALETTE.green, data?.cpu, percent),
    peakSeries('CPU 峰值', PALETTE.green, data?.cpu_max, percent),
  ]
})

const memSeries = computed<LineSeries[]>(() => {
  const data = metricsData.value
  return [
    lineSeries('内存均值', PALETTE.pink, data?.mem_percent, percent),
    peakSeries('内存峰值', PALETTE.pink, data?.mem_percent_max, percent),
    lineSeries('磁盘均值', PALETTE.orange, data?.disk_percent, percent),
  ]
})

const userSeries = computed<LineSeries[]>(() => {
  const data = metricsData.value
  return [
    lineSeries('在线均值', PALETTE.blue, data?.online_users, people),
    peakSeries('在线峰值', PALETTE.blue, data?.online_users_max, people),
  ]
})

const formatPercentTick = (value: number) => `${value}%`
const formatCountTick = (value: number) => String(value)

const rangeOptions: SegmentedOption[] = [
  { id: '1h', text: '1 小时' },
  { id: '6h', text: '6 小时' },
  { id: '24h', text: '24 小时' },
  { id: '7d', text: '7 天' },
  { id: '30d', text: '30 天' },
]

/** CsSegmentedControl 的 v-model 类型是 string | number，这里收窄回本页的档位联合类型。 */
function onRangeChange(detail: { selectedId: string | number }) {
  range.value = detail.selectedId as typeof range.value
}

function updateMobileState() {
  isMobile.value = window.innerWidth <= 768
}

async function loadData() {
  if (!props.serverId) return
  loading.value = true
  try {
    const { data } = await getServerMetrics(props.serverId, range.value)
    if (data.code === 0) {
      metricsData.value = data.data
    } else {
      ElMessage.error(data.message)
    }
  } catch (e: any) {
    ElMessage.error(errMsg(e, '加载监控数据失败'))
  } finally {
    loading.value = false
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      updateMobileState()
      loadData()
      refreshTimer = setInterval(loadData, 15000)
    } else {
      if (refreshTimer) clearInterval(refreshTimer)
      refreshTimer = null
    }
  },
)

watch(range, () => {
  loadData()
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<template>
  <el-dialog
    :model-value="modelValue"
    :title="`时序性能监控 · ${serverName}`"
    :width="isMobile ? '94vw' : '820px'"
    top="5vh"
    align-center
    destroy-on-close
    class="server-metrics-dialog"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="metrics-modal-body" v-loading="loading">
      <!-- 顶栏时间范围切换 -->
      <div class="range-toolbar">
        <div class="range-tabs">
          <CsSegmentedControl
            :model-value="range"
            :options="rangeOptions"
            label="监控时间范围"
            @change="onRangeChange"
          />
        </div>
        <div class="range-actions">
          <span class="auto-tip">实线均值 · 虚线峰值</span>
          <span class="auto-tip">15s 自动刷新</span>
          <CsButton :loading="loading" @click="loadData">刷新</CsButton>
        </div>
      </div>

      <!-- 四张指标卡片 -->
      <div class="charts-grid">
        <!-- 1. 网络 I/O 带宽 -->
        <div class="chart-card">
          <div class="chart-header">
            <div class="title">网络吞吐速率 (Network Bandwidth)</div>
            <div class="sub">物理网卡动态聚合速率 (Mbps)</div>
          </div>
          <CsLineChart
            class="chart-box"
            :series="netSeries"
            :height="210"
            :loading="loading"
            y-title="Mbps"
            :y-tick-formatter="formatCountTick"
            aria-label="网络吞吐速率 (Network Bandwidth)"
            empty-text="暂无监控数据"
          />
        </div>

        <!-- 2. CPU 负载 -->
        <div class="chart-card">
          <div class="chart-header">
            <div class="title">CPU 使用率 (CPU Utilization)</div>
            <div class="sub">系统全核综合利用百分比 (%)</div>
          </div>
          <CsLineChart
            class="chart-box"
            :series="cpuSeries"
            :height="210"
            :loading="loading"
            y-title="%"
            :y-tick-formatter="formatPercentTick"
            aria-label="CPU 使用率 (CPU Utilization)"
            empty-text="暂无监控数据"
          />
        </div>

        <!-- 3. 内存与磁盘 -->
        <div class="chart-card">
          <div class="chart-header">
            <div class="title">内存与磁盘空间占用 (Memory & Storage)</div>
            <div class="sub">系统物理内存与根分区挂载负载 (%)</div>
          </div>
          <CsLineChart
            class="chart-box"
            :series="memSeries"
            :height="210"
            :loading="loading"
            y-title="%"
            :y-tick-formatter="formatPercentTick"
            aria-label="内存与磁盘空间占用 (Memory & Storage)"
            empty-text="暂无监控数据"
          />
        </div>

        <!-- 4. 在线连接数 -->
        <div class="chart-card">
          <div class="chart-header">
            <div class="title">当前在线连接数 (Active Clients)</div>
            <div class="sub">服务器实时心跳活跃连接设备统计</div>
          </div>
          <CsLineChart
            class="chart-box"
            :series="userSeries"
            :height="210"
            :loading="loading"
            y-title="人"
            :y-tick-formatter="formatCountTick"
            aria-label="当前在线连接数 (Active Clients)"
            empty-text="暂无监控数据"
          />
        </div>
      </div>
    </div>
  </el-dialog>
</template>

<style scoped lang="scss">
:deep(.el-dialog__body) {
  // 高度上限统一由全局 .el-dialog 的 max-height:85vh + body 弹性滚动控制。
  // 这里以前写 76vh、移动端又写 calc(85vh - 70px)，同一弹窗不同入口高度不同。
  overflow-x: hidden;
  padding: 14px 18px 20px;
}

.metrics-modal-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.range-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  padding: 10px 14px;
  background: var(--x-bg, #f8fafc);
  border: 1px solid var(--x-border, #e6e8f0);
  border-radius: var(--x-radius, 10px);
}

.range-tabs {
  flex: 0 0 auto;

  /* 官方 segment 继承 reset 的 white-space: normal，被 flex 压缩时会把
     「1 小时 (1m)」折成两行；分段标签本身应始终单行。 */
  :deep(button) {
    white-space: nowrap;
  }
}

.range-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.auto-tip {
  font-size: 12px;
  color: var(--x-text-2, #6b7280);
}

.charts-grid {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.chart-card {
  background: var(--x-card, #ffffff);
  border: 1px solid var(--x-border, #e6e8f0);
  border-radius: var(--x-radius, 10px);
  padding: 14px 16px;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.03);
}

.chart-header {
  margin-bottom: 6px;
}

.chart-header .title {
  font-size: 14px;
  font-weight: 600;
  color: var(--x-text, #1e2333);
}

.chart-header .sub {
  font-size: 11.5px;
  color: var(--x-text-2, #6b7280);
  margin-top: 2px;
}

/* 图表自身按 `height` 绘制 SVG，这里只负责宽度；固定高度会把图例挤出卡片。 */
.chart-box {
  width: 100%;
}

@media (max-width: 768px) {
  :deep(.el-dialog__body) {
    padding: 10px 12px 16px !important;
  }

  .metrics-modal-body {
    gap: 12px;
  }

  .range-toolbar {
    flex-direction: column;
    align-items: stretch;
    gap: 8px;
    padding: 8px 10px;

    .range-tabs {
      width: 100%;
      overflow-x: auto;
    }
  }

  .range-actions {
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 6px;
  }

  .chart-card {
    padding: 10px 8px;

    .chart-header .title {
      font-size: 13px;
    }

    .chart-header .sub {
      font-size: 11px;
    }
  }

  .chart-box {
    width: 100%;
  }
}
</style>
