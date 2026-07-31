<script setup lang="ts">
import { Check, CreditCard, Box } from '@element-plus/icons-vue'
import { mockPlans, mockOrders } from '@/mock/data'
import { formatMoney } from '@/utils/format'

const orderStatusMap = {
  pending: { type: 'warning' as const, text: '待确认' },
  paid: { type: 'success' as const, text: '已生效' },
  cancelled: { type: 'info' as const, text: '已取消' },
}
</script>

<template>
  <div class="x-client-body">
    <!-- 套餐（移动单列 / 桌面三列） -->
    <div class="x-plan-grid">
      <div v-for="(p, idx) in mockPlans.filter((x) => x.enabled)" :key="p.id" class="x-plan-card" :class="{ featured: idx === 0 }">
        <span v-if="idx === 0" class="x-plan-tag">推荐</span>
        <div style="font-size: 15px; font-weight: 600">{{ p.name }}</div>
        <div class="x-plan-price">{{ formatMoney(p.price) }}<small> / {{ p.durationDays }} 天</small></div>
        <ul class="x-plan-feats">
          <li><el-icon color="#10b981"><Check /></el-icon>{{ p.trafficGb >= 1024 ? `${(p.trafficGb / 1024).toFixed(0)} TB` : `${p.trafficGb} GB` }} 流量 / {{ p.durationDays }} 天</li>
          <li><el-icon color="#10b981"><Check /></el-icon>{{ p.speedLimit ?? '不限速' }} · 全节点可用</li>
          <li><el-icon color="#10b981"><Check /></el-icon>支持 Clash 系客户端</li>
        </ul>
        <el-button :type="idx === 0 ? 'primary' : 'default'" style="width: 100%">立即购买</el-button>
      </div>
    </div>

    <!-- 下单流程提示 -->
    <div class="x-pill-note" style="margin-top: 16px">
      <el-icon><CreditCard /></el-icon>
      <span>下单后请按提示完成线下转账，管理员确认收款后套餐自动生效。</span>
    </div>

    <!-- 我的订单 -->
    <div class="x-card" style="margin-top: 16px">
      <div class="x-card-head"><span>我的订单</span><a class="muted" style="font-size: 12px" href="#">全部</a></div>
      <div v-for="o in mockOrders" :key="o.id" class="x-order-item">
        <div class="x-order-icon"><el-icon><Box /></el-icon></div>
        <div class="x-order-meta">
          <div class="x-order-name">{{ o.planName }}</div>
          <div class="x-order-sub">{{ o.orderNo }} · {{ o.createdAt }}</div>
        </div>
        <div style="text-align: right">
          <div class="x-order-amount">{{ formatMoney(o.amount) }}</div>
          <el-tag :type="orderStatusMap[o.status].type" size="small" style="margin-top: 4px">{{ orderStatusMap[o.status].text }}</el-tag>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.muted { color: var(--x-text-3); }
</style>
