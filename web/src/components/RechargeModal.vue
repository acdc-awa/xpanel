<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import {
  CsAlert,
  CsButton,
  CsFormField,
  CsInput,
  CsModal,
  CsSpaceBetween,
  toast,
} from '@/components/cloudscape'
import { createRecharge, getPaymentChannels } from '@/api/shop'
import { errMsg } from '@/api/http'
import type { PaymentChannelInfo } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { formatMoney } from '@/utils/format'

const props = withDefaults(
  defineProps<{
    visible: boolean
    suggestedAmountCents?: number
  }>(),
  {
    suggestedAmountCents: 0,
  },
)

const emit = defineEmits<{
  (e: 'update:visible', val: boolean): void
  (e: 'recharged'): void
}>()

const router = useRouter()
const auth = useAuthStore()

const loadingChannels = ref(false)
const submitting = ref(false)
const channels = ref<PaymentChannelInfo[]>([])
const selectedChannelId = ref('')
const cardCode = ref('')
const amountYuan = ref('50')

const presetAmounts = [10, 50, 100, 200]

const selectedChannel = computed(
  () => channels.value.find((c) => c.id === selectedChannelId.value) ?? null,
)

const modalVisible = computed({
  get: () => props.visible,
  set: (v: boolean) => emit('update:visible', v),
})

async function loadChannels() {
  loadingChannels.value = true
  try {
    const { data } = await getPaymentChannels('recharge')
    if (data.code === 0) {
      channels.value = data.data.items
      if (
        channels.value.length > 0 &&
        !channels.value.some((c) => c.id === selectedChannelId.value)
      ) {
        selectedChannelId.value = channels.value[0].id
      }
    }
  } catch {
    toast.error('加载充值渠道失败')
  } finally {
    loadingChannels.value = false
  }
}

watch(
  () => props.visible,
  (open) => {
    if (open) {
      cardCode.value = ''
      if (props.suggestedAmountCents > 0) {
        amountYuan.value = (Math.ceil(props.suggestedAmountCents) / 100).toFixed(2)
      }
      loadChannels()
    }
  },
)

function parseAmountCents(): number {
  const num = Number(amountYuan.value)
  if (!Number.isFinite(num) || num <= 0) return 0
  return Math.round(num * 100)
}

async function handleSubmit(replacePending = false) {
  const ch = selectedChannel.value
  if (!ch) return

  if (!ch.requires_amount) {
    const code = cardCode.value.trim()
    if (!code) {
      toast.warning('请输入充值卡密')
      return
    }
    submitting.value = true
    try {
      const { data } = await createRecharge({ channel: ch.id, code })
      if (data.code === 0) {
        const face = data.data.face_value_cents ?? data.data.order?.amount_cents ?? 0
        toast.success(`充值成功，已到账 ${formatMoney(face)}`)
        cardCode.value = ''
        modalVisible.value = false
        await auth.fetchMe()
        emit('recharged')
      } else {
        toast.error(data.message)
      }
    } catch (e: unknown) {
      toast.error(errMsg(e, '充值失败'))
    } finally {
      submitting.value = false
    }
    return
  }

  const cents = parseAmountCents()
  if (cents <= 0) {
    toast.warning('请输入有效的充值金额')
    return
  }
  submitting.value = true
  try {
    const { data } = await createRecharge({
      channel: ch.id,
      amount_cents: cents,
      replace_pending: replacePending,
    })
    if (data.code === 0) {
      modalVisible.value = false
      router.push(`/checkout/${data.data.order.order_no}`)
    } else {
      toast.error(data.message)
    }
  } catch (err: unknown) {
    const resp = (err as { response?: { status?: number; data?: { pending_order_no?: string; message?: string } } })
      ?.response
    if (resp?.status === 409 && resp.data?.pending_order_no && !replacePending) {
      modalVisible.value = false
      toast.info('您有一笔待支付充值订单，已为您跳转')
      router.push(`/checkout/${resp.data.pending_order_no}`)
      return
    }
    toast.error(errMsg(err, '创建充值订单失败'))
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <CsModal
    v-model:visible="modalVisible"
    header="账户充值"
    size="small"
    :close-on-click-overlay="false"
  >
    <CsSpaceBetween direction="vertical" size="m">
      <div v-if="loadingChannels" class="recharge-loading">正在加载充值渠道...</div>

      <CsAlert v-else-if="channels.length === 0" type="warning" header="暂无可用充值渠道">
        管理员尚未启用任何充值渠道，请稍后再试。
      </CsAlert>

      <template v-else>
        <CsFormField label="选择充值方式">
          <div class="channel-grid">
            <button
              v-for="ch in channels"
              :key="ch.id"
              type="button"
              class="channel-option"
              :class="{ active: selectedChannelId === ch.id }"
              @click="selectedChannelId = ch.id"
            >
              <span class="channel-name">{{ ch.name }}</span>
              <span v-if="ch.description" class="channel-desc">{{ ch.description }}</span>
            </button>
          </div>
        </CsFormField>

        <template v-if="selectedChannel && !selectedChannel.requires_amount">
          <CsFormField label="充值卡密" description="核销后将按卡密面值即时计入账户余额并生成充值订单。">
            <CsInput
              v-model="cardCode"
              placeholder="请输入 16 位充值卡密"
              class="cell-mono"
              @keyup.enter="handleSubmit(false)"
            />
          </CsFormField>
        </template>

        <template v-else-if="selectedChannel && selectedChannel.requires_amount">
          <CsFormField label="充值金额（元）">
            <CsSpaceBetween direction="vertical" size="xs">
              <div class="preset-row">
                <button
                  v-for="amt in presetAmounts"
                  :key="amt"
                  type="button"
                  class="preset-chip"
                  :class="{ active: Number(amountYuan) === amt }"
                  @click="amountYuan = String(amt)"
                >
                  ¥{{ amt }}
                </button>
              </div>
              <CsInput
                v-model="amountYuan"
                type="number"
                placeholder="请输入充值金额"
                class="cell-mono"
                @keyup.enter="handleSubmit(false)"
              />
            </CsSpaceBetween>
          </CsFormField>
        </template>
      </template>
    </CsSpaceBetween>

    <template #footer>
      <CsSpaceBetween direction="horizontal" size="xs">
        <CsButton @click="modalVisible = false">取消</CsButton>
        <CsButton
          variant="primary"
          :disabled="!selectedChannel || loadingChannels"
          :loading="submitting"
          @click="handleSubmit(false)"
        >
          {{ selectedChannel && !selectedChannel.requires_amount ? '立即兑换' : '创建充值订单' }}
        </CsButton>
      </CsSpaceBetween>
    </template>
  </CsModal>
</template>

<style scoped>
.recharge-loading {
  color: var(--color-text-body-secondary-66ygbb, #5f6b7a);
  font-size: 13px;
  padding: 12px 0;
}

.channel-grid {
  display: grid;
  grid-template-columns: 1fr;
  gap: 8px;
}

.channel-option {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  width: 100%;
  padding: 10px 12px;
  border: 1px solid var(--color-border-divider-default-diwd4w, #d5dbdb);
  border-radius: 6px;
  background: var(--color-background-container-content-mjfil9, #ffffff);
  color: var(--color-text-body-default-b2us04, #0f141a);
  text-align: left;
  cursor: pointer;
  transition: border-color 0.15s, background-color 0.15s;
}

.channel-option.active {
  border-color: var(--color-border-item-selected-7104rh, #0972d3);
  background: var(--color-background-item-selected-dotkup, rgba(9, 114, 211, 0.08));
}

.channel-name {
  font-weight: 600;
  font-size: 14px;
}

.channel-desc {
  font-size: 12px;
  color: var(--color-text-body-secondary-66ygbb, #5f6b7a);
}

.preset-row {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.preset-chip {
  padding: 4px 12px;
  border-radius: 4px;
  border: 1px solid var(--color-border-divider-default-diwd4w, #d5dbdb);
  background: var(--color-background-container-content-mjfil9, #ffffff);
  color: var(--color-text-body-default-b2us04, #0f141a);
  font-family: var(--font-family-monospace, monospace);
  font-size: 13px;
  cursor: pointer;
}

.preset-chip.active {
  border-color: var(--color-border-item-selected-7104rh, #0972d3);
  background: var(--color-background-item-selected-dotkup, rgba(9, 114, 211, 0.12));
  color: var(--color-text-link-default-qabnuk, #0972d3);
}
</style>
