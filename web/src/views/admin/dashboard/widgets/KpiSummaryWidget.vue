<script setup lang="ts">
import { computed, inject } from 'vue'
import { CsDashboardWidget } from '@/components/cloudscape'
import { formatBytes } from '@/utils/format'
import type { DashboardContext } from '../types'
import type { WidgetLayoutItem } from '@/components/cloudscape'

defineProps<{
  layoutItem: WidgetLayoutItem
}>()

const ctx = inject<DashboardContext>('adminDashboardContext')!
const dashData = ctx.dashData

const onlineServersCount = computed(() => dashData.value?.summary.online_servers || 0)
const totalServersCount = computed(() => dashData.value?.summary.total_servers || 0)
const onlineServersRatio = computed(() => {
  if (!totalServersCount.value) return 100
  return Math.round((onlineServersCount.value / totalServersCount.value) * 100)
})
const isClusterHealthy = computed(() => {
  if (!totalServersCount.value) return true
  return onlineServersCount.value === totalServersCount.value
})

function formatBandwidth(bytesPerSec: number): string {
  if (!bytesPerSec || bytesPerSec <= 0) return '0.00 Mbps'
  const mbps = (bytesPerSec * 8) / 1_000_000
  if (mbps >= 1000) return `${(mbps / 1000).toFixed(2)} Gbps`
  return `${mbps.toFixed(2)} Mbps`
}

function formatCompactRate(bytesPerSec: number): string {
  if (!bytesPerSec || bytesPerSec <= 0) return '0.00 M'
  const mbps = (bytesPerSec * 8) / 1_000_000
  if (mbps >= 1000) return `${(mbps / 1000).toFixed(1)} G`
  return `${mbps.toFixed(2)} M`
}
</script>

<template>
  <CsDashboardWidget
    id="kpi-summary"
    title="核心业务与运行指标"
    badge="实时采样"
    :x="layoutItem.x"
    :y="layoutItem.y"
    :w="layoutItem.w"
    :h="layoutItem.h"
    :min-w="layoutItem.minW"
    :min-h="layoutItem.minH"
    @refresh="ctx.refreshDashboard"
  >
    <div class="kpi-grid">
      <div class="kpi-tile">
        <div class="kpi-label">今日营收</div>
        <div class="kpi-value"><span class="kpi-unit">¥</span>{{ ((dashData?.summary.today_revenue_cents || 0) / 100).toFixed(2) }}</div>
        <div class="kpi-sub">已核销 {{ dashData?.summary.today_used_cards_count || 0 }} 张礼品卡</div>
      </div>

      <div class="kpi-tile">
        <div class="kpi-label">本月营收</div>
        <div class="kpi-value"><span class="kpi-unit">¥</span>{{ ((dashData?.summary.month_revenue_cents || 0) / 100).toFixed(2) }}</div>
        <div class="kpi-sub">累计 ¥ {{ ((dashData?.summary.total_revenue_cents || 0) / 100).toFixed(2) }}</div>
      </div>

      <div class="kpi-tile">
        <div class="kpi-label">今日流量</div>
        <div class="kpi-value">{{ formatBytes(dashData?.summary.today_traffic_total || 0) }}</div>
        <div class="kpi-sub">
          上行 {{ formatBytes(dashData?.summary.today_traffic_up || 0) }} · 下行
          {{ formatBytes(dashData?.summary.today_traffic_down || 0) }}
        </div>
      </div>

      <div class="kpi-tile">
        <div class="kpi-label">实时网络吞吐</div>
        <div class="kpi-value">
          {{ formatBandwidth((dashData?.summary.realtime_rx_rate || 0) + (dashData?.summary.realtime_tx_rate || 0)) }}
        </div>
        <div class="kpi-sub">
          下行 {{ formatCompactRate(dashData?.summary.realtime_rx_rate || 0) }} · 上行
          {{ formatCompactRate(dashData?.summary.realtime_tx_rate || 0) }}
        </div>
      </div>

      <div class="kpi-tile">
        <div class="kpi-label">服务器在线</div>
        <div class="kpi-value">{{ onlineServersCount }}<span class="kpi-unit"> / {{ totalServersCount }}</span></div>
        <div class="kpi-progress">
          <div class="kpi-progress-track">
            <div class="kpi-progress-fill" :class="{ 'is-warning': !isClusterHealthy }" :style="{ width: `${onlineServersRatio}%` }"></div>
          </div>
          <span class="kpi-sub">{{ onlineServersRatio }}%</span>
        </div>
      </div>

      <div class="kpi-tile">
        <div class="kpi-label">用户与订单</div>
        <div class="kpi-value">{{ dashData?.summary.active_users || 0 }}<span class="kpi-unit"> / {{ dashData?.summary.total_users || 0 }}</span></div>
        <div class="kpi-sub">今日 {{ dashData?.summary.today_orders || 0 }} 单 · 累计 {{ dashData?.summary.total_orders || 0 }} 单</div>
      </div>
    </div>
  </CsDashboardWidget>
</template>

<style scoped lang="scss">
.kpi-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(168px, 1fr));
  gap: 12px;
  height: 100%;
  width: 100%;
  align-content: start;
  box-sizing: border-box;
}

.kpi-tile {
  background: var(--color-background-container-content-mjfil9, #ffffff);
  border: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
  border-radius: var(--border-radius-container-33hhdh, 16px);
  padding: 12px 16px;
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  color: var(--color-text-body-default-b2us04, #0f141a);
}

.kpi-label {
  font-size: var(--font-size-body-s-89y7qg, 12px);
  line-height: var(--line-height-body-s-gako33, 16px);
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.kpi-value {
  font-size: var(--font-size-heading-l-cax9xk, 20px);
  line-height: var(--line-height-heading-l-zmt48b, 24px);
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.kpi-unit {
  font-size: var(--font-size-body-m-tq5a14, 14px);
  font-weight: 400;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.kpi-sub {
  font-size: 11px;
  line-height: 15px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.kpi-progress {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-top: 2px;
}

.kpi-progress-track {
  flex: 1;
  height: 6px;
  border-radius: 3px;
  background: var(--color-background-control-disabled-m61b8s, #dedee3);
  overflow: hidden;
}

.kpi-progress-fill {
  height: 100%;
  border-radius: 3px;
  background: var(--color-text-status-success-1i2jf8, #1d8102);

  &.is-warning {
    background: var(--color-text-status-warning-fabq6h, #b7791f);
  }
}
</style>
