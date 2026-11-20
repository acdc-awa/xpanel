<script setup lang="ts">
import { formatMoney, formatMoneyNumber } from '@/utils/format'
import { formatDateTimeMinute } from '@/utils/timezone'
import { computed, onMounted, reactive, ref } from 'vue'
import {
  CsAlert,
  CsButton,
  CsContainer,
  CsDatePicker,
  CsForm,
  CsFormItem,
  CsInput,
  CsInputNumber,
  CsModal,
  CsOption,
  CsPage,
  CsPagination,
  CsPaginationBar,
  CsSelect,
  CsTable,
  CsToolbar,
  ElMessage,
  ElMessageBox,
} from '@/components/cloudscape'
import type { TableColumn } from '@/components/cloudscape'
import {
  getAdminGiftCards,
  batchCreateGiftCards,
  voidGiftCard,
  purgeGiftCard,
} from '@/api/gift_card'
import { errMsg } from '@/api/http'
import type { GiftCard } from '@/api/types'

const list = ref<GiftCard[]>([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const size = ref(20)
const statusFilter = ref('')
const keyword = ref('')

function fmtTime(t: string | null) {
  return t ? formatDateTimeMinute(t) : '—'
}

function fmtMoney(cents: number) {
  return formatMoney(cents)
}

async function load() {
  loading.value = true
  try {
    const { data } = await getAdminGiftCards({
      page: page.value,
      size: size.value,
      status: statusFilter.value || undefined,
      search: keyword.value || undefined,
    })
    if (data.code === 0) {
      list.value = data.data.items
      total.value = data.data.total
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载礼品卡失败'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

// ---- 批量生成 ----
const createOpen = ref(false)
const creating = ref(false)
const createForm = reactive({
  count: 10,
  name: '通用礼品卡',
  face_value_yuan: 50,
  expires_at: '',
})

const resultOpen = ref(false)
const createdCards = ref<GiftCard[]>([])

function openCreate() {
  Object.assign(createForm, {
    count: 10,
    name: '通用礼品卡',
    face_value_yuan: 50,
    expires_at: '',
  })
  createOpen.value = true
}

async function submitCreate() {
  if (createForm.count < 1 || createForm.count > 500) {
    ElMessage.warning('生成数量需在 1~500 之间')
    return
  }
  if (createForm.face_value_yuan <= 0) {
    ElMessage.warning('面值必须大于 0')
    return
  }
  creating.value = true
  try {
    const payload = {
      count: createForm.count,
      name: createForm.name,
      face_value_cents: Math.round(createForm.face_value_yuan * 100),
      expires_at: createForm.expires_at ? new Date(createForm.expires_at).toISOString() : undefined,
    }
    const { data } = await batchCreateGiftCards(payload)
    if (data.code === 0) {
      ElMessage.success(`成功生成 ${data.data.count} 张礼品卡`)
      createOpen.value = false
      createdCards.value = data.data.items
      resultOpen.value = true
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '生成失败'))
  } finally {
    creating.value = false
  }
}

async function copyText(text: string, label: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`${label}已复制`)
  } catch {
    ElMessage.warning('复制失败，请手动复制')
  }
}

function copyAllCreated() {
  const codes = createdCards.value.map((c) => c.code).join('\n')
  copyText(codes, '全部卡密')
}

// ---- 作废 / 彻底删除 ----
async function voidCard(row: any) {
  try {
    await ElMessageBox.confirm(
      `确认作废卡密「${row.code}」？作废后不可兑换，记录保留可追溯。`,
      '作废卡密',
      { type: 'warning' },
    )
  } catch {
    return
  }
  try {
    const { data } = await voidGiftCard(row.id)
    if (data.code === 0) {
      ElMessage.success('已作废')
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '作废失败'))
  }
}

async function purgeCard(row: any) {
  try {
    await ElMessageBox.confirm(
      `确认彻底删除卡密「${row.code}」？删除后不可恢复，历史记录一并消失。`,
      '彻底删除',
      { type: 'error', confirmButtonText: '删除', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  try {
    const { data } = await purgeGiftCard(row.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

function onFilterChange() {
  page.value = 1
  load()
}

const emptyText = computed(() =>
  statusFilter.value || keyword.value.trim()
    ? '未找到符合条件的礼品卡记录'
    : '暂无礼品卡记录，点击右上角「批量生成礼品卡」',
)

const giftCardColumns: TableColumn[] = [
  { id: 'name', header: '批次名称' },
  { id: 'code', header: '卡密（点击复制）' },
  { id: 'face_value', header: '面值' },
  { id: 'status', header: '状态' },
  { id: 'used_by', header: '兑换用户' },
  { id: 'expires_at', header: '有效期' },
  { id: 'used_at', header: '兑换时间' },
  { id: 'actions', header: '操作' },
]

function statusOf(row: GiftCard): { className: string; text: string } {
  if (row.status === 'unused' && isExpired(row)) {
    return { className: 'gc-status-danger', text: '已过期' }
  }
  if (row.status === 'unused') return { className: 'gc-status-unused', text: '未使用' }
  if (row.status === 'used') return { className: 'gc-status-used', text: '已使用' }
  if (row.status === 'disabled') return { className: 'gc-status-danger', text: '已作废' }
  return { className: 'gc-status-used', text: row.status }
}

// 过期不落库，现场推导；过期的未使用卡不可兑换，排序上也排在使用可用项之后
function isExpired(row: GiftCard) {
  return !!row.expires_at && new Date(row.expires_at).getTime() < Date.now()
}

function isMutedRow(row: GiftCard) {
  return row.status !== 'unused' || isExpired(row)
}
</script>

<template>
  <CsPage>
    <CsToolbar>
      <CsInput
        v-model="keyword"
        placeholder="搜索卡密 / 批次名称"
        prefix-icon="search"
        clearable
        style="width: 220px"
        @keyup.enter="onFilterChange"
        @clear="onFilterChange"
      />
      <CsSelect v-model="statusFilter" placeholder="全部状态" clearable style="width: 140px" @change="onFilterChange">
        <CsOption label="全部状态" value="" />
        <CsOption label="未使用" value="unused" />
        <CsOption label="已使用" value="used" />
        <CsOption label="已作废" value="disabled" />
      </CsSelect>
      <CsButton icon-name="refresh" @click="load">刷新</CsButton>
      <template #actions>
        <CsButton variant="primary" icon-name="add-plus" @click="openCreate">批量生成礼品卡</CsButton>
      </template>
    </CsToolbar>

    <!-- 数据表格 -->
    <CsContainer header="卡密列表">
      <CsTable
        compact
        :wrap-lines="false"
        :items="list"
        :columns="giftCardColumns"
        :loading="loading"
        loading-text="加载中…"
        track-by="id"
        :empty-text="emptyText"
      >
        <template #cell-name="{ row }">
          <span :class="{ 'cell-muted': isMutedRow(row) }">{{ row.name || '通用礼品卡' }}</span>
        </template>
        <template #cell-code="{ row }">
          <button
            type="button"
            class="gift-card-code cell-mono"
            :class="{ 'is-muted': isMutedRow(row) }"
            title="点击复制卡密"
            @click="copyText(row.code, '卡密')"
          >
            {{ row.code }}
          </button>
        </template>
        <template #cell-face_value="{ row }">
          <span class="cell-mono" :class="{ 'cell-muted': isMutedRow(row) }">
            <strong>{{ fmtMoney(row.face_value_cents) }}</strong>
          </span>
        </template>
        <template #cell-status="{ row }">
          <span :class="statusOf(row).className">{{ statusOf(row).text }}</span>
        </template>
        <template #cell-used_by="{ row }">
          <span v-if="row.used_by" :class="{ 'cell-mono': !!row.used_by_email }">
            {{ row.used_by_email || `用户 #${row.used_by}` }}
          </span>
          <span v-else class="cell-muted">待兑换</span>
        </template>
        <template #cell-expires_at="{ row }">
          <span v-if="row.expires_at" class="cell-mono" :class="{ 'cell-muted': isMutedRow(row) }">
            {{ fmtTime(row.expires_at) }}
          </span>
          <span v-else class="cell-muted">永久有效</span>
        </template>
        <template #cell-used_at="{ row }">
          <span v-if="row.used_at" class="cell-mono">{{ fmtTime(row.used_at) }}</span>
          <span v-else class="cell-muted">—</span>
        </template>
        <template #cell-actions="{ row }">
          <CsButton
            v-if="row.status === 'unused'"
            variant="danger-outline"
            :wrap-text="false"
            @click="voidCard(row)"
          >
            作废
          </CsButton>
          <CsButton
            v-else-if="row.status === 'disabled'"
            variant="danger-outline"
            :wrap-text="false"
            @click="purgeCard(row)"
          >
            彻底删除
          </CsButton>
          <span v-else class="cell-muted">—</span>
        </template>
      </CsTable>

      <template #footer>
        <CsPaginationBar v-if="total > 0">
          <CsPagination
            v-model:current-page="page"
            v-model:page-size="size"
            :total="total"
            :page-sizes="[20, 50, 100]"
            layout="total, sizes, prev, pager, next"
            simple
            @current-change="load"
            @size-change="() => { page = 1; load() }"
          />
        </CsPaginationBar>
      </template>
    </CsContainer>

    <!-- 批量生成弹窗 -->
    <CsModal v-model:visible="createOpen" header="批量生成礼品卡" size="small" :close-on-click-overlay="false">
      <CsForm label-position="top">
        <CsFormItem label="批次名称 / 备注">
          <CsInput v-model="createForm.name" placeholder="如 新年促销50元礼品卡" />
        </CsFormItem>
        <div class="form-grid-2">
          <CsFormItem label="生成数量（张）">
            <CsInputNumber v-model="createForm.count" :min="1" :max="500" style="width: 100%" />
          </CsFormItem>
          <CsFormItem label="单张面值（元）">
            <CsInputNumber v-model="createForm.face_value_yuan" :min="0.01" :step="10" :precision="2" style="width: 100%" />
          </CsFormItem>
        </div>
        <CsFormItem label="过期时间（选填，留空永久有效）">
          <CsDatePicker
            v-model="createForm.expires_at"
            type="datetime"
            placeholder="选择过期时间"
            style="width: 100%"
          />
        </CsFormItem>
      </CsForm>
      <template #footer>
        <CsButton @click="createOpen = false">取消</CsButton>
        <CsButton variant="primary" :loading="creating" @click="submitCreate">立即生成</CsButton>
      </template>
    </CsModal>

    <!-- 生成结果弹窗 -->
    <CsModal v-model:visible="resultOpen" header="礼品卡生成成功" size="medium" :close-on-click-overlay="false">
      <CsAlert type="success" style="margin-bottom: 14px">
        本次已成功生成 {{ createdCards.length }} 张面值为 ¥
        {{ formatMoneyNumber(createdCards[0]?.face_value_cents || 0) }}
        的卡密：
      </CsAlert>
      <CsInput
        :model-value="createdCards.map((c) => c.code).join('\n')"
        type="textarea"
        :rows="8"
        read-only
        class="cs-text-mono"
        style="font-size: 13px"
      />
      <template #footer>
        <CsButton variant="primary" icon-name="copy" @click="copyAllCreated">一键复制全部卡密</CsButton>
        <CsButton @click="resultOpen = false">关闭</CsButton>
      </template>
    </CsModal>
  </CsPage>
</template>

<style scoped lang="scss">
.cell-muted {
  color: var(--color-text-status-inactive-fqjc97, #656871);
}

.gc-status-unused {
  color: var(--color-text-status-success-1i2jf8, #00802f);
}

.gc-status-used {
  color: var(--color-text-status-inactive-fqjc97, #656871);
}

.gc-status-danger {
  color: var(--color-text-status-error-ri38gt, #d91515);
}

.gift-card-code {
  appearance: none;
  border: 0;
  background: transparent;
  margin: 0;
  padding: 0;
  font: inherit;
  font-weight: 600;
  color: var(--color-text-link-default-qabnuk, #006ce0);
  cursor: pointer;
  text-align: start;

  &:hover {
    text-decoration: underline;
  }

  &.is-muted {
    color: var(--color-text-status-inactive-fqjc97, #656871);
  }
}
</style>
