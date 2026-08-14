<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Check, CreditCard, Box, Ticket, ShoppingCart, InfoFilled } from '@element-plus/icons-vue'
import { createOrder, getMyOrders, getPublicPlans } from '@/api/shop'
import { errMsg } from '@/api/http'
import type { Order, Plan } from '@/api/types'

const plans = ref<Plan[]>([])
const orders = ref<Order[]>([])
const loading = ref(false)
const buyingId = ref<number | null>(null)

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
  buyingId.value = plan.id
  try {
    const { data } = await createOrder(plan.id)
    if (data.code === 0) {
      ElMessage.success('下单成功！请按提示完成线下转账，管理员确认后自动生效')
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '下单失败'))
  } finally {
    buyingId.value = null
  }
}
</script>

<template>
  <div class="x-client-body" v-loading="loading">
    <!-- 头部横幅 -->
    <div class="shop-hero">
      <div class="shop-badge"><el-icon><ShoppingCart /></el-icon>&nbsp;订阅套餐与增值服务</div>
      <h1 class="shop-title">套餐商店</h1>
      <p class="shop-desc">高速节点无缝中转，4K 极速秒开，全平台 Clash / Mihomo 客户端即插即用。</p>
    </div>

    <!-- 定价方案网格 -->
    <div class="pricing-grid">
      <div
        v-for="(p, idx) in plans"
        :key="p.id"
        class="pricing-card"
        :class="{ featured: idx === 0 }"
      >
        <span v-if="idx === 0" class="featured-badge">🔥 热门推荐</span>

        <div class="card-top">
          <div class="plan-title">{{ p.name }}</div>
          <div class="plan-traffic-pill">{{ fmtGb(p.traffic_gb) }} 流量</div>
        </div>

        <div class="plan-price-wrap">
          <span class="currency">¥</span>
          <span class="price-val">{{ (p.price_cents / 100).toFixed(2) }}</span>
          <span class="price-period">/ {{ p.duration_days }} 天</span>
        </div>

        <div class="plan-divider" />

        <ul class="plan-features">
          <li>
            <el-icon class="check-icon"><Check /></el-icon>
            <span>包含 <b>{{ fmtGb(p.traffic_gb) }}</b> 周期高速流量</span>
          </li>
          <li>
            <el-icon class="check-icon"><Check /></el-icon>
            <span>{{ p.speed_limit_kbps ? `限速 ${(p.speed_limit_kbps / 1000).toFixed(1)} Mbps` : '不限速峰值吞吐' }}</span>
          </li>
          <li>
            <el-icon class="check-icon"><Check /></el-icon>
            <span>全量节点与中转链路授权</span>
          </li>
          <li>
            <el-icon class="check-icon"><Check /></el-icon>
            <span>深度兼容 Clash / Mihomo / Stash</span>
          </li>
        </ul>

        <div class="card-action">
          <el-button
            :type="idx === 0 ? 'primary' : 'default'"
            size="large"
            class="buy-btn"
            :class="{ 'glow-btn': idx === 0 }"
            :loading="buyingId === p.id"
            @click="buy(p)"
          >
            立即订购
          </el-button>
        </div>
      </div>

      <div v-if="!plans.length" class="empty-plan-box">
        <el-icon style="font-size: 32px; color: var(--x-text-3)"><InfoFilled /></el-icon>
        <p class="muted" style="margin-top: 8px">暂无上架套餐，请稍后再来看看</p>
      </div>
    </div>

    <!-- 支付与转账提示 -->
    <div class="x-pill-note" style="margin-top: 24px">
      <el-icon style="font-size: 18px"><CreditCard /></el-icon>
      <span>下单后请按提示完成付款/线下转账，管理员确认收款后套餐将自动充值到账并即时生效。</span>
    </div>

    <!-- 我的订单记录 -->
    <div class="x-card" style="margin-top: 20px">
      <div class="x-card-head">
        <span><el-icon><Box /></el-icon>&nbsp;我的订单记录</span>
      </div>
      <div v-if="orders.length">
        <div v-for="o in orders" :key="o.id" class="x-order-item">
          <div class="x-order-icon"><el-icon><Box /></el-icon></div>
          <div class="x-order-meta">
            <div class="x-order-name">{{ o.plan_name }}</div>
            <div class="x-order-sub">
              <code class="cell-mono">{{ o.order_no }}</code> · {{ String(o.created_at).replace('T', ' ').slice(0, 16) }}
            </div>
          </div>
          <div style="text-align: right">
            <div class="x-order-amount cell-mono">{{ fmtMoney(o.amount_cents) }}</div>
            <el-tag :type="orderStatusMap[o.status]?.type ?? 'info'" size="small" style="margin-top: 4px">
              {{ orderStatusMap[o.status]?.text ?? o.status }}
            </el-tag>
          </div>
        </div>
      </div>
      <div v-else class="muted" style="padding: 24px 18px; font-size: 13px; text-align: center">
        <el-icon><Ticket /></el-icon>&nbsp;暂无订单记录
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.shop-hero {
  margin-bottom: 24px;
  .shop-badge {
    display: inline-flex;
    align-items: center;
    padding: 3px 10px;
    background: var(--x-primary-soft);
    color: var(--x-primary);
    border-radius: 20px;
    font-size: 12px;
    font-weight: 600;
    margin-bottom: 8px;
  }
  .shop-title {
    font-size: 24px;
    font-weight: 800;
    color: var(--x-text);
    margin: 0;
  }
  .shop-desc {
    color: var(--x-text-2);
    font-size: 13.5px;
    margin-top: 6px;
  }
}

.pricing-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
  gap: 20px;
}

.pricing-card {
  background: var(--x-card);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);
  padding: 24px;
  box-shadow: var(--x-shadow);
  position: relative;
  display: flex;
  flex-direction: column;
  justify-content: space-between;
  transition: all 0.25s ease;

  &:hover {
    transform: translateY(-4px);
    box-shadow: var(--x-shadow-lg);
    border-color: rgba(99, 102, 241, 0.4);
  }

  &.featured {
    border-color: var(--x-primary);
    box-shadow: 0 8px 24px rgba(99, 102, 241, 0.15);
    background: linear-gradient(180deg, rgba(99, 102, 241, 0.03) 0%, var(--x-card) 100%);
  }

  .featured-badge {
    position: absolute;
    top: -11px;
    right: 20px;
    background: linear-gradient(135deg, #6366f1, #8b5cf6);
    color: #fff;
    font-size: 11px;
    font-weight: 700;
    padding: 3px 12px;
    border-radius: 20px;
    box-shadow: 0 4px 10px rgba(99, 102, 241, 0.4);
  }

  .card-top {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;

    .plan-title {
      font-size: 17px;
      font-weight: 700;
      color: var(--x-text);
    }
    .plan-traffic-pill {
      font-size: 11px;
      font-weight: 600;
      padding: 2px 8px;
      background: var(--x-primary-soft);
      color: var(--x-primary);
      border-radius: 12px;
    }
  }

  .plan-price-wrap {
    display: flex;
    align-items: baseline;
    gap: 4px;
    margin: 8px 0 16px;
    font-family: var(--x-font-mono);

    .currency {
      font-size: 18px;
      font-weight: 600;
      color: var(--x-text);
    }
    .price-val {
      font-size: 32px;
      font-weight: 800;
      color: var(--x-text);
    }
    .price-period {
      font-size: 13px;
      color: var(--x-text-3);
      font-family: var(--el-font-family);
    }
  }

  .plan-divider {
    height: 1px;
    background: var(--x-border);
    margin-bottom: 16px;
  }

  .plan-features {
    list-style: none;
    padding: 0;
    margin: 0 0 24px;
    display: grid;
    gap: 10px;

    li {
      display: flex;
      gap: 10px;
      align-items: center;
      font-size: 13px;
      color: var(--x-text-2);

      .check-icon {
        color: var(--x-success);
        font-size: 15px;
        flex: none;
      }
    }
  }

  .card-action {
    margin-top: auto;
    .buy-btn {
      width: 100%;
      font-weight: 700;
    }
    .glow-btn {
      box-shadow: 0 4px 14px rgba(99, 102, 241, 0.35);
    }
  }
}

.empty-plan-box {
  grid-column: 1 / -1;
  background: var(--x-card);
  border: 1px dashed var(--x-border);
  border-radius: var(--x-radius);
  padding: 40px;
  text-align: center;
}

.x-order-item {
  padding: 14px 18px;
  border-bottom: 1px solid var(--x-border);
  display: flex;
  align-items: center;
  gap: 14px;
  &:last-child {
    border-bottom: none;
  }
}

.x-order-icon {
  width: 40px;
  height: 40px;
  border-radius: 10px;
  background: var(--x-primary-soft);
  color: var(--x-primary);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  flex: none;
}

.x-order-meta {
  flex: 1;
  min-width: 0;
  .x-order-name {
    font-size: 14px;
    font-weight: 600;
    color: var(--x-text);
  }
  .x-order-sub {
    font-size: 12px;
    color: var(--x-text-3);
    margin-top: 2px;
  }
}

.x-order-amount {
  font-weight: 700;
  font-size: 15px;
  color: var(--x-text);
}

.muted {
  color: var(--x-text-3);
}
</style>