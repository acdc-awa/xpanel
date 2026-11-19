<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  CsAlert,
  CsBadge,
  CsBox,
  CsButton,
  CsContainer,
  CsForm,
  CsFormItem,
  CsHeader,
  CsInput,
  CsModal,
  CsSpaceBetween,
  CsStatusIndicator,
  ElMessage,
} from '@/components/cloudscape'
import { payOrderByBalance, getMyOrders, getPublicPlans } from '@/api/shop'
import { redeemGiftCard } from '@/api/gift_card'
import { useAuthStore } from '@/stores/auth'
import { errMsg } from '@/api/http'
import { formatMoney } from '@/utils/format'
import { formatDateTimeMinute } from '@/utils/timezone'
import type { Order, Plan } from '@/api/types'

const auth = useAuthStore()

const plans = ref<Plan[]>([])
const orders = ref<Order[]>([])
const loading = ref(false)

// 「热门推荐」徽标与高亮卡片：管理端标记 is_featured 的第一个套餐；一个都没标时回退列表第一项
const featuredIdx = computed(() => {
  const i = plans.value.findIndex((p) => p.is_featured)
  return i === -1 ? 0 : i
})

// 结账收银台
const checkoutOpen = ref(false)
const selectedPlan = ref<Plan | null>(null)
const paying = ref(false)

// 现场快速充值弹窗
const topupModalOpen = ref(false)
const quickRedeemCode = ref('')
const quickRedeeming = ref(false)

const userBalanceCents = computed(() => auth.user?.balance_cents ?? 0)
const currentPlanId = computed(() => auth.user?.plan_id ?? 0)
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

const orderStatusMap: Record<string, { type: 'pending' | 'success' | 'stopped'; text: string }> = {
  pending: { type: 'pending', text: '处理中' },
  paid: { type: 'success', text: '已生效' },
  cancelled: { type: 'stopped', text: '已取消' },
}

function fmtMoney(cents: number) {
  return formatMoney(cents)
}

function fmtGb(gb: number) {
  return gb >= 1024 ? `${(gb / 1024).toFixed(1)} TB` : `${gb} GB`
}

function getPlanFeatures(plan: Plan): string[] {
  const defaultTemplate = `包含 $TRAFFIC$ 高速流量
有效期 $DURATION$ 天
全量优质节点与中转链路
全面兼容 Mihomo / Clash 生态`

  const rawText = plan.description?.trim() ? plan.description : defaultTemplate
  const trafficStr = fmtGb(plan.traffic_gb)
  const durationStr = `${plan.duration_days}`

  return rawText
    .split('\n')
    .map((s) => s.trim())
    .filter(Boolean)
    .map((line) =>
      line
        .replace(/(\$TRAFFIC\$|\{traffic\}|\{\{traffic\}\})/gi, trafficStr)
        .replace(/(\$DURATION\$|\{duration\}|\{\{duration\}\}|\$DAYS\$|\{days\})/gi, durationStr)
        .replace(/(\$DEVICE_LIMIT\$|\{device_limit\}|\{\{device_limit\}\}|\$DEVICES\$|\{devices\})/gi, ''),
    )
    .filter((line) => line !== '')
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

async function handleQuickRedeem() {
  const code = quickRedeemCode.value.trim()
  if (!code) {
    ElMessage.warning('请输入卡密')
    return
  }
  quickRedeeming.value = true
  try {
    const { data } = await redeemGiftCard(code)
    if (data.code === 0) {
      ElMessage.success(`充值成功，已到账 ¥${(data.data.face_value_cents / 100).toFixed(2)}`)
      quickRedeemCode.value = ''
      topupModalOpen.value = false
      await auth.fetchMe()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '卡密核销失败'))
  } finally {
    quickRedeeming.value = false
  }
}

async function confirmPay() {
  if (!selectedPlan.value) return
  paying.value = true
  try {
    const { data } = await payOrderByBalance(selectedPlan.value.id)
    if (data.code === 0) {
      ElMessage.success('订购成功，套餐已即时生效')
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
  <CsSpaceBetween direction="vertical" size="l">
    <CsHeader variant="h1" description="高性能节点集群与全球链路中转，支持即时订阅下发与按需升级。">
      套餐商店
      <template #actions>
        <CsSpaceBetween direction="horizontal" size="xs" align-items="center">
          <CsBadge color="grey">余额 ¥{{ balanceYuan }}</CsBadge>
          <CsButton
            @click="
              quickRedeemCode = '';
              topupModalOpen = true
            "
          >
            卡密充值
          </CsButton>
        </CsSpaceBetween>
      </template>
    </CsHeader>

    <!-- 套餐列表 -->
    <div v-if="plans.length" class="pricing-grid">
      <CsContainer
        v-for="(p, idx) in plans"
        :key="p.id"
        class="pricing-card"
        :class="{ 'is-featured': idx === featuredIdx }"
      >
        <template #header>
          <CsHeader variant="h2">
            {{ p.name }}
            <template #actions>
              <CsBadge v-if="idx === featuredIdx" color="blue">热门推荐</CsBadge>
              <CsBadge v-else color="grey">{{ fmtGb(p.traffic_gb) }}</CsBadge>
            </template>
          </CsHeader>
        </template>

        <CsSpaceBetween direction="vertical" size="m">
          <div class="plan-price-row">
            <span class="plan-price cell-mono">¥{{ (p.price_cents / 100).toFixed(2) }}</span>
            <span class="plan-period">/ {{ p.duration_days }} 天</span>
          </div>

          <ul class="plan-features">
            <li v-for="(feat, fIdx) in getPlanFeatures(p)" :key="fIdx" class="plan-feature-item">
              <CsStatusIndicator type="success">{{ feat }}</CsStatusIndicator>
            </li>
          </ul>
        </CsSpaceBetween>

        <template #footer>
          <CsButton
            :variant="idx === featuredIdx ? 'primary' : 'normal'"
            class="plan-buy-btn"
            @click="openCheckout(p)"
          >
            {{ p.id === currentPlanId ? '续费' : '立即订购' }}
          </CsButton>
        </template>
      </CsContainer>
    </div>

    <CsContainer v-else>
      <CsBox color="text-body-secondary" font-size="body-s">
        {{ loading ? '正在加载在售套餐…' : '暂无在售套餐' }}
      </CsBox>
    </CsContainer>

    <!-- 订单记录 -->
    <CsContainer>
      <template #header>
        <CsHeader variant="h2">服务订购记录</CsHeader>
      </template>

      <div v-if="orders.length" class="order-list">
        <div v-for="o in orders" :key="o.id" class="order-row">
          <div class="order-main">
            <div class="order-name">{{ o.plan_name }}</div>
            <div class="order-meta">
              <code class="cell-mono">{{ o.order_no }}</code>
              <span>{{ formatDateTimeMinute(o.created_at) }}</span>
              <CsBadge color="grey">余额支付</CsBadge>
            </div>
          </div>
          <div class="order-right">
            <div class="order-amount cell-mono">{{ fmtMoney(o.amount_cents) }}</div>
            <CsStatusIndicator :type="orderStatusMap[o.status]?.type ?? 'stopped'">
              {{ orderStatusMap[o.status]?.text ?? o.status }}
            </CsStatusIndicator>
          </div>
        </div>
      </div>
      <CsBox v-else color="text-body-secondary" font-size="body-s">暂无订购记录</CsBox>
    </CsContainer>

    <!-- 卡密核销弹窗 -->
    <CsModal
      v-model:visible="topupModalOpen"
      header="卡密核销"
      size="small"
      :close-on-click-overlay="false"
    >
      <CsForm @submit="handleQuickRedeem">
        <CsFormItem label="卡密兑换码" description="输入 16 位卡密，核销后余额即时到账。">
          <CsInput
            v-model="quickRedeemCode"
            placeholder="请输入 16 位卡密"
            class="cell-mono"
          />
        </CsFormItem>
      </CsForm>
      <template #footer>
        <CsSpaceBetween direction="horizontal" size="xs">
          <CsButton @click="topupModalOpen = false">取消</CsButton>
          <CsButton variant="primary" :loading="quickRedeeming" @click="handleQuickRedeem">
            立即核销
          </CsButton>
        </CsSpaceBetween>
      </template>
    </CsModal>

    <!-- 结账收银台弹窗 -->
    <CsModal
      v-model:visible="checkoutOpen"
      header="结算收银台"
      size="small"
      :close-on-click-overlay="false"
    >
      <CsSpaceBetween v-if="selectedPlan" direction="vertical" size="m">
        <div class="checkout-summary">
          <div class="checkout-plan-name">{{ selectedPlan.name }}</div>
          <div class="checkout-plan-meta">
            {{ fmtGb(selectedPlan.traffic_gb) }} 高速配额，有效期 {{ selectedPlan.duration_days }} 天
          </div>
        </div>

        <div class="checkout-kv">
          <span>应付金额</span>
          <strong class="cell-mono">¥{{ (selectedPlan.price_cents / 100).toFixed(2) }}</strong>
        </div>
        <div class="checkout-kv">
          <span>账户余额</span>
          <span class="cell-mono">¥{{ balanceYuan }}</span>
        </div>

        <CsAlert v-if="!isBalanceSufficient" type="warning" header="账户余额不足">
          当前还差 ¥{{ balanceDeficitYuan }}，可直接输入卡密即时充值。
        </CsAlert>

        <CsForm v-if="!isBalanceSufficient" @submit="handleQuickRedeem">
          <CsFormItem label="卡密充值">
            <CsSpaceBetween direction="horizontal" size="xs">
              <CsInput
                v-model="quickRedeemCode"
                placeholder="输入 16 位卡密"
                class="cell-mono"
              />
              <CsButton :loading="quickRedeeming" @click="handleQuickRedeem">核销</CsButton>
            </CsSpaceBetween>
          </CsFormItem>
        </CsForm>
      </CsSpaceBetween>

      <template #footer>
        <CsSpaceBetween direction="horizontal" size="xs">
          <CsButton @click="checkoutOpen = false">取消</CsButton>
          <CsButton
            variant="primary"
            :disabled="!isBalanceSufficient"
            :loading="paying"
            @click="confirmPay"
          >
            确认支付
          </CsButton>
        </CsSpaceBetween>
      </template>
    </CsModal>
  </CsSpaceBetween>
</template>

<style scoped lang="scss">
.pricing-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
  align-items: stretch;
}

.plan-price-row {
  display: flex;
  align-items: baseline;
  gap: 6px;
}

.plan-price {
  font-size: 24px;
  font-weight: 700;
  color: var(--x-text);
}

.plan-period {
  font-size: 13px;
  color: var(--x-text-secondary);
}

.plan-features {
  margin: 0;
  padding: 0;
  list-style: none;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.plan-feature-item {
  font-size: 13px;
  color: var(--x-text);
}

.plan-buy-btn {
  inline-size: 100%;
}

.order-list {
  display: flex;
  flex-direction: column;
}

.order-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 0;
  border-bottom: 1px solid var(--x-border);

  &:last-child {
    border-bottom: none;
    padding-bottom: 0;
  }

  &:first-child {
    padding-top: 0;
  }
}

.order-main {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-inline-size: 0;
}

.order-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--x-text);
}

.order-meta {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  color: var(--x-text-secondary);
}

.order-right {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 4px;
  flex-shrink: 0;
}

.order-amount {
  font-size: 14px;
  font-weight: 700;
  color: var(--x-text);
}

.checkout-summary {
  padding: 12px 14px;
  border: 1px solid var(--x-border);
  border-radius: 8px;
}

.checkout-plan-name {
  font-size: 16px;
  font-weight: 700;
  color: var(--x-text);
}

.checkout-plan-meta {
  margin-top: 4px;
  font-size: 13px;
  color: var(--x-text-secondary);
}

.checkout-kv {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 14px;
  color: var(--x-text);
}

@media (max-width: $bp-content-max) {
  .pricing-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: $bp-tablet-max) {
  .pricing-grid {
    grid-template-columns: 1fr;
  }
}
</style>
