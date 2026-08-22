<script setup lang="ts">
import { ref, watch, onUnmounted, nextTick, computed, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import * as echarts from 'echarts'
import { getServerMetrics } from '@/api/admin'
import type { ServerMetricsData } from '@/api/types'

import { useThemeStore } from '@/stores/theme'

const theme = useThemeStore()

const props = defineProps<{
  modelValue: boolean
  serverId: number
  serverName: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', val: boolean): void
}>()

const range = ref<'1h' | '6h' | '24h' | '7d'>('1h')
const loading = ref(false)
const metricsData = ref<ServerMetricsData | null>(null)
const isMobile = ref(typeof window !== 'undefined' ? window.innerWidth <= 768 : false)

const netChartRef = ref<HTMLDivElement | null>(null)
const cpuChartRef = ref<HTMLDivElement | null>(null)
const memChartRef = ref<HTMLDivElement | null>(null)
const userChartRef = ref<HTMLDivElement | null>(null)

let netChart: echarts.ECharts | null = null
let cpuChart: echarts.ECharts | null = null
let memChart: echarts.ECharts | null = null
let userChart: echarts.ECharts | null = null
let refreshTimer: any = null

const commonGrid = computed(() => ({
  top: 30,
  right: isMobile.value ? 10 : 20,
  bottom: 25,
  left: isMobile.value ? 42 : 55,
}))

const tooltipConfig = computed(() => ({
  backgroundColor: theme.isDark ? 'rgba(19, 27, 46, 0.95)' : 'rgba(15, 23, 42, 0.9)',
  borderColor: theme.isDark ? '#25334d' : '#334155',
  borderWidth: 1,
  padding: [8, 12],
  textStyle: { color: '#ffffff', fontSize: 12 },
  extraCssText: 'box-shadow: 0 8px 24px rgba(0,0,0,0.25); border-radius: 8px;',
}))

function updateMobileState() {
  isMobile.value = window.innerWidth <= 768
}

function initCharts() {
  if (netChartRef.value && !netChart) {
    netChart = echarts.init(netChartRef.value)
  }
  if (cpuChartRef.value && !cpuChart) {
    cpuChart = echarts.init(cpuChartRef.value)
  }
  if (memChartRef.value && !memChart) {
    memChart = echarts.init(memChartRef.value)
  }
  if (userChartRef.value && !userChart) {
    userChart = echarts.init(userChartRef.value)
  }
}

function resizeCharts() {
  updateMobileState()
  netChart?.resize()
  cpuChart?.resize()
  memChart?.resize()
  userChart?.resize()
}

function updateCharts() {
  if (!metricsData.value) return
  const data = metricsData.value
  const ts = data.timestamps || []
  const isDark = theme.isDark

  const textColor = isDark ? '#94a3b8' : '#64748b'
  const axisLineColor = isDark ? '#25334d' : '#e2e8f0'
  const splitLineColor = isDark ? '#1e293b' : '#f1f5f9'

  // 1. 网络带宽图
  netChart?.setOption({
    tooltip: {
      trigger: 'axis',
      ...tooltipConfig.value,
      formatter: (params: any) => {
        let res = `<div style="font-weight:600;margin-bottom:4px;color:#f8fafc">${params[0]?.axisValue}</div>`
        params.forEach((item: any) => {
          res += `<div style="display:flex;align-items:center;gap:6px;margin-top:2px">${item.marker} <span style="color:#cbd5e1">${item.seriesName}:</span> <b style="color:#fff">${item.value} Mbps</b></div>`
        })
        return res
      },
    },
    legend: { data: ['下行 / 入口 (Rx)', '上行 / 出口 (Tx)'], top: 0, textStyle: { color: textColor, fontSize: 12 } },
    grid: commonGrid.value,
    xAxis: { type: 'category', data: ts, axisLine: { lineStyle: { color: axisLineColor } }, axisLabel: { color: textColor, fontSize: 11, hideOverlap: true } },
    yAxis: { type: 'value', name: 'Mbps', nameTextStyle: { color: textColor }, splitLine: { lineStyle: { color: splitLineColor } }, axisLabel: { color: textColor } },
    series: [
      {
        name: '下行 / 入口 (Rx)',
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: data.rx_mbps,
        itemStyle: { color: '#0284c7' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: isDark ? 'rgba(2, 132, 199, 0.35)' : 'rgba(2, 132, 199, 0.25)' },
            { offset: 1, color: 'rgba(2, 132, 199, 0.0)' },
          ]),
        },
      },
      {
        name: '上行 / 出口 (Tx)',
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: data.tx_mbps,
        itemStyle: { color: '#6366f1' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: isDark ? 'rgba(99, 102, 241, 0.35)' : 'rgba(99, 102, 241, 0.25)' },
            { offset: 1, color: 'rgba(99, 102, 241, 0.0)' },
          ]),
        },
      },
    ],
  })

  // 2. CPU 使用率
  cpuChart?.setOption({
    tooltip: { trigger: 'axis', ...tooltipConfig.value, valueFormatter: (val: any) => `${val} %` },
    grid: commonGrid.value,
    xAxis: { type: 'category', data: ts, axisLine: { lineStyle: { color: axisLineColor } }, axisLabel: { color: textColor, fontSize: 11, hideOverlap: true } },
    yAxis: { type: 'value', min: 0, max: 100, splitLine: { lineStyle: { color: splitLineColor } }, axisLabel: { color: textColor, formatter: '{value}%' } },
    series: [
      {
        name: 'CPU 使用率',
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: data.cpu,
        itemStyle: { color: '#10b981' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: isDark ? 'rgba(16, 185, 129, 0.35)' : 'rgba(16, 185, 129, 0.25)' },
            { offset: 1, color: 'rgba(16, 185, 129, 0.0)' },
          ]),
        },
        markLine: {
          silent: true,
          lineStyle: { color: '#ef4444', type: 'dashed' },
          data: [{ yAxis: 80, label: { formatter: '80% 警戒', color: '#ef4444' } }],
        },
      },
    ],
  })

  // 3. 内存 & 磁盘负载
  memChart?.setOption({
    tooltip: { trigger: 'axis', ...tooltipConfig.value, valueFormatter: (val: any) => `${val} %` },
    legend: { data: ['内存占用率', '磁盘占用率'], top: 0, textStyle: { color: textColor, fontSize: 12 } },
    grid: commonGrid.value,
    xAxis: { type: 'category', data: ts, axisLine: { lineStyle: { color: axisLineColor } }, axisLabel: { color: textColor, fontSize: 11, hideOverlap: true } },
    yAxis: { type: 'value', min: 0, max: 100, splitLine: { lineStyle: { color: splitLineColor } }, axisLabel: { color: textColor, formatter: '{value}%' } },
    series: [
      {
        name: '内存占用率',
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: data.mem_percent,
        itemStyle: { color: '#3b82f6' },
      },
      {
        name: '磁盘占用率',
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: data.disk_percent,
        itemStyle: { color: '#8b5cf6' },
      },
    ],
  })

  // 4. 在线用户数
  userChart?.setOption({
    tooltip: { trigger: 'axis', ...tooltipConfig.value, valueFormatter: (val: any) => `${val} 人` },
    grid: commonGrid.value,
    xAxis: { type: 'category', data: ts, axisLine: { lineStyle: { color: axisLineColor } }, axisLabel: { color: textColor, fontSize: 11, hideOverlap: true } },
    yAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { color: splitLineColor } }, axisLabel: { color: textColor } },
    series: [
      {
        name: '在线设备数',
        type: 'line',
        step: 'start',
        showSymbol: false,
        data: data.online_users,
        itemStyle: { color: '#6366f1' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: isDark ? 'rgba(99, 102, 241, 0.3)' : 'rgba(99, 102, 241, 0.2)' },
            { offset: 1, color: 'rgba(99, 102, 241, 0.0)' },
          ]),
        },
      },
    ],
  })
}

// 监听主题切换，动态重绘
watch(
  () => theme.isDark,
  async () => {
    await nextTick()
    updateCharts()
  }
)

async function loadData() {
  if (!props.serverId) return
  loading.value = true
  try {
    const { data } = await getServerMetrics(props.serverId, range.value)
    if (data.code === 0) {
      metricsData.value = data.data
      await nextTick()
      initCharts()
      updateCharts()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e: any) {
    ElMessage.error(e?.message || '加载监控数据失败')
  } finally {
    loading.value = false
  }
}

watch(
  () => props.modelValue,
  async (open) => {
    if (open) {
      updateMobileState()
      await nextTick()
      initCharts()
      window.addEventListener('resize', resizeCharts)
      loadData()
      refreshTimer = setInterval(loadData, 15000)
    } else {
      if (refreshTimer) clearInterval(refreshTimer)
      window.removeEventListener('resize', resizeCharts)
    }
  },
)

watch(range, () => {
  loadData()
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
  window.removeEventListener('resize', resizeCharts)
  netChart?.dispose()
  cpuChart?.dispose()
  memChart?.dispose()
  userChart?.dispose()
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
          <el-radio-group v-model="range" size="small">
            <el-radio-button value="1h">1 小时 (1m)</el-radio-button>
            <el-radio-button value="6h">6 小时 (3m)</el-radio-button>
            <el-radio-button value="24h">24 小时 (10m)</el-radio-button>
            <el-radio-button value="7d">7 天 (1h)</el-radio-button>
          </el-radio-group>
        </div>
        <div class="range-actions">
          <span class="auto-tip">15s 自动刷新</span>
          <el-button size="small" :icon="Refresh" circle @click="loadData" />
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
          <div ref="netChartRef" class="echart-box"></div>
        </div>

        <!-- 2. CPU 负载 -->
        <div class="chart-card">
          <div class="chart-header">
            <div class="title">CPU 使用率 (CPU Utilization)</div>
            <div class="sub">系统全核综合利用百分比 (%)</div>
          </div>
          <div ref="cpuChartRef" class="echart-box"></div>
        </div>

        <!-- 3. 内存与磁盘 -->
        <div class="chart-card">
          <div class="chart-header">
            <div class="title">内存与磁盘空间占用 (Memory & Storage)</div>
            <div class="sub">系统物理内存与根分区挂载负载 (%)</div>
          </div>
          <div ref="memChartRef" class="echart-box"></div>
        </div>

        <!-- 4. 在线连接数 -->
        <div class="chart-card">
          <div class="chart-header">
            <div class="title">当前在线连接数 (Active Clients)</div>
            <div class="sub">节点实时心跳活跃连接设备统计</div>
          </div>
          <div ref="userChartRef" class="echart-box"></div>
        </div>
      </div>
    </div>
  </el-dialog>
</template>

<style scoped lang="scss">
:deep(.el-dialog__body) {
  max-height: 76vh;
  overflow-y: auto;
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
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  background: var(--x-bg, #f8fafc);
  border: 1px solid var(--x-border, #e6e8f0);
  border-radius: var(--x-radius, 10px);
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

.echart-box {
  width: 100%;
  height: 210px;
}

@media (max-width: 768px) {
  :deep(.el-dialog__body) {
    max-height: calc(85vh - 70px) !important;
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

      .el-radio-group {
        display: flex;
        width: 100%;

        .el-radio-button {
          flex: 1;

          :deep(.el-radio-button__inner) {
            width: 100%;
            padding: 6px 2px;
            font-size: 11px;
          }
        }
      }
    }

    .range-actions {
      justify-content: space-between;
    }
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

  .echart-box {
    height: 185px;
  }
}
</style>
