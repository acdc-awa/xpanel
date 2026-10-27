<script setup lang="ts">
import { computed, inject, ref } from 'vue'
import {
  CsDashboardWidget,
  CsSegmentedControl,
  CsTable,
  CsTabs,
} from '@/components/cloudscape'
import type { TableColumn } from '@/components/cloudscape'
import type { RankPeriod } from '@/api/admin'
import { formatBytes } from '@/utils/format'
import { rankPeriodOptions } from '../rank-period'
import type { DashboardContext } from '../types'
import type { WidgetLayoutItem } from '@/components/cloudscape'

defineProps<{
  layoutItem: WidgetLayoutItem
}>()

const ctx = inject<DashboardContext>('adminDashboardContext')!

/**
 * 只认自己的数据：`rankingData` 由 dashboard.vue 维护，粒度与饼图相同时它指向
 * 主数据、不同时是单独拉的那份。这里不做 `|| dashData` 兜底——否则饼图换粒度
 * 的瞬间主数据先变、排行数据还没到，会闪一下饼图的粒度。
 */
const rankSource = computed(() => ctx.rankingData.value)

const activeTab = ref('users')

function onPeriodChange(detail: { selectedId: string | number }) {
  void ctx.setRankingPeriod(detail.selectedId as RankPeriod)
}

const dashboardTabs = computed(() => [
  { id: 'users', label: '用户排行' },
  { id: 'nodes', label: '节点排行' },
  { id: 'cards', label: '卡密核销' },
  { id: 'orders', label: '订单流水' },
])

const userColumns = computed<TableColumn[]>(() => [
  { id: 'rank', header: '#', width: 40 },
  { id: 'user-main', header: '用户', minWidth: 110 },
  { id: 'user-plan', header: '套餐', width: 90 },
  { id: 'user-usage', header: '用量', width: 105 },
])

const nodeColumns = computed<TableColumn[]>(() => [
  { id: 'rank', header: '#', width: 40 },
  { id: 'node-main', header: '节点', minWidth: 110 },
  { id: 'node-percent', header: '占比', width: 70 },
  { id: 'node-usage', header: '用量', width: 105 },
])

const cardColumns: TableColumn[] = [
  { id: 'card-code', header: '卡密', minWidth: 110 },
  { id: 'card-user', header: '使用人', width: 85, key: 'used_by_username' },
  { id: 'card-face', header: '面值', width: 90 },
  { id: 'card-time', header: '核销时间', width: 95 },
]

const orderColumns: TableColumn[] = [
  { id: 'order-no', header: '订单号', minWidth: 110 },
  { id: 'order-user', header: '用户', width: 85, key: 'username' },
  { id: 'order-amount', header: '金额', width: 80 },
  { id: 'order-status', header: '状态', width: 80 },
]

/**
 * 列宽合计作为表格最小宽度：不设时 table 会把每列挤到几十像素（文字逐字换行），
 * 既难读也永远不会产生横向滚动。设了之后窄卡片里由表格自带的 wrapper 横向滚动。
 */
function columnsMinWidth(columns: TableColumn[]): number {
  return columns.reduce((sum, column) => sum + Number(column.width ?? column.minWidth ?? 120), 0)
}
</script>

<template>
  <CsDashboardWidget
    id="analytics-ranking"
    title="数据洞察与业务流水"
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
      <CsSegmentedControl
        :model-value="ctx.rankingPeriod.value"
        :options="rankPeriodOptions"
        label="统计粒度"
        @change="onPeriodChange"
      />
    </template>
    <div class="ranking-tab-container">
      <div class="tabs-header-wrapper">
        <CsTabs v-model="activeTab" :tabs="dashboardTabs" disable-content-paddings>
          <!-- 1. 用户流量排行 -->
          <div
            v-if="activeTab === 'users'"
            class="ranking-tab-content"
            :style="{ '--analytics-table-min-width': `${columnsMinWidth(userColumns)}px` }"
          >
        <CsTable
          compact
          :items="rankSource?.user_rank || []"
          :columns="userColumns"
          empty-text="暂无用户流量数据"
        >
          <template #cell-rank="{ index }">
            <span class="tabular-num rank-badge" :class="`rank-pos-${index + 1}`">
              {{ index + 1 }}
            </span>
          </template>
          <template #cell-user-main="{ row }">
            <span class="user-main-name">{{ row.username }}</span>
            <div class="user-sub-email tabular-num">{{ row.email || '—' }}</div>
          </template>
          <template #cell-user-plan="{ row }">
            <span class="user-plan">{{ row.plan_name }}</span>
          </template>
          <template #cell-user-usage="{ row }">
            <span class="tabular-num usage-val">
              {{ formatBytes(row.total_bytes) }}
            </span>
          </template>
        </CsTable>
      </div>

      <!-- 2. 节点流量排行 -->
      <div
        v-else-if="activeTab === 'nodes'"
        class="ranking-tab-content"
        :style="{ '--analytics-table-min-width': `${columnsMinWidth(nodeColumns)}px` }"
      >
        <CsTable
          compact
          :items="rankSource?.server_rank || []"
          :columns="nodeColumns"
          empty-text="暂无节点流量数据"
        >
          <template #cell-rank="{ index }">
            <span class="tabular-num rank-badge" :class="`rank-pos-${index + 1}`">
              {{ index + 1 }}
            </span>
          </template>
          <template #cell-node-main="{ row }">
            <span class="user-main-name">
              {{ row.name || `服务器#${row.server_id}` }}
              <span v-if="row.deleted" class="deleted-tag">已删除</span>
            </span>
            <div class="user-sub-email tabular-num">{{ row.location || '—' }}</div>
          </template>
          <template #cell-node-percent="{ row }">
            <span class="tabular-num muted-percent">{{ (row.percent || 0).toFixed(1) }}%</span>
          </template>
          <template #cell-node-usage="{ row }">
            <span class="tabular-num usage-val">
              {{ formatBytes(row.total_bytes) }}
            </span>
          </template>
        </CsTable>
      </div>

      <!-- 3. 最近礼品卡核销 -->
      <div
        v-else-if="activeTab === 'cards'"
        class="ranking-tab-content"
        :style="{ '--analytics-table-min-width': `${columnsMinWidth(cardColumns)}px` }"
      >
        <CsTable
          compact
          :items="rankSource?.recent_gift_cards || []"
          :columns="cardColumns"
          empty-text="暂无核销记录"
        >
          <template #cell-card-code="{ row }">
            <code class="aws-code-token">{{ row.code_masked }}</code>
          </template>
          <template #cell-card-face="{ row }">
            <span class="tabular-num money-gain">
              +¥ {{ (row.face_value_cents / 100).toFixed(2) }}
            </span>
          </template>
          <template #cell-card-time="{ row }">
            <span class="user-sub-email tabular-num">
              {{ String(row.used_at).replace('T', ' ').slice(5, 16) }}
            </span>
          </template>
        </CsTable>
      </div>

      <!-- 4. 最近套餐订单 -->
      <div
        v-else-if="activeTab === 'orders'"
        class="ranking-tab-content"
        :style="{ '--analytics-table-min-width': `${columnsMinWidth(orderColumns)}px` }"
      >
        <CsTable
          compact
          :items="rankSource?.recent_orders || []"
          :columns="orderColumns"
          empty-text="暂无订单"
        >
          <template #cell-order-no="{ row }">
            <code class="aws-code-token">{{ row.order_no }}</code>
          </template>
          <template #cell-order-amount="{ row }">
            <span class="tabular-num font-semibold">¥ {{ (row.amount_cents / 100).toFixed(2) }}</span>
          </template>
          <template #cell-order-status="{ row }">
            <span
              class="aws-order-status"
              :class="row.status === 'paid' ? 'status-paid' : (row.status === 'pending' ? 'status-pending' : 'status-cancel')"
            >
              {{ row.status === 'paid' ? '已生效' : (row.status === 'pending' ? '待支付' : '已取消') }}
            </span>
          </template>
        </CsTable>
      </div>
        </CsTabs>
      </div>
    </div>
  </CsDashboardWidget>
</template>

<style scoped lang="scss">
.tabs-header-wrapper {
  width: 100%;
  padding: 0 16px;
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

/* 卡片是固定高度时让页签内容撑满剩余空间：否则表格只按内容高度渲染，
   卡片下半截全是空白，纵向可读的行数也被浪费。 */
:deep(.cs-tabs-root) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

:deep(.cs-tabs-body) {
  flex: 1;
  min-height: 0;
  display: flex !important;
  flex-direction: column;
}

:deep([class*='tabs-content-wrapper']) {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

:deep([class*='tabs-content-active']) {
  flex: 1;
  min-height: 0;
  display: flex !important;
  flex-direction: column;
  overflow: hidden;
}

.ranking-tab-container {
  width: 100%;
  height: 100%;
  min-height: 0;
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  box-sizing: border-box;
  padding-bottom: 16px;
}

.ranking-tab-content {
  width: 100%;
  height: 100%;
  min-height: 0;
  min-width: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  overflow: auto;
}

/* 窄卡片里页签标签不要折成两行：交给 CsTabs 的左右翻页按钮横向滚动 */
.ranking-tab-container :deep([class*='tabs-tab-label']) {
  white-space: nowrap;
}

/* 列宽合计作为表格下限，窄卡片里由 CsTable 自带的 wrapper 横向滚动 */
.ranking-tab-content :deep(table) {
  min-inline-size: var(--analytics-table-min-width, 0);
}

.user-plan {
  font-size: 12px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  white-space: nowrap;
}

.tabular-num {
  font-variant-numeric: tabular-nums;
  font-feature-settings: 'tnum' 1;
}

.rank-badge {
  font-size: 11.5px;
  font-weight: 700;

  &.rank-pos-1 {
    color: var(--color-text-status-warning-fabq6h, #ec7211);
  }

  &.rank-pos-2 {
    color: var(--color-text-body-secondary-66ygbb, #656871);
  }

  &.rank-pos-3 {
    color: var(--color-text-status-inactive-fqjc97, #879596);
  }
}

.user-main-name {
  font-weight: 700;
  color: var(--color-text-body-default-b2us04, #0f141a);
  font-size: 12.5px;
}

.user-sub-email {
  font-size: 11px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  margin-top: 1px;
}

.usage-val {
  font-weight: 700;
  color: var(--color-text-link-default-qabnuk, #006ce0);
  font-size: 12px;
}

.muted-percent {
  color: var(--color-text-body-secondary-66ygbb, #656871);
  font-size: 11px;
}

.aws-code-token {
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
  background: var(--color-background-container-header-irktp5, #f2f3f3);
  color: var(--color-text-body-default-b2us04, #0f141a);
  padding: 1px 5px;
  border-radius: 4px;
  border: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
}

.money-gain {
  color: var(--color-text-status-success-1i2jf8, #1d8102);
  font-weight: 700;
}

.deleted-tag {
  display: inline-block;
  margin-inline-start: 6px;
  padding: 0 6px;
  border-radius: 4px;
  font-size: 10.5px;
  font-weight: 600;
  line-height: 16px;
  vertical-align: 1px;
  background: var(--color-background-container-header-irktp5, #f2f3f3);
  color: var(--color-text-status-inactive-fqjc97, #879596);
}

.aws-order-status {
  font-size: 10.5px;
  font-weight: 700;
  padding: 1px 6px;
  border-radius: 4px;

  &.status-paid {
    background: var(--color-background-status-success-1woffs, #f2f8f0);
    color: var(--color-text-status-success-1i2jf8, #1d8102);
  }

  &.status-pending {
    background: var(--color-background-status-warning-52cylj, #fff8e1);
    color: var(--color-text-status-warning-fabq6h, #b7791f);
  }

  &.status-cancel {
    background: var(--color-background-container-header-irktp5, #f2f3f3);
    color: var(--color-text-status-inactive-fqjc97, #879596);
  }
}
</style>
