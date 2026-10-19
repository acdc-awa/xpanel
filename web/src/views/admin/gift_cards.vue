<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import {
  CsAlert,
  CsBadge,
  CsButton,
  CsCard,
  CsCardFact,
  CsCardFacts,
  CsCardGrid,
  CsContainer,
  CsDatePicker,
  CsEmpty,
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
  CsSpinner,
  CsToolbar,
  ElMessage,
  ElMessageBox,
} from '@/components/cloudscape'
import {
  getAdminGiftCards,
  batchCreateGiftCards,
  deleteGiftCard,
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
  if (!t) return '—'
  return String(t).replace('T', ' ').slice(0, 16)
}

function fmtMoney(cents: number) {
  return `¥ ${(cents / 100).toFixed(2)}`
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

// ---- 删除/作废 ----
async function remove(row: any) {
  try {
    await ElMessageBox.confirm(`确认作废/删除卡密「${row.code}」？`, '作废卡密', { type: 'warning' })
  } catch {
    return
  }
  try {
    const { data } = await deleteGiftCard(row.id)
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

const statusMap: Record<string, { color: 'green' | 'grey' | 'red'; text: string }> = {
  unused: { color: 'green', text: '未使用' },
  used: { color: 'grey', text: '已使用' },
  disabled: { color: 'red', text: '已作废' },
}

// 过期不落库，现场推导；过期的未使用卡不可兑换，排序上也排在使用可用项之后
function isExpired(row: GiftCard) {
  return !!row.expires_at && new Date(row.expires_at).getTime() < Date.now()
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
        @keyup.enter="load"
      />
      <CsSelect v-model="statusFilter" placeholder="全部状态" clearable style="width: 140px" @change="load">
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
      <!-- 桌面端表格视图 (双行紧凑聚合自适应展示) -->
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <CsSpinner size="large" />
      </div>

      <CsEmpty v-else-if="list.length === 0" icon-name="ticket" description="暂无礼品卡记录，点击右上角「批量生成礼品卡」" />

      <!-- 官方 ItemCard 卡片网格：卡头 16/700，正文 14/20，面额作为卡片主指标 -->
      <CsCardGrid v-else>
        <CsCard
          v-for="row in list"
          :key="row.id"
          :class="{ 'gift-card-muted': row.status !== 'unused' || isExpired(row) }"
        >
          <template #header>
            <div class="gift-card-title">{{ row.name || '通用礼品卡' }}</div>
            <div class="gift-card-id">#{{ row.id }}</div>
          </template>
          <template #actions>
            <div class="gift-card-summary">
              <span class="gift-card-metric">{{ fmtMoney(row.face_value_cents) }}</span>
              <CsBadge v-if="row.status === 'unused' && isExpired(row)" color="red">已过期</CsBadge>
              <CsBadge v-else :color="statusMap[row.status]?.color || 'grey'">
                {{ statusMap[row.status]?.text || row.status }}
              </CsBadge>
            </div>
          </template>

          <CsCardFacts>
            <CsCardFact label="卡密（点击复制）" full-width mono>
              <button type="button" class="gift-card-code" title="点击复制卡密" @click="copyText(row.code, '卡密')">
                {{ row.code }}
              </button>
            </CsCardFact>
            <CsCardFact
              label="兑换用户"
              :value="row.used_by ? (row.used_by_username || `用户 #${row.used_by}`) : '待兑换'"
            />
            <CsCardFact label="有效期" :value="row.expires_at ? fmtTime(row.expires_at) : '永久有效'" mono />
            <CsCardFact v-if="row.used_at" label="兑换时间" :value="fmtTime(row.used_at)" mono full-width />
          </CsCardFacts>

          <template #footer>
            <CsButton icon-name="copy" @click="copyText(row.code, '卡密')">复制卡密</CsButton>
            <CsButton icon-name="remove" :disabled="row.status === 'used'" @click="remove(row)">
              作废/删除
            </CsButton>
          </template>
        </CsCard>
      </CsCardGrid>

      <template #footer>
        <CsPaginationBar>
          <CsPagination
            v-model:current-page="page"
            v-model:page-size="size"
            :total="total"
            :page-sizes="[20, 50, 100]"
            layout="total, sizes, prev, pager, next"
            @current-change="load"
            @size-change="load"
          />
        </CsPaginationBar>
      </template>
    </CsContainer>

    <!-- 批量生成弹窗 -->
    <CsModal v-model:visible="createOpen" header="批量生成礼品卡" size="small">
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
    <CsModal v-model:visible="resultOpen" header="礼品卡生成成功" size="medium">
      <CsAlert type="success" style="margin-bottom: 14px">
        本次已成功生成 {{ createdCards.length }} 张面值为 ¥
        {{ createdCards[0]?.face_value_cents ? (createdCards[0].face_value_cents / 100).toFixed(2) : '0' }}
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
.gift-card-title {
  overflow-wrap: anywhere;
}

.gift-card-id {
  margin-block-start: 2px;
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: 12px;
  line-height: 16px;
  font-weight: 400;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.gift-card-summary {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 6px;
}

.gift-card-metric {
  font-size: 18px;
  line-height: 22px;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  color: var(--color-text-status-success-1i2jf8, #00802f);
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
  overflow-wrap: anywhere;

  &:hover {
    text-decoration: underline;
  }
}

.gift-card-muted .gift-card-code {
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.gift-card-muted .gift-card-metric {
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

</style>
