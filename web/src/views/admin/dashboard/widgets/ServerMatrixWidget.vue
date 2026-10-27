<script setup lang="ts">
import { computed, inject, onMounted, onUnmounted, ref } from 'vue'
import {
  CsDashboardWidget,
  CsTextFilter,
  CsStatusIndicator,
  CsButton,
  CsTable,
  CsProgressBar,
} from '@/components/cloudscape'
import type { TableColumn } from '@/components/cloudscape'
import { formatDateTime } from '@/utils/timezone'
import type { ServerMatrixItem } from '@/api/types'
import type { DashboardContext } from '../types'
import type { WidgetLayoutItem } from '@/components/cloudscape'

defineProps<{
  layoutItem: WidgetLayoutItem
}>()

const ctx = inject<DashboardContext>('adminDashboardContext')!
const dashData = ctx.dashData

const serverColumns: TableColumn[] = [
  { id: 'name', header: '服务器 / 域名', minWidth: 130 },
  { id: 'status', header: '状态', width: 130 },
  { id: 'load', header: '负载 (CPU / 内存)', width: 150 },
  { id: 'rate', header: '速率 / 在线', minWidth: 135 },
  { id: 'actions', header: '历史数据', width: 72, align: 'center' },
]

/** 窄卡片下让表格保持可读列宽并横向滚动，而不是把每列压到逐字换行。 */
const tableMinWidth = serverColumns.reduce(
  (sum, column) => sum + Number(column.width ?? column.minWidth ?? 120),
  0
)

const serverSearchText = ref('')
const filteredServerMatrix = computed(() => {
  const list = dashData.value?.server_matrix || []
  if (!serverSearchText.value.trim()) return list
  const q = serverSearchText.value.trim().toLowerCase()
  return list.filter((s) => s.name?.toLowerCase().includes(q) || s.host?.toLowerCase().includes(q))
})

/**
 * 五列表格最小需要 617px；卡片比这更窄时横向滚动要靠一条被 vendor 隐藏的滚动条，
 * 「历史数据」列常常整个落在视野外。窄卡片改成逐台服务器一张信息卡，
 * 操作按钮固定在卡片右上角，不依赖横向滚动。
 */
const bodyRef = ref<HTMLElement | null>(null)
const compactLayout = ref(false)
/** 更窄时两列负载条会被压到十几像素，改成一行一个指标 */
const tightMetrics = ref(false)
const COMPACT_LAYOUT_WIDTH = 560
const TIGHT_METRICS_WIDTH = 280
let bodyObserver: ResizeObserver | null = null

function syncCompactLayout() {
  const width = bodyRef.value?.clientWidth ?? 0
  if (width > 0) {
    compactLayout.value = width < COMPACT_LAYOUT_WIDTH
    tightMetrics.value = width < TIGHT_METRICS_WIDTH
  }
}

onMounted(() => {
  syncCompactLayout()
  if (bodyRef.value && typeof ResizeObserver !== 'undefined') {
    bodyObserver = new ResizeObserver(syncCompactLayout)
    bodyObserver.observe(bodyRef.value)
  }
})

onUnmounted(() => {
  bodyObserver?.disconnect()
  bodyObserver = null
})

function cpuPercent(row: ServerMatrixItem): number {
  return Math.min(100, Math.round(row.cpu || 0))
}

function memPercent(row: ServerMatrixItem): number {
  return row.mem_total ? Math.min(100, Math.round((row.mem / row.mem_total) * 100)) : 0
}

function loadColor(percent: number): string {
  if (percent > 80) return 'var(--color-text-status-error-ri38gt)'
  if (percent > 50) return 'var(--color-text-status-warning-fabq6h)'
  return 'var(--color-text-status-success-1i2jf8)'
}

function formatBandwidth(bytesPerSec: number): string {
  if (!bytesPerSec || bytesPerSec <= 0) return '0.00 Mbps'
  const mbps = (bytesPerSec * 8) / 1_000_000
  if (mbps >= 1000) {
    return `${(mbps / 1000).toFixed(2)} Gbps`
  }
  return `${mbps.toFixed(2)} Mbps`
}

function fmtTime(t: string | null) {
  if (!t) return '—'
  return formatDateTime(t, '—')
}
</script>

<template>
  <CsDashboardWidget
    id="server-matrix"
    title="服务器监控矩阵"
    :badge="`${dashData?.server_matrix?.length || 0} 实例`"
    no-padding
    :x="layoutItem.x"
    :y="layoutItem.y"
    :w="layoutItem.w"
    :h="layoutItem.h"
    :min-w="layoutItem.minW"
    :min-h="layoutItem.minH"
    @refresh="ctx.refreshDashboard"
  >
    <template #actions>
      <CsTextFilter
        v-model="serverSearchText"
        filtering-placeholder="按名称或主机筛选..."
        :count-text="serverSearchText ? `${filteredServerMatrix.length} 台` : ''"
        class="compact-filter"
      />
    </template>

    <div ref="bodyRef" class="matrix-body">
      <!-- 窄卡片：每台服务器一张信息卡，历史数据按钮固定在右上角 -->
      <ul v-if="compactLayout" :class="['server-cards', tightMetrics && 'is-tight']">
        <li v-for="row in filteredServerMatrix" :key="row.id" class="server-card">
          <div class="sc-top">
            <span class="aws-sort-badge">#{{ row.sort_order ?? 0 }}</span>
            <span class="sc-name" :title="row.name">{{ row.name }}</span>
            <CsStatusIndicator
              class="sc-status"
              :status="row.status === 1 ? 'online' : 'offline'"
              :text="row.status === 1 ? '在线' : '离线'"
              :pulse="row.status === 1"
            />
            <CsButton
              class="sc-history"
              variant="icon"
              icon-name="history"
              :aria-label="`查看 ${row.name || row.host} 的历史数据`"
              @click="ctx.openServerMetrics(row as ServerMatrixItem)"
            />
          </div>
          <div class="sc-host tabular-num" :title="row.host">
            {{ row.host }}
            <span class="sc-heartbeat">· 心跳 {{ row.last_seen_at ? fmtTime(row.last_seen_at).slice(5, 16) : '—' }}</span>
          </div>
          <div class="sc-metrics tabular-num">
            <div class="aws-load-row">
              <span class="load-lbl">CPU</span>
              <CsProgressBar
                class="load-bar"
                :value="cpuPercent(row)"
                :bar-height="5"
                hide-text
                :color="loadColor(cpuPercent(row))"
                :aria-label="`CPU 使用率 ${cpuPercent(row)}%`"
              />
              <span class="load-val">{{ cpuPercent(row) }}%</span>
            </div>
            <div class="aws-load-row">
              <span class="load-lbl">MEM</span>
              <CsProgressBar
                class="load-bar"
                :value="memPercent(row)"
                :bar-height="5"
                hide-text
                color="var(--color-text-link-default-qabnuk)"
                :aria-label="`内存使用率 ${memPercent(row)}%`"
              />
              <span class="load-val">{{ row.mem_total ? `${memPercent(row)}%` : '—' }}</span>
            </div>
          </div>
          <div class="sc-foot tabular-num">
            <span class="sc-rate">
              <span class="rate-rx">↓ {{ formatBandwidth(row.rx_rate) }}</span>
              <span class="rate-tx">↑ {{ formatBandwidth(row.tx_rate) }}</span>
            </span>
            <span class="online-users-sub">
              在线：<b class="aws-interactive-link" @click="ctx.openOnlineUsers(row as ServerMatrixItem)">{{ row.online_users || 0 }}</b> 人
            </span>
          </div>
        </li>
        <li v-if="!filteredServerMatrix.length" class="server-cards-empty">暂无服务器</li>
      </ul>

      <div v-else class="table-container" :style="{ '--server-table-min-width': `${tableMinWidth}px` }">
        <CsTable
          compact
          :items="filteredServerMatrix"
          :columns="serverColumns"
          :wrap-lines="false"
          track-by="id"
          empty-text="暂无服务器"
        >
          <!-- 1. 节点与地址 -->
          <template #cell-name="{ row }">
            <div class="server-name-cell">
              <span class="aws-sort-badge">#{{ row.sort_order ?? 0 }}</span>
              <span class="server-name">{{ row.name }}</span>
            </div>
            <div class="server-subhost tabular-num">{{ row.host }}</div>
          </template>

          <!-- 2. 状态与心跳 -->
          <template #cell-status="{ row }">
            <CsStatusIndicator
              :status="row.status === 1 ? 'online' : 'offline'"
              :text="row.status === 1 ? '在线 (Normal)' : '离线 (Offline)'"
              :pulse="row.status === 1"
            />
            <div class="server-heartbeat tabular-num">
              {{ row.last_seen_at ? fmtTime(row.last_seen_at).slice(5, 16) : '—' }}
            </div>
          </template>

          <!-- 3. 系统负载 -->
          <template #cell-load="{ row }">
            <div class="aws-load-row tabular-num">
              <span class="load-lbl">CPU</span>
              <CsProgressBar
                class="load-bar"
                :value="Math.min(100, Math.round(row.cpu || 0))"
                :bar-height="5"
                hide-text
                :color="
                  row.cpu > 80
                    ? 'var(--color-text-status-error-ri38gt)'
                    : row.cpu > 50
                      ? 'var(--color-text-status-warning-fabq6h)'
                      : 'var(--color-text-status-success-1i2jf8)'
                "
                :aria-label="`CPU 使用率 ${Math.round(row.cpu || 0)}%`"
              />
              <span class="load-val">{{ (row.cpu || 0).toFixed(0) }}%</span>
            </div>
            <div class="aws-load-row tabular-num" style="margin-top: 3px">
              <span class="load-lbl">MEM</span>
              <CsProgressBar
                class="load-bar"
                :value="row.mem_total ? Math.min(100, Math.round((row.mem / row.mem_total) * 100)) : 0"
                :bar-height="5"
                hide-text
                color="var(--color-text-link-default-qabnuk)"
                :aria-label="`内存使用率 ${row.mem_total ? Math.round((row.mem / row.mem_total) * 100) : 0}%`"
              />
              <span class="load-val">{{ row.mem_total ? `${Math.round((row.mem / row.mem_total) * 100)}%` : '—' }}</span>
            </div>
          </template>

          <!-- 4. 实时速率与在线设备 -->
          <template #cell-rate="{ row }">
            <div class="tabular-num rate-dual-line">
              <span class="rate-rx">↓ {{ formatBandwidth(row.rx_rate) }}</span>
              <span class="rate-tx">↑ {{ formatBandwidth(row.tx_rate) }}</span>
            </div>
            <div class="online-users-sub tabular-num">
              在线：<b class="aws-interactive-link" @click="ctx.openOnlineUsers(row as ServerMatrixItem)">{{ row.online_users || 0 }}</b> 人
            </div>
          </template>

          <!-- 5. 历史数据 -->
          <template #cell-actions="{ row }">
            <CsButton
              variant="icon"
              icon-name="history"
              :aria-label="`查看 ${row.name || row.host} 的历史数据`"
              @click="ctx.openServerMetrics(row as ServerMatrixItem)"
            />
          </template>
        </CsTable>
      </div>
    </div>
  </CsDashboardWidget>
</template>

<style scoped lang="scss">
.matrix-body {
  width: 100%;
  height: 100%;
  min-height: 0;
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
}

.table-container {
  width: 100%;
  height: 100%;
  min-height: 0;
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: auto;
  box-sizing: border-box;
  padding: 0 16px 16px;

  :deep(table) {
    min-width: var(--server-table-min-width, 0);
  }
}

/* 窄卡片：逐台信息卡。操作按钮在右上角，不随横向滚动跑出视野。 */
.server-cards {
  list-style: none;
  margin: 0;
  padding: 0 12px 12px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  flex: 1;
  min-height: 0;
  overflow: auto;
  box-sizing: border-box;
}

.server-card {
  min-width: 0;
  padding: 8px 10px 9px;
  border: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
  border-radius: 8px;
  background: var(--color-background-container-content-mjfil9, #ffffff);
}

.sc-top {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
}

.sc-name {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 700;
  font-size: 12.5px;
  color: var(--color-text-body-default-b2us04, #0f141a);
}

.sc-status,
.sc-history {
  flex: 0 0 auto;
}

.sc-host {
  margin-top: 2px;
  font-size: 11px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sc-heartbeat {
  color: var(--color-text-status-inactive-fqjc97, #879596);
}

.sc-metrics {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 4px 14px;
  margin-top: 6px;

  .aws-load-row {
    min-width: 0;
  }

  .load-bar {
    min-inline-size: 0;
  }
}

.server-cards.is-tight .sc-metrics {
  grid-template-columns: minmax(0, 1fr);
  gap: 4px;
}

/* 极窄卡片：速率与在线人数放不下一行时让在线人数换行，避免速率被省略号截断 */
.server-cards.is-tight .sc-foot {
  flex-wrap: wrap;
  row-gap: 2px;
}

.sc-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
  margin-top: 6px;
  min-width: 0;
  font-size: 11px;

  .online-users-sub {
    margin-top: 0;
    flex: 0 0 auto;
    white-space: nowrap;
  }
}

.sc-rate {
  display: flex;
  gap: 8px;
  min-width: 0;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
  font-size: 11.5px;
  font-weight: 600;

  .rate-rx {
    color: var(--color-text-link-default-qabnuk, #006ce0);
  }

  .rate-tx {
    color: var(--color-text-accent-1mglsg, #4338ca);
  }
}

.server-cards-empty {
  padding: 16px 0;
  text-align: center;
  font-size: 12px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.compact-filter :deep(input) {
  block-size: 28px !important;
}

.tabular-num {
  font-variant-numeric: tabular-nums;
  font-feature-settings: 'tnum' 1;
}

.aws-sort-badge {
  flex: 0 0 auto;
  font-size: 10px;
  font-weight: 600;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  background: var(--color-background-container-header-irktp5, #f2f3f3);
  padding: 1px 4px;
  border-radius: 4px;
  border: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
}

.server-name-cell {
  display: flex;
  align-items: center;
  gap: 6px;

  .server-name {
    font-weight: 700;
    color: var(--color-text-body-default-b2us04, #0f141a);
  }
}

.server-subhost {
  font-size: 11px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  margin-top: 1px;
}

.server-heartbeat {
  font-size: 11px;
  color: var(--color-text-status-inactive-fqjc97, #879596);
  margin-top: 2px;
}

.aws-load-row {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 11px;

  .load-lbl {
    font-size: 10px;
    font-weight: 700;
    color: var(--color-text-body-secondary-66ygbb, #656871);
    width: 30px;
    flex-shrink: 0;
  }

  .load-bar {
    flex: 1;
  }

  .load-val {
    font-size: 11px;
    font-weight: 600;
    width: 32px;
    text-align: right;
    flex-shrink: 0;
  }
}

.rate-dual-line {
  display: flex;
  flex-direction: column;
  gap: 1px;
  font-size: 11.5px;
  font-weight: 600;

  .rate-rx {
    color: var(--color-text-link-default-qabnuk, #006ce0);
  }

  .rate-tx {
    color: var(--color-text-accent-1mglsg, #4338ca);
  }
}

.online-users-sub {
  font-size: 11px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  margin-top: 2px;
}

.aws-interactive-link {
  color: var(--color-text-link-default-qabnuk, #006ce0);
  cursor: pointer;
  border-bottom: 1px dashed var(--color-text-link-default-qabnuk, #006ce0);

  &:hover {
    color: var(--color-text-link-hover-n1r6g0, #004a9e);
    border-bottom-style: solid;
  }
}
</style>

<!-- 非 scoped：选择器要带上 CsDashboardGrid 挂在祖先上的 .is-mobile-stack -->
<style lang="scss">
/* 单列移动端卡片按内容撑高，滚动交给页面（与 CsDashboardGrid 里 .table-container 的规则一致） */
.is-mobile-stack .matrix-body {
  flex: none;
  height: auto;
}

.is-mobile-stack .server-cards {
  flex: none;
  min-height: 0;
  overflow: visible;
}
</style>
