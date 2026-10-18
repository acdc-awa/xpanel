<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { ElMessage } from '@/components/cloudscape'
import {
  CsBadge,
  CsButton,
  CsCard,
  CsCardFact,
  CsCardFacts,
  CsCardGrid,
  CsContainer,
  CsIcon,
  CsInput,
  CsOption,
  CsPagination,
  CsSelect,
  CsSpinner,
} from '@/components/cloudscape'
import { getOrders, type Order } from '@/api/admin'
import { errMsg } from '@/api/http'

const list = ref<Order[]>([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const size = ref(20)
const filterStatus = ref('')
const keyword = ref('')

async function load() {
  loading.value = true
  try {
    const { data } = await getOrders(
      page.value,
      size.value,
      filterStatus.value || undefined,
      keyword.value.trim() || undefined,
    )
    if (data.code === 0) {
      list.value = data.data.items
      total.value = data.data.total
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载订单失败'))
  } finally {
    loading.value = false
  }
}

function onFilterChange() {
  page.value = 1
  load()
}

function resetFilters() {
  filterStatus.value = ''
  keyword.value = ''
  page.value = 1
  load()
}

onMounted(load)

function fmtTime(t: string | null) {
  if (!t) return '—'
  return String(t).replace('T', ' ').slice(0, 16)
}

function statusOf(row: Order): { color: 'green' | 'grey' | 'severity-medium'; text: string } {
  if (row.status === 'paid') return { color: 'green', text: '已生效' }
  if (row.status === 'pending') return { color: 'severity-medium', text: '待支付' }
  return { color: 'grey', text: '已取消' }
}

function amountClassOf(row: Order) {
  if (row.status === 'paid') return 'order-amount--paid'
  if (row.status === 'pending') return 'order-amount--pending'
  return 'order-amount--muted'
}

function paymentLabel(row: Order) {
  return row.payment_method === 'balance' ? '余额支付' : row.payment_method || '—'
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left" style="display: flex; gap: 10px; align-items: center; flex-wrap: wrap">
        <CsSelect
          v-model="filterStatus"
          placeholder="全部状态"
          clearable
          style="width: 140px"
          @change="onFilterChange"
        >
          <CsOption label="全部状态" value="" />
          <CsOption label="已生效 (paid)" value="paid" />
          <CsOption label="待支付 (pending)" value="pending" />
          <CsOption label="已取消 (cancelled)" value="cancelled" />
        </CsSelect>
        <CsInput
          v-model="keyword"
          placeholder="搜索流水号 / 用户名..."
          clearable
          prefix-icon="search"
          style="width: 220px"
          @keyup.enter="onFilterChange"
          @clear="onFilterChange"
        />
        <CsButton variant="primary" icon-name="search" @click="onFilterChange">查询</CsButton>
        <CsButton @click="resetFilters">重置</CsButton>
        <CsButton icon-name="refresh" @click="load">刷新</CsButton>
      </div>
    </div>

    <CsContainer header="服务订购记录">
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <CsSpinner size="large" />
      </div>

      <div v-else-if="list.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
        <CsIcon name="ticket" size="large" />
        <p style="margin-top: 8px">{{ filterStatus || keyword ? '未找到符合条件的订单记录' : '暂无订购记录' }}</p>
      </div>

      <!-- 官方 ItemCard 卡片网格：卡头 16/700，金额作为卡片主指标 -->
      <CsCardGrid v-else>
        <CsCard v-for="row in list" :key="row.id">
          <template #header>
            <div class="order-plan">{{ row.plan_name }}</div>
            <div class="order-meta">
              <span class="order-id">#{{ row.id }}</span>
              <CsBadge :color="statusOf(row).color">{{ statusOf(row).text }}</CsBadge>
            </div>
          </template>
          <template #actions>
            <span class="order-amount" :class="amountClassOf(row)">
              ¥ {{ (row.amount_cents / 100).toFixed(2) }}
            </span>
          </template>

          <CsCardFacts>
            <CsCardFact label="订购用户" :value="row.username || '—'" />
            <CsCardFact label="支付方式" :value="paymentLabel(row)" />
            <CsCardFact label="订单流水号" :value="row.order_no" full-width mono />
            <CsCardFact label="创建时间" :value="fmtTime(row.created_at)" mono />
            <CsCardFact label="支付时间" :value="row.paid_at ? fmtTime(row.paid_at) : '—'" mono />
          </CsCardFacts>
        </CsCard>
      </CsCardGrid>

      <div class="x-pager">
        <CsPagination
          v-model:current-page="page"
          v-model:page-size="size"
          :total="total"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          @current-change="load"
          @size-change="() => { page = 1; load() }"
        />
      </div>
    </CsContainer>
  </div>
</template>

<style scoped lang="scss">
.order-plan {
  overflow-wrap: anywhere;
}

.order-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  margin-block-start: 4px;
}

.order-id {
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: 12px;
  line-height: 16px;
  font-weight: 400;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.order-amount {
  font-size: 18px;
  line-height: 22px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.order-amount--paid {
  color: var(--color-text-status-success-1i2jf8, #00802f);
}

.order-amount--pending {
  color: var(--color-text-body-default-b2us04, #0f141a);
}

.order-amount--muted {
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.x-pager {
  display: flex;
  justify-content: flex-end;
  padding: 14px 0 0;
  margin-top: 16px;
  border-top: 1px solid var(--x-border-light, #f1f5f9);
}
</style>
