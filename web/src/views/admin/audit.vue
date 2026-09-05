<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Refresh, Loading, DocumentCopy, Search } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import { getAuditLogs, type AuditLog } from '@/api/admin'
import { getActionMeta, renderAudit, type AuditView } from '@/utils/auditRender'
import { errMsg } from '@/api/http'

const list = ref<AuditLog[]>([])
const total = ref(0)
const loading = ref(false)
const page = ref(1)
const size = ref(20)

const category = ref('')
const keyword = ref('')

async function load() {
  loading.value = true
  try {
    const { data } = await getAuditLogs(page.value, size.value, {
      category: category.value || undefined,
      keyword: keyword.value.trim() || undefined,
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

// 卡片内字段表最多展示条数，超出折叠进弹窗
const CARD_FIELD_LIMIT = 6

const detailModalOpen = ref(false)
const detailTab = ref<'translated' | 'raw'>('translated')
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
        <el-select v-model="category" placeholder="全部分类" clearable style="width: 130px" @change="onFilterChange">
          <el-option label="全部分类" value="" />
          <el-option label="节点管理" value="servers" />
          <el-option label="用户管理" value="users" />
          <el-option label="财务套餐" value="billing" />
          <el-option label="入站证书" value="inbounds" />
          <el-option label="系统设置" value="settings" />
          <el-option label="身份认证" value="auth" />
        </el-select>
        <el-input
          v-model="keyword"
          placeholder="搜索操作内容、IP、路由..."
          clearable
          style="width: 240px"
          @keyup.enter="onFilterChange"
          @clear="onFilterChange"
        >
          <template #prefix><el-icon><Search /></el-icon></template>
        </el-input>
        <el-button type="primary" @click="onFilterChange"><el-icon><Search /></el-icon>&nbsp;查询</el-button>
        <el-button @click="resetFilters">重置</el-button>
        <el-button @click="load"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
      </div>
    </div>

    <BaseCard title="系统审计日志">
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <el-icon class="is-loading" style="font-size: 26px; color: var(--x-primary)"><Loading /></el-icon>
      </div>

      <div v-else-if="rows.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
        <el-icon style="font-size: 32px; color: var(--x-text-3)"><DocumentCopy /></el-icon>
        <p style="margin-top: 8px">暂无审计日志</p>
      </div>

      <!-- 审计日志卡片网格流（自适应 1~4 列）：摘要 + 字段表 + 大文本预览 -->
      <div v-else class="audit-card-grid">
        <div v-for="rv in rows" :key="rv.row.id" class="audit-card">
          <!-- 头部 -->
          <div class="card-head">
            <div class="head-title">
              <span class="x-chip" :class="rv.row.operator_type === 'admin' ? 'orange' : rv.row.operator_type === 'system' ? 'gray' : 'blue'" style="font-size: 10px; padding: 1px 5px">
                {{ rv.row.operator_type === 'admin' ? '管理员' : rv.row.operator_type === 'system' ? '系统' : '用户' }}
                {{ rv.row.operator_type === 'system' ? '' : rv.row.operator_username ? rv.row.operator_username : `#${rv.row.operator_id}` }}
              </span>
              <el-tag :type="rv.categoryColor" size="small" effect="plain" style="font-size: 11px">
                {{ rv.categoryName }}
              </el-tag>
              <span class="action-name">{{ rv.title }}</span>
            </div>
            <code class="cell-mono muted" style="font-size: 11px">{{ rv.row.ip || '—' }}</code>
          </div>

          <!-- 动作原始技术标识 -->
          <div class="action-code-bar">
            <span class="action-code cell-mono" :title="rv.row.action">{{ rv.row.action }}</span>
          </div>

          <!-- 翻译视图：一句话摘要 + 字段表 + 大文本预览 -->
          <div class="card-grid">
            <div class="grid-item full-width">
              <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 2px">
                <span class="item-label">操作内容</span>
                <el-button type="primary" link size="small" style="font-size: 11px; padding: 0" @click="openDetail(rv.view)">
                  展开 / 原文
                </el-button>
              </div>
              <div class="audit-summary" :title="rv.view.summary">{{ rv.view.summary || '—' }}</div>
              <div v-if="rv.view.fields.length" class="audit-fields">
                <div v-for="(f, i) in rv.view.fields.slice(0, CARD_FIELD_LIMIT)" :key="i" class="audit-field-row">
                  <span class="af-label">{{ f.label }}</span>
                  <span class="af-value" :class="{ 'cell-mono': f.mono }">{{ f.value }}</span>
                </div>
                <el-button
                  v-if="rv.view.fields.length > CARD_FIELD_LIMIT"
                  type="primary"
                  link
                  size="small"
                  style="font-size: 11px; padding: 0"
                  @click="openDetail(rv.view)"
                >
                  全部 {{ rv.view.fields.length }} 项…
                </el-button>
              </div>
              <div v-if="rv.view.texts.length" class="audit-text-chips">
                <span v-for="(t, i) in rv.view.texts" :key="i" class="audit-text-chip" :title="t.preview" @click="openDetail(rv.view)">
                  {{ t.title }} · {{ t.meta }}
                </span>
              </div>
            </div>
            <div class="grid-item full-width">
              <span class="item-label">记录时间</span>
              <div class="item-value cell-mono muted font-11">{{ fmtTime(rv.row.created_at) }}</div>
            </div>
          </div>
        </div>
      </div>

      <div class="x-pager">
        <el-pagination
          v-model:current-page="page"
          v-model:page-size="size"
          :total="total"
          :page-sizes="[10, 20, 50]"
          layout="total, sizes, prev, pager, next"
          @current-change="load"
          @size-change="load"
        />
      </div>
    </BaseCard>

    <!-- 详情弹窗：翻译视图 / 原文 -->
    <el-dialog v-model="detailModalOpen" title="操作详情" width="640px" append-to-body>
      <el-tabs v-if="currentView" v-model="detailTab">
        <el-tab-pane label="翻译视图" name="translated">
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
        </el-tab-pane>
        <el-tab-pane label="原文" name="raw">
          <pre class="audit-pre">{{ currentView.raw }}</pre>
        </el-tab-pane>
      </el-tabs>
      <template #footer>
        <el-button @click="detailModalOpen = false">关 闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: var(--x-font-mono); font-size: 12.5px; color: var(--x-text-2); }
.muted { color: var(--x-text-3); }

/* ================= 全局统一审计日志卡片网格流 ================= */
.audit-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}

.audit-card {
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

    .action-name {
      font-weight: 600;
      font-size: 13px;
      color: var(--x-text, #111827);
    }
  }

  .action-code-bar {
    padding: 6px 0 2px;
    .action-code {
      font-size: 11px;
      color: var(--x-text-3);
      background: var(--x-fill-2, #f8fafc);
      padding: 2px 6px;
      border-radius: 4px;
      border: 1px solid var(--x-border-light, #f1f5f9);
    }
  }

  .card-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px 12px;
    padding: 10px 0 2px;

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
}

/* 翻译视图：摘要一句话 */
.audit-summary {
  font-size: 13px;
  font-weight: 600;
  color: var(--x-text, #111827);
  line-height: 1.5;
  word-break: break-all;
  background: var(--x-bg, #f9fafb);
  padding: 6px 8px;
  border-radius: 6px;
  border: 1px solid var(--x-border-light, #f3f4f6);
  display: -webkit-box;
  -webkit-line-clamp: 2;
  -webkit-box-orient: vertical;
  overflow: hidden;
}

/* 翻译视图：字段表 */
.audit-fields {
  margin-top: 6px;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.audit-field-row {
  display: flex;
  gap: 8px;
  align-items: baseline;
  font-size: 12px;
  line-height: 1.5;

  .af-label {
    flex: 0 0 auto;
    min-width: 64px;
    color: var(--x-text-3, #9ca3af);
  }

  .af-value {
    flex: 1 1 auto;
    color: var(--x-text-2, #374151);
    word-break: break-all;
  }
}

/* 翻译视图：大文本预览 chip（模板/正文/PEM） */
.audit-text-chips {
  margin-top: 6px;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.audit-text-chip {
  font-size: 11px;
  color: var(--x-primary);
  background: var(--x-fill-2, #f1f5f9);
  border: 1px dashed var(--x-border, #e5e7eb);
  border-radius: 4px;
  padding: 2px 8px;
  cursor: pointer;

  &:hover {
    border-color: var(--x-primary);
  }
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

@media (max-width: 640px) {
  .audit-card-grid {
    grid-template-columns: 1fr;
  }
}
</style>
