<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  Check,
  Box,
  Ticket,
  ShoppingCart,
  InfoFilled,
  Wallet,
} from '@element-plus/icons-vue'
import { payOrderByBalance, getMyOrders, getPublicPlans } from '@/api/shop'
import { redeemGiftCard } from '@/api/gift_card'
import { useAuthStore } from '@/stores/auth'
import { errMsg } from '@/api/http'
import type { Order, Plan } from '@/api/types'

const auth = useAuthStore()

const plans = ref<Plan[]>([])
const orders = ref<Order[]>([])
const loading = ref(false)

// 结账收银台
const checkoutOpen = ref(false)
const selectedPlan = ref<Plan | null>(null)
const paying = ref(false)

// 收银台内快捷兑换卡密
const quickRedeemCode = ref('')
const quickRedeeming = ref(false)

const userBalanceCents = computed(() => auth.user?.balance_cents ?? 0)
const balanceYuan = computed(() => (userBalanceCents.value / 100).toFixed(2))

const isBalanceSufficient = computed(() => {
  if (!selectedPlan.value) return false
  return userBalanceCents.value >= selectedPlan.value.price_cents
})

const balanceDeficitYuan = computed(() => {
  if (!selectedPlan.value) return '0.00'
  const diff = selectedPlan.value.price_cents - userBalanceCents.value
  return diff > 0 ? (diff / 100).toFixed(2) : '0.00'
})

const orderStatusMap: Record<string, { type: 'warning' | 'success' | 'info'; text: string }> = {
  pending: { type: 'warning', text: '待处理' },
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
    const [p, o] = await Promise.all([getPublicPlans(), getMyOrders(), auth.fetchMe()])
    if (p.data.code === 0) plans.value = p.data.data.items
    if (o.data.code === 0) orders.value = o.data.data.items
  } catch (e) {
    ElMessage.error(errMsg(e, '加载失败'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

function openCheckout(plan: Plan) {
  selectedPlan.value = plan
  quickRedeemCode.value = ''
  checkoutOpen.value = true
}

// 快捷卡密充值
async function handleQuickRedeem() {
  const code = quickRedeemCode.value.trim()
  if (!code) {
    ElMessage.warning('请输入充值卡密')
    return
  }
  quickRedeeming.value = true
  try {
    const { data } = await redeemGiftCard(code)
    if (data.code === 0) {
      ElMessage.success(`充值成功！已到账 ¥ ${(data.data.face_value_cents / 100).toFixed(2)}`)
      quickRedeemCode.value = ''
      await auth.fetchMe()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '兑换失败'))
  } finally {
    quickRedeeming.value = false
  }
}

// 确认结账支付
async function confirmPay() {
  if (!selectedPlan.value) return
  paying.value = true
  try {
    const { data } = await payOrderByBalance(selectedPlan.value.id)
    if (data.code === 0) {
      ElMessage.success('🎉 购买成功！套餐已即时开通生效')
      checkoutOpen.value = false
      await Promise.all([load(), auth.fetchMe()])
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '支付失败'))
  } finally {
    paying.value = false
  }
}
</script>

<template>
  <div class="x-client-body" v-loading="loading">
    <!-- 头部横幅与用户余额卡 -->
    <div class="shop-header-row">
      <div class="shop-hero">
        <div class="shop-badge"><el-icon><ShoppingCart /></el-icon>&nbsp;订阅套餐与增值服务</div>
        <h1 class="shop-title">套餐商店</h1>
        <p class="shop-desc">高速节点无缝中转，4K 极速秒开，全平台 Clash / Mihomo 客户端即插即用。</p>
      </div>

      <div class="shop-wallet-badge">
        <div class="wallet-icon"><el-icon><Wallet /></el-icon></div>
        <div class="wallet-text">
          <span class="wallet-lbl">账户可用余额</span>
          <span class="wallet-val cell-mono">¥ {{ balanceYuan }}</span>
        </div>
        <router-link to="/account">
          <el-button size="small" type="primary" plain style="margin-left: 8px">充值</el-button>
        </router-link>
      </div>
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
            <span>{{ p.device_limit ? `同时在线上限 ${p.device_limit} 台设备` : '不限制同时在线设备数' }}</span>
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
            @click="openCheckout(p)"
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

    <!-- 我的订单记录 -->
    <div class="x-card" style="margin-top: 24px">
      <div class="x-card-head">
        <span><el-icon><Box /></el-icon>&nbsp;我的订购记录</span>
      </div>
      <div v-if="orders.length">
        <div v-for="o in orders" :key="o.id" class="x-order-item">
          <div class="x-order-icon"><el-icon><Box /></el-icon></div>
          <div class="x-order-meta">
            <div class="x-order-name">{{ o.plan_name }}</div>
            <div class="x-order-sub">
              <code class="cell-mono">{{ o.order_no }}</code> · {{ String(o.created_at).replace('T', ' ').slice(0, 16) }}
              <el-tag size="small" type="success" effect="plain" style="margin-left: 6px">
                余额直付
              </el-tag>
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

    <!-- 结账收银台弹窗 -->
    <el-dialog
      v-model="checkoutOpen"
      title="🛒 结算收银台"
      width="420px"
      align-center
      :append-to-body="true"
    >
      <div v-if="selectedPlan" class="checkout-body">
        <!-- 选购商品概要 -->
        <div class="checkout-plan-card">
          <div>
            <div class="cp-title">{{ selectedPlan.name }}</div>
            <div class="cp-sub">{{ fmtGb(selectedPlan.traffic_gb) }} 高速流量 · 有效期 {{ selectedPlan.duration_days }} 天</div>
          </div>
          <div class="cp-price cell-mono">
            {{ fmtMoney(selectedPlan.price_cents) }}
          </div>
        </div>

        <!-- 账户余额支付方式 -->
        <div class="pay-method-wrap">
          <div class="pay-option active">
            <div class="opt-info">
              <div class="opt-title">
                <el-icon color="#6366f1"><Wallet /></el-icon>&nbsp;账户余额支付
                <el-tag size="small" type="success" effect="dark" style="margin-left: 6px; font-size: 10px">即时开通</el-tag>
              </div>
              <div class="opt-desc cell-mono">
                当前可用余额: <b>¥ {{ balanceYuan }}</b>
              </div>
            </div>
          </div>

          <!-- 余额不足时的卡密充值辅助栏 -->
          <div v-if="!isBalanceSufficient" class="deficit-topup-box">
            <div class="deficit-msg">
              <el-icon color="#f59e0b"><InfoFilled /></el-icon>
              <span>余额不足，还差 <b>¥ {{ balanceDeficitYuan }}</b>。可直接输入充值卡密：</span>
            </div>
            <div class="topup-input-row">
              <el-input
                v-model="quickRedeemCode"
                placeholder="输入礼品卡卡密 GIFT-..."
                class="cell-mono"
                size="small"
                @keyup.enter="handleQuickRedeem"
              />
              <el-button
                type="primary"
                size="small"
                :loading="quickRedeeming"
                @click="handleQuickRedeem"
              >
                充值
              </el-button>
            </div>
          </div>
        </div>
      </div>

      <template #footer>
        <div class="checkout-footer">
          <div class="total-bar">
            <span>实付金额:</span>
            <span class="total-amount cell-mono">
              {{ selectedPlan ? fmtMoney(selectedPlan.price_cents) : '¥ 0.00' }}
            </span>
          </div>
          <div class="footer-btn-group">
            <el-button @click="checkoutOpen = false">取消</el-button>
            <el-button
              type="primary"
              :disabled="!isBalanceSufficient"
              :loading="paying"
              @click="confirmPay"
            >
              确认余额支付
            </el-button>
          </div>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.shop-header-row {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  flex-wrap: wrap;
  gap: 16px;
  margin-bottom: 24px;
}

.shop-hero {
  flex: 1;
  min-width: 280px;

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

.shop-wallet-badge {
  background: var(--x-card);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);
  padding: 12px 16px;
  display: flex;
  align-items: center;
  gap: 12px;
  box-shadow: var(--x-shadow);

  .wallet-icon {
    width: 36px;
    height: 36px;
    border-radius: 10px;
    background: var(--x-primary-soft);
    color: var(--x-primary);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 18px;
  }

  .wallet-text {
    display: flex;
    flex-direction: column;
  }
  .wallet-lbl {
    font-size: 11px;
    color: var(--x-text-3);
  }
  .wallet-val {
    font-size: 17px;
    font-weight: 800;
    color: var(--x-text);
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

/* 结算收银台 */
.checkout-body {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.checkout-plan-card {
  background: var(--x-bg);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);
  padding: 12px 14px;
  display: flex;
  justify-content: space-between;
  align-items: center;

  .cp-title {
    font-size: 15px;
    font-weight: 700;
    color: var(--x-text);
  }
  .cp-sub {
    font-size: 12px;
    color: var(--x-text-3);
    margin-top: 2px;
  }
  .cp-price {
    font-size: 18px;
    font-weight: 800;
    color: var(--x-text);
  }
}

.pay-method-wrap {
  .pay-option {
    border: 1.5px solid var(--x-primary);
    background: var(--x-primary-soft);
    border-radius: var(--x-radius);
    padding: 12px 14px;
    display: flex;
    align-items: center;
    gap: 12px;

    .opt-info {
      flex: 1;
      .opt-title {
        font-size: 14px;
        font-weight: 600;
        color: var(--x-text);
        display: flex;
        align-items: center;
      }
      .opt-desc {
        font-size: 12px;
        color: var(--x-text-3);
        margin-top: 2px;
      }
    }
  }

  .deficit-topup-box {
    background: #fffbeb;
    border: 1px dashed #fde68a;
    border-radius: var(--x-radius);
    padding: 10px 14px;
    margin-top: 8px;

    .deficit-msg {
      font-size: 12px;
      color: #92400e;
      display: flex;
      align-items: center;
      gap: 6px;
      margin-bottom: 8px;
    }
    .topup-input-row {
      display: flex;
      gap: 8px;
    }
  }
}

.checkout-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
  gap: 12px;

  .total-bar {
    font-size: 13px;
    color: var(--x-text-2);
    display: flex;
    align-items: baseline;
    .total-amount {
      font-size: 18px;
      font-weight: 800;
      color: var(--x-primary);
      margin-left: 6px;
    }
  }

  .footer-btn-group {
    display: flex;
    gap: 8px;
    align-items: center;
  }
}

@media (max-width: 480px) {
  .checkout-footer {
    flex-direction: column;
    align-items: stretch;
    gap: 12px;

    .total-bar {
      justify-content: space-between;
      width: 100%;
    }

    .footer-btn-group {
      display: grid;
      grid-template-columns: 1fr 1.5fr;
      width: 100%;
      gap: 8px;

      .el-button {
        margin: 0 !important;
      }
    }
  }
}

.muted {
  color: var(--x-text-3);
}
</style>