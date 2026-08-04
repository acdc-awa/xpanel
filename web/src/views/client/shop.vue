<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Check, CreditCard, Box, Ticket } from '@element-plus/icons-vue'
import { createOrder, getMyOrders, getPublicPlans } from '@/api/shop'
import { errMsg } from '@/api/http'
import type { Order, Plan } from '@/api/types'

const plans = ref<Plan[]>([])
const orders = ref<Order[]>([])
const loading = ref(false)
const buying = ref(false)

const orderStatusMap: Record<string, { type: 'warning' | 'success' | 'info'; text: string }> = {
  pending: { type: 'warning', text: '待确认' },
  paid: { type: 'success', text: '已生效' },
  cancelled: { type: 'info', text: '已取消' },
}

function fmtMoney(cents: number) {
  return `¥ ${(cents / 100).toFixed(2)}`
}

function fmtGb(gb: number) {
  return gb >= 1024 ? `${(gb / 1024).toFixed(1)} TB` : `${gb} GB`
}

async function load() {
  loading.value = true
  try {
    const [p, o] = await Promise.all([getPublicPlans(), getMyOrders()])
    if (p.data.code === 0) plans.value = p.data.data.items
    if (o.data.code === 0) orders.value = o.data.data.items
  } catch (e) {
    ElMessage.error(errMsg(e, '加载失败'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

async function buy(plan: Plan) {
  buying.value = true
  try {
    const { data } = await createOrder(plan.id)
    if (data.code === 0) {
      ElMessage.success('下单成功，请按提示完成线下转账，等待管理员确认')
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '下单失败'))
  } finally {
    buying.value = false
  }
}
</script>

<template>
  <div class="x-client-body" v-loading="loading">
    <!-- 套餐 -->
    <div class="x-plan-grid">
      <div v-for="(p, idx) in plans" :key="p.id" class="x-plan-card" :class="{ featured: idx === 0 }">
        <span v-if="idx === 0" class="x-plan-tag">推荐</span>
        <div style="font-size: 15px; font-weight: 600">{{ p.name }}</div>
        <div class="x-plan-price">{{ fmtMoney(p.price_cents) }}<small> / {{ p.duration_days }} 天</small></div>
        <ul class="x-plan-feats">
          <li><el-icon color="#10b981"><Check /></el-icon>{{ fmtGb(p.traffic_gb) }} 流量 / {{ p.duration_days }} 天</li>
          <li><el-icon color="#10b981"><Check /></el-icon>{{ p.speed_limit_kbps ? `${(p.speed_limit_kbps / 1000).toFixed(1)} Mbps` : '不限速' }} · 全节点可用</li>
          <li><el-icon color="#10b981"><Check /></el-icon>支持 Clash 系客户端</li>
        </ul>
        <el-button :type="idx === 0 ? 'primary' : 'default'" style="width: 100%" :loading="buying" @click="buy(p)">
          立即购买
        </el-button>
      </div>
      <div v-if="!plans.length" class="x-plan-card" style="grid-column: 1 / -1">
        <div class="muted" style="text-align: center; padding: 20px 0">暂无上架套餐</div>
      </div>
    </div>

    <!-- 下单流程提示 -->
    <div class="x-pill-note" style="margin-top: 16px">
      <el-icon><CreditCard /></el-icon>
      <span>下单后请按提示完成线下转账，管理员确认收款后套餐自动生效。</span>
    </div>

    <!-- 我的订单 -->
    <div class="x-card" style="margin-top: 16px">
      <div class="x-card-head"><span>我的订单</span></div>
      <div v-if="orders.length">
        <div v-for="o in orders" :key="o.id" class="x-order-item">
          <div class="x-order-icon"><el-icon><Box /></el-icon></div>
          <div class="x-order-meta">
            <div class="x-order-name">{{ o.plan_name }}</div>
            <div class="x-order-sub">{{ o.order_no }} · {{ String(o.created_at).replace('T', ' ').slice(0, 16) }}</div>
          </div>
          <div style="text-align: right">
            <div class="x-order-amount">{{ fmtMoney(o.amount_cents) }}</div>
            <el-tag :type="orderStatusMap[o.status]?.type ?? 'info'" size="small" style="margin-top: 4px">
              {{ orderStatusMap[o.status]?.text ?? o.status }}
            </el-tag>
          </div>
        </div>
      </div>
      <div v-else class="muted" style="padding: 20px 16px; font-size: 13px">
        <el-icon><Ticket /></el-icon>&nbsp;暂无订单，选择上方套餐购买
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.muted { color: var(--x-text-3); }
</style>