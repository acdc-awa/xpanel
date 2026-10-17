<script setup lang="ts">
import { computed, inject, ref } from 'vue'
import {
  CsDashboardWidget,
  CsTabs,
} from '@/components/cloudscape'
import { formatBytes } from '@/utils/format'
import type { DashboardContext } from '../types'
import type { WidgetLayoutItem } from '@/components/cloudscape'

defineProps<{
  layoutItem: WidgetLayoutItem
}>()

const ctx = inject<DashboardContext>('adminDashboardContext')!
const dashData = ctx.dashData

const activeTab = ref('users')

const RANK_PERIOD_LABELS: Record<string, string> = { today: '今日', '7d': '近 7 天', '30d': '近 30 天', month: '近 30 天' }
const rankLabel = computed(() => dashData.value?.rank_label || RANK_PERIOD_LABELS[ctx.rankPeriod.value] || '今日')

const dashboardTabs = computed(() => [
  { id: 'users', label: `用户排行 · ${rankLabel.value}` },
  { id: 'nodes', label: `节点排行 · ${rankLabel.value}` },
  { id: 'cards', label: '卡密核销' },
  { id: 'orders', label: '订单流水' },
])
</script>

<template>
  <CsDashboardWidget
    id="analytics-ranking"
    no-padding
    :x="layoutItem.x"
    :y="layoutItem.y"
    :w="layoutItem.w"
    :h="layoutItem.h"
    :min-w="layoutItem.minW"
    :min-h="layoutItem.minH"
    @refresh="ctx.refreshDashboard"
  >
    <div class="ranking-tab-container">
      <div class="tabs-header-wrapper">
        <CsTabs v-model="activeTab" :tabs="dashboardTabs" disable-content-paddings>
          <!-- 1. 用户流量排行 -->
          <div v-if="activeTab === 'users'" class="ranking-tab-content">
        <el-table
          :data="dashData?.user_rank || []"
          size="small"
          class="aws-cloudscape-table"
          style="width: 100%"
        >
          <el-table-column label="#" width="40">
            <template #default="{ $index }">
              <span class="tabular-num rank-badge" :class="`rank-pos-${$index + 1}`">
                {{ $index + 1 }}
              </span>
            </template>
          </el-table-column>
          <el-table-column label="用户" min-width="110">
            <template #default="{ row }">
              <span class="user-main-name">{{ row.username }}</span>
              <div class="user-sub-email tabular-num">{{ row.email || '—' }}</div>
            </template>
          </el-table-column>
          <el-table-column prop="plan_name" label="套餐" width="90">
            <template #default="{ row }">
              <span class="user-plan">{{ row.plan_name }}</span>
            </template>
          </el-table-column>
          <el-table-column :label="`${rankLabel}用量`" width="105">
            <template #default="{ row }">
              <span class="tabular-num usage-val">
                {{ formatBytes(row.total_bytes) }}
              </span>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- 2. 节点流量排行 -->
      <div v-else-if="activeTab === 'nodes'" class="ranking-tab-content">
        <el-table
          :data="dashData?.server_rank || []"
          size="small"
          class="aws-cloudscape-table"
          style="width: 100%"
        >
          <el-table-column label="#" width="40">
            <template #default="{ $index }">
              <span class="tabular-num rank-badge" :class="`rank-pos-${$index + 1}`">
                {{ $index + 1 }}
              </span>
            </template>
          </el-table-column>
          <el-table-column label="节点" min-width="110">
            <template #default="{ row }">
              <span class="user-main-name">{{ row.name || `服务器#${row.server_id}` }}</span>
              <div class="user-sub-email tabular-num">{{ row.location || '—' }}</div>
            </template>
          </el-table-column>
          <el-table-column label="占比" width="70">
            <template #default="{ row }">
              <span class="tabular-num muted-percent">{{ (row.percent || 0).toFixed(1) }}%</span>
            </template>
          </el-table-column>
          <el-table-column :label="`${rankLabel}用量`" width="105">
            <template #default="{ row }">
              <span class="tabular-num usage-val">
                {{ formatBytes(row.total_bytes) }}
              </span>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- 3. 最近礼品卡核销 -->
      <div v-else-if="activeTab === 'cards'" class="ranking-tab-content">
        <el-table
          :data="dashData?.recent_gift_cards || []"
          size="small"
          class="aws-cloudscape-table"
          style="width: 100%"
        >
          <el-table-column label="卡密" min-width="110">
            <template #default="{ row }">
              <code class="aws-code-token">{{ row.code_masked }}</code>
            </template>
          </el-table-column>
          <el-table-column prop="used_by_username" label="使用人" width="85" />
          <el-table-column label="面值" width="90">
            <template #default="{ row }">
              <span class="tabular-num money-gain">
                +¥ {{ (row.face_value_cents / 100).toFixed(2) }}
              </span>
            </template>
          </el-table-column>
          <el-table-column label="核销时间" width="95">
            <template #default="{ row }">
              <span class="user-sub-email tabular-num">
                {{ String(row.used_at).replace('T', ' ').slice(5, 16) }}
              </span>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- 4. 最近套餐订单 -->
      <div v-else-if="activeTab === 'orders'" class="ranking-tab-content">
        <el-table
          :data="dashData?.recent_orders || []"
          size="small"
          class="aws-cloudscape-table"
          style="width: 100%"
        >
          <el-table-column label="订单号" min-width="110">
            <template #default="{ row }">
              <code class="aws-code-token">{{ row.order_no }}</code>
            </template>
          </el-table-column>
          <el-table-column prop="username" label="用户" width="85" />
          <el-table-column label="金额" width="80">
            <template #default="{ row }">
              <span class="tabular-num font-semibold">¥ {{ (row.amount_cents / 100).toFixed(2) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="80">
            <template #default="{ row }">
              <span
                class="aws-order-status"
                :class="row.status === 'paid' ? 'status-paid' : (row.status === 'pending' ? 'status-pending' : 'status-cancel')"
              >
                {{ row.status === 'paid' ? '已生效' : (row.status === 'pending' ? '待支付' : '已取消') }}
              </span>
            </template>
          </el-table-column>
        </el-table>
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
