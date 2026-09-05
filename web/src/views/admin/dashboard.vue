<script setup lang="ts">
import { nextTick, onMounted, onUnmounted, ref, watch } from 'vue'
import {
  TrendCharts,
  Money,
  Coin,
  Histogram,
  Connection,
  User,
  Refresh,
} from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import * as echarts from 'echarts'
import { getDashboard } from '@/api/admin'
import { errMsg } from '@/api/http'
import type { DashboardData, ServerMatrixItem } from '@/api/types'
import { formatBytes } from '@/utils/format'
import { useThemeStore } from '@/stores/theme'
import ServerMetricsDrawer from './servers/ServerMetricsDrawer.vue'
import OnlineUsersPanel from './servers/OnlineUsersPanel.vue'

const theme = useThemeStore()
const loading = ref(false)
const dashData = ref<DashboardData | null>(null)
const activeTab = ref('users')
const lastUpdatedTime = ref('')

// 抽屉监控
const metricsOpen = ref(false)
const selectedServer = ref<ServerMatrixItem | null>(null)

// 在线用户弹窗（复用服务器抽屉的 OnlineUsersPanel）
const onlineDialogOpen = ref(false)
const onlineServer = ref<ServerMatrixItem | null>(null)

function openOnlineUsers(s: ServerMatrixItem) {
  onlineServer.value = s
  onlineDialogOpen.value = true
}

// 吞吐趋势范围档位（3/7/30 天，天粒度）
const trendRange = ref<3 | 7 | 30>(30)

// ECharts
const trendChartRef = ref<HTMLDivElement | null>(null)
const donutChartRef = ref<HTMLDivElement | null>(null)
let trendChart: echarts.ECharts | null = null
let donutChart: echarts.ECharts | null = null
let autoRefreshTimer: any = null

function formatBandwidth(bytesPerSec: number): string {
  if (!bytesPerSec || bytesPerSec <= 0) return '0.00 Mbps'
  const mbps = (bytesPerSec * 8) / 1_000_000
  if (mbps >= 1000) {
    return `${(mbps / 1000).toFixed(2)} Gbps`
  }
  return `${mbps.toFixed(2)} Mbps`
}

function formatCompactRate(bytesPerSec: number): string {
  if (!bytesPerSec || bytesPerSec <= 0) return '0.00 M'
  const mbps = (bytesPerSec * 8) / 1_000_000
  if (mbps >= 1000) {
    return `${(mbps / 1000).toFixed(1)} G`
  }
  return `${mbps.toFixed(2)} M`
}

function fmtTime(t: string | null) {
  if (!t) return '—'
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}

function initCharts() {
  if (trendChartRef.value && !trendChart) {
    trendChart = echarts.init(trendChartRef.value)
  }
  if (donutChartRef.value && !donutChart) {
    donutChart = echarts.init(donutChartRef.value)
  }
}

function resizeCharts() {
  updateCharts()
  trendChart?.resize()
  donutChart?.resize()
}

function updateCharts() {
  if (!dashData.value) return
  const isMob = typeof window !== 'undefined' ? window.innerWidth <= 768 : false
  const isDark = theme.isDark

  const textColor = isDark ? '#94a3b8' : '#64748b'
  const axisLineColor = isDark ? '#25334d' : '#e2e8f0'
  const splitLineColor = isDark ? '#1e293b' : '#f1f5f9'
  const tooltipBg = isDark ? 'rgba(19, 27, 46, 0.95)' : 'rgba(15, 23, 42, 0.9)'
  const tooltipBorder = isDark ? '#25334d' : '#334155'
  const pieBorderColor = isDark ? '#131b2e' : '#ffffff'

  // 1. 流量趋势柱状图 (堆叠柱状图)
  const trend = dashData.value.traffic_trend || []
  const dates = trend.map((t) => t.date.slice(5))
  const upData = trend.map((t) => Number((t.up_bytes / (1024 * 1024 * 1024)).toFixed(2)))
  const downData = trend.map((t) => Number((t.down_bytes / (1024 * 1024 * 1024)).toFixed(2)))

  trendChart?.setOption({
    tooltip: {
      trigger: 'axis',
      confine: true,
      axisPointer: {
        type: 'shadow',
        shadowStyle: {
          color: isDark ? 'rgba(255, 255, 255, 0.04)' : 'rgba(0, 0, 0, 0.03)',
        },
      },
      backgroundColor: tooltipBg,
      borderColor: tooltipBorder,
      borderWidth: 1,
      padding: [8, 12],
      textStyle: { color: '#ffffff', fontSize: 12 },
      extraCssText: 'box-shadow: 0 8px 24px rgba(0,0,0,0.25); border-radius: 8px; z-index: 99;',
      formatter: (params: any) => {
        let res = `<div style="font-weight:600;margin-bottom:4px;color:#f8fafc">${params[0]?.axisValue}</div>`
        let total = 0
        params.forEach((item: any) => {
          const val = Number(item.value || 0)
          total += val
          res += `<div style="display:flex;align-items:center;justify-content:space-between;gap:12px;margin-top:2px">
            <span style="display:flex;align-items:center;gap:6px">${item.marker} <span style="color:#cbd5e1">${item.seriesName}</span></span>
            <b style="color:#fff">${val.toFixed(2)} GB</b>
          </div>`
        })
        res += `<div style="border-top:1px dashed rgba(255,255,255,0.2);margin-top:6px;padding-top:4px;display:flex;justify-content:space-between;color:#e2e8f0;font-size:11.5px"><span>合计吞吐:</span> <b style="color:#38bdf8">${total.toFixed(2)} GB</b></div>`
        return res
      },
    },
    legend: {
      data: isMob ? ['上行', '下行'] : ['上行流量 (Upload)', '下行流量 (Download)'],
      top: 0,
      right: isMob ? 'center' : 10,
      textStyle: { color: textColor, fontSize: isMob ? 11 : 12 },
    },
    grid: {
      top: 35,
      right: isMob ? 12 : 20,
      bottom: 25,
      left: isMob ? 42 : 55,
    },
    xAxis: {
      type: 'category',
      data: dates,
      axisLine: { lineStyle: { color: axisLineColor } },
      axisLabel: { color: textColor, fontSize: 11, hideOverlap: true },
    },
    yAxis: {
      type: 'value',
      name: 'GB',
      nameTextStyle: { color: textColor },
      splitLine: { lineStyle: { color: splitLineColor } },
      axisLabel: { color: textColor },
    },
    series: [
      {
        name: isMob ? '上行' : '上行流量 (Upload)',
        type: 'bar',
        stack: 'traffic',
        barMaxWidth: 16,
        data: upData,
        itemStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: '#818cf8' },
            { offset: 1, color: '#6366f1' },
          ]),
        },
      },
      {
        name: isMob ? '下行' : '下行流量 (Download)',
        type: 'bar',
        stack: 'traffic',
        barMaxWidth: 16,
        data: downData,
        itemStyle: {
          borderRadius: [3, 3, 0, 0],
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: '#34d399' },
            { offset: 1, color: '#059669' },
          ]),
        },
      },
    ],
  })

  // 2. 节点流量占比环形图 (自适应容器宽度，彻底杜绝图例与图表重合)
  const breakdown = dashData.value.server_breakdown || []
  const pieData = breakdown
    .filter((b) => b.total_bytes > 0)
    .map((b) => ({
      name: b.name || `节点#${b.server_id}`,
      value: Number((b.total_bytes / (1024 * 1024 * 1024)).toFixed(2)),
    }))

  const donutWidth = donutChartRef.value?.clientWidth || 360
  const isNarrow = isMob || donutWidth < 410

  donutChart?.setOption({
    color: ['#6366f1', '#3b82f6', '#10b981', '#f59e0b', '#ec4899', '#8b5cf6', '#06b6d4', '#14b8a6'],
    tooltip: {
      trigger: 'item',
      confine: true, // 核心修复：限制在容器内，永不发生遮挡与截断
      backgroundColor: tooltipBg,
      borderColor: tooltipBorder,
      borderWidth: 1,
      padding: [8, 12],
      textStyle: { color: '#ffffff', fontSize: 12 },
      extraCssText: 'box-shadow: 0 8px 24px rgba(0,0,0,0.25); border-radius: 8px; z-index: 99;',
      formatter: (params: any) => {
        return `<div style="font-weight:600;margin-bottom:4px;color:#f8fafc">${params.name}</div>
        <div style="display:flex;align-items:center;gap:6px">
          ${params.marker}
          <span style="color:#cbd5e1">承载流量:</span>
          <b style="color:#38bdf8">${params.value} GB</b>
          <span style="color:#94a3b8">(${params.percent}%)</span>
        </div>`
      },
    },
    legend: isNarrow
      ? {
          orient: 'horizontal',
          bottom: 0,
          left: 'center',
          itemWidth: 10,
          itemHeight: 10,
          textStyle: { color: textColor, fontSize: 11 },
        }
      : {
          orient: 'vertical',
          right: '4%',
          top: 'middle',
          itemWidth: 10,
          itemHeight: 10,
          textStyle: { color: textColor, fontSize: 11.5 },
          formatter: (name: string) => {
            return name.length > 12 ? name.slice(0, 11) + '…' : name
          },
        },
    series: [
      {
        name: '节点流量',
        type: 'pie',
        radius: isNarrow ? ['38%', '60%'] : ['40%', '68%'],
        center: isNarrow ? ['50%', '38%'] : ['28%', '50%'],
        avoidLabelOverlap: false,
        itemStyle: {
          borderRadius: 6,
          borderColor: pieBorderColor,
          borderWidth: 2,
        },
        label: { show: false },
        labelLine: { show: false },
        emphasis: {
          scale: true,
          scaleSize: 6,
          label: { show: false },
          labelLine: { show: false },
        },
        data: pieData.length ? pieData : [{ name: '暂无流量', value: 0 }],
      },
    ],
  })
}

// 监听主题切换，动态重绘 ECharts
watch(
  () => theme.isDark,
  async () => {
    await nextTick()
    updateCharts()
  }
)

async function load() {
  loading.value = true
  try {
    const { data } = await getDashboard(trendRange.value)
    if (data.code === 0) {
      dashData.value = data.data
      lastUpdatedTime.value = new Date().toLocaleTimeString('zh-CN', { hour12: false })
      await nextTick()
      initCharts()
      updateCharts()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载仪表盘失败'))
  } finally {
    loading.value = false
  }
}

// 档位切换即重拉（30s 轮询沿用当前档位）
watch(trendRange, () => load())

function openServerMetrics(s: ServerMatrixItem) {
  selectedServer.value = s
  metricsOpen.value = true
}

onMounted(() => {
  load()
  window.addEventListener('resize', resizeCharts)
  autoRefreshTimer = setInterval(load, 30000)
})

onUnmounted(() => {
  if (autoRefreshTimer) clearInterval(autoRefreshTimer)
  window.removeEventListener('resize', resizeCharts)
  trendChart?.dispose()
  donutChart?.dispose()
})
</script>

<template>
  <div class="x-page dash-page">
    <!-- 顶栏标题与刷新 -->
    <div class="x-toolbar" style="margin-bottom: 20px">
      <div class="x-toolbar-left">
        <div class="dash-title-wrap">
          <div class="dash-main-title">运维监控大盘</div>
          <div class="dash-sub-title">集群状态、吞吐速率、运营营收与用量指标大盘</div>
        </div>
      </div>
      <div style="display: flex; align-items: center; gap: 12px">
        <span class="muted cell-mono" style="font-size: 12px">最后更新: {{ lastUpdatedTime || '—' }} (30s 轮询)</span>
        <el-button :loading="loading" @click="load">
          <el-icon><Refresh /></el-icon>&nbsp;刷新
        </el-button>
      </div>
    </div>

    <!-- 6 张核心指标卡片 -->
    <div class="kpi-grid">
      <!-- 1. 今日营收 (卡密激活) -->
      <div class="kpi-card card-cyan">
        <div class="kpi-icon-box"><el-icon><Coin /></el-icon></div>
        <div class="kpi-content">
          <div class="kpi-label">今日营收</div>
          <div class="kpi-val cell-mono">¥ {{ ((dashData?.summary.today_revenue_cents || 0) / 100).toFixed(2) }}</div>
          <div class="kpi-sub">卡密激活 {{ dashData?.summary.today_used_cards_count || 0 }} 张</div>
        </div>
      </div>

      <!-- 2. 本月累计营收 -->
      <div class="kpi-card card-purple">
        <div class="kpi-icon-box"><el-icon><Money /></el-icon></div>
        <div class="kpi-content">
          <div class="kpi-label">本月营收</div>
          <div class="kpi-val cell-mono">¥ {{ ((dashData?.summary.month_revenue_cents || 0) / 100).toFixed(2) }}</div>
          <div class="kpi-sub">累计营收 ¥ {{ ((dashData?.summary.total_revenue_cents || 0) / 100).toFixed(2) }}</div>
        </div>
      </div>

      <!-- 3. 今日消耗流量 -->
      <div class="kpi-card card-blue">
        <div class="kpi-icon-box"><el-icon><Histogram /></el-icon></div>
        <div class="kpi-content">
          <div class="kpi-label">今日流量</div>
          <div class="kpi-val cell-mono">{{ formatBytes(dashData?.summary.today_traffic_total || 0) }}</div>
          <div class="kpi-sub">
            ↑ {{ formatBytes(dashData?.summary.today_traffic_up || 0) }} · ↓ {{ formatBytes(dashData?.summary.today_traffic_down || 0) }}
          </div>
        </div>
      </div>

      <!-- 4. 实时出口带宽 (优化紧凑速率展示，彻底告别遮挡) -->
      <div class="kpi-card card-emerald">
        <div class="kpi-icon-box"><el-icon><TrendCharts /></el-icon></div>
        <div class="kpi-content">
          <div class="kpi-label">实时吞吐</div>
          <div class="kpi-val cell-mono">{{ formatBandwidth((dashData?.summary.realtime_rx_rate || 0) + (dashData?.summary.realtime_tx_rate || 0)) }}</div>
          <div class="kpi-sub">
            ↓ {{ formatCompactRate(dashData?.summary.realtime_rx_rate || 0) }} · ↑ {{ formatCompactRate(dashData?.summary.realtime_tx_rate || 0) }}
          </div>
        </div>
      </div>

      <!-- 5. 节点在线集群 -->
      <div class="kpi-card card-indigo">
        <div class="kpi-icon-box"><el-icon><Connection /></el-icon></div>
        <div class="kpi-content">
          <div class="kpi-label">节点状态</div>
          <div class="kpi-val cell-mono">
            {{ dashData?.summary.online_servers || 0 }} / {{ dashData?.summary.total_servers || 0 }}
            <span style="font-size: 13px; font-weight: 500; color: var(--x-text-3)">在线</span>
          </div>
          <div class="kpi-sub">
            在线率 {{ dashData?.summary.total_servers ? (((dashData?.summary.online_servers || 0) / dashData.summary.total_servers) * 100).toFixed(0) : 100 }}%
          </div>
        </div>
      </div>

      <!-- 6. 活跃用户 / 订单 -->
      <div class="kpi-card card-amber">
        <div class="kpi-icon-box"><el-icon><User /></el-icon></div>
        <div class="kpi-content">
          <div class="kpi-label">有效用户</div>
          <div class="kpi-val cell-mono">
            {{ dashData?.summary.active_users || 0 }} / {{ dashData?.summary.total_users || 0 }}
          </div>
          <div class="kpi-sub">
            今日 {{ dashData?.summary.today_orders || 0 }} 单 · 累计 {{ dashData?.summary.total_orders || 0 }} 单
          </div>
        </div>
      </div>
    </div>

    <!-- 中部图表：吞吐趋势（3/7/30 天可切换） + 节点流量占比 -->
    <div class="charts-row">
      <div class="chart-panel trend-panel">
        <div class="panel-head">
          <div style="display: flex; align-items: center; justify-content: space-between; gap: 8px; flex-wrap: wrap">
            <div>
              <div class="title">吞吐趋势 (近 {{ trendRange }} 天)</div>
              <div class="sub">每日上行 / 下行流量吞吐分布汇总 (GB)</div>
            </div>
            <el-radio-group v-model="trendRange" size="small">
              <el-radio-button :value="3">3 天</el-radio-button>
              <el-radio-button :value="7">7 天</el-radio-button>
              <el-radio-button :value="30">30 天</el-radio-button>
            </el-radio-group>
          </div>
        </div>
        <div ref="trendChartRef" class="echart-container"></div>
      </div>

      <div class="chart-panel donut-panel">
        <div class="panel-head">
          <div class="title">节点流量分布</div>
          <div class="sub">各节点累计承载流量占比</div>
        </div>
        <div ref="donutChartRef" class="echart-container"></div>
      </div>
    </div>

    <!-- 下部双栏：服务器健康度矩阵 + 右侧榜单与流水 -->
    <div class="bottom-grid">
      <!-- 左侧：服务器健康度与系统负载矩阵 -->
      <div class="matrix-card">
        <div class="panel-head" style="margin-bottom: 14px">
          <div class="title">节点监控矩阵</div>
          <div class="sub">心跳、CPU、内存与实时带宽监控</div>
        </div>

        <!-- 桌面端表格视图 (双行紧凑聚合，告别溢出与横向滚动) -->
        <div class="desktop-table-view">
          <el-table :data="dashData?.server_matrix || []" size="small" style="width: 100%">
            <!-- 1. 节点与地址 -->
            <el-table-column label="节点" min-width="110">
              <template #default="{ row }">
                <div style="font-weight: 600; color: var(--x-text); font-size: 13px">{{ row.name }}</div>
                <div class="muted cell-mono" style="font-size: 11px">{{ row.host }}</div>
              </template>
            </el-table-column>

            <!-- 2. 状态与心跳 -->
            <el-table-column label="状态" width="105">
              <template #default="{ row }">
                <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small">
                  <span class="x-status-dot" :class="row.status === 1 ? 'online' : 'offline'" />
                  {{ row.status === 1 ? '在线' : '离线' }}
                </el-tag>
                <div class="muted cell-mono" style="font-size: 10.5px; margin-top: 2px">
                  {{ row.last_seen_at ? fmtTime(row.last_seen_at).slice(5, 16) : '—' }}
                </div>
              </template>
            </el-table-column>

            <!-- 3. 系统负载 (CPU + 内存复合) -->
            <el-table-column label="负载 (CPU / 内存)" width="145">
              <template #default="{ row }">
                <div style="display: flex; align-items: center; gap: 4px; font-size: 11px" class="cell-mono">
                  <span class="muted" style="font-size: 10px; width: 28px">CPU</span>
                  <el-progress
                    :percentage="Math.min(100, Math.round(row.cpu || 0))"
                    :color="row.cpu > 80 ? '#ef4444' : row.cpu > 50 ? '#f59e0b' : '#10b981'"
                    :stroke-width="4"
                    :show-text="false"
                    style="width: 42px"
                  />
                  <span>{{ (row.cpu || 0).toFixed(0) }}%</span>
                </div>
                <div style="display: flex; align-items: center; gap: 4px; font-size: 11px; margin-top: 2px" class="cell-mono">
                  <span class="muted" style="font-size: 10px; width: 28px">MEM</span>
                  <el-progress
                    :percentage="row.mem_total ? Math.min(100, Math.round((row.mem / row.mem_total) * 100)) : 0"
                    color="#3b82f6"
                    :stroke-width="4"
                    :show-text="false"
                    style="width: 42px"
                  />
                  <span>{{ row.mem_total ? `${Math.round((row.mem / row.mem_total) * 100)}%` : '—' }}</span>
                </div>
              </template>
            </el-table-column>

            <!-- 4. 实时速率与在线设备 -->
            <el-table-column label="速率 / 在线" min-width="125">
              <template #default="{ row }">
                <div class="cell-mono" style="font-size: 11px">
                  <span style="color: #0284c7">↓{{ formatBandwidth(row.rx_rate) }}</span>
                  <span style="color: #6366f1; margin-left: 4px">↑{{ formatBandwidth(row.tx_rate) }}</span>
                </div>
                <div class="muted cell-mono" style="font-size: 10.5px; margin-top: 2px">
                  在线: <b class="online-link" @click="openOnlineUsers(row as ServerMatrixItem)">{{ row.online_users || 0 }}</b> 台
                </div>
              </template>
            </el-table-column>

            <!-- 5. 操作 -->
            <el-table-column label="操作" width="65" align="center">
              <template #default="{ row }">
                <el-button size="small" text type="primary" style="padding: 2px 4px" @click="openServerMetrics(row as ServerMatrixItem)">
                  <el-icon><TrendCharts /></el-icon>&nbsp;图表
                </el-button>
              </template>
            </el-table-column>
          </el-table>
        </div>

        <!-- 移动端卡片流视图 -->
        <div class="mobile-cards-view">
          <div v-if="!dashData?.server_matrix || dashData.server_matrix.length === 0" style="text-align: center; padding: 24px 0; color: var(--x-text-3); font-size: 13px">
            暂无节点监控数据
          </div>
          <div v-else class="mobile-data-card-list">
            <div v-for="row in dashData.server_matrix" :key="row.id" class="mobile-data-card">
              <div class="card-head">
                <div class="head-title">
                  <span class="x-status-dot" :class="row.status === 1 ? 'online' : 'offline'" />
                  <span style="font-weight: 700">{{ row.name }}</span>
                  <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small">
                    {{ row.status === 1 ? '在线' : '离线' }}
                  </el-tag>
                </div>
                <el-button size="small" type="primary" plain @click="openServerMetrics(row as ServerMatrixItem)">
                  <el-icon><TrendCharts /></el-icon>&nbsp;监控
                </el-button>
              </div>

              <div class="card-grid">
                <div class="grid-item full-width">
                  <span class="item-label">节点地址</span>
                  <div class="item-value cell-mono" style="font-size: 12px">{{ row.host }}</div>
                </div>
                <div class="grid-item">
                  <span class="item-label">CPU 负载</span>
                  <div class="item-value cell-mono" style="display: flex; align-items: center; gap: 6px">
                    <el-progress
                      :percentage="Math.min(100, Math.round(row.cpu || 0))"
                      :color="row.cpu > 80 ? '#ef4444' : row.cpu > 50 ? '#f59e0b' : '#10b981'"
                      :stroke-width="5"
                      :show-text="false"
                      style="width: 40px"
                    />
                    <span>{{ (row.cpu || 0).toFixed(0) }}%</span>
                  </div>
                </div>
                <div class="grid-item">
                  <span class="item-label">内存负载</span>
                  <div class="item-value cell-mono" style="display: flex; align-items: center; gap: 6px">
                    <el-progress
                      :percentage="row.mem_total ? Math.min(100, Math.round((row.mem / row.mem_total) * 100)) : 0"
                      color="#3b82f6"
                      :stroke-width="5"
                      :show-text="false"
                      style="width: 40px"
                    />
                    <span>{{ row.mem_total ? `${Math.round((row.mem / row.mem_total) * 100)}%` : '—' }}</span>
                  </div>
                </div>
                <div class="grid-item">
                  <span class="item-label">实时吞吐 (下行/上行)</span>
                  <div class="item-value cell-mono" style="font-size: 11.5px">
                    <span style="color: #0284c7">↓{{ formatBandwidth(row.rx_rate) }}</span>
                    <span style="color: #6366f1; margin-left: 4px">↑{{ formatBandwidth(row.tx_rate) }}</span>
                  </div>
                </div>
                <div class="grid-item">
                  <span class="item-label">在线设备数</span>
                  <div class="item-value cell-mono online-link" style="font-weight: 600" @click="openOnlineUsers(row as ServerMatrixItem)">
                    {{ row.online_users || 0 }} 台
                  </div>
                </div>
                <div class="grid-item full-width">
                  <span class="item-label">最后心跳</span>
                  <div class="item-value cell-mono muted" style="font-size: 11.5px">{{ fmtTime(row.last_seen_at) }}</div>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      <!-- 右侧：榜单与流水切换 Tabs -->
      <div class="ledger-card">
        <el-tabs v-model="activeTab" class="ledger-tabs">
          <!-- 1. 用户流量排行 -->
          <el-tab-pane label="流量排行" name="users">
            <!-- 桌面端表格 -->
            <div class="desktop-table-view">
              <el-table :data="dashData?.user_rank || []" size="small">
                <el-table-column label="#" width="38">
                  <template #default="{ $index }">
                    <span class="cell-mono" :style="{ fontWeight: $index < 3 ? '700' : '400', color: $index === 0 ? '#f59e0b' : $index === 1 ? '#64748b' : $index === 2 ? '#b45309' : 'inherit' }">
                      {{ $index + 1 }}
                    </span>
                  </template>
                </el-table-column>
                <el-table-column label="用户" min-width="100">
                  <template #default="{ row }">
                    <span style="font-weight: 600; color: var(--x-text)">{{ row.username }}</span>
                    <div class="muted cell-mono" style="font-size: 11px">{{ row.email || '—' }}</div>
                  </template>
                </el-table-column>
                <el-table-column prop="plan_name" label="套餐" width="80">
                  <template #default="{ row }">
                    <el-tag size="small" type="info" effect="plain">{{ row.plan_name }}</el-tag>
                  </template>
                </el-table-column>
                <el-table-column label="已用总流量" width="95">
                  <template #default="{ row }">
                    <span class="cell-mono" style="font-weight: 700; color: var(--x-primary)">
                      {{ formatBytes(row.total_bytes) }}
                    </span>
                  </template>
                </el-table-column>
              </el-table>
            </div>

            <!-- 移动端流式卡片 -->
            <div class="mobile-cards-view">
              <div v-if="!dashData?.user_rank || dashData.user_rank.length === 0" style="text-align: center; padding: 20px 0; color: var(--x-text-3); font-size: 13px">
                暂无流量排行数据
              </div>
              <div v-else class="mobile-data-card-list">
                <div v-for="(row, idx) in dashData.user_rank" :key="row.user_id" class="mobile-data-card" style="padding: 10px 12px">
                  <div class="card-head" style="padding-bottom: 6px">
                    <div class="head-title">
                      <span class="cell-mono" :style="{ fontWeight: idx < 3 ? '700' : '400', color: idx === 0 ? '#f59e0b' : idx === 1 ? '#64748b' : idx === 2 ? '#b45309' : 'inherit', fontSize: '12px' }">
                        #{{ idx + 1 }}
                      </span>
                      <span style="font-weight: 700">{{ row.username }}</span>
                      <el-tag size="small" type="info" effect="plain">{{ row.plan_name }}</el-tag>
                    </div>
                    <span class="cell-mono" style="font-weight: 700; color: var(--x-primary); font-size: 13px">
                      {{ formatBytes(row.total_bytes) }}
                    </span>
                  </div>
                  <div class="muted cell-mono" style="font-size: 11px; margin-top: 4px">{{ row.email || '—' }}</div>
                </div>
              </div>
            </div>
          </el-tab-pane>

          <!-- 2. 最近卡密激活流水 -->
          <el-tab-pane label="卡密流水" name="cards">
            <!-- 桌面端表格 -->
            <div class="desktop-table-view">
              <el-table :data="dashData?.recent_gift_cards || []" size="small">
                <el-table-column label="卡密" min-width="110">
                  <template #default="{ row }">
                    <code class="cell-mono" style="font-size: 11px">{{ row.code_masked }}</code>
                  </template>
                </el-table-column>
                <el-table-column label="面值" width="75">
                  <template #default="{ row }">
                    <span class="cell-mono" style="font-weight: 700; color: #059669">
                      +¥ {{ (row.face_value_cents / 100).toFixed(2) }}
                    </span>
                  </template>
                </el-table-column>
                <el-table-column prop="used_by_username" label="用户" width="80" />
                <el-table-column label="时间" width="95">
                  <template #default="{ row }">
                    <span class="muted cell-mono" style="font-size: 11px">
                      {{ String(row.used_at).replace('T', ' ').slice(5, 16) }}
                    </span>
                  </template>
                </el-table-column>
              </el-table>
            </div>

            <!-- 移动端流式卡片 -->
            <div class="mobile-cards-view">
              <div v-if="!dashData?.recent_gift_cards || dashData.recent_gift_cards.length === 0" style="text-align: center; padding: 20px 0; color: var(--x-text-3); font-size: 13px">
                暂无卡密激活记录
              </div>
              <div v-else class="mobile-data-card-list">
                <div v-for="row in dashData.recent_gift_cards" :key="row.code_masked" class="mobile-data-card" style="padding: 10px 12px">
                  <div class="card-head" style="padding-bottom: 6px">
                    <div class="head-title">
                      <code class="cell-mono" style="font-weight: 600; font-size: 12px">{{ row.code_masked }}</code>
                    </div>
                    <span class="cell-mono" style="font-weight: 700; color: #059669; font-size: 13px">
                      +¥ {{ (row.face_value_cents / 100).toFixed(2) }}
                    </span>
                  </div>
                  <div style="display: flex; justify-content: space-between; align-items: center; margin-top: 4px">
                    <span style="font-size: 12px; font-weight: 500">用户：{{ row.used_by_username || '—' }}</span>
                    <span class="muted cell-mono" style="font-size: 11px">{{ String(row.used_at).replace('T', ' ').slice(5, 16) }}</span>
                  </div>
                </div>
              </div>
            </div>
          </el-tab-pane>

          <!-- 3. 最近套餐订单 -->
          <el-tab-pane label="订单流水" name="orders">
            <!-- 桌面端表格 -->
            <div class="desktop-table-view">
              <el-table :data="dashData?.recent_orders || []" size="small">
                <el-table-column label="订单号" min-width="110">
                  <template #default="{ row }">
                    <code class="cell-mono" style="font-size: 11px">{{ row.order_no }}</code>
                  </template>
                </el-table-column>
                <el-table-column prop="username" label="用户" width="90" />
                <el-table-column label="金额" width="85">
                  <template #default="{ row }">
                    <span class="cell-mono" style="font-weight: 600">¥ {{ (row.amount_cents / 100).toFixed(2) }}</span>
                  </template>
                </el-table-column>
                <el-table-column label="状态" width="80">
                  <template #default>
                    <el-tag type="success" size="small">已生效</el-tag>
                  </template>
                </el-table-column>
              </el-table>
            </div>

            <!-- 移动端流式卡片 -->
            <div class="mobile-cards-view">
              <div v-if="!dashData?.recent_orders || dashData.recent_orders.length === 0" style="text-align: center; padding: 20px 0; color: var(--x-text-3); font-size: 13px">
                暂无订单流水记录
              </div>
              <div v-else class="mobile-data-card-list">
                <div v-for="row in dashData.recent_orders" :key="row.order_no" class="mobile-data-card" style="padding: 10px 12px">
                  <div class="card-head" style="padding-bottom: 6px">
                    <div class="head-title">
                      <code class="cell-mono" style="font-size: 11.5px">{{ row.order_no }}</code>
                      <el-tag type="success" size="small" effect="light">已生效</el-tag>
                    </div>
                    <span class="cell-mono" style="font-weight: 700; color: #059669; font-size: 13px">
                      ¥ {{ (row.amount_cents / 100).toFixed(2) }}
                    </span>
                  </div>
                  <div style="font-size: 12px; margin-top: 4px">购买用户：<b>{{ row.username }}</b></div>
                </div>
              </div>
            </div>
          </el-tab-pane>
        </el-tabs>
      </div>
    </div>

    <!-- 挂载节点时序性能监控抽屉 -->
    <ServerMetricsDrawer
      v-model="metricsOpen"
      :server-id="selectedServer?.id || 0"
      :server-name="selectedServer?.name || ''"
    />

    <!-- 在线设备 → 在线用户名单弹窗（复用服务器详情抽屉的在线用户面板） -->
    <el-dialog
      v-model="onlineDialogOpen"
      :title="onlineServer ? `在线用户 · ${onlineServer.name}` : '在线用户'"
      width="560px"
      :append-to-body="true"
    >
      <OnlineUsersPanel v-if="onlineServer" :server-id="onlineServer.id" />
      <template #footer>
        <el-button type="primary" @click="onlineDialogOpen = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.dash-page {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.dash-title-wrap {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.dash-main-title {
  font-size: 18px;
  font-weight: 700;
  color: var(--x-text);
}

.dash-sub-title {
  font-size: 12.5px;
  color: var(--x-text-2);
}

/* 6 张 KPI 卡片（严格三端分级响应式：桌面 6列 -> Pad 3列 -> 手机 2列 -> 窄屏 1列） */
.kpi-grid {
  display: grid;
  grid-template-columns: repeat(6, minmax(0, 1fr));
  gap: 12px;

  @media (max-width: 1300px) {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 10px;
  }

  @media (max-width: 768px) {
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 8px;
  }

  @media (max-width: 440px) {
    grid-template-columns: 1fr;
  }
}

.kpi-card {
  position: relative;
  background: var(--x-card);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);
  box-shadow: var(--x-shadow);
  padding: 14px 16px;
  display: flex;
  align-items: center;
  gap: 12px;
  min-width: 0;
  overflow: hidden;
  transition: transform 0.2s ease, box-shadow 0.2s ease, border-color 0.2s ease;

  @media (max-width: 768px) {
    padding: 10px 12px;
    gap: 8px;
  }

  &:hover {
    transform: translateY(-2px);
    box-shadow: var(--x-shadow-md);
    border-color: rgba(99, 102, 241, 0.25);
  }
}

.kpi-icon-box {
  width: 38px;
  height: 38px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 18px;
  flex-shrink: 0;

  @media (max-width: 768px) {
    width: 32px;
    height: 32px;
    font-size: 16px;
  }
}

.card-cyan .kpi-icon-box { background: var(--x-info-soft, #e0f2fe); color: var(--x-info, #0284c7); }
.card-purple .kpi-icon-box { background: var(--x-primary-soft, #f3e8ff); color: var(--x-primary, #9333ea); }
.card-blue .kpi-icon-box { background: var(--x-info-soft, #dbeafe); color: #3b82f6; }
.card-emerald .kpi-icon-box { background: var(--x-success-soft, #d1fae5); color: var(--x-success, #059669); }
.card-indigo .kpi-icon-box { background: var(--x-primary-soft, #e0e7ff); color: var(--x-primary, #4338ca); }
.card-amber .kpi-icon-box { background: var(--x-warning-soft, #fef3c7); color: var(--x-warning, #d97706); }

.kpi-content {
  display: flex;
  flex-direction: column;
  min-width: 0;
  flex: 1;
  overflow: hidden;
}

.kpi-label {
  font-size: 12px;
  color: var(--x-text-2);
  font-weight: 500;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;

  @media (max-width: 768px) {
    font-size: 11px;
  }
}

.kpi-val {
  font-size: 17px;
  font-weight: 700;
  color: var(--x-text);
  margin: 2px 0 1px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-family: var(--x-font-mono);

  @media (max-width: 768px) {
    font-size: 14.5px;
  }
}

.kpi-sub {
  font-size: 11px;
  color: var(--x-text-3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;

  @media (max-width: 768px) {
    font-size: 10px;
  }
}

/* 图表双栏 */
.charts-row {
  display: grid;
  grid-template-columns: 1.45fr 1fr;
  gap: 16px;
  min-width: 0;
  max-width: 100%;

  @media (max-width: 1180px) {
    grid-template-columns: 1fr;
  }
}

.chart-panel {
  background: var(--x-card);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);
  box-shadow: var(--x-shadow);
  padding: 18px 20px;
  min-width: 0;
  max-width: 100%;
  box-sizing: border-box;

  @media (max-width: 768px) {
    padding: 14px 12px;
  }
}

.panel-head .title {
  font-size: 14.5px;
  font-weight: 600;
  color: var(--x-text);
}

.panel-head .sub {
  font-size: 12px;
  color: var(--x-text-2);
  margin-top: 2px;
}

.echart-container {
  width: 100%;
  max-width: 100%;
  height: 260px;

  @media (max-width: 768px) {
    height: 240px;
  }
}

/* 下部双栏 */
.bottom-grid {
  display: grid;
  grid-template-columns: 1.35fr 1fr;
  gap: 16px;
  min-width: 0;
  max-width: 100%;

  @media (max-width: 1180px) {
    grid-template-columns: 1fr;
  }
}

.matrix-card,
.ledger-card {
  background: var(--x-card);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);
  box-shadow: var(--x-shadow);
  padding: 18px 20px;
  min-width: 0;
  max-width: 100%;
  overflow: hidden;
  box-sizing: border-box;

  @media (max-width: 768px) {
    padding: 14px 12px;
  }
}

.cell-mono {
  font-family: var(--x-font-mono);
}

.muted {
  color: var(--x-text-3);
}

.online-link {
  color: var(--x-text);
  cursor: pointer;
  border-bottom: 1px dashed var(--x-border);
  &:hover {
    color: var(--x-primary);
    border-bottom-color: var(--x-primary);
  }
}
</style>