<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  CsAlert,
  CsBadge,
  CsBox,
  CsButton,
  CsColumnLayout,
  CsContainer,
  CsFormField,
  CsHeader,
  CsInput,
  CsSpaceBetween,
  ElMessage,
} from '@/components/cloudscape'
import { cancelOrder, getOrder, payOrder } from '@/api/shop'
import type { Order } from '@/api/types'
import { formatMoney } from '@/utils/format'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()

const orderNo = computed(() => String(route.params.orderNo || ''))
const loading = ref(true)
const paying = ref(false)
const cancelling = ref(false)
const order = ref<Order | null>(null)
const balanceCents = ref(0)
const availableMethods = ref<string[]>(['balance', 'gift_card'])
const selectedMethod = ref<'balance' | 'gift_card'>('balance')
const giftCode = ref('')
const nowMs = ref(Date.now())
let timer: ReturnType<typeof setInterval> | null = null

const remainingSeconds = computed(() => {
  if (!order.value?.expires_at || order.value.status !== 'pending') return 0
  const diff = Math.floor((new Date(order.value.expires_at).getTime() - nowMs.value) / 1000)
  return Math.max(0, diff)
})

const remainingText = computed(() => {
  const s = remainingSeconds.value
  const m = Math.floor(s / 60)
  const sec = s % 60
  return `${String(m).padStart(2, '0')}:${String(sec).padStart(2, '0')}`
})

const balanceEnough = computed(() => (order.value ? balanceCents.value >= order.value.amount_cents : false))
const diffCents = computed(() => (order.value ? Math.max(0, order.value.amount_cents - balanceCents.value) : 0))

async function load() {
  if (!orderNo.value) return
  loading.value = true
  try {
    const res = await getOrder(orderNo.value)
    const payload = res.data.data
    order.value = payload.order
    balanceCents.value = payload.balance_cents
    if (payload.available_methods?.length) {
      availableMethods.value = payload.available_methods
    }
    if (order.value?.status === 'pending' && !balanceEnough.value && availableMethods.value.includes('gift_card')) {
      selectedMethod.value = 'gift_card'
    }
  } catch (e: unknown) {
    ElMessage.error(e instanceof Error ? e.message : '加载订单失败')
  } finally {
    loading.value = false
  }
}

async function handlePay() {
  if (!order.value || order.value.status !== 'pending') return
  if (selectedMethod.value === 'gift_card' && !giftCode.value.trim()) {
    ElMessage.warning('请输入卡密')
    return
  }
  paying.value = true
  try {
    const res = await payOrder(
      order.value.order_no,
      selectedMethod.value,
      selectedMethod.value === 'gift_card' ? giftCode.value.trim() : undefined,
    )
    const payload = res.data.data
    if (payload.paid) {
      order.value = payload.order
      await auth.fetchMe()
      ElMessage.success('支付成功，套餐已生效')
      router.push('/subscribe')
    }
  } catch (e: unknown) {
    ElMessage.error(e instanceof Error ? e.message : '支付失败')
    await load()
    await auth.fetchMe()
  } finally {
    paying.value = false
  }
}

async function handleCancel() {
  if (!order.value || order.value.status !== 'pending') return
  cancelling.value = true
  try {
    const res = await cancelOrder(order.value.order_no)
    order.value = res.data.data.order
    ElMessage.success('订单已取消')
  } catch (e: unknown) {
    ElMessage.error(e instanceof Error ? e.message : '取消失败')
  } finally {
    cancelling.value = false
  }
}

onMounted(() => {
  load()
  timer = setInterval(() => {
    nowMs.value = Date.now()
    if (order.value?.status === 'pending' && order.value.expires_at && remainingSeconds.value === 0) {
      load()
    }
  }, 1000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<template>
  <CsSpaceBetween size="l">
    <CsHeader
      variant="h1"
      description="请在有效期内完成支付，超时订单将自动关闭"
    >
      收银台
      <template #actions>
        <CsButton variant="normal" @click="router.push('/shop')">返回商店</CsButton>
      </template>
    </CsHeader>

    <CsContainer v-if="!loading && !order">
      <CsAlert type="error" header="订单不存在">
        未找到对应订单，请返回商店重新选购。
      </CsAlert>
    </CsContainer>

    <template v-else-if="order">
      <CsAlert
        v-if="order.status === 'pending'"
        type="info"
        :header="`待支付 · 剩余支付时间 ${remainingText}`"
      >
        订单已为您锁定当前套餐价格，请在倒计时结束前完成支付。
      </CsAlert>
      <CsAlert v-else-if="order.status === 'paid'" type="success" header="订单已支付">
        该订单已完成支付，套餐已生效。
      </CsAlert>
      <CsAlert v-else type="warning" header="订单已取消">
        {{ order.cancel_reason === 'timeout' ? '订单超时未支付已自动关闭。' : '该订单已被取消。' }}
      </CsAlert>

      <CsColumnLayout :columns="2" variant="text-grid">
        <CsContainer>
          <template #header>
            <CsHeader variant="h2">订单详情</CsHeader>
          </template>
          <CsSpaceBetween size="s">
            <div class="kv-row">
              <span class="kv-label">订单号</span>
              <span class="kv-val cell-mono">{{ order.order_no }}</span>
            </div>
            <div class="kv-row">
              <span class="kv-label">套餐名称</span>
              <span class="kv-val">{{ order.plan_name || `#${order.plan_id}` }}</span>
            </div>
            <div class="kv-row">
              <span class="kv-label">应付金额</span>
              <span class="kv-val cell-mono"><strong>{{ formatMoney(order.amount_cents) }}</strong></span>
            </div>
            <div class="kv-row">
              <span class="kv-label">订单状态</span>
              <span class="kv-val">
                {{
                  order.status === 'paid'
                    ? '已支付'
                    : order.status === 'cancelled'
                      ? '已取消'
                      : '待支付'
                }}
              </span>
            </div>
          </CsSpaceBetween>
        </CsContainer>

        <CsContainer v-if="order.status === 'pending'">
          <template #header>
            <CsHeader variant="h2">选择支付方式</CsHeader>
          </template>
          <CsSpaceBetween size="m">
            <div class="method-list">
              <div
                v-if="availableMethods.includes('balance')"
                class="method-card"
                :class="{ active: selectedMethod === 'balance' }"
                @click="selectedMethod = 'balance'"
              >
                <CsSpaceBetween direction="horizontal" size="xs" align-items="center">
                  <strong>账户余额支付</strong>
                  <CsBadge :color="balanceEnough ? 'green' : 'grey'">
                    当前余额 {{ formatMoney(balanceCents) }}
                  </CsBadge>
                </CsSpaceBetween>
                <CsBox variant="small" color="text-body-secondary">
                  {{
                    balanceEnough
                      ? '余额充足，点击下方按钮立即完成支付'
                      : `余额不足，还差 ${formatMoney(diffCents)}`
                  }}
                </CsBox>
              </div>

              <div
                v-if="availableMethods.includes('gift_card')"
                class="method-card"
                :class="{ active: selectedMethod === 'gift_card' }"
                @click="selectedMethod = 'gift_card'"
              >
                <CsSpaceBetween direction="horizontal" size="xs" align-items="center">
                  <strong>卡密充值并支付</strong>
                  <CsBadge color="blue">自动抵扣</CsBadge>
                </CsSpaceBetween>
                <CsBox variant="small" color="text-body-secondary">
                  卡密核销后自动充入余额并完成订单支付，多余金额保留在账户余额中
                </CsBox>
              </div>
            </div>

            <CsFormField v-if="selectedMethod === 'gift_card'" label="输入卡密">
              <CsInput v-model="giftCode" placeholder="请输入礼品卡 / 充值卡密" />
            </CsFormField>

            <CsSpaceBetween direction="horizontal" size="s">
              <CsButton
                variant="primary"
                :loading="paying"
                :disabled="selectedMethod === 'balance' && !balanceEnough"
                @click="handlePay"
              >
                {{
                  selectedMethod === 'balance'
                    ? `确认支付 ${formatMoney(order.amount_cents)}`
                    : '核销卡密并支付'
                }}
              </CsButton>
              <CsButton variant="normal" :loading="cancelling" @click="handleCancel">
                取消订单
              </CsButton>
            </CsSpaceBetween>
          </CsSpaceBetween>
        </CsContainer>

        <CsContainer v-else>
          <template #header>
            <CsHeader variant="h2">后续操作</CsHeader>
          </template>
          <CsSpaceBetween direction="horizontal" size="s">
            <CsButton v-if="order.status === 'paid'" variant="primary" @click="router.push('/subscribe')">
              查看我的订阅
            </CsButton>
            <CsButton variant="normal" @click="router.push('/shop')">
              返回套餐商店
            </CsButton>
          </CsSpaceBetween>
        </CsContainer>
      </CsColumnLayout>
    </template>
  </CsSpaceBetween>
</template>

<style scoped>
.kv-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 6px 0;
  border-bottom: 1px solid var(--color-border-divider-default-diwd4w, #eaeded);
}

.kv-label {
  color: var(--color-text-body-secondary-66ygbb, #5f6b7a);
  font-size: 13px;
}

.kv-val {
  font-size: 14px;
}

.method-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.method-card {
  padding: 12px 14px;
  border-radius: 8px;
  border: 1px solid var(--color-border-divider-default-diwd4w, #d5dbdb);
  cursor: pointer;
  transition: border-color 0.15s ease, background-color 0.15s ease;
}

.method-card.active {
  border-color: var(--color-border-item-selected-7104rh, #0972d3);
  background: var(--color-background-item-selected-dotkup, rgba(9, 114, 211, 0.06));
}
</style>
