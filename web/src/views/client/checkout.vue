<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import QRCode from 'qrcode'
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
  CsLink,
  CsSpinner,
  CsSpaceBetween,
  toast,
} from '@/components/cloudscape'
import { cancelOrder, getOrder, payOrder } from '@/api/shop'
import { errMsg } from '@/api/http'
import type { Order, PaymentChannelInfo } from '@/api/types'
import RechargeModal from '@/components/RechargeModal.vue'
import { formatMoney, formatOrderContent } from '@/utils/format'
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
const channels = ref<PaymentChannelInfo[]>([])
const selectedMethod = ref<string>('balance')
const giftCode = ref('')
const rechargeModalOpen = ref(false)
const payUrl = ref('')
const qrCodeText = ref('')
const qrDataUrl = ref('')
const nowMs = ref(Date.now())
let timer: ReturnType<typeof setInterval> | null = null
let pollTimer: ReturnType<typeof setInterval> | null = null
let expiredRefreshed = false
const POLL_INTERVAL_MS = 4000

const isRecharge = computed(() => order.value?.order_type === 'recharge')

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

async function load(silent = false) {
  if (!orderNo.value) return
  if (!silent) loading.value = true
  try {
    const res = await getOrder(orderNo.value)
    const payload = res.data.data
    order.value = payload.order
    balanceCents.value = payload.balance_cents
    if (payload.channels?.length) {
      channels.value = payload.channels
    }
    if (payload.available_methods?.length) {
      availableMethods.value = payload.available_methods
    }
    if (order.value?.pay_payload) {
      try {
        const parsed = JSON.parse(order.value.pay_payload) as { pay_url?: string; qr_code?: string }
        payUrl.value = parsed.pay_url || ''
        qrCodeText.value = parsed.qr_code || ''
      } catch {
        // ignore invalid payload
      }
    }
    if (order.value?.status === 'pending') {
      if (isRecharge.value) {
        if (order.value.payment_channel && availableMethods.value.includes(order.value.payment_channel)) {
          selectedMethod.value = order.value.payment_channel
        } else if (availableMethods.value.length > 0) {
          selectedMethod.value = availableMethods.value[0]
        }
      } else if (!balanceEnough.value && availableMethods.value.includes('gift_card')) {
        selectedMethod.value = 'gift_card'
      }
    }
  } catch (e: unknown) {
    if (!silent) toast.error(errMsg(e, '加载订单失败'))
  } finally {
    loading.value = false
  }
}

watch(
  qrCodeText,
  async (text) => {
    if (!text) {
      qrDataUrl.value = ''
      return
    }
    try {
      qrDataUrl.value = await QRCode.toDataURL(text, {
        width: 220,
        margin: 2,
        color: { dark: '#000000', light: '#ffffff' },
      })
    } catch {
      qrDataUrl.value = ''
    }
  },
  { immediate: true },
)

// 支付中：已生成支付链接或二维码且订单仍待支付，才轮询订单状态
const awaitingPayment = computed(
  () => order.value?.status === 'pending' && Boolean(payUrl.value || qrCodeText.value),
)

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

async function pollOnce() {
  const before = order.value?.status
  await load(true)
  if (order.value?.status !== 'pending') {
    stopPolling()
    if (before === 'pending' && order.value?.status === 'paid') {
      await auth.fetchMe()
      toast.success(isRecharge.value ? '充值成功，余额已到账' : '支付成功，套餐已生效')
    }
  }
}

watch(awaitingPayment, (waiting) => {
  stopPolling()
  if (waiting) pollTimer = setInterval(pollOnce, POLL_INTERVAL_MS)
})

// 路由切换到另一笔订单时重置本页状态并重新加载
watch(orderNo, () => {
  stopPolling()
  expiredRefreshed = false
  payUrl.value = ''
  qrCodeText.value = ''
  order.value = null
  giftCode.value = ''
  load()
})

async function handlePay() {
  if (!order.value || order.value.status !== 'pending') return
  if (selectedMethod.value === 'gift_card' && !giftCode.value.trim()) {
    toast.warning('请输入卡密')
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
      toast.success(isRecharge.value ? '充值成功，余额已到账' : '支付成功，套餐已生效')
    } else {
      payUrl.value = payload.pay_url || ''
      qrCodeText.value = payload.qr_code || ''
      if (payload.pay_url) {
        window.open(payload.pay_url, '_blank', 'noopener')
      }
    }
  } catch (e: unknown) {
    toast.error(errMsg(e, '支付失败'))
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
    toast.success('订单已取消')
  } catch (e: unknown) {
    toast.error(errMsg(e, '取消失败'))
  } finally {
    cancelling.value = false
  }
}

async function handleRecharged() {
  await Promise.all([load(), auth.fetchMe()])
  if (!isRecharge.value && balanceEnough.value) {
    selectedMethod.value = 'balance'
  }
}

onMounted(() => {
  load()
  timer = setInterval(() => {
    nowMs.value = Date.now()
    // 倒计时归零后仅刷新一次，避免每秒重复请求
    if (
      !expiredRefreshed &&
      order.value?.status === 'pending' &&
      order.value.expires_at &&
      remainingSeconds.value === 0
    ) {
      expiredRefreshed = true
      load(true)
    }
  }, 1000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
  stopPolling()
})
</script>

<template>
  <CsSpaceBetween size="l">
    <CsHeader variant="h1">
      {{ order?.status === 'paid' ? '订单完成' : '收银台' }}
      <template v-if="order?.status !== 'paid'" #actions>
        <CsButton variant="normal" @click="router.push('/shop')">返回商店</CsButton>
      </template>
    </CsHeader>

    <CsContainer v-if="loading && !order">
      <CsBox text-align="center" padding="l">
        <CsSpinner size="large" />
      </CsBox>
    </CsContainer>

    <CsContainer v-else-if="!loading && !order">
      <CsAlert type="error" header="订单不存在">
        未找到对应订单，请返回商店重新选购。
      </CsAlert>
    </CsContainer>

    <template v-else-if="order">
      <CsAlert
        v-if="order.status === 'pending'"
        type="info"
        :header="`待支付，剩余支付时间 ${remainingText}`"
      >
        <template v-if="isRecharge">
          请在倒计时结束前完成充值支付，成功后将自动计入账户余额。
        </template>
      </CsAlert>
      <CsAlert v-else-if="order.status === 'paid'" type="success" header="订单已完成">
        {{ isRecharge ? '该笔充值已完成，金额已计入您的账户余额。' : '该订单已完成支付，套餐已生效。' }}
      </CsAlert>
      <CsAlert v-else type="warning" header="订单已取消">
        {{ order.cancel_reason === 'timeout' ? '订单超时未支付已自动关闭。' : '该订单已被取消。' }}
      </CsAlert>

      <CsColumnLayout :columns="2" variant="default">
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
              <span class="kv-label">订单内容</span>
              <span class="kv-val">{{ formatOrderContent(order) }}</span>
            </div>
            <div class="kv-row">
              <span class="kv-label">{{ order.status === 'paid' ? '实付金额' : '应付金额' }}</span>
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
              <button
                v-if="!isRecharge && availableMethods.includes('balance')"
                type="button"
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
                <CsBox v-if="!balanceEnough" variant="small" color="text-body-secondary">
                  余额不足，还差 {{ formatMoney(diffCents) }}
                </CsBox>
              </button>

              <button
                v-if="!isRecharge && availableMethods.includes('gift_card')"
                type="button"
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
              </button>

              <button
                type="button"
                v-for="ch in (isRecharge ? channels.filter((c) => c.requires_amount && c.id !== 'balance' && c.id !== 'gift_card') : [])"
                :key="ch.id"
                class="method-card"
                :class="{ active: selectedMethod === ch.id }"
                @click="selectedMethod = ch.id"
              >
                <CsSpaceBetween direction="horizontal" size="xs" align-items="center">
                  <strong>{{ ch.name }}</strong>
                  <CsBadge color="blue">在线支付</CsBadge>
                </CsSpaceBetween>
                <CsBox v-if="ch.description" variant="small" color="text-body-secondary">
                  {{ ch.description }}
                </CsBox>
              </button>
            </div>

            <CsFormField v-if="selectedMethod === 'gift_card'" label="输入卡密">
              <CsInput v-model="giftCode" placeholder="请输入礼品卡 / 充值卡密" />
            </CsFormField>

            <CsAlert v-if="payUrl" type="info" header="已生成支付链接">
              请在新窗口完成支付；如未自动跳转，可
              <CsLink :href="payUrl" external>点击此处前往支付</CsLink>。
              支付完成后本页会自动更新订单状态。
            </CsAlert>

            <CsAlert v-if="qrCodeText" type="info" header="请扫码完成支付">
              <div class="qr-wrap">
                <img v-if="qrDataUrl" :src="qrDataUrl" alt="支付二维码" class="qr-img" />
                <code v-else class="cell-mono">{{ qrCodeText }}</code>
              </div>
            </CsAlert>

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
                    : selectedMethod === 'gift_card'
                      ? (isRecharge ? '确认兑换' : '核销卡密并支付')
                      : `立即支付 ${formatMoney(order.amount_cents)}`
                }}
              </CsButton>
              <CsButton
                v-if="!isRecharge && !balanceEnough"
                variant="normal"
                @click="rechargeModalOpen = true"
              >
                去充值
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
            <CsButton v-if="order.status === 'paid' && !isRecharge" variant="primary" @click="router.push('/subscribe')">
              查看我的订阅
            </CsButton>
            <CsButton v-if="order.status === 'paid' && isRecharge" variant="primary" @click="router.push('/account')">
              查看我的订单
            </CsButton>
            <CsButton variant="normal" @click="router.push('/shop')">
              返回套餐商店
            </CsButton>
          </CsSpaceBetween>
        </CsContainer>
      </CsColumnLayout>
    </template>

    <RechargeModal
      v-model:visible="rechargeModalOpen"
      :suggested-amount-cents="diffCents"
      @recharged="handleRecharged"
    />
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

.qr-wrap {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  inline-size: 220px;
  block-size: 220px;
  padding: 8px;
  border: 1px solid var(--x-border);
  border-radius: 8px;
  background-color: #ffffff;
}

.qr-img {
  inline-size: 100%;
  block-size: 100%;
  object-fit: contain;
}

.method-card {
  display: block;
  inline-size: 100%;
  text-align: start;
  font: inherit;
  color: inherit;
  background: transparent;
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
