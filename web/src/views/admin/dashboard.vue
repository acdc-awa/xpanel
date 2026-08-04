<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  Share,
  User,
  Document,
  Box,
  Bell,
  Refresh,
} from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import StatCard from '@/components/base/StatCard.vue'
import { getDashboard } from '@/api/admin'
import { errMsg } from '@/api/http'
import type { AdminDashboard } from '@/api/types'
import { mockServers, mockOrders, mockTraffic } from '@/mock/data'
import { formatMoney } from '@/utils/format'

// 统计卡：数据来自 /admin/dashboard（真实计数），loading 时显示 —
const dash = ref<AdminDashboard | null>(null)
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const { data } = await getDashboard()
    if (data.code === 0) dash.value = data.data
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载仪表盘失败'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

const stats = computed(() => [
  { icon: Share, value: dash.value ? `${dash.value.online_servers}` : '—', label: '在线节点（P1 接入心跳）', tone: 'purple' as const },
  { icon: User, value: dash.value ? `${dash.value.total_users}` : '—', label: '注册用户', tone: 'blue' as const },
  { icon: Document, value: dash.value ? `${dash.value.total_orders}` : '—', label: '累计订单', tone: 'orange' as const },
  { icon: Box, value: dash.value ? `${dash.value.total_plans}` : '—', label: '上架套餐', tone: 'green' as const },
  { icon: Bell, value: dash.value ? `${dash.value.pending_orders}` : '—', label: '待确认订单', tone: 'orange' as const },
])

const serverStatusMap: Record<string, { type: 'success' | 'warning' | 'info'; text: string }> = {
  online: { type: 'success', text: '在线' },
  connecting: { type: 'warning', text: '重连中' },
  offline: { type: 'info', text: '离线' },
}

const orderStatusMap = {
  pending: { type: 'warning' as const, text: '待确认' },
  paid: { type: 'success' as const, text: '已生效' },
  cancelled: { type: 'info' as const, text: '已取消' },
}
const orderStatusOf = (status: string) =>
  orderStatusMap[status as keyof typeof orderStatusMap] ?? orderStatusMap.cancelled
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
            <span class="demo-tag">演示数据</span>
            <el-button size="small" text :loading="loading" @click="load"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
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
          <template #extra><span class="demo-tag">演示数据</span></template>
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
          <template #extra><span class="demo-tag">演示数据</span></template>
          <div class="x-bar-chart">
            <div v-for="t in mockTraffic" :key="t.day" class="x-bar" :style="{ height: t.value + '%' }" />
          </div>
          <div class="x-chart-x"><span v-for="t in mockTraffic" :key="t.day">{{ t.day }}</span></div>
          <p class="placeholder-note">占位图表：P2 接入流量统计后替换为真实趋势图</p>
        </BaseCard>

        <BaseCard title="待办">
          <ul class="todo-list">
            <li>· {{ dash?.pending_orders ?? 0 }} 笔订单待确认收款（P4 上线确认流程）</li>
            <li>· 节点在线状态待 P1 心跳接入</li>
            <li>· 套餐/订阅管理待 P5 管理端完善</li>
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
.demo-tag {
  font-size: 11px;
  color: var(--x-warning);
  background: rgba(245, 158, 11, 0.12);
  border: 1px solid rgba(245, 158, 11, 0.35);
  border-radius: 6px;
  padding: 1px 7px;
  margin-right: 8px;
  vertical-align: middle;
}
.placeholder-note { color: var(--x-text-3); font-size: 12px; margin-top: 12px; }
.todo-list { list-style: none; display: grid; gap: 8px; color: var(--x-text-2); font-size: 13px; }
@media (max-width: 900px) {
  .dash-grid { grid-template-columns: 1fr; }
}
</style>