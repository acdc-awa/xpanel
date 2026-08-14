<script setup lang="ts">
import { ref, watch, onUnmounted, nextTick } from 'vue'
import { ElMessage } from 'element-plus'
import { Refresh } from '@element-plus/icons-vue'
import * as echarts from 'echarts'
import { getServerMetrics } from '@/api/admin'
import type { ServerMetricsData } from '@/api/types'

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

const netChartRef = ref<HTMLDivElement | null>(null)
const cpuChartRef = ref<HTMLDivElement | null>(null)
const memChartRef = ref<HTMLDivElement | null>(null)
const userChartRef = ref<HTMLDivElement | null>(null)

let netChart: echarts.ECharts | null = null
let cpuChart: echarts.ECharts | null = null
let memChart: echarts.ECharts | null = null
let userChart: echarts.ECharts | null = null
let refreshTimer: any = null

const commonGrid = {
  top: 30,
  right: 20,
  bottom: 25,
  left: 55,
}

const tooltipConfig = {
  backgroundColor: 'rgba(15, 23, 42, 0.9)',
  borderColor: '#334155',
  borderWidth: 1,
  padding: [8, 12],
  textStyle: { color: '#ffffff', fontSize: 12 },
  extraCssText: 'box-shadow: 0 8px 24px rgba(0,0,0,0.15); border-radius: 8px;',
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
  netChart?.resize()
  cpuChart?.resize()
  memChart?.resize()
  userChart?.resize()
}

function updateCharts() {
  if (!metricsData.value) return
  const data = metricsData.value
  const ts = data.timestamps || []

  // 1. 网络带宽图
  netChart?.setOption({
    tooltip: {
      trigger: 'axis',
      ...tooltipConfig,
      formatter: (params: any) => {
        let res = `<div style="font-weight:600;margin-bottom:4px;color:#f8fafc">${params[0]?.axisValue}</div>`
        params.forEach((item: any) => {
          res += `<div style="display:flex;align-items:center;gap:6px;margin-top:2px">${item.marker} <span style="color:#cbd5e1">${item.seriesName}:</span> <b style="color:#fff">${item.value} Mbps</b></div>`
        })
        return res
      },
    },
    legend: { data: ['下行 / 入口 (Rx)', '上行 / 出口 (Tx)'], top: 0, textStyle: { color: '#64748b', fontSize: 12 } },
    grid: commonGrid,
    xAxis: { type: 'category', data: ts, axisLine: { lineStyle: { color: '#e2e8f0' } }, axisLabel: { color: '#64748b', fontSize: 11 } },
    yAxis: { type: 'value', name: 'Mbps', nameTextStyle: { color: '#64748b' }, splitLine: { lineStyle: { color: '#f1f5f9' } }, axisLabel: { color: '#64748b' } },
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
            { offset: 0, color: 'rgba(2, 132, 199, 0.25)' },
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
            { offset: 0, color: 'rgba(99, 102, 241, 0.25)' },
            { offset: 1, color: 'rgba(99, 102, 241, 0.0)' },
          ]),
        },
      },
    ],
  })

  // 2. CPU 使用率
  cpuChart?.setOption({
    tooltip: { trigger: 'axis', ...tooltipConfig, valueFormatter: (val: any) => `${val} %` },
    grid: commonGrid,
    xAxis: { type: 'category', data: ts, axisLine: { lineStyle: { color: '#e2e8f0' } }, axisLabel: { color: '#64748b', fontSize: 11 } },
    yAxis: { type: 'value', min: 0, max: 100, splitLine: { lineStyle: { color: '#f1f5f9' } }, axisLabel: { color: '#64748b', formatter: '{value}%' } },
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
            { offset: 0, color: 'rgba(16, 185, 129, 0.25)' },
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
    tooltip: { trigger: 'axis', ...tooltipConfig, valueFormatter: (val: any) => `${val} %` },
    legend: { data: ['内存占用率', '磁盘占用率'], top: 0, textStyle: { color: '#64748b', fontSize: 12 } },
    grid: commonGrid,
    xAxis: { type: 'category', data: ts, axisLine: { lineStyle: { color: '#e2e8f0' } }, axisLabel: { color: '#64748b', fontSize: 11 } },
    yAxis: { type: 'value', min: 0, max: 100, splitLine: { lineStyle: { color: '#f1f5f9' } }, axisLabel: { color: '#64748b', formatter: '{value}%' } },
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
    tooltip: { trigger: 'axis', ...tooltipConfig, valueFormatter: (val: any) => `${val} 人` },
    grid: commonGrid,
    xAxis: { type: 'category', data: ts, axisLine: { lineStyle: { color: '#e2e8f0' } }, axisLabel: { color: '#64748b', fontSize: 11 } },
    yAxis: { type: 'value', minInterval: 1, splitLine: { lineStyle: { color: '#f1f5f9' } }, axisLabel: { color: '#64748b' } },
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
            { offset: 0, color: 'rgba(99, 102, 241, 0.2)' },
            { offset: 1, color: 'rgba(99, 102, 241, 0.0)' },
          ]),
        },
      },
    ],
  })
}

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
  <el-drawer
    :model-value="modelValue"
    :title="`时序性能监控 · ${serverName}`"
    size="760px"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="metrics-drawer-body" v-loading="loading">
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
  </el-drawer>
</template>

<style scoped>
.metrics-drawer-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
  min-height: 100%;
}

.range-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 10px 14px;
  background: var(--x-card, #ffffff);
  border: 1px solid var(--x-border, #e6e8f0);
  border-radius: var(--x-radius, 12px);
  box-shadow: var(--x-shadow, 0 1px 3px rgba(23, 27, 46, 0.06));
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
  gap: 16px;
}

.chart-card {
  background: var(--x-card, #ffffff);
  border: 1px solid var(--x-border, #e6e8f0);
  border-radius: var(--x-radius, 12px);
  box-shadow: var(--x-shadow, 0 1px 3px rgba(23, 27, 46, 0.06));
  padding: 16px 18px;
}

.chart-header {
  margin-bottom: 8px;
}

.chart-header .title {
  font-size: 14.5px;
  font-weight: 600;
  color: var(--x-text, #1e2333);
}

.chart-header .sub {
  font-size: 12px;
  color: var(--x-text-2, #6b7280);
  margin-top: 2px;
}

.echart-box {
  width: 100%;
  height: 220px;
}
</style>
