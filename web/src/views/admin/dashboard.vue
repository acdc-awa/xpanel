<script setup lang="ts">
import { computed } from 'vue'
import {
  Share,
  User,
  Document,
  Lightning,
  Bell,
  Monitor,
  Refresh,
} from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import StatCard from '@/components/base/StatCard.vue'
import { mockServers, mockOrders, mockTraffic } from '@/mock/data'
import { formatMoney } from '@/utils/format'

// 数据驱动统计卡：新增卡片 = 数组加一项，无需改布局
const stats = computed(() => [
  { icon: Share, value: '3 / 5', label: '节点在线 / 总数', tone: 'purple' as const },
  { icon: User, value: '128', label: '注册用户（本月 +12）', tone: 'blue' as const },
  { icon: Document, value: '12', label: '今日订单（待确认 3）', tone: 'orange' as const },
  { icon: Lightning, value: '1.2 TB', label: '本月总流量（↑520G / ↓700G）', tone: 'green' as const },
  { icon: Bell, value: '2', label: '待处理告警（节点离线/异常）', tone: 'orange' as const },
])

const serverStatusMap: Record<string, { type: 'success' | 'warning' | 'info'; text: string }> = {
  online: { type: 'success', text: '在线' },
  connecting: { type: 'warning', text: '重连中' },
  offline: { type: 'info', text: '离线' },
}

const orderStatusOf = (status: string) =>
  orderStatusMap[status as keyof typeof orderStatusMap] ?? orderStatusMap.cancelled

const orderStatusMap = {
  pending: { type: 'warning' as const, text: '待确认' },
  paid: { type: 'success' as const, text: '已生效' },
  cancelled: { type: 'info' as const, text: '已取消' },
}
</script>

<template>
  <div class="x-page">
    <div class="x-stat-grid">
      <StatCard v-for="s in stats" :key="s.label" :icon="s.icon" :value="s.value" :label="s.label" :tone="s.tone" />
    </div>

    <div class="dash-grid">
      <div>
        <BaseCard title="节点状态">
          <template #extra>
            <el-button size="small" text><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
          </template>
          <el-table :data="mockServers">
            <el-table-column label="节点" min-width="140">
              <template #default="{ row }"><span class="cell-strong">{{ row.name }}</span></template>
            </el-table-column>
            <el-table-column prop="location" label="地区" width="110" />
            <el-table-column prop="ip" label="IP" width="140">
              <template #default="{ row }"><code class="cell-mono">{{ row.ip }}</code></template>
            </el-table-column>
            <el-table-column label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="serverStatusMap[row.status].type" size="small">
                  <span class="x-status-dot" :class="row.status" />{{ serverStatusMap[row.status].text }}
                </el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="lastSeenAt" label="最后心跳" width="110" />
          </el-table>
        </BaseCard>

        <BaseCard title="最近订单">
          <template #extra><el-button size="small" text>查看全部</el-button></template>
          <el-table :data="mockOrders">
            <el-table-column prop="orderNo" label="订单号" width="130">
              <template #default="{ row }"><code class="cell-mono">{{ row.orderNo }}</code></template>
            </el-table-column>
            <el-table-column prop="username" label="用户" width="90" />
            <el-table-column prop="planName" label="套餐" width="110" />
            <el-table-column label="金额" width="90">
              <template #default="{ row }">{{ formatMoney(row.amount) }}</template>
            </el-table-column>
            <el-table-column label="状态" width="100">
              <template #default="{ row }">
                <el-tag :type="orderStatusOf(row.status).type" size="small">{{ orderStatusOf(row.status).text }}</el-tag>
              </template>
            </el-table-column>
            <el-table-column prop="createdAt" label="时间" />
          </el-table>
        </BaseCard>
      </div>

      <div>
        <BaseCard title="近 7 日流量">
          <div class="x-bar-chart">
            <div v-for="t in mockTraffic" :key="t.day" class="x-bar" :style="{ height: t.value + '%' }" />
          </div>
          <div class="x-chart-x"><span v-for="t in mockTraffic" :key="t.day">{{ t.day }}</span></div>
          <p class="placeholder-note">占位图表：接入 ECharts 后替换为真实趋势图</p>
        </BaseCard>

        <BaseCard title="待办">
          <ul class="todo-list">
            <li>· 3 笔订单待确认收款</li>
            <li>· 1 个节点离线超过 1 小时</li>
            <li>· 2 个订阅 token 将于 3 天后过期</li>
          </ul>
        </BaseCard>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.dash-grid { display: grid; grid-template-columns: 2fr 1fr; gap: 20px; align-items: start; }
.cell-strong { font-weight: 600; }
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-text-2); }
.placeholder-note { color: var(--x-text-3); font-size: 12px; margin-top: 12px; }
.todo-list { list-style: none; display: grid; gap: 8px; color: var(--x-text-2); font-size: 13px; }
@media (max-width: 900px) {
  .dash-grid { grid-template-columns: 1fr; }
}
</style>

