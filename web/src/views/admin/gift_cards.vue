<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import {
  CsAlert,
  CsBadge,
  CsButton,
  CsContainer,
  CsDatePicker,
  CsForm,
  CsFormItem,
  CsIcon,
  CsInput,
  CsInputNumber,
  CsModal,
  CsOption,
  CsPagination,
  CsSelect,
  CsSpinner,
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
  <div class="x-page">
    <!-- 工具栏 -->
    <div class="x-toolbar">
      <div class="x-toolbar-left">
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
      </div>
      <CsButton variant="primary" icon-name="add-plus" @click="openCreate">批量生成礼品卡</CsButton>
    </div>

    <!-- 数据表格 -->
    <CsContainer header="卡密列表">
      <!-- 桌面端表格视图 (双行紧凑聚合自适应展示) -->
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <CsSpinner size="large" />
      </div>

      <div v-else-if="list.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
        <CsIcon name="ticket" size="large" />
        <p style="margin-top: 8px">暂无礼品卡记录，点击右上角「批量生成礼品卡」</p>
      </div>

      <!-- 全局统一礼品卡卡片网格流 (自适应 1~4 列) -->
      <div v-else class="gift-card-grid">
        <div v-for="row in list" :key="row.id" class="gift-card" :class="{ used: row.status === 'used' }">
          <!-- 头部 -->
          <div class="card-head">
            <div class="head-title">
              <span class="cell-mono muted" style="font-size: 11px">#{{ row.id }}</span>
              <span class="card-name">{{ row.name || '通用礼品卡' }}</span>
              <CsBadge v-if="row.status === 'unused' && isExpired(row)" color="red">已过期</CsBadge>
              <CsBadge v-else :color="statusMap[row.status]?.color || 'grey'">
                {{ statusMap[row.status]?.text || row.status }}
              </CsBadge>
            </div>
            <span class="cell-mono font-14" style="font-weight: 700; color: #059669">
              {{ fmtMoney(row.face_value_cents) }}
            </span>
          </div>

          <!-- 卡密与属性网格 -->
          <div class="card-grid">
            <div class="grid-item full-width">
              <span class="item-label">卡密（点击复制）</span>
              <div class="item-value">
                <code
                  class="cell-mono font-12"
                  style="cursor: pointer; color: var(--x-primary); font-weight: 600"
                  title="点击复制卡密"
                  @click="copyText(row.code, '卡密')"
                >
                  {{ row.code }}
                </code>
              </div>
            </div>
            <div class="grid-item">
              <span class="item-label">兑换用户</span>
              <div class="item-value cell-mono">
                {{ row.used_by ? (row.used_by_username || `用户 #${row.used_by}`) : '待兑换' }}
              </div>
            </div>
            <div class="grid-item">
              <span class="item-label">有效期</span>
              <div class="item-value cell-mono font-11">
                {{ row.expires_at ? fmtTime(row.expires_at) : '永久有效' }}
              </div>
            </div>
            <div v-if="row.used_at" class="grid-item full-width">
              <span class="item-label">兑换时间</span>
              <div class="item-value cell-mono muted font-11">{{ fmtTime(row.used_at) }}</div>
            </div>
          </div>

          <!-- 底部操作栏 -->
          <div class="card-foot-actions">
            <CsButton icon-name="copy" @click="copyText(row.code, '卡密')">复制卡密</CsButton>
            <CsButton
              icon-name="remove"
              :disabled="row.status === 'used'"
              @click="remove(row)"
            >
              作废/删除
            </CsButton>
          </div>
        </div>
      </div>

      <div class="x-pager">
        <CsPagination
          v-model:current-page="page"
          v-model:page-size="size"
          :total="total"
          :page-sizes="[20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          @current-change="load"
          @size-change="load"
        />
      </div>
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
        class="cell-mono"
        style="font-size: 13px"
      />
      <template #footer>
        <CsButton variant="primary" icon-name="copy" @click="copyAllCreated">一键复制全部卡密</CsButton>
        <CsButton @click="resultOpen = false">关闭</CsButton>
      </template>
    </CsModal>
  </div>
</template>

<style scoped lang="scss">
/* ================= 全局统一礼品卡卡片网格流 ================= */
.gift-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(300px, 1fr));
  gap: 14px;
}

.gift-card {
  background: var(--x-card, #ffffff);
  border: 1px solid var(--x-border, #e5e7eb);
  border-radius: var(--x-radius, 10px);
  padding: 14px;
  transition: all 0.2s cubic-bezier(0.2, 0, 0, 1);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  display: flex;
  flex-direction: column;
  justify-content: space-between;

  &:hover {
    border-color: var(--x-border-hover, #cbd5e1);
    box-shadow: var(--x-shadow-md);
    transform: translateY(-1px);
  }

  &.used {
    opacity: 0.75;
    background: var(--x-fill-2, rgba(0, 0, 0, 0.02));
  }

  .card-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding-bottom: 10px;
    border-bottom: 1px dashed var(--x-border, #e5e7eb);

    .head-title {
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
    }

    .card-name {
      font-weight: 600;
      font-size: 13.5px;
      color: var(--x-text, #111827);
    }
  }

  .card-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px 12px;
    padding: 10px 0;

    .grid-item {
      display: flex;
      flex-direction: column;
      gap: 2px;

      &.full-width {
        grid-column: 1 / -1;
      }

      .item-label {
        font-size: 11px;
        color: var(--x-text-3, #9ca3af);
      }

      .item-value {
        font-size: 12.5px;
        color: var(--x-text, #1f2937);
        font-weight: 500;
      }
    }
  }

  .card-foot-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    padding-top: 10px;
    border-top: 1px solid var(--x-border-light, #f1f5f9);
    margin-top: 6px;

    button {
      flex: 1;
      margin: 0;
    }
  }
}

.x-pager {
  padding: 14px 0 0;
  margin-top: 16px;
  display: flex;
  justify-content: flex-end;
  border-top: 1px solid var(--x-border-light, #f1f5f9);
}
.muted {
  color: var(--x-text-3);
}

@media (max-width: 640px) {
  .gift-card-grid {
    grid-template-columns: 1fr;
  }
}
</style>
