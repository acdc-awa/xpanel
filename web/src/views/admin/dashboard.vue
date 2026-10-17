<script setup lang="ts">
import { computed, onMounted, onUnmounted, provide, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { getDashboard, getDashboardRealtime } from '@/api/admin'
import type { RankPeriod } from '@/api/admin'
import { errMsg } from '@/api/http'
import type { DashboardData, ServerMatrixItem } from '@/api/types'
import { useThemeStore } from '@/stores/theme'
import ServerMetricsDrawer from './servers/ServerMetricsDrawer.vue'
import OnlineUsersPanel from './servers/OnlineUsersPanel.vue'

import {
  CsAlert,
  CsBox,
  CsBreadcrumbGroup,
  CsButton,
  CsDashboardGrid,
  CsHeader,
  CsModal,
  CsSpaceBetween,
  CsStatusIndicator,
  CsToggle,
  loadSavedLayout,
  saveLayout,
  resetLayout,
  WIDGET_DEFINITIONS,
} from '@/components/cloudscape'
import type { WidgetLayoutItem } from '@/components/cloudscape'

import KpiSummaryWidget from './dashboard/widgets/KpiSummaryWidget.vue'
import TrafficTrendWidget from './dashboard/widgets/TrafficTrendWidget.vue'
import TrafficShareWidget from './dashboard/widgets/TrafficShareWidget.vue'
import ServerMatrixWidget from './dashboard/widgets/ServerMatrixWidget.vue'
import AnalyticsRankingWidget from './dashboard/widgets/AnalyticsRankingWidget.vue'
import AddWidgetDrawer from './dashboard/AddWidgetDrawer.vue'
import type { DashboardContext } from './dashboard/types'

const theme = useThemeStore()

const loading = ref(false)
const dashData = ref<DashboardData | null>(null)
const lastUpdatedTime = ref('')
const pollFailed = ref(false)

// 演示数据开关（默认关闭，优先直接使用数据库真实聚合数据）
const useMockData = ref(false)
const trendRange = ref<number>(30)
const rankPeriod = ref<RankPeriod>('today')

// 小组件网格布局状态
const layout = ref<WidgetLayoutItem[]>(loadSavedLayout())
const gridKey = ref(0)
const addDrawerVisible = ref(false)

// 抽屉监控
const metricsOpen = ref(false)
const selectedServer = ref<ServerMatrixItem | null>(null)

// 面包屑
const breadcrumbs = [
  { text: '运维监控', href: '#' },
  { text: '仪表盘' },
]

// 集群整体健康度计算
const onlineServersCount = computed(() => dashData.value?.summary.online_servers || 0)
const totalServersCount = computed(() => dashData.value?.summary.total_servers || 0)
const isClusterHealthy = computed(() => {
  if (!totalServersCount.value) return true
  return onlineServersCount.value === totalServersCount.value
})

// 在线用户弹窗
const onlineDialogOpen = ref(false)
const onlineServer = ref<ServerMatrixItem | null>(null)

function openOnlineUsers(s: ServerMatrixItem) {
  onlineServer.value = s
  onlineDialogOpen.value = true
}

function openServerMetrics(s: ServerMatrixItem) {
  selectedServer.value = s
  metricsOpen.value = true
}

// 布局变更与管理事件处理
function handleLayoutChange(newLayout: WidgetLayoutItem[]) {
  layout.value = newLayout
  saveLayout(newLayout)
}

function handleWidgetRemove(id: string) {
  const def = WIDGET_DEFINITIONS[id]
  const title = def ? def.title : id
  layout.value = layout.value.filter((item) => item.id !== id)
  saveLayout(layout.value)
  ElMessage.success({
    message: `已从大盘移除「${title}」，可随时在右上角「添加小组件」中恢复。`,
    duration: 3000,
  })
}

async function handleResetLayout() {
  try {
    await ElMessageBox.confirm(
      '确定要恢复官方默认排版吗？当前所有自定义卡片位置与大小都将被重置。',
      '重置大盘布局',
      {
        confirmButtonText: '确定重置',
        cancelButtonText: '取消',
        type: 'warning',
      }
    )
    layout.value = resetLayout()
    gridKey.value++
    ElMessage.success('已恢复为官方默认仪表盘布局')
  } catch {}
}

function handleAddWidget(id: string) {
  const def = WIDGET_DEFINITIONS[id]
  if (!def) return
  if (layout.value.some((item) => item.id === id)) {
    ElMessage.info('该小组件已在大盘中')
    return
  }

  const maxY = layout.value.reduce((acc, cur) => Math.max(acc, (cur.y || 0) + (cur.h || 1)), 0)

  layout.value.push({
    id,
    x: 0,
    y: maxY,
    w: def.defaultW,
    h: def.defaultH,
    minW: def.minW,
    minH: def.minH,
  })

  saveLayout(layout.value)
  gridKey.value++
  ElMessage.success(`已添加「${def.title}」至大盘`)
}

// 提供全局上下文给所有 Widget
provide<DashboardContext>('adminDashboardContext', {
  dashData,
  loading,
  pollFailed,
  useMockData,
  trendRange,
  rankPeriod,
  openServerMetrics,
  openOnlineUsers,
  refreshDashboard: () => load(),
})

let autoRefreshTimer: any = null
let realtimeTimer: any = null
let inFlight = false
let realtimeInFlight = false

const REALTIME_POLL_INTERVAL_MS = 3000
const POLL_INTERVAL_MS = 30000
const POLL_HINT = '实时网速与监控矩阵每 3 秒刷新（纯内存）；营收、流量趋势与排行榜每 30 秒拉取。'

async function load(opts: { silent?: boolean } = {}) {
  if (inFlight) return
  inFlight = true
  if (!opts.silent) loading.value = true
  try {
    const { data } = await getDashboard(trendRange.value as any, rankPeriod.value as any)
    if (data.code === 0) {
      dashData.value = data.data
      lastUpdatedTime.value = new Date().toLocaleTimeString('zh-CN', { hour12: false })
      pollFailed.value = false
    } else if (opts.silent) {
      pollFailed.value = true
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    if (opts.silent) {
      pollFailed.value = true
    } else {
      ElMessage.error(errMsg(e, '加载仪表盘失败'))
    }
  } finally {
    inFlight = false
    if (!opts.silent) loading.value = false
  }
}

async function loadRealtime() {
  if (realtimeInFlight || !dashData.value) return
  realtimeInFlight = true
  try {
    const { data } = await getDashboardRealtime()
    if (data.code === 0 && data.data && dashData.value) {
      dashData.value.summary.realtime_rx_rate = data.data.realtime_rx_rate
      dashData.value.summary.realtime_tx_rate = data.data.realtime_tx_rate
      dashData.value.summary.online_servers = data.data.online_servers
      dashData.value.summary.total_servers = data.data.total_servers
      const currentList = dashData.value.server_matrix || []
      dashData.value.server_matrix = data.data.server_matrix.map((newItem) => {
        const existing = currentList.find((s) => s.id === newItem.id)
        if (existing && newItem.status === 0 && !newItem.mem_total && existing.mem_total) {
          return {
            ...newItem,
            cpu: existing.cpu,
            mem: existing.mem,
            mem_total: existing.mem_total,
            disk_total: existing.disk_total,
            online_users: existing.online_users,
          }
        }
        return newItem
      })
    }
  } catch {
    // 静默兜底
  } finally {
    realtimeInFlight = false
  }
}

function startPolling() {
  stopPolling()
  realtimeTimer = setInterval(() => {
    if (!document.hidden) loadRealtime()
  }, REALTIME_POLL_INTERVAL_MS)

  autoRefreshTimer = setInterval(() => {
    if (!document.hidden) load({ silent: true })
  }, POLL_INTERVAL_MS)
}

function stopPolling() {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
  if (realtimeTimer) {
    clearInterval(realtimeTimer)
    realtimeTimer = null
  }
}

function onVisibilityChange() {
  if (document.hidden) {
    stopPolling()
    return
  }
  load({ silent: true })
  startPolling()
}

onMounted(() => {
  load()
  document.addEventListener('visibilitychange', onVisibilityChange)
  startPolling()
})

onUnmounted(() => {
  stopPolling()
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<template>
  <div class="aws-console-page" :class="{ 'awsui-dark-mode': theme.isDark }">
    <CsSpaceBetween direction="vertical" size="l">
      <CsBreadcrumbGroup :items="breadcrumbs" />

      <CsHeader
        variant="h1"
        description="实时监控节点健康状态、全网吞吐速率、运营营收与用量指标；拖拽卡片左上角手柄可调整位置，右下角可缩放。"
      >
        运维监控大盘
        <template #info>
          <CsStatusIndicator
            :type="pollFailed ? 'warning' : isClusterHealthy ? 'success' : 'warning'"
            :text="pollFailed ? '数据拉取受阻' : isClusterHealthy ? '集群健康运行中' : '部分节点异常'"
          />
        </template>

        <template #actions>
          <CsSpaceBetween direction="horizontal" size="xs" align-items="center">
            <CsToggle v-model="useMockData" label="演示数据 (Mock)" />
            <CsBox variant="span" color="text-body-secondary" font-size="body-s" :title="POLL_HINT">
              最后同步：{{ lastUpdatedTime || '—' }} · 3s 实时 / 30s 统计
            </CsBox>
            <CsButton icon-name="refresh" :loading="loading" @click="load()">刷新数据</CsButton>
            <CsButton icon-name="add-plus" @click="addDrawerVisible = true">添加小组件</CsButton>
            <CsButton icon-name="undo" @click="handleResetLayout">重置布局</CsButton>
            <router-link to="/admin/servers" custom v-slot="{ navigate }">
              <CsButton variant="primary" @click="navigate">管理服务器</CsButton>
            </router-link>
          </CsSpaceBetween>
        </template>
      </CsHeader>

      <CsAlert
        v-if="pollFailed"
        type="warning"
        header="监控数据实时同步受阻"
        dismissible
        @dismiss="pollFailed = false"
      >
        后台自动拉取接口暂时无响应，系统正在尝试重连。您也可以手动点击右上角「刷新数据」按钮。
      </CsAlert>

      <CsDashboardGrid
        :key="gridKey"
        :layout="layout"
        @layout-change="handleLayoutChange"
        @widget-remove="handleWidgetRemove"
      >
        <template v-for="item in layout" :key="item.id">
          <KpiSummaryWidget v-if="item.id === 'kpi-summary'" :layout-item="item" />
          <TrafficTrendWidget v-else-if="item.id === 'traffic-trend'" :layout-item="item" />
          <TrafficShareWidget v-else-if="item.id === 'traffic-share'" :layout-item="item" />
          <ServerMatrixWidget v-else-if="item.id === 'server-matrix'" :layout-item="item" />
          <AnalyticsRankingWidget v-else-if="item.id === 'analytics-ranking'" :layout-item="item" />
        </template>
      </CsDashboardGrid>

      <ServerMetricsDrawer
        v-model="metricsOpen"
        :server-id="selectedServer?.id || 0"
        :server-name="selectedServer?.name || ''"
      />

      <CsModal
        v-model:visible="onlineDialogOpen"
        :header="onlineServer ? `在线用户名单 · ${onlineServer.name}` : '在线用户名单'"
        size="large"
        @close="onlineServer = null"
      >
        <OnlineUsersPanel v-if="onlineServer" :server-id="onlineServer.id" />
        <template #footer>
          <CsButton @click="onlineDialogOpen = false">关闭</CsButton>
        </template>
      </CsModal>

      <AddWidgetDrawer
        v-model:visible="addDrawerVisible"
        :current-layout="layout"
        @add-widget="handleAddWidget"
      />
    </CsSpaceBetween>
  </div>
</template>

<style scoped lang="scss">
.aws-console-page {
  font-family: var(--font-family-base-c9u5cr, 'Open Sans', 'Helvetica Neue', Roboto, Arial, sans-serif);
  background-color: var(--color-background-layout-main-y4565g, #f2f3f3);
  color: var(--color-text-body-default-b2us04, #0f141a);
  min-height: 100%;
  width: 100%;
  max-width: none;
  margin: 0;
  padding: 16px 20px 32px;
  box-sizing: border-box;
}

.awsui-dark-mode {
  background-color: var(--color-background-layout-main-y4565g, #0f1b2a);
  color: var(--color-text-body-default-b2us04, #f9fafb);
}

:deep(a) {
  color: var(--color-text-link-default-qabnuk, #006ce0);
}
</style>
