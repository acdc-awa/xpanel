<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref } from 'vue'
import {
  TrendCharts,
  Money,
  Coin,
  Histogram,
  Connection,
  User,
  Refresh,
  Wallet,
  Present,
  ShoppingCart,
  Cpu,
} from '@element-plus/icons-vue'
import * as echarts from 'echarts'
import { getDashboard } from '@/api/admin'
import { errMsg } from '@/api/http'
import type { DashboardData, ServerMatrixItem } from '@/api/types'
import { formatBytes } from '@/utils/format'
import ServerMetricsDrawer from './servers/ServerMetricsDrawer.vue'

const loading = ref(false)
const dashData = ref<DashboardData | null>(null)
const activeTab = ref('users')
const lastUpdatedTime = ref('')

// 抽屉监控
const metricsOpen = ref(false)
const selectedServer = ref<ServerMatrixItem | null>(null)

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

function initCharts() {
  if (trendChartRef.value && !trendChart) {
    trendChart = echarts.init(trendChartRef.value)
  }
  if (donutChartRef.value && !donutChart) {
    donutChart = echarts.init(donutChartRef.value)
  }
}

function resizeCharts() {
  trendChart?.resize()
  donutChart?.resize()
}

function updateCharts() {
  if (!dashData.value) return

  // 1. 流量趋势面积图
  const trend = dashData.value.traffic_trend || []
  const dates = trend.map((t) => t.date.slice(5))
  const upData = trend.map((t) => Number((t.up_bytes / (1024 * 1024 * 1024)).toFixed(2)))
  const downData = trend.map((t) => Number((t.down_bytes / (1024 * 1024 * 1024)).toFixed(2)))

  trendChart?.setOption({
    tooltip: {
      trigger: 'axis',
      backgroundColor: 'rgba(15, 23, 42, 0.9)',
      borderColor: '#334155',
      borderWidth: 1,
      padding: [8, 12],
      textStyle: { color: '#ffffff', fontSize: 12 },
      extraCssText: 'box-shadow: 0 8px 24px rgba(0,0,0,0.15); border-radius: 8px;',
      formatter: (params: any) => {
        let res = `<div style="font-weight:600;margin-bottom:4px;color:#f8fafc">${params[0]?.axisValue}</div>`
        params.forEach((item: any) => {
          res += `<div style="display:flex;align-items:center;gap:6px;margin-top:2px">${item.marker} <span style="color:#cbd5e1">${item.seriesName}:</span> <b style="color:#fff">${item.value} GB</b></div>`
        })
        return res
      },
    },
    legend: {
      data: ['上行流量 (Upload)', '下行流量 (Download)'],
      top: 0,
      right: 10,
      textStyle: { color: '#64748b', fontSize: 12 },
    },
    grid: { top: 35, right: 20, bottom: 25, left: 55 },
    xAxis: {
      type: 'category',
      data: dates,
      axisLine: { lineStyle: { color: '#e2e8f0' } },
      axisLabel: { color: '#64748b', fontSize: 11 },
    },
    yAxis: {
      type: 'value',
      name: 'GB',
      nameTextStyle: { color: '#64748b' },
      splitLine: { lineStyle: { color: '#f1f5f9' } },
      axisLabel: { color: '#64748b' },
    },
    series: [
      {
        name: '上行流量 (Upload)',
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: upData,
        itemStyle: { color: '#6366f1' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(99, 102, 241, 0.25)' },
            { offset: 1, color: 'rgba(99, 102, 241, 0.0)' },
          ]),
        },
      },
      {
        name: '下行流量 (Download)',
        type: 'line',
        smooth: true,
        showSymbol: false,
        data: downData,
        itemStyle: { color: '#10b981' },
        areaStyle: {
          color: new echarts.graphic.LinearGradient(0, 0, 0, 1, [
            { offset: 0, color: 'rgba(16, 185, 129, 0.25)' },
            { offset: 1, color: 'rgba(16, 185, 129, 0.0)' },
          ]),
        },
      },
    ],
  })

  // 2. 节点流量占比环形图
  const breakdown = dashData.value.server_breakdown || []
  const pieData = breakdown
    .filter((b) => b.total_bytes > 0)
    .map((b) => ({
      name: b.name || `节点#${b.server_id}`,
      value: Number((b.total_bytes / (1024 * 1024 * 1024)).toFixed(2)),
    }))

  donutChart?.setOption({
    color: ['#6366f1', '#3b82f6', '#10b981', '#f59e0b', '#ec4899', '#8b5cf6', '#06b6d4', '#14b8a6'],
    tooltip: {
      trigger: 'item',
      backgroundColor: 'rgba(15, 23, 42, 0.9)',
      borderColor: '#334155',
      borderWidth: 1,
      padding: [8, 12],
      textStyle: { color: '#ffffff', fontSize: 12 },
      extraCssText: 'box-shadow: 0 8px 24px rgba(0,0,0,0.15); border-radius: 8px;',
      formatter: '{b}: <b>{c} GB</b> ({d}%)',
    },
    legend: {
      orient: 'vertical',
      right: '2%',
      top: 'middle',
      textStyle: { color: '#64748b', fontSize: 11 },
    },
    series: [
      {
        name: '节点流量',
        type: 'pie',
        radius: ['50%', '76%'],
        center: ['38%', '50%'],
        avoidLabelOverlap: false,
        itemStyle: {
          borderRadius: 6,
          borderColor: '#ffffff',
          borderWidth: 2,
        },
        label: { show: false },
        emphasis: {
          label: {
            show: true,
            fontSize: 13,
            fontWeight: 'bold',
            color: '#1e293b',
          },
        },
        data: pieData.length ? pieData : [{ name: '暂无流量', value: 0 }],
      },
    ],
  })
}

async function load() {
  loading.value = true
  try {
    const { data } = await getDashboard()
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
          <div class="dash-main-title">运营数据中心</div>
          <div class="dash-sub-title">实时营收、网络吞吐、集群健康度与用户用量大盘</div>
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
          <div class="kpi-label">今日激活营收</div>
          <div class="kpi-val cell-mono">¥ {{ ((dashData?.summary.today_revenue_cents || 0) / 100).toFixed(2) }}</div>
          <div class="kpi-sub">今日激活 {{ dashData?.summary.today_used_cards_count || 0 }} 张充值卡</div>
        </div>
      </div>

      <!-- 2. 本月累计营收 -->
      <div class="kpi-card card-purple">
        <div class="kpi-icon-box"><el-icon><Money /></el-icon></div>
        <div class="kpi-content">
          <div class="kpi-label">本月累计营收</div>
          <div class="kpi-val cell-mono">¥ {{ ((dashData?.summary.month_revenue_cents || 0) / 100).toFixed(2) }}</div>
          <div class="kpi-sub">历史累计 ¥ {{ ((dashData?.summary.total_revenue_cents || 0) / 100).toFixed(2) }}</div>
        </div>
      </div>

      <!-- 3. 今日消耗流量 -->
      <div class="kpi-card card-blue">
        <div class="kpi-icon-box"><el-icon><Histogram /></el-icon></div>
        <div class="kpi-content">
          <div class="kpi-label">今日全网流量</div>
          <div class="kpi-val cell-mono">{{ formatBytes(dashData?.summary.today_traffic_total || 0) }}</div>
          <div class="kpi-sub">
            ↑ {{ formatBytes(dashData?.summary.today_traffic_up || 0) }} / ↓ {{ formatBytes(dashData?.summary.today_traffic_down || 0) }}
          </div>
        </div>
      </div>

      <!-- 4. 实时出口带宽 -->
      <div class="kpi-card card-emerald">
        <div class="kpi-icon-box"><el-icon><TrendCharts /></el-icon></div>
        <div class="kpi-content">
          <div class="kpi-label">实时出口速率</div>
          <div class="kpi-val cell-mono">{{ formatBandwidth((dashData?.summary.realtime_rx_rate || 0) + (dashData?.summary.realtime_tx_rate || 0)) }}</div>
          <div class="kpi-sub">
            Rx: {{ formatBandwidth(dashData?.summary.realtime_rx_rate || 0) }} | Tx: {{ formatBandwidth(dashData?.summary.realtime_tx_rate || 0) }}
          </div>
        </div>
      </div>

      <!-- 5. 节点在线集群 -->
      <div class="kpi-card card-indigo">
        <div class="kpi-icon-box"><el-icon><Connection /></el-icon></div>
        <div class="kpi-content">
          <div class="kpi-label">节点集群状态</div>
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
          <div class="kpi-label">有效用户数</div>
          <div class="kpi-val cell-mono">
            {{ dashData?.summary.active_users || 0 }} / {{ dashData?.summary.total_users || 0 }}
          </div>
          <div class="kpi-sub">
            今日订单 {{ dashData?.summary.today_orders || 0 }} 笔 / 累计 {{ dashData?.summary.total_orders || 0 }}
          </div>
        </div>
      </div>
    </div>

    <!-- 中部图表：30 天吞吐趋势 + 节点流量占比 -->
    <div class="charts-row">
      <div class="chart-panel trend-panel">
        <div class="panel-head">
          <div class="title">全网吞吐趋势 (近 30 天)</div>
          <div class="sub">每日上下行流量分布汇总 (GB)</div>
        </div>
        <div ref="trendChartRef" class="echart-container"></div>
      </div>

      <div class="chart-panel donut-panel">
        <div class="panel-head">
          <div class="title">节点流量占比分布</div>
          <div class="sub">各节点累计入站承载流量占比</div>
        </div>
        <div ref="donutChartRef" class="echart-container"></div>
      </div>
    </div>

    <!-- 下部双栏：服务器健康度矩阵 + 右侧榜单与流水 -->
    <div class="bottom-grid">
      <!-- 左侧：服务器健康度与系统负载矩阵 -->
      <div class="matrix-card">
        <div class="panel-head" style="margin-bottom: 14px">
          <div class="title">节点健康度与系统负载矩阵</div>
          <div class="sub">节点实时心跳、CPU、内存与出口带宽监控</div>
        </div>
        <el-table :data="dashData?.server_matrix || []" size="small" style="width: 100%">
          <el-table-column label="节点" min-width="120">
            <template #default="{ row }">
              <div style="font-weight: 600; color: var(--x-text)">{{ row.name }}</div>
              <div class="muted cell-mono" style="font-size: 11px">{{ row.host }}</div>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small">
                <span class="x-status-dot" :class="row.status === 1 ? 'online' : 'offline'" />
                {{ row.status === 1 ? '在线' : '离线' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="CPU" width="110">
            <template #default="{ row }">
              <div style="display: flex; align-items: center; gap: 6px">
                <el-progress
                  :percentage="Math.min(100, Math.round(row.cpu || 0))"
                  :color="row.cpu > 80 ? '#ef4444' : row.cpu > 50 ? '#f59e0b' : '#10b981'"
                  :stroke-width="6"
                  :show-text="false"
                  style="width: 45px"
                />
                <span class="cell-mono" style="font-size: 11px">{{ (row.cpu || 0).toFixed(0) }}%</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="内存" width="110">
            <template #default="{ row }">
              <div style="display: flex; align-items: center; gap: 6px">
                <el-progress
                  :percentage="row.mem_total ? Math.min(100, Math.round((row.mem / row.mem_total) * 100)) : 0"
                  color="#3b82f6"
                  :stroke-width="6"
                  :show-text="false"
                  style="width: 45px"
                />
                <span class="cell-mono" style="font-size: 11px">
                  {{ row.mem_total ? `${Math.round((row.mem / row.mem_total) * 100)}%` : '—' }}
                </span>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="实时带宽" min-width="130">
            <template #default="{ row }">
              <div class="cell-mono" style="font-size: 11.5px">
                <span style="color: #0284c7">↓{{ formatBandwidth(row.rx_rate) }}</span>
                <span style="color: #6366f1; margin-left: 6px">↑{{ formatBandwidth(row.tx_rate) }}</span>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="在线设备" width="80">
            <template #default="{ row }">
              <span class="cell-mono" style="font-weight: 600">{{ row.online_users || 0 }} 台</span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="90" fixed="right">
            <template #default="{ row }">
              <el-button size="small" text type="primary" @click="openServerMetrics(row as ServerMatrixItem)">
                <el-icon><TrendCharts /></el-icon>&nbsp;图表
              </el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- 右侧：榜单与流水切换 Tabs -->
      <div class="ledger-card">
        <el-tabs v-model="activeTab" class="ledger-tabs">
          <!-- 1. 用户流量排行 -->
          <el-tab-pane label="🏆 流量排行榜" name="users">
            <el-table :data="dashData?.user_rank || []" size="small">
              <el-table-column label="#" width="45">
                <template #default="{ $index }">
                  <span v-if="$index === 0" style="color: #f59e0b; font-weight: 700">🥇</span>
                  <span v-else-if="$index === 1" style="color: #94a3b8; font-weight: 700">🥈</span>
                  <span v-else-if="$index === 2" style="color: #b45309; font-weight: 700">🥉</span>
                  <span v-else class="cell-mono muted">{{ $index + 1 }}</span>
                </template>
              </el-table-column>
              <el-table-column label="用户" min-width="120">
                <template #default="{ row }">
                  <span style="font-weight: 600; color: var(--x-text)">{{ row.username }}</span>
                  <div class="muted cell-mono" style="font-size: 11px">{{ row.email || '—' }}</div>
                </template>
              </el-table-column>
              <el-table-column prop="plan_name" label="套餐" width="95">
                <template #default="{ row }">
                  <el-tag size="small" type="info" effect="plain">{{ row.plan_name }}</el-tag>
                </template>
              </el-table-column>
              <el-table-column label="已用总流量" width="105">
                <template #default="{ row }">
                  <span class="cell-mono" style="font-weight: 700; color: var(--x-primary)">
                    {{ formatBytes(row.total_bytes) }}
                  </span>
                </template>
              </el-table-column>
            </el-table>
          </el-tab-pane>

          <!-- 2. 最近卡密激活流水 -->
          <el-tab-pane label="🎫 最近卡密激活" name="cards">
            <el-table :data="dashData?.recent_gift_cards || []" size="small">
              <el-table-column label="卡密" min-width="130">
                <template #default="{ row }">
                  <code class="cell-mono" style="font-size: 11.5px">{{ row.code_masked }}</code>
                </template>
              </el-table-column>
              <el-table-column label="面值" width="90">
                <template #default="{ row }">
                  <span class="cell-mono" style="font-weight: 700; color: #059669">
                    +¥ {{ (row.face_value_cents / 100).toFixed(2) }}
                  </span>
                </template>
              </el-table-column>
              <el-table-column prop="used_by_username" label="兑换用户" width="100" />
              <el-table-column label="激活时间" width="120">
                <template #default="{ row }">
                  <span class="muted cell-mono" style="font-size: 11px">
                    {{ String(row.used_at).replace('T', ' ').slice(5, 16) }}
                  </span>
                </template>
              </el-table-column>
            </el-table>
          </el-tab-pane>

          <!-- 3. 最近套餐订单 -->
          <el-tab-pane label="📦 最近套餐订单" name="orders">
            <el-table :data="dashData?.recent_orders || []" size="small">
              <el-table-column label="订单号" min-width="130">
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
                <template #default="{ row }">
                  <el-tag type="success" size="small">已生效</el-tag>
                </template>
              </el-table-column>
            </el-table>
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

/* 6 张 KPI 卡片 */
.kpi-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 16px;
}

.kpi-card {
  position: relative;
  background: var(--x-card);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);
  box-shadow: var(--x-shadow);
  padding: 16px 18px;
  display: flex;
  align-items: center;
  gap: 14px;
  overflow: hidden;
  transition: transform 0.2s ease, box-shadow 0.2s ease, border-color 0.2s ease;

  &:hover {
    transform: translateY(-2px);
    box-shadow: var(--x-shadow-md);
    border-color: rgba(99, 102, 241, 0.25);
  }
}

.kpi-icon-box {
  width: 44px;
  height: 44px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 22px;
  flex-shrink: 0;
}

.card-cyan .kpi-icon-box { background: #e0f2fe; color: #0284c7; }
.card-purple .kpi-icon-box { background: #f3e8ff; color: #9333ea; }
.card-blue .kpi-icon-box { background: #dbeafe; color: #2563eb; }
.card-emerald .kpi-icon-box { background: #d1fae5; color: #059669; }
.card-indigo .kpi-icon-box { background: #e0e7ff; color: #4338ca; }
.card-amber .kpi-icon-box { background: #fef3c7; color: #d97706; }

.kpi-content {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.kpi-label {
  font-size: 12.5px;
  color: var(--x-text-2);
  font-weight: 500;
}

.kpi-val {
  font-size: 20px;
  font-weight: 700;
  color: var(--x-text);
  margin: 3px 0 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  font-family: var(--x-font-mono);
}

.kpi-sub {
  font-size: 11.5px;
  color: var(--x-text-3);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 图表双栏 */
.charts-row {
  display: grid;
  grid-template-columns: 2fr 1fr;
  gap: 16px;

  @media (max-width: 1024px) {
    grid-template-columns: 1fr;
  }
}

.chart-panel {
  background: var(--x-card);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);
  box-shadow: var(--x-shadow);
  padding: 18px 20px;
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
  height: 250px;
}

/* 下部双栏 */
.bottom-grid {
  display: grid;
  grid-template-columns: 1.4fr 1fr;
  gap: 16px;

  @media (max-width: 1120px) {
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
}

.cell-mono {
  font-family: var(--x-font-mono);
}

.muted {
  color: var(--x-text-3);
}
</style>