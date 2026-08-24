<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Refresh, Tickets } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import { getOrders, type Order } from '@/api/admin'
import { errMsg } from '@/api/http'

const list = ref<Order[]>([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const size = ref(20)

async function load() {
  loading.value = true
  try {
    const { data } = await getOrders(page.value, size.value)
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
  return String(t).replace('T', ' ').slice(0, 16)
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <span class="x-toolbar-hint">余额直付记录（充值=兑换码，购买=余额）</span>
        <el-button @click="load"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
      </div>
    </div>

    <BaseCard>
      <!-- 桌面端表格视图 -->
      <div class="desktop-table-view">
        <el-table v-loading="loading" :data="list">
          <el-table-column prop="order_no" label="订单号" min-width="190">
            <template #default="{ row }"><code class="cell-mono">{{ row.order_no }}</code></template>
          </el-table-column>
          <el-table-column prop="username" label="用户" width="110">
            <template #default="{ row }"><span style="font-weight: 600">{{ row.username }}</span></template>
          </el-table-column>
          <el-table-column prop="plan_name" label="套餐名称" min-width="130" />
          <el-table-column label="支付金额" width="110">
            <template #default="{ row }">
              <span class="cell-mono" style="font-weight: 700; color: var(--x-text)">
                ¥ {{ (row.amount_cents / 100).toFixed(2) }}
              </span>
            </template>
          </el-table-column>
          <el-table-column label="支付方式" width="110">
            <template #default="{ row }">
              <span v-if="row.payment_method === 'balance'" class="x-chip green">
                余额直付
              </span>
              <span v-else class="x-chip blue">
                {{ row.payment_method }}
              </span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="100">
            <template #default>
              <span class="x-chip green">已生效</span>
            </template>
          </el-table-column>
          <el-table-column label="支付时间" width="160">
            <template #default="{ row }"><span class="cell-mono muted" style="font-size: 12px">{{ fmtTime(row.paid_at) }}</span></template>
          </el-table-column>
          <el-table-column label="下单时间" width="160">
            <template #default="{ row }"><span class="cell-mono muted" style="font-size: 12px">{{ fmtTime(row.created_at) }}</span></template>
          </el-table-column>
          <template #empty>
            <div style="padding: 30px 0; color: var(--x-text-3)">
              <el-icon style="font-size: 32px"><Tickets /></el-icon>
              <p style="margin-top: 8px">暂无订单记录</p>
            </div>
          </template>
        </el-table>
      </div>

      <!-- 移动端卡片流视图 -->
      <div class="mobile-cards-view">
        <div v-if="list.length === 0" style="text-align: center; padding: 36px 0; color: var(--x-text-3); font-size: 13.5px">
          暂无订单记录
        </div>
        <div v-else class="mobile-data-card-list">
          <div v-for="row in list" :key="row.id" class="mobile-data-card">
            <div class="card-head">
              <div class="head-title">
                <span style="font-weight: 700">{{ row.plan_name }}</span>
                <el-tag type="success" size="small" effect="light">已生效</el-tag>
              </div>
              <span class="cell-mono" style="font-weight: 700; color: #059669; font-size: 14px">
                ¥ {{ (row.amount_cents / 100).toFixed(2) }}
              </span>
            </div>

            <div class="card-grid">
              <div class="grid-item">
                <span class="item-label">购买用户</span>
                <div class="item-value" style="font-weight: 600">{{ row.username }}</div>
              </div>
              <div class="grid-item">
                <span class="item-label">支付方式</span>
                <div class="item-value">
                  <el-tag v-if="row.payment_method === 'balance'" type="success" size="small" effect="plain">
                    余额直付
                  </el-tag>
                  <span v-else>{{ row.payment_method }}</span>
                </div>
              </div>
              <div class="grid-item full-width">
                <span class="item-label">订单号</span>
                <div class="item-value"><code class="cell-mono" style="font-size: 11.5px">{{ row.order_no }}</code></div>
              </div>
              <div class="grid-item full-width">
                <span class="item-label">支付时间</span>
                <div class="item-value cell-mono muted" style="font-size: 11.5px">{{ fmtTime(row.paid_at) }}</div>
              </div>
            </div>
          </div>
        </div>
      </div>

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
.cell-mono { font-family: var(--x-font-mono); font-size: 12.5px; color: var(--x-text-2); }
.muted { color: var(--x-text-3); }
.x-pager { display: flex; justify-content: flex-end; padding: 14px 0 4px; }
.x-toolbar-hint { font-size: 13px; color: var(--x-text-3); margin-right: 8px; }
</style>