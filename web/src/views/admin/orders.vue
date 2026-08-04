<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Refresh, Check, Close } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import { cancelOrder, confirmOrder, getOrders, type Order } from '@/api/admin'
import { errMsg } from '@/api/http'

const list = ref<Order[]>([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const size = ref(20)
const statusFilter = ref('')

const statusMap: Record<string, { type: 'warning' | 'success' | 'info'; text: string }> = {
  pending: { type: 'warning', text: '待确认' },
  paid: { type: 'success', text: '已生效' },
  cancelled: { type: 'info', text: '已取消' },
}

async function load() {
  loading.value = true
  try {
    const { data } = await getOrders(page.value, size.value, statusFilter.value || undefined)
    if (data.code === 0) {
      list.value = data.data.items
      total.value = data.data.total
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载订单失败'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

function fmtTime(t: string | null) {
  if (!t) return '—'
  return t.replace('T', ' ').slice(0, 16)
}

async function confirm(row: any) {
  try {
    await ElMessageBox.confirm(
      `确认已收到「${row.username}」的订单 ${row.order_no} 付款？确认后套餐自动生效。`,
      '确认收款',
      { type: 'warning' },
    )
  } catch {
    return
  }
  try {
    const { data } = await confirmOrder(row.id)
    if (data.code === 0) {
      ElMessage.success('已确认，套餐已生效')
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '操作失败'))
  }
}

async function cancel(row: any) {
  try {
    await ElMessageBox.confirm(`确认取消订单 ${row.order_no}？`, '取消订单', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await cancelOrder(row.id)
    if (data.code === 0) {
      ElMessage.success('已取消')
      load()
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '操作失败'))
  }
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-select v-model="statusFilter" placeholder="全部状态" clearable style="width: 140px" @change="page = 1; load()">
          <el-option label="待确认" value="pending" />
          <el-option label="已生效" value="paid" />
          <el-option label="已取消" value="cancelled" />
        </el-select>
        <el-button @click="load"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
      </div>
    </div>

    <BaseCard>
      <el-table v-loading="loading" :data="list">
        <el-table-column prop="order_no" label="订单号" min-width="150">
          <template #default="{ row }"><code class="cell-mono">{{ row.order_no }}</code></template>
        </el-table-column>
        <el-table-column prop="username" label="用户" width="100" />
        <el-table-column prop="plan_name" label="套餐" min-width="110" />
        <el-table-column label="金额" width="90">
          <template #default="{ row }">¥ {{ (row.amount_cents / 100).toFixed(2) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="statusMap[row.status]?.type ?? 'info'" size="small">{{ statusMap[row.status]?.text ?? row.status }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" width="160">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="170" fixed="right">
          <template #default="{ row }">
            <template v-if="row.status === 'pending'">
              <el-button size="small" type="primary" plain @click="confirm(row)"><el-icon><Check /></el-icon>&nbsp;确认收款</el-button>
              <el-button size="small" text type="danger" @click="cancel(row)"><el-icon><Close /></el-icon></el-button>
            </template>
            <span v-else class="muted">—</span>
          </template>
        </el-table-column>
        <template #empty>
          <div style="padding: 30px 0; color: var(--x-text-3)">暂无订单（用户下单后在此确认收款）</div>
        </template>
      </el-table>
      <div class="x-pager">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="size"
          :total="total"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          @current-change="load"
          @size-change="load"
        />
      </div>
    </BaseCard>
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-text-2); }
.muted { color: var(--x-text-3); }
.x-pager { display: flex; justify-content: flex-end; padding: 14px 0 4px; }
</style>