<script setup lang="ts">
import { computed, onMounted, onUnmounted, provide, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from '@/components/cloudscape'
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
  CsButton,
  CsDashboardGrid,
  CsHeader,
  CsModal,
  CsSelect,
  CsSpaceBetween,
  CsStatusIndicator,
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

// 数据只来自主控 DB 聚合（不再提供前端演示数据开关）
const trendRange = ref<number>(30)
/** 饼图（服务器流量分布）的统计粒度。 */
const rankPeriod = ref<RankPeriod>('today')
/** 排行卡片的统计粒度；与饼图互相独立，接口不变、各自按需取数。 */
const rankingPeriod = ref<RankPeriod>('today')
const rankingData = ref<DashboardData | null>(null)

// 小组件网格布局状态
const layout = ref<WidgetLayoutItem[]>(loadSavedLayout())
const gridKey = ref(0)
const addDrawerVisible = ref(false)

// 抽屉监控
const metricsOpen = ref(false)
const selectedServer = ref<ServerMatrixItem | null>(null)

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
  trendRange,
  rankPeriod,
  rankingPeriod,
  rankingData,
  openServerMetrics,
  openOnlineUsers,
  refreshDashboard: () => load(),
  setRankingPeriod,
})

let autoRefreshTimer: any = null
let realtimeTimer: any = null
let inFlight = false
let reloadQueued = false
let realtimeInFlight = false
let rankingInFlight = false
let rankingReloadQueued = false

const POLL_INTERVAL_MS = 30000

// 实时刷新间隔是「这台浏览器」的偏好，不入库、不联动节点心跳：
// 数据新鲜度上限由 agent 心跳周期决定（设置页 agent_heartbeat_interval，默认 5s），
// 刷新快于心跳只会重复读到同一份内存快照，故允许调大或关闭。
const REALTIME_INTERVAL_STORAGE_KEY = 'dashboard_realtime_interval'
const DEFAULT_REALTIME_INTERVAL_MS = 3000
const REALTIME_INTERVAL_OPTIONS = [
  { label: '3 秒', value: 3000 },
  { label: '5 秒', value: 5000 },
  { label: '10 秒', value: 10000 },
  { label: '30 秒', value: 30000 },
  { label: '关闭', value: 0 },
]

function isRealtimeInterval(v: number) {
  return REALTIME_INTERVAL_OPTIONS.some((o) => o.value === v)
}

function loadSavedRealtimeInterval(): number {
  try {
    const raw = localStorage.getItem(REALTIME_INTERVAL_STORAGE_KEY)
    if (!raw) return DEFAULT_REALTIME_INTERVAL_MS
    const n = Number(raw)
    return isRealtimeInterval(n) ? n : DEFAULT_REALTIME_INTERVAL_MS
  } catch {
    return DEFAULT_REALTIME_INTERVAL_MS
  }
}

const realtimeInterval = ref<number>(loadSavedRealtimeInterval())

function setRealtimeInterval(v: string | number | (string | number)[]) {
  const n = Number(Array.isArray(v) ? v[0] : v)
  realtimeInterval.value = isRealtimeInterval(n) ? n : 0
}

const realtimeIntervalLabel = computed(
  () => REALTIME_INTERVAL_OPTIONS.find((o) => o.value === realtimeInterval.value)?.label || '',
)

const refreshHint = computed(() => `最后同步：${lastUpdatedTime.value || '—'}`)

const pollHint = computed(() =>
  realtimeInterval.value
    ? `实时网速与监控矩阵每 ${realtimeIntervalLabel.value}拉取一次（纯内存）；营收、流量趋势与排行榜每 30 秒拉取。数据新鲜度由节点心跳周期决定（默认 5 秒一帧），刷新快于心跳不会得到更新的数据。`
    : '实时高频刷新已关闭（30 秒统计仍会同步实时网速与监控矩阵）；可随时在右上角重新开启或手动点「刷新数据」。',
)

watch(realtimeInterval, (v) => {
  try {
    localStorage.setItem(REALTIME_INTERVAL_STORAGE_KEY, String(v))
  } catch {
    /* 隐私模式等无法写入时保持内存值 */
  }
  startRealtimeTimer()
  if (v) loadRealtime() // 切换后立刻给一次新数据，不必等下一个周期
})

/**
 * 排行卡片的数据始终对应 rankingPeriod，不随饼图粒度变化：
 * 粒度相同复用主数据（零额外请求），不同则单独按排行粒度取一份。
 * 不能把「排行数据为空时回退到主数据」写成兜底——饼图换粒度会让主数据
 * 立刻变成新粒度，排行会短暂显示成饼图的粒度再跳回来。
 */
async function loadRanking() {
  const period = rankingPeriod.value
  if (period === rankPeriod.value) {
    rankingData.value = dashData.value
    return
  }
  if (rankingInFlight) {
    // 连点粒度时让进行中的请求结束后补拉最新粒度，而不是直接丢弃。
    rankingReloadQueued = true
    return
  }
  rankingInFlight = true
  try {
    const { data } = await getDashboard(trendRange.value as any, period as any)
    // 请求期间粒度又变了：丢弃这份过期结果，交给排队的那次重拉。
    if (data.code === 0 && period === rankingPeriod.value) rankingData.value = data.data
  } catch {
    // 静默失败：排行沿用上一份数据，不影响主面板
  } finally {
    rankingInFlight = false
    if (rankingReloadQueued) {
      rankingReloadQueued = false
      void loadRanking()
    }
  }
}

async function setRankingPeriod(period: RankPeriod) {
  if (period === rankingPeriod.value) return
  rankingPeriod.value = period
  await loadRanking()
}

async function load(opts: { silent?: boolean } = {}) {
  if (inFlight) {
    // 30s 轮询或上一次请求未结束时，用户切换粒度/排行榜也要在结束后补一次最新值。
    reloadQueued = true
    return
  }
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
    // 排行粒度与饼图不同源时，跟随主轮询一起刷新
    void loadRanking()
  } catch (e) {
    if (opts.silent) {
      pollFailed.value = true
    } else {
      ElMessage.error(errMsg(e, '加载仪表盘失败'))
    }
  } finally {
    inFlight = false
    if (!opts.silent) loading.value = false
    if (reloadQueued) {
      reloadQueued = false
      void load({ silent: true })
    }
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

function startRealtimeTimer() {
  if (realtimeTimer) {
    clearInterval(realtimeTimer)
    realtimeTimer = null
  }
  if (!realtimeInterval.value) return
  realtimeTimer = setInterval(() => {
    if (!document.hidden) loadRealtime()
  }, realtimeInterval.value)
}

function startPolling() {
  stopPolling()
  startRealtimeTimer()

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
      <CsHeader variant="h1">
        运维监控大盘
        <template #info>
          <CsStatusIndicator
            :type="pollFailed ? 'warning' : isClusterHealthy ? 'success' : 'warning'"
            :text="pollFailed ? '数据拉取受阻' : isClusterHealthy ? '集群健康运行中' : '部分节点异常'"
          />
        </template>

        <template #actions>
          <div class="dashboard-actions">
            <CsBox
              class="dashboard-actions-hint"
              variant="span"
              color="text-body-secondary"
              font-size="body-s"
              :title="pollHint"
            >
              {{ refreshHint }}
            </CsBox>
            <div class="realtime-interval">
              <span class="realtime-interval-label">实时刷新</span>
              <CsSelect
                :model-value="realtimeInterval"
                :options="REALTIME_INTERVAL_OPTIONS"
                aria-label="实时刷新间隔"
                @change="setRealtimeInterval"
              />
            </div>
            <div class="dashboard-actions-buttons">
              <CsButton class="dashboard-action" icon-name="refresh" :loading="loading" @click="load()">刷新数据</CsButton>
              <CsButton class="dashboard-action" icon-name="add-plus" @click="addDrawerVisible = true">添加小组件</CsButton>
              <CsButton class="dashboard-action" icon-name="undo" @click="handleResetLayout">重置布局</CsButton>
            </div>
          </div>
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

/* CsSelect 根节点固定 inline-size:100%，间隔选择器需要按内容收窄 */
.realtime-interval {
  display: flex;
  align-items: center;
  gap: 6px;
}

.realtime-interval-label {
  font-size: 12px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  white-space: nowrap;
}

.realtime-interval :deep(.cs-select) {
  inline-size: 96px;
}

/* 操作区：桌面与提示文案同排，窄屏纵向堆叠 */
.dashboard-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

/* 操作按钮自成一个横向组，宽度随内容，只有放不下时才换行 */
.dashboard-actions-buttons {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px;
}

@media (max-width: $bp-tablet-max) {
  .dashboard-actions {
    inline-size: 100%;
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
