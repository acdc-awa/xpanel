<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  CsBadge,
  CsBox,
  CsButton,
  CsContainer,
  CsHeader,
  CsSpaceBetween,
  toast,
  messageBox,
} from '@/components/cloudscape'
import { createOrder, getPublicPlans } from '@/api/shop'
import RechargeModal from '@/components/RechargeModal.vue'
import { useAuthStore } from '@/stores/auth'
import { renderPlanMarkdown } from '@/utils/markdown'
import { errMsg } from '@/api/http'
import type { Plan } from '@/api/types'

const auth = useAuthStore()

const plans = ref<Plan[]>([])
const loading = ref(false)
const creatingPlanId = ref<number | null>(null)

// 「热门推荐」徽标与高亮卡片：管理端标记 is_featured 的第一个套餐；一个都没标时回退列表第一项
const featuredPlanId = computed(() => {
  const p = plans.value.find((item) => item.is_featured) ?? plans.value[0]
  return p?.id ?? 0
})

// 统一充值弹窗
const topupModalOpen = ref(false)

const userBalanceCents = computed(() => auth.user?.balance_cents ?? 0)
const currentPlanId = computed(() => auth.user?.plan_id ?? 0)
const currentPlan = computed(() =>
  currentPlanId.value > 0 ? (plans.value.find((p) => p.id === currentPlanId.value) ?? null) : null,
)
const otherPlans = computed(() =>
  currentPlanId.value > 0 ? plans.value.filter((p) => p.id !== currentPlanId.value) : plans.value,
)
const hasUnexpiredPlan = computed(() => {
  if (!currentPlanId.value || !auth.user?.expire_at) return false
  const exp = Date.parse(auth.user.expire_at)
  return Number.isFinite(exp) && exp > Date.now()
})
const balanceYuan = computed(() => (userBalanceCents.value / 100).toFixed(2))

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
    const [p] = await Promise.all([getPublicPlans(), auth.fetchMe()])
    if (p.data.code === 0) plans.value = p.data.data.items
  } catch (e) {
    toast.error(errMsg(e, '加载失败'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

const router = useRouter()

async function handleSelectPlan(plan: Plan) {
  if (creatingPlanId.value !== null) return
  if (hasUnexpiredPlan.value) {
    const isSame = plan.id === currentPlanId.value
    const title = isSame ? '确认续费当前套餐' : '确认更换套餐'
    const message = isSame
      ? '续费将在支付完成后从当前时刻起重新计算有效期并重置流量，当前套餐的剩余有效期与剩余流量将作废清零。确定继续前往收银台吗？'
      : `即将更换为「${plan.name}」。支付完成后将从当前时刻起重新计算有效期并重置流量，当前套餐的剩余有效期与剩余流量将作废清零。确定继续前往收银台吗？`
    try {
      await messageBox.confirm(message, title, {
        confirmButtonText: '继续前往收银台',
        cancelButtonText: '取消',
        type: 'warning',
      })
    } catch {
      return
    }
  }
  await openCheckout(plan)
}

async function openCheckout(plan: Plan) {
  creatingPlanId.value = plan.id
  try {
    const { data } = await createOrder(plan.id)
    if (data.code === 0 && data.data?.order) {
      const order = data.data.order
      if (order.status === 'paid') {
        await auth.fetchMe()
        toast.success('订购成功，套餐已生效')
        router.push('/subscribe')
        return
      }
      router.push(`/checkout/${order.order_no}`)
    }
  } catch (e: unknown) {
    const err = e as { response?: { status?: number; data?: { pending_order_no?: string; message?: string } } }
    if (err?.response?.status === 409 && err.response.data?.pending_order_no) {
      toast.warning('您有一笔待支付订单，正在前往收银台')
      router.push(`/checkout/${err.response.data.pending_order_no}`)
      return
    }
    toast.error(err?.response?.data?.message || '创建订单失败')
  } finally {
    creatingPlanId.value = null
  }
}

async function handleRecharged() {
  await load()
}
</script>

<template>
  <CsSpaceBetween direction="vertical" size="l">
    <CsHeader variant="h1">
      套餐商店
      <template #actions>
        <CsSpaceBetween direction="horizontal" size="xs" align-items="center">
          <span class="shop-balance-chip">
            当前余额 <strong class="cell-mono">¥{{ balanceYuan }}</strong>
          </span>
          <CsButton variant="normal" :wrap-text="false" @click="topupModalOpen = true">
            充值
          </CsButton>
        </CsSpaceBetween>
      </template>
    </CsHeader>

    <!-- 套餐列表 -->
    <template v-if="plans.length || currentPlanId > 0">
      <CsSpaceBetween v-if="currentPlanId > 0" direction="vertical" size="s">
        <CsHeader variant="h2">当前订阅套餐</CsHeader>
        <div v-if="currentPlan" class="pricing-grid">
          <CsContainer
            class="pricing-card"
            :class="{ 'is-featured': currentPlan.id === featuredPlanId }"
          >
            <template #header>
              <CsHeader variant="h2">
                {{ currentPlan.name }}
                <template #actions>
                  <CsBadge color="green">当前订阅</CsBadge>
                </template>
              </CsHeader>
            </template>

            <div class="plan-markdown" v-html="renderPlanBody(currentPlan)" />

            <template #footer>
              <CsButton
                :variant="currentPlan.id === featuredPlanId ? 'primary' : 'normal'"
                class="plan-buy-btn"
                :loading="creatingPlanId === currentPlan.id"
                :disabled="creatingPlanId !== null && creatingPlanId !== currentPlan.id"
                @click="handleSelectPlan(currentPlan)"
              >
                续费
              </CsButton>
            </template>
          </CsContainer>
        </div>
        <CsContainer v-else>
          <CsBox color="text-body-secondary" font-size="body-s">
            您当前订阅的套餐（{{ auth.user?.plan_name || `#${currentPlanId}` }}）已下架或暂不支持续费，请在下方选择其他套餐更换。
          </CsBox>
        </CsContainer>
      </CsSpaceBetween>

      <CsSpaceBetween v-if="otherPlans.length" direction="vertical" size="s">
        <CsHeader v-if="currentPlanId > 0" variant="h2">其他可选套餐</CsHeader>
        <div class="pricing-grid">
          <CsContainer
            v-for="p in otherPlans"
            :key="p.id"
            class="pricing-card"
            :class="{ 'is-featured': p.id === featuredPlanId }"
          >
            <template #header>
              <CsHeader variant="h2">
                {{ p.name }}
                <template v-if="p.id === featuredPlanId" #actions>
                  <CsBadge color="blue">热门推荐</CsBadge>
                </template>
              </CsHeader>
            </template>

            <div class="plan-markdown" v-html="renderPlanBody(p)" />

            <template #footer>
              <CsButton
                :variant="p.id === featuredPlanId ? 'primary' : 'normal'"
                class="plan-buy-btn"
                :loading="creatingPlanId === p.id"
                :disabled="creatingPlanId !== null && creatingPlanId !== p.id"
                @click="handleSelectPlan(p)"
              >
                {{ currentPlanId > 0 ? '更换套餐' : '立即订购' }}
              </CsButton>
            </template>
          </CsContainer>
        </div>
      </CsSpaceBetween>
    </template>

    <CsContainer v-else>
      <CsBox color="text-body-secondary" font-size="body-s">
        {{ loading ? '正在加载在售套餐…' : '暂无在售套餐' }}
      </CsBox>
    </CsContainer>

    <!-- 统一账户充值弹窗 -->
    <RechargeModal v-model:visible="topupModalOpen" @recharged="handleRecharged" />
  </CsSpaceBetween>
</template>

<style scoped lang="scss">
.shop-balance-chip {
  font-size: 13px;
  color: var(--x-text-secondary);

  strong {
    color: var(--x-text);
  }
}

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

  &.is-featured {
    border-color: var(--color-border-item-selected-7104rh, #006ce0);
    box-shadow: 0 0 0 2px var(--color-border-item-selected-7104rh, #006ce0);

    &::before {
      border-color: var(--color-border-item-selected-7104rh, #006ce0);
    }
  }
}

.pricing-card > :deep([class*='awsui_content-wrapper_']),
.pricing-card > :deep([class*='awsui_content_']) {
  flex: 1;
}

.pricing-card > :deep([class*='awsui_footer_']) {
  margin-top: auto;
}
</style>
