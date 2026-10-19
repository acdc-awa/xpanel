<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import {
  CsBadge,
  CsButton,
  CsCard,
  CsCardGrid,
  CsContainer,
  CsDatePicker,
  CsIcon,
  CsInput,
  CsModal,
  CsOption,
  CsPagination,
  CsSelect,
  CsSpinner,
  CsTabs,
} from '@/components/cloudscape'
import type { TabItem } from '@/components/cloudscape'
import { getAuditLogs, type AuditLog } from '@/api/admin'
import { getActionMeta, renderAudit, type AuditView } from '@/utils/auditRender'
import { errMsg } from '@/api/http'
import { ElMessage } from '@/components/cloudscape'

const list = ref<AuditLog[]>([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const size = ref(20)

const category = ref('')
const keyword = ref('')
const operatorType = ref('')
const startTime = ref<string | null>(null)
const endTime = ref<string | null>(null)

async function load() {
  loading.value = true
  try {
    const { data } = await getAuditLogs(page.value, size.value, {
      category: category.value || undefined,
      keyword: keyword.value.trim() || undefined,
      operator_type: operatorType.value || undefined,
      start_time: startTime.value ? new Date(startTime.value).toISOString() : undefined,
      end_time: endTime.value ? new Date(endTime.value).toISOString() : undefined,
    })
    if (data.code === 0) {
      list.value = data.data.items
      total.value = data.data.total
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载审计日志失败'))
  } finally {
    loading.value = false
  }
}

function onFilterChange() {
  page.value = 1
  load()
}

function resetFilters() {
  category.value = ''
  keyword.value = ''
  operatorType.value = ''
  startTime.value = null
  endTime.value = null
  page.value = 1
  load()
}

onMounted(load)

// 翻译层：每行一次解析（摘要 + 字段表 + 大文本预览 + 分类元信息）
interface RowView {
  row: AuditLog
  view: AuditView
  categoryName: string
  categoryColor: 'primary' | 'success' | 'warning' | 'danger' | 'info'
  title: string
}
const rows = computed<RowView[]>(() =>
  list.value.map((row) => {
    const view = renderAudit(row.action, row.detail)
    const meta = getActionMeta(row.action, view.method, row.detail)
    return { row, view, categoryName: meta.categoryName, categoryColor: meta.categoryColor, title: meta.title }
  }),
)

const CATEGORY_BADGE: Record<RowView['categoryColor'], 'blue' | 'green' | 'red' | 'grey'> = {
  primary: 'blue',
  info: 'grey',
  success: 'green',
  warning: 'grey',
  danger: 'red',
}

// 卡片内字段表最多展示条数，超出折叠进弹窗
const CARD_FIELD_LIMIT = 6

const detailModalOpen = ref(false)
const detailTab = ref('translated')
const detailTabs: TabItem[] = [
  { id: 'translated', label: '翻译视图' },
  { id: 'raw', label: '原文' },
]
const currentView = ref<AuditView | null>(null)
function openDetail(view: AuditView) {
  if (!view.raw && !view.fields.length && !view.texts.length) return
  currentView.value = view
  detailTab.value = 'translated'
  detailModalOpen.value = true
}

function fmtTime(t: string) {
  return t ? t.replace('T', ' ').slice(0, 19) : ''
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left" style="display: flex; gap: 10px; align-items: center; flex-wrap: wrap;">
        <CsSelect v-model="category" placeholder="全部分类" clearable style="width: 140px" @change="onFilterChange">
          <CsOption label="全部分类" value="" />
          <CsOption label="服务器管理" value="servers" />
          <CsOption label="用户管理" value="users" />
          <CsOption label="财务套餐" value="billing" />
          <CsOption label="入站证书" value="inbounds" />
          <CsOption label="系统设置" value="settings" />
          <CsOption label="身份认证" value="auth" />
        </CsSelect>
        <CsSelect v-model="operatorType" placeholder="操作人类型" clearable style="width: 140px" @change="onFilterChange">
          <CsOption label="全部类型" value="" />
          <CsOption label="管理员" value="admin" />
          <CsOption label="普通用户" value="user" />
          <CsOption label="系统任务" value="system" />
        </CsSelect>
        <div class="range-filter">
          <CsDatePicker
            v-model="startTime"
            type="datetimerange"
            placeholder="开始时间"
            style="width: 165px"
            @change="onFilterChange"
          />
          <span class="range-sep">至</span>
          <CsDatePicker
            v-model="endTime"
            type="datetimerange"
            placeholder="结束时间"
            style="width: 165px"
            @change="onFilterChange"
          />
        </div>
        <CsInput
          v-model="keyword"
          placeholder="搜索操作内容、IP、路由..."
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

    <CsContainer header="系统审计日志">
      <div v-if="loading" class="state-block">
        <CsSpinner size="large" />
      </div>

      <div v-else-if="rows.length === 0" class="state-block state-empty">
        <CsIcon name="file" size="large" />
        <p>暂无审计日志</p>
      </div>

      <!-- 审计日志卡片网格（官方 ItemCard）：摘要 + 字段表 + 大文本预览 -->
      <CsCardGrid v-else>
        <CsCard v-for="rv in rows" :key="rv.row.id">
          <template #header>
            <div class="audit-head">
              <span class="action-name">{{ rv.title }}</span>
              <div class="audit-meta">
                <CsBadge :color="CATEGORY_BADGE[rv.categoryColor]">{{ rv.categoryName }}</CsBadge>
                <CsBadge :color="rv.row.operator_type === 'admin' ? 'severity-medium' : rv.row.operator_type === 'system' ? 'grey' : 'blue'">
                  {{ rv.row.operator_type === 'admin' ? '管理员' : rv.row.operator_type === 'system' ? '系统' : '用户' }}
                </CsBadge>
                <span v-if="rv.row.operator_type !== 'system'" class="audit-operator">
                  {{ rv.row.operator_username || `#${rv.row.operator_id}` }}
                </span>
              </div>
              <code class="action-code">{{ rv.row.action }}</code>
            </div>
          </template>

          <template #actions>
            <code class="audit-ip">{{ rv.row.ip || '—' }}</code>
          </template>

          <div class="audit-summary" :title="rv.view.summary">{{ rv.view.summary || '—' }}</div>

          <div v-if="rv.view.fields.length" class="audit-fields">
            <div v-for="(f, i) in rv.view.fields.slice(0, CARD_FIELD_LIMIT)" :key="i" class="audit-field-row">
              <span class="af-label">{{ f.label }}</span>
              <span class="af-value" :class="{ 'af-value--mono': f.mono }">{{ f.value }}</span>
            </div>
          </div>

          <div v-if="rv.view.texts.length" class="audit-text-chips">
            <span v-for="(t, i) in rv.view.texts" :key="i" class="audit-text-chip" :title="t.preview" @click="openDetail(rv.view)">
              {{ t.title }} · {{ t.meta }}
            </span>
          </div>

          <template #footer>
            <span class="audit-time">{{ fmtTime(rv.row.created_at) }}</span>
            <CsButton variant="inline-link" @click="openDetail(rv.view)">
              {{ rv.view.fields.length > CARD_FIELD_LIMIT ? `全部 ${rv.view.fields.length} 项 / 原文` : '展开 / 原文' }}
            </CsButton>
          </template>
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

    <!-- 详情弹窗：翻译视图 / 原文 -->
    <CsModal v-model:visible="detailModalOpen" header="操作详情" size="large">
      <CsTabs v-if="currentView" v-model="detailTab" :tabs="detailTabs">
        <div v-if="detailTab === 'translated'">
          <div class="audit-modal-summary">{{ currentView.summary || '—' }}</div>
          <div v-if="currentView.fields.length" class="modal-fields">
            <div v-for="(f, i) in currentView.fields" :key="i" class="modal-field-row">
              <span class="mf-label">{{ f.label }}</span>
              <span class="mf-value" :class="{ 'cell-mono': f.mono }">{{ f.value }}</span>
            </div>
          </div>
          <div v-for="(t, i) in currentView.texts" :key="i" class="modal-text">
            <div class="mt-head">
              {{ t.title }}<span class="mt-meta">{{ t.meta }}</span>
            </div>
            <pre class="audit-pre">{{ t.preview }}</pre>
          </div>
          <div v-if="!currentView.fields.length && !currentView.texts.length" style="color: var(--x-text-3); font-size: 12.5px; padding: 8px 0">
            无结构化字段，可切到「原文」查看。
          </div>
        </div>
        <div v-else>
          <pre class="audit-pre">{{ currentView.raw }}</pre>
        </div>
      </CsTabs>
      <template #footer>
        <CsButton @click="detailModalOpen = false">关闭</CsButton>
      </template>
    </CsModal>
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: var(--x-font-mono); font-size: 12.5px; color: var(--x-text-2); }
.muted { color: var(--x-text-3); }

.range-filter {
  display: flex;
  align-items: center;
  gap: 6px;
}

.range-sep {
  font-size: 13px;
  color: var(--x-text-3);
}

.state-block {
  padding: 48px 0;
  text-align: center;
}

.state-empty {
  color: var(--x-text-3);
  font-size: 13.5px;

  p {
    margin-top: 8px;
  }
}

/* ================= 审计日志卡片（官方 ItemCard 口径） ================= */
.audit-head {
  min-inline-size: 0;
}

.action-name {
  display: block;
  font-size: var(--font-size-heading-s-hm1hv8, 16px);
  line-height: var(--line-height-heading-s-3z7qjv, 20px);
  font-weight: 700;
  color: var(--color-text-body-default-b2us04, #0f141a);
  overflow-wrap: anywhere;
}

.audit-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  margin-block-start: 4px;
}

.audit-operator {
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  overflow-wrap: anywhere;
}

.action-code {
  display: inline-block;
  margin-block-start: 4px;
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  overflow-wrap: anywhere;
}

.audit-ip {
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  white-space: nowrap;
}

/* 翻译视图：摘要一句话 */
.audit-summary {
  padding: 8px 10px;
  border: 1px solid var(--color-border-divider-default-diwd4w, #dedee3);
  border-radius: var(--border-radius-input-k1q34z, 8px);
  background: var(--color-background-container-content-mjfil9, #ffffff);
  font-size: var(--font-size-body-s, 12px);
  line-height: 18px;
  color: var(--color-text-body-default-b2us04, #0f141a);
  overflow-wrap: anywhere;
  display: -webkit-box;
  -webkit-line-clamp: 3;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* 翻译视图：字段表 */
.audit-fields {
  margin-block-start: 10px;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.audit-field-row {
  display: flex;
  gap: 8px;
  align-items: baseline;
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;

  .af-label {
    flex: 0 0 auto;
    min-width: 64px;
    color: var(--color-text-body-secondary-66ygbb, #656871);
  }

  .af-value {
    flex: 1 1 auto;
    color: var(--color-text-body-default-b2us04, #0f141a);
    overflow-wrap: anywhere;
  }

  .af-value--mono {
    font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  }
}

/* 翻译视图：大文本预览 chip（模板/正文/PEM） */
.audit-text-chips {
  margin-block-start: 10px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.audit-text-chip {
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-link-default-qabnuk, #006ce0);
  border: 1px dashed var(--color-border-divider-default-diwd4w, #dedee3);
  border-radius: var(--border-radius-input-k1q34z, 8px);
  padding: 2px 8px;
  cursor: pointer;

  &:hover {
    border-color: var(--color-text-link-default-qabnuk, #006ce0);
  }
}

.audit-time {
  margin-inline-end: auto;
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.x-pager {
  display: flex;
  justify-content: flex-end;
  padding: 14px 0 0;
  margin-top: 16px;
  border-top: 1px solid var(--x-border-light, #f1f5f9);
}

/* 详情弹窗：翻译视图 */
.audit-modal-summary {
  font-size: 13.5px;
  font-weight: 600;
  color: var(--x-text);
  padding: 2px 0 10px;
  word-break: break-all;
}

.modal-fields {
  display: flex;
  flex-direction: column;
  border: 1px solid var(--x-border-light, #f1f5f9);
  border-radius: 8px;
  overflow: hidden;
}

.modal-field-row {
  display: flex;
  gap: 10px;
  padding: 6px 10px;
  font-size: 12.5px;
  line-height: 1.5;

  &:nth-child(odd) {
    background: var(--x-bg, #f9fafb);
  }

  .mf-label {
    flex: 0 0 128px;
    color: var(--x-text-3, #9ca3af);
  }

  .mf-value {
    flex: 1;
    color: var(--x-text-2, #374151);
    word-break: break-all;
  }
}

.modal-text {
  margin-top: 10px;

  .mt-head {
    font-size: 12px;
    color: var(--x-text-2);
    margin-bottom: 4px;
    display: flex;
    align-items: baseline;
    gap: 8px;

    .mt-meta {
      color: var(--x-text-3);
      font-size: 11px;
    }
  }
}

.audit-pre {
  background: var(--x-fill-2, #f1f5f9);
  padding: 12px 14px;
  border-radius: 8px;
  font-family: var(--x-font-mono, monospace);
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--x-text);
  white-space: pre-wrap;
  word-break: break-all;
  max-height: 380px;
  overflow-y: auto;
  margin: 0;
}
</style>
