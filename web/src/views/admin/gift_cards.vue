<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Plus, Refresh, Search, Delete, CopyDocument, Ticket } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
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
  name: '通用充值卡',
  face_value_yuan: 50,
  expires_at: '',
})

const resultOpen = ref(false)
const createdCards = ref<GiftCard[]>([])

function openCreate() {
  Object.assign(createForm, {
    count: 10,
    name: '通用充值卡',
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

const statusMap: Record<string, { type: 'success' | 'info' | 'danger'; text: string }> = {
  unused: { type: 'success', text: '未使用' },
  used: { type: 'info', text: '已使用' },
  disabled: { type: 'danger', text: '已作废' },
}
</script>

<template>
  <div class="x-page">
    <!-- 工具栏 -->
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-input
          v-model="keyword"
          placeholder="搜索卡密 / 批次名称"
          :prefix-icon="Search"
          clearable
          style="width: 220px"
          @keyup.enter="load"
        />
        <el-select v-model="statusFilter" placeholder="全部状态" clearable style="width: 140px" @change="load">
          <el-option label="全部状态" value="" />
          <el-option label="未使用" value="unused" />
          <el-option label="已使用" value="used" />
          <el-option label="已作废" value="disabled" />
        </el-select>
        <el-button @click="load"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
      </div>
      <el-button type="primary" @click="openCreate">
        <el-icon><Plus /></el-icon>&nbsp;批量生成充值卡
      </el-button>
    </div>

    <!-- 数据表格 -->
    <BaseCard title="充值卡密列表">
      <!-- 桌面端表格视图 (双行紧凑聚合自适应展示) -->
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <el-icon class="is-loading" style="font-size: 26px; color: var(--x-primary)"><Loading /></el-icon>
      </div>

      <div v-else-if="list.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
        <el-icon style="font-size: 32px; color: var(--x-text-3)"><Ticket /></el-icon>
        <p style="margin-top: 8px">暂无充值卡记录，点击右上角「批量生成充值卡」</p>
      </div>

      <!-- 全局统一礼品卡卡片网格流 (自适应 1~4 列) -->
      <div v-else class="gift-card-grid">
        <div v-for="row in list" :key="row.id" class="gift-card" :class="{ used: row.status === 'used' }">
          <!-- 头部 -->
          <div class="card-head">
            <div class="head-title">
              <span class="cell-mono muted" style="font-size: 11px">#{{ row.id }}</span>
              <span class="card-name">{{ row.name || '通用充值卡' }}</span>
              <el-tag :type="statusMap[row.status]?.type || 'info'" size="small">
                {{ statusMap[row.status]?.text || row.status }}
              </el-tag>
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
            <el-button size="small" type="primary" plain @click="copyText(row.code, '卡密')">
              <el-icon><CopyDocument /></el-icon>&nbsp;复制卡密
            </el-button>
            <el-button
              size="small"
              type="danger"
              plain
              :disabled="row.status === 'used'"
              @click="remove(row)"
            >
              <el-icon><Delete /></el-icon>&nbsp;作废/删除
            </el-button>
          </div>
        </div>
      </div>

      <div class="x-pager">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="size"
          :total="total"
          :page-sizes="[20, 50, 100]"
          layout="total, sizes, prev, pager, next"
          @current-change="load"
          @size-change="load"
        />
      </div>
    </BaseCard>

    <!-- 批量生成弹窗 -->
    <el-dialog v-model="createOpen" title="批量生成充值卡" width="460px">
      <el-form label-position="top">
        <el-form-item label="批次名称 / 备注">
          <el-input v-model="createForm.name" placeholder="如 新年促销50元充值卡" />
        </el-form-item>
        <div class="form-grid-2">
          <el-form-item label="生成数量（张）">
            <el-input-number v-model="createForm.count" :min="1" :max="500" style="width: 100%" />
          </el-form-item>
          <el-form-item label="单张面值（元）">
            <el-input-number v-model="createForm.face_value_yuan" :min="0.01" :step="10" :precision="2" style="width: 100%" />
          </el-form-item>
        </div>
        <el-form-item label="过期时间（选填，留空永久有效）">
          <el-date-picker
            v-model="createForm.expires_at"
            type="datetime"
            placeholder="选择过期时间"
            style="width: 100%"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="createOpen = false">取消</el-button>
        <el-button type="primary" :loading="creating" @click="submitCreate">立即生成</el-button>
      </template>
    </el-dialog>

    <!-- 生成结果弹窗 -->
    <el-dialog v-model="resultOpen" title="卡密生成成功" width="520px">
      <el-alert
        type="success"
        :closable="false"
        show-icon
        :title="`本次已成功生成 ${createdCards.length} 张面值为 ¥ ${createdCards[0]?.face_value_cents ? (createdCards[0].face_value_cents / 100).toFixed(2) : '0'} 的充值卡密：`"
        style="margin-bottom: 14px"
      />
      <el-input
        :model-value="createdCards.map((c) => c.code).join('\n')"
        type="textarea"
        :rows="8"
        readonly
        class="cell-mono"
        style="font-size: 13px"
      />
      <template #footer>
        <el-button type="primary" @click="copyAllCreated">
          <el-icon><CopyDocument /></el-icon>&nbsp;一键复制全部卡密
        </el-button>
        <el-button @click="resultOpen = false">关闭</el-button>
      </template>
    </el-dialog>
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

    .el-button {
      flex: 1;
      margin: 0;
      font-size: 12px;
      padding: 6px 8px;
      height: 30px;
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
