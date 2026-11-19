<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  CsAlert,
  CsBadge,
  CsBox,
  CsButton,
  CsContainer,
  CsHeader,
  CsModal,
  CsSpaceBetween,
  ElMessage,
} from '@/components/cloudscape'
import { createOrder, payOrderByBalance, getMyOrders, getPublicPlans } from '@/api/shop'
import RechargeModal from '@/components/RechargeModal.vue'
import { useAuthStore } from '@/stores/auth'
import { renderPlanMarkdown } from '@/utils/markdown'
import { errMsg } from '@/api/http'
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

// 统一充值弹窗
const topupModalOpen = ref(false)

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

function fmtGb(gb: number) {
  return gb >= 1024 ? `${(gb / 1024).toFixed(1)} TB` : `${gb} GB`
}

function renderPlanBody(plan: Plan): string {
  return renderPlanMarkdown(plan.description, {
    trafficGb: plan.traffic_gb,
    priceCents: plan.price_cents,
    durationDays: plan.duration_days,
  })
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

const router = useRouter()

async function openCheckout(plan: Plan) {
  try {
    const { data } = await createOrder(plan.id)
    if (data.code === 0 && data.data?.order) {
      const order = data.data.order
      if (order.status === 'paid') {
        await auth.fetchMe()
        ElMessage.success('订购成功，套餐已生效')
        router.push('/subscribe')
        return
      }
      router.push(`/checkout/${order.order_no}`)
    }
  } catch (e: unknown) {
    const err = e as { response?: { status?: number; data?: { pending_order_no?: string; message?: string } } }
    if (err?.response?.status === 409 && err.response.data?.pending_order_no) {
      ElMessage.warning('您有一笔待支付订单，正在前往收银台')
      router.push(`/checkout/${err.response.data.pending_order_no}`)
      return
    }
    ElMessage.error(err?.response?.data?.message || '创建订单失败')
  }
}

async function handleRecharged() {
  await load()
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
            <template v-if="idx === featuredIdx" #actions>
              <CsBadge color="blue">热门推荐</CsBadge>
            </template>
          </CsHeader>
        </template>

        <div class="plan-markdown" v-html="renderPlanBody(p)" />

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

    <!-- 统一账户充值弹窗 -->
    <RechargeModal v-model:visible="topupModalOpen" @recharged="handleRecharged" />

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
          当前还差 ¥{{ balanceDeficitYuan }}，请先充值后再支付。
        </CsAlert>

        <div v-if="!isBalanceSufficient">
          <CsButton variant="normal" @click="topupModalOpen = true">去充值</CsButton>
        </div>
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

.plan-markdown {
  font-size: 13px;
  line-height: 1.65;
  color: var(--x-text);

  :deep(h1),
  :deep(h2),
  :deep(h3),
  :deep(h4) {
    margin: 0 0 8px;
    font-weight: 700;
    line-height: 1.35;
    color: var(--x-text);
  }

  :deep(h1) {
    font-size: 24px;
  }

  :deep(h2) {
    font-size: 20px;
  }

  :deep(h3) {
    font-size: 16px;
  }

  :deep(h4) {
    font-size: 14px;
  }

  :deep(p) {
    margin: 0 0 8px;
  }

  :deep(ul),
  :deep(ol) {
    margin: 0 0 8px;
    padding-inline-start: 18px;
  }

  :deep(li + li) {
    margin-top: 4px;
  }

  :deep(del),
  :deep(s) {
    color: var(--x-text-secondary);
  }

  :deep(> :last-child) {
    margin-bottom: 0;
  }
}

.plan-buy-btn {
  inline-size: 100%;
  display: flex !important;
  align-items: center;
  justify-content: center !important;
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

.pricing-card {
  display: flex;
  flex-direction: column;
  height: 100%;
}

.pricing-card > :deep([class*='awsui_content-wrapper_']),
.pricing-card > :deep([class*='awsui_content_']) {
  flex: 1;
}

.pricing-card > :deep([class*='awsui_footer_']) {
  margin-top: auto;
}
</style>
