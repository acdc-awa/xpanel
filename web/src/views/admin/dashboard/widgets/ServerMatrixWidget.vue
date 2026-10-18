<script setup lang="ts">
import { computed, inject, ref } from 'vue'
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
  { id: 'actions', header: '操作', width: 64, align: 'center' },
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

    <div class="table-container" :style="{ '--server-table-min-width': `${tableMinWidth}px` }">
      <CsTable
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

        <!-- 5. 操作 -->
        <template #cell-actions="{ row }">
          <CsButton
            variant="icon"
            icon-name="view-vertical"
            aria-label="查看性能图表"
            @click="ctx.openServerMetrics(row as ServerMatrixItem)"
          />
        </template>
      </CsTable>
    </div>
  </CsDashboardWidget>
</template>

<style scoped lang="scss">
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

.compact-filter :deep(input) {
  block-size: 28px !important;
}

.tabular-num {
  font-variant-numeric: tabular-nums;
  font-feature-settings: 'tnum' 1;
}

.server-name-cell {
  display: flex;
  align-items: center;
  gap: 6px;

  .aws-sort-badge {
    font-size: 10px;
    font-weight: 600;
    color: var(--color-text-body-secondary-66ygbb, #656871);
    background: var(--color-background-container-header-irktp5, #f2f3f3);
    padding: 1px 4px;
    border-radius: 4px;
    border: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
  }

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
