<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { formatMoney, formatOrderContent } from '@/utils/format'
import { formatDateTimeMinute } from '@/utils/timezone'
import { toast } from '@/components/cloudscape'
import {
  CsBadge,
  CsButton,
  CsContainer,
  CsInput,
  CsOption,
  CsPage,
  CsPagination,
  CsPaginationBar,
  CsSelect,
  CsTable,
  CsToolbar,
} from '@/components/cloudscape'
import type { BadgeColor, TableColumn } from '@/components/cloudscape'
import { getOrders, type Order } from '@/api/admin'
import { errMsg } from '@/api/http'

const list = ref<Order[]>([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const size = ref(10)
const filterStatus = ref('')
const filterOrderType = ref('')
const keyword = ref('')

async function load() {
  loading.value = true
  try {
    const { data } = await getOrders(
      page.value,
      size.value,
      filterStatus.value || undefined,
      keyword.value.trim() || undefined,
      filterOrderType.value || undefined,
    )
    if (data.code === 0) {
      list.value = data.data.items
      total.value = data.data.total
    } else {
      toast.error(data.message)
    }
  } catch (e) {
    toast.error(errMsg(e, '加载订单失败'))
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
  filterOrderType.value = ''
  keyword.value = ''
  page.value = 1
  load()
}

onMounted(load)

const hasFilters = computed(() =>
  Boolean(filterStatus.value || filterOrderType.value || keyword.value.trim()),
)

const emptyText = computed(() =>
  hasFilters.value ? '未找到符合条件的订单记录' : '暂无订购记录',
)

const orderColumns: TableColumn[] = [
  { id: 'id', header: '#ID' },
  { id: 'plan_name', header: '订单内容' },
  { id: 'email', header: '订购用户' },
  { id: 'status', header: '状态' },
  { id: 'amount', header: '金额' },
  { id: 'payment_method', header: '支付方式' },
  { id: 'created_at', header: '创建时间' },
  { id: 'paid_at', header: '支付时间' },
  { id: 'order_no', header: '订单号' },
]

const orderStatusMap: Record<string, { color: BadgeColor; text: string }> = {
  pending: { color: 'severity-medium', text: '待支付' },
  paid: { color: 'green', text: '已支付' },
  cancelled: { color: 'grey', text: '已取消' },
}

function fmtTime(t: string | null) {
  return t ? formatDateTimeMinute(t) : '—'
}

function orderHasAmountChange(row: Order) {
  return row.status === 'paid' && row.amount_cents > 0
}

function formatOrderAmount(row: Order) {
  if (row.amount_cents === 0) return formatMoney(0)
  const prefix = row.order_type === 'recharge' ? '+' : '-'
  return `${prefix}${formatMoney(row.amount_cents)}`
}

function orderMethodLabel(row: Order) {
  if (row.payment_method === 'balance') return '余额'
  if (row.payment_method === 'gift_card') return '卡密'
  return row.payment_channel || row.payment_method || '—'
}
</script>

<template>
  <CsPage>
    <CsToolbar>
      <CsSelect
          v-model="filterOrderType"
          placeholder="全部类型"
          clearable
          style="width: min(100%, 130px)"
          @change="onFilterChange"
        >
          <CsOption label="全部类型" value="" />
          <CsOption label="套餐订单" value="plan" />
          <CsOption label="充值订单" value="recharge" />
        </CsSelect>
      <CsSelect
          v-model="filterStatus"
          placeholder="全部状态"
          clearable
          style="width: min(100%, 140px)"
          @change="onFilterChange"
        >
          <CsOption label="全部状态" value="" />
          <CsOption label="已支付" value="paid" />
          <CsOption label="待支付" value="pending" />
          <CsOption label="已取消" value="cancelled" />
        </CsSelect>
        <CsInput
          v-model="keyword"
          placeholder="搜索流水号 / 邮箱..."
          clearable
          prefix-icon="search"
          style="width: min(100%, 220px)"
          @keyup.enter="onFilterChange"
          @clear="onFilterChange"
        />
        <CsButton v-if="hasFilters" @click="resetFilters">重置</CsButton>
      <CsButton icon-name="refresh" @click="load">刷新</CsButton>
    </CsToolbar>

    <CsContainer header="服务订购记录">
      <CsTable
        compact
        :wrap-lines="false"
        :items="list"
        :columns="orderColumns"
        :loading="loading"
        loading-text="加载中…"
        track-by="id"
        :empty-text="emptyText"
      >
        <template #cell-id="{ row }">
          <span class="cell-mono order-id">#{{ row.id }}</span>
        </template>
        <template #cell-plan_name="{ row }">
          <span>{{ formatOrderContent(row) }}</span>
        </template>
        <template #cell-email="{ row }">
          <span v-if="row.email" class="cell-mono">{{ row.email }}</span>
          <span v-else class="cell-muted">—</span>
        </template>
        <template #cell-status="{ row }">
          <CsBadge class="nowrap-badge" :color="orderStatusMap[row.status]?.color ?? 'grey'">
            {{ orderStatusMap[row.status]?.text ?? row.status }}
          </CsBadge>
        </template>
        <template #cell-amount="{ row }">
          <span
            class="cell-mono"
            :class="orderHasAmountChange(row) ? 'amount-changed' : 'amount-unchanged'"
          >
            <strong>{{ formatOrderAmount(row) }}</strong>
          </span>
        </template>
        <template #cell-payment_method="{ row }">
          <span v-if="orderMethodLabel(row) !== '—'">{{ orderMethodLabel(row) }}</span>
          <span v-else class="cell-muted">—</span>
        </template>
        <template #cell-created_at="{ row }">
          <span class="cell-mono">{{ fmtTime(row.created_at) }}</span>
        </template>
        <template #cell-paid_at="{ row }">
          <span v-if="row.paid_at" class="cell-mono">{{ fmtTime(row.paid_at) }}</span>
          <span v-else class="cell-muted">—</span>
        </template>
        <template #cell-order_no="{ row }">
          <code class="cell-mono">{{ row.order_no }}</code>
        </template>
      </CsTable>

      <template #footer>
        <CsPaginationBar v-if="total > 0">
          <CsPagination
            v-model:current-page="page"
            v-model:page-size="size"
            :total="total"
            :page-sizes="[10, 20, 50, 100]"
            layout="total, sizes, prev, pager, next"
            simple
            @current-change="load"
            @size-change="() => { page = 1; load() }"
          />
        </CsPaginationBar>
      </template>
    </CsContainer>
  </CsPage>
</template>

<style scoped lang="scss">
.order-id {
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.cell-muted {
  color: var(--color-text-status-inactive-fqjc97, #656871);
}

.amount-changed {
  color: var(--color-text-link-default-qabnuk, #0972d3);
}

.amount-unchanged {
  color: var(--color-text-body-default-b2us04, #0f141a);
}
</style>
