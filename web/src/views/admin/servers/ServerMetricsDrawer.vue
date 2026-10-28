<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import { ElMessage } from '@/components/cloudscape'
import { getServerMetrics } from '@/api/admin'
import { errMsg } from '@/api/http'
import { formatAxisTime } from '@/utils/timezone'
import type { ServerMetricsData } from '@/api/types'
import { CsButton, CsContainer, CsHeader, CsLineChart, CsModal, CsSegmentedControl } from '@/components/cloudscape'
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
/**
 * 图表占位开关：只在「当前没有可展示曲线」时打开（首开、切档、手动刷新）。
 * 15s 轮询走静默路径，不动它 —— 否则每次轮询曲线都会被 spinner 顶掉再画回来，
 * 看起来就像整张图被重新渲染了一遍。
 */
const loading = ref(false)
/** 仍在飞行中的「非静默」请求数：并发时由最后一个结束的人负责关掉占位。 */
let visiblePending = 0
/** 请求序号：只用来丢弃过期响应（切档后回来的旧档数据），不参与占位开关。 */
let requestSeq = 0
const metricsData = ref<ServerMetricsData | null>(null)

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

/**
 * 拉取监控数据。
 * @param background 后台轮询：保留当前曲线原地更新，不显示加载态、失败不弹提示（下一轮会重试）。
 */
async function loadData(background = false) {
  if (!props.serverId) return
  // 手动刷新/切档期间跳过这一轮轮询：既省一次请求，也避免两个请求互相判过期。
  if (background && visiblePending > 0) return
  const seq = ++requestSeq
  if (!background) {
    visiblePending += 1
    loading.value = true
  }
  try {
    const { data } = await getServerMetrics(props.serverId, range.value)
    if (seq !== requestSeq) return
    if (data.code === 0) {
      metricsData.value = data.data
    } else if (!background) {
      ElMessage.error(data.message)
    }
  } catch (e: any) {
    if (seq === requestSeq && !background) ElMessage.error(errMsg(e, '加载监控数据失败'))
  } finally {
    if (!background) {
      visiblePending = Math.max(0, visiblePending - 1)
      loading.value = visiblePending > 0
    }
  }
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      loadData()
      refreshTimer = setInterval(() => loadData(true), 15000)
    } else {
      if (refreshTimer) clearInterval(refreshTimer)
      refreshTimer = null
    }
  },
)

watch(range, () => {
  loadData()
})

// 换了一台服务器就别再显示上一台残留的曲线（弹窗关闭期间 serverId 也会变）。
watch(
  () => props.serverId,
  () => {
    metricsData.value = null
    if (props.modelValue) loadData()
  },
)

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
})
</script>

<template>
  <CsModal
    :visible="modelValue"
    :header="`性能监控 · ${serverName}`"
    size="large"
    scrollable
    @update:visible="emit('update:modelValue', $event)"
  >
    <div class="metrics-modal-body">
      <!-- 顶栏时间范围切换：官方口径是工具栏行，四条曲线共用，不套卡片 -->
      <div class="range-toolbar">
        <div class="range-tabs">
          <CsSegmentedControl
            :model-value="range"
            :options="rangeOptions"
            label="监控时间范围"
            @change="onRangeChange"
          />
        </div>
        <!-- 传 event 会被当成 background 参数，必须显式调用 -->
        <CsButton :loading="loading" @click="loadData()">刷新</CsButton>
      </div>

      <div class="charts-grid">
        <!-- 1. 网络 I/O 带宽 -->
        <CsContainer>
          <template #header>
            <CsHeader variant="h2" description="物理网卡动态聚合速率 (Mbps)">
              网络吞吐速率 (Network Bandwidth)
            </CsHeader>
          </template>
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
        </CsContainer>

        <!-- 2. CPU 负载 -->
        <CsContainer>
          <template #header>
            <CsHeader variant="h2" description="系统全核综合利用百分比 (%)">
              CPU 使用率 (CPU Utilization)
            </CsHeader>
          </template>
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
        </CsContainer>

        <!-- 3. 内存与磁盘 -->
        <CsContainer>
          <template #header>
            <CsHeader variant="h2" description="系统物理内存与根分区挂载负载 (%)">
              内存与磁盘空间占用 (Memory & Storage)
            </CsHeader>
          </template>
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
        </CsContainer>

        <!-- 4. 在线连接数 -->
        <CsContainer>
          <template #header>
            <CsHeader variant="h2" description="服务器实时心跳活跃连接设备统计">
              当前在线连接数 (Active Clients)
            </CsHeader>
          </template>
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
        </CsContainer>
      </div>
    </div>
  </CsModal>
</template>

<style scoped lang="scss">
.metrics-modal-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.range-toolbar {
  display: flex;
  flex-wrap: nowrap;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}

/* 分段控件在剩余空间里横向滚动，刷新按钮保持完整宽度不被挤压。 */
.range-tabs {
  flex: 1 1 auto;
  min-width: 0;
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: thin;

  /* 官方 segment 继承 reset 的 white-space: normal，被 flex 压缩时会把
     「1 小时」折成两行；分段标签本身应始终单行。 */
  :deep(button) {
    white-space: nowrap;
  }
}

.charts-grid {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* 图表自身按 `height` 绘制 SVG，这里只负责宽度；固定高度会把图例挤出卡片。 */
.chart-box {
  width: 100%;
}

@media (max-width: 768px) {
  .metrics-modal-body {
    gap: 12px;
  }

  .charts-grid {
    gap: 8px;
  }

  .range-toolbar {
    gap: 8px;
  }

  .chart-box {
    width: 100%;
  }
}
</style>
