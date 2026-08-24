<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Plus, Edit, Delete, Refresh, Search } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import BaseCard from '@/components/base/BaseCard.vue'
import {
  getAdminNotices,
  createAdminNotice,
  updateAdminNotice,
  deleteAdminNotice,
  toggleAdminNotice,
} from '@/api/admin'
import type { NoticeItem, NoticePayload } from '@/api/types'
import { errMsg } from '@/api/http'
import { formatDate } from '@/utils/format'
import { renderMarkdown } from '@/utils/markdown'

const list = ref<NoticeItem[]>([])
const loading = ref(false)
const keyword = ref('')
const statusFilter = ref<number | undefined>(undefined)

async function load() {
  loading.value = true
  try {
    const params: { keyword?: string; status?: number } = {}
    if (keyword.value.trim()) params.keyword = keyword.value.trim()
    if (statusFilter.value !== undefined) params.status = statusFilter.value

    const { data } = await getAdminNotices(params)
    if (data.code === 0) {
      list.value = data.data
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载公告列表失败'))
  } finally {
    loading.value = false
  }
}

onMounted(load)

// 编辑与新建弹窗
const formOpen = ref(false)
const editing = ref(false)
const saving = ref(false)
const form = reactive<NoticePayload & { id: number }>({
  id: 0,
  title: '',
  content: '',
  is_pinned: false,
  is_popup: false,
  status: 1,
  sort_order: 0,
})

function openCreate() {
  editing.value = false
  Object.assign(form, {
    id: 0,
    title: '',
    content: '',
    is_pinned: false,
    is_popup: false,
    status: 1,
    sort_order: 0,
  })
  formOpen.value = true
}

function openEdit(row: NoticeItem) {
  editing.value = true
  Object.assign(form, {
    id: row.id,
    title: row.title,
    content: row.content,
    is_pinned: row.is_pinned,
    is_popup: row.is_popup,
    status: row.status,
    sort_order: row.sort_order,
  })
  formOpen.value = true
}

// 快捷插入 Markdown 语法辅助
function insertMarkdown(prefix: string, suffix = '') {
  const cur = form.content || ''
  form.content = cur + prefix + suffix
}

// 4x4 表格选择控件交互逻辑
const tablePopoverOpen = ref(false)
const hoverRows = ref(2)
const hoverCols = ref(2)

function onGridHover(row: number, col: number) {
  hoverRows.value = row
  hoverCols.value = col
}

function insertTable(rows: number, cols: number) {
  let tableMd = ''
  // 表头
  const headers: string[] = []
  const dividers: string[] = []
  for (let c = 1; c <= cols; c++) {
    headers.push(`列 ${c}`)
    dividers.push('---')
  }
  tableMd += `| ${headers.join(' | ')} |\n`
  tableMd += `| ${dividers.join(' | ')} |\n`

  // 数据行
  for (let r = 1; r <= rows; r++) {
    const cells: string[] = []
    for (let c = 1; c <= cols; c++) {
      cells.push(`内容 ${r}-${c}`)
    }
    tableMd += `| ${cells.join(' | ')} |\n`
  }

  const cur = form.content || ''
  form.content = (cur ? cur.trimEnd() + '\n\n' : '') + tableMd.trim() + '\n'
  tablePopoverOpen.value = false
}

async function save() {
  if (!form.title.trim() || !form.content.trim()) {
    ElMessage.warning('请填写公告标题与内容')
    return
  }

  saving.value = true
  try {
    const payload: NoticePayload = {
      title: form.title.trim(),
      content: form.content,
      is_pinned: form.is_pinned,
      is_popup: form.is_popup,
      status: form.status,
      sort_order: form.sort_order,
    }

    const { data } = editing.value
      ? await updateAdminNotice(form.id, payload)
      : await createAdminNotice(payload)

    if (data.code === 0) {
      ElMessage.success(editing.value ? '公告已更新' : '公告已发布')
      formOpen.value = false
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存公告失败'))
  } finally {
    saving.value = false
  }
}

async function handleToggle(row: NoticeItem, field: 'status' | 'is_pinned' | 'is_popup') {
  try {
    const { data } = await toggleAdminNotice(row.id, field)
    if (data.code === 0) {
      ElMessage.success('状态已更新')
      Object.assign(row, data.data)
    } else {
      ElMessage.error(data.message)
      load()
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '切换状态失败'))
    load()
  }
}

async function remove(row: NoticeItem) {
  try {
    await ElMessageBox.confirm(`确定要删除公告「${row.title}」吗？`, '删除确认', {
      type: 'warning',
      confirmButtonText: '确定删除',
      cancelButtonText: '取消',
    })
    const { data } = await deleteAdminNotice(row.id)
    if (data.code === 0) {
      ElMessage.success('公告已删除')
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch {
    /* 用户取消 */
  }
}
</script>

<template>
  <div class="x-page">
    <!-- 顶部工具栏（带标准留白与对齐） -->
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-input
          v-model="keyword"
          placeholder="搜索公告标题或内容..."
          clearable
          style="width: 220px"
          :prefix-icon="Search"
          @keyup.enter="load"
          @clear="load"
        />
        <el-select
          v-model="statusFilter"
          placeholder="全部状态"
          clearable
          style="width: 120px"
          @change="load"
        >
          <el-option label="已启用" :value="1" />
          <el-option label="已隐藏" :value="0" />
        </el-select>
        <el-button :icon="Refresh" :loading="loading" @click="load">刷新</el-button>
      </div>
      <el-button type="primary" :icon="Plus" @click="openCreate">发布公告</el-button>
    </div>

    <!-- 表格列表 -->
    <BaseCard>
      <!-- 桌面端表格视图 -->
      <div class="desktop-table-view">
        <el-table v-loading="loading" :data="list" row-key="id" stripe>
          <el-table-column prop="id" label="ID" width="70" />

          <el-table-column label="标题" min-width="200">
            <template #default="{ row }">
              <div class="notice-title-cell">
                <span v-if="(row as NoticeItem).is_pinned" class="x-chip red mr-1">
                  置顶
                </span>
                <span v-if="(row as NoticeItem).is_popup" class="x-chip orange mr-1">
                  强弹窗
                </span>
                <span class="notice-title-text">{{ (row as NoticeItem).title }}</span>
              </div>
            </template>
          </el-table-column>

          <el-table-column label="内容概要" min-width="260" show-overflow-tooltip>
            <template #default="{ row }">
              <span class="text-muted">{{ (row as NoticeItem).content }}</span>
            </template>
          </el-table-column>

          <el-table-column label="置顶" width="90" align="center">
            <template #default="{ row }">
              <el-switch
                :model-value="(row as NoticeItem).is_pinned"
                size="small"
                @change="() => handleToggle(row as NoticeItem, 'is_pinned')"
              />
            </template>
          </el-table-column>

          <el-table-column label="强弹窗" width="90" align="center">
            <template #default="{ row }">
              <el-switch
                :model-value="(row as NoticeItem).is_popup"
                size="small"
                @change="() => handleToggle(row as NoticeItem, 'is_popup')"
              />
            </template>
          </el-table-column>

          <el-table-column label="状态" width="90" align="center">
            <template #default="{ row }">
              <el-switch
                :model-value="(row as NoticeItem).status === 1"
                size="small"
                @change="() => handleToggle(row as NoticeItem, 'status')"
              />
            </template>
          </el-table-column>

          <el-table-column label="更新时间" width="170">
            <template #default="{ row }">
              <span class="font-mono text-muted text-xs">{{ formatDate((row as NoticeItem).updated_at) }}</span>
            </template>
          </el-table-column>

          <el-table-column label="操作" width="140" align="right">
            <template #default="{ row }">
              <el-button link type="primary" :icon="Edit" @click="openEdit(row as NoticeItem)">编辑</el-button>
              <el-button link type="danger" :icon="Delete" @click="remove(row as NoticeItem)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </div>

      <!-- 移动端卡片流视图 -->
      <div class="mobile-cards-view">
        <div v-if="list.length === 0" style="text-align: center; padding: 36px 0; color: var(--x-text-3); font-size: 13.5px">
          暂无公告，点击右上角「发布公告」
        </div>
        <div v-else class="mobile-data-card-list">
          <div v-for="row in list" :key="row.id" class="mobile-data-card">
            <div class="card-head">
              <div class="head-title">
                <span class="cell-mono muted" style="font-size: 11px">#{{ row.id }}</span>
                <el-tag v-if="(row as NoticeItem).is_pinned" size="small" type="danger" effect="dark">置顶</el-tag>
                <el-tag v-if="(row as NoticeItem).is_popup" size="small" type="warning" effect="dark">强弹窗</el-tag>
                <span style="font-weight: 700">{{ (row as NoticeItem).title }}</span>
              </div>
              <el-switch
                :model-value="(row as NoticeItem).status === 1"
                size="small"
                @change="() => handleToggle(row as NoticeItem, 'status')"
              />
            </div>

            <div class="card-grid">
              <div class="grid-item full-width">
                <span class="item-label">内容概要</span>
                <div class="item-value text-muted" style="font-size: 12px; display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden">
                  {{ (row as NoticeItem).content }}
                </div>
              </div>
              <div class="grid-item">
                <span class="item-label">置顶显示</span>
                <div class="item-value">
                  <el-switch
                    :model-value="(row as NoticeItem).is_pinned"
                    size="small"
                    @change="() => handleToggle(row as NoticeItem, 'is_pinned')"
                  />
                </div>
              </div>
              <div class="grid-item">
                <span class="item-label">登录强弹窗</span>
                <div class="item-value">
                  <el-switch
                    :model-value="(row as NoticeItem).is_popup"
                    size="small"
                    @change="() => handleToggle(row as NoticeItem, 'is_popup')"
                  />
                </div>
              </div>
              <div class="grid-item full-width">
                <span class="item-label">更新时间</span>
                <div class="item-value cell-mono muted" style="font-size: 11.5px">{{ formatDate((row as NoticeItem).updated_at) }}</div>
              </div>
            </div>

            <div class="card-foot-actions">
              <el-button size="small" type="primary" plain :icon="Edit" @click="openEdit(row as NoticeItem)">编辑公告</el-button>
              <el-button size="small" type="danger" plain :icon="Delete" @click="remove(row as NoticeItem)">删除</el-button>
            </div>
          </div>
        </div>
      </div>
    </BaseCard>

    <!-- 发布/编辑公告对话框（支持双栏 Markdown 实时渲染预览与 4x4 表格拾取器） -->
    <el-dialog
      v-model="formOpen"
      :title="editing ? '编辑公告' : '发布新公告'"
      width="960px"
      destroy-on-close
      class="notice-editor-dialog"
    >
      <el-form label-position="top">
        <el-form-item label="公告标题" required>
          <el-input v-model="form.title" placeholder="如：系统升级维护通知 / 节点新增上线" maxlength="100" show-word-limit />
        </el-form-item>

        <!-- 双栏 Markdown 编辑与实时预览区 -->
        <div class="md-editor-split">
          <!-- 左栏：Markdown 源码输入与工整工具条 -->
          <div class="md-pane md-edit-pane">
            <div class="md-pane-header">
              <span class="md-pane-title">Markdown 编辑</span>
              <span class="text-muted text-xs">快捷工具栏</span>
            </div>

            <!-- 工整独立的快捷按钮工具栏条 -->
            <div class="md-toolbar-strip">
              <div class="btn-group">
                <button type="button" class="tool-btn" title="一级标题" @click="insertMarkdown('# ')">H1</button>
                <button type="button" class="tool-btn" title="二级标题" @click="insertMarkdown('## ')">H2</button>
                <button type="button" class="tool-btn" title="三级标题" @click="insertMarkdown('### ')">H3</button>
              </div>

              <div class="btn-group">
                <button type="button" class="tool-btn font-bold" title="粗体" @click="insertMarkdown('**粗体**')">B</button>
                <button type="button" class="tool-btn font-italic" title="斜体" @click="insertMarkdown('*斜体*')">I</button>
                <button type="button" class="tool-btn" title="引用块" @click="insertMarkdown('> ')">“</button>
              </div>

              <div class="btn-group">
                <button type="button" class="tool-btn" title="无序列表" @click="insertMarkdown('- ')">• 列表</button>
                <button type="button" class="tool-btn" title="行内代码" @click="insertMarkdown('`code`')">`代码`</button>
                <button type="button" class="tool-btn" title="插入链接" @click="insertMarkdown('[链接文字](url)')">链接</button>
              </div>

              <!-- 4x4 自由表格选择器 -->
              <el-popover
                v-model:visible="tablePopoverOpen"
                placement="bottom-start"
                :width="170"
                trigger="click"
                popper-class="table-grid-popover"
              >
                <template #reference>
                  <button type="button" class="tool-btn highlight-btn" title="插入表格 (最大 4x4)">
                    ⊞ 表格 ▾
                  </button>
                </template>

                <div class="table-picker-card">
                  <div class="table-picker-label">
                    <span>插入表格：</span>
                    <strong class="text-primary">{{ hoverRows }} × {{ hoverCols }}</strong>
                  </div>
                  <div class="table-grid-4x4">
                    <div
                      v-for="r in 4"
                      :key="'r-' + r"
                      class="grid-row"
                    >
                      <div
                        v-for="c in 4"
                        :key="'c-' + c"
                        class="grid-cell"
                        :class="{ 'cell-active': r <= hoverRows && c <= hoverCols }"
                        @mouseenter="onGridHover(r, c)"
                        @click="insertTable(r, c)"
                      />
                    </div>
                  </div>
                </div>
              </el-popover>
            </div>

            <el-input
              v-model="form.content"
              type="textarea"
              :rows="13"
              placeholder="在此输入 Markdown 源码..."
              class="md-textarea"
            />
          </div>

          <!-- 右栏：实时渲染预览效果 -->
          <div class="md-pane md-preview-pane">
            <div class="md-pane-header">
              <span class="md-pane-title">实时渲染预览</span>
              <span class="text-muted text-xs">所见即所得</span>
            </div>
            <div
              class="md-preview-body markdown-content"
              v-html="renderMarkdown(form.content) || '<div class=\'preview-empty-hint\'>在左侧输入内容，此处将实时渲染 Markdown 视觉效果...</div>'"
            />
          </div>
        </div>

        <div class="form-row-grid mt-4">
          <el-form-item label="置顶显示">
            <div class="switch-field">
              <el-switch v-model="form.is_pinned" />
              <span class="switch-hint">置顶公告将排在列表最前</span>
            </div>
          </el-form-item>

          <el-form-item label="首页强弹窗提醒">
            <div class="switch-field">
              <el-switch v-model="form.is_popup" />
              <span class="switch-hint">用户登录后将自动弹出该公告</span>
            </div>
          </el-form-item>
        </div>

        <div class="form-row-grid">
          <el-form-item label="发布状态">
            <el-radio-group v-model="form.status">
              <el-radio :value="1">立即启用发布</el-radio>
              <el-radio :value="0">隐藏暂不显示</el-radio>
            </el-radio-group>
          </el-form-item>

          <el-form-item label="排序权重">
            <el-input-number v-model="form.sort_order" :min="0" :max="9999" />
          </el-form-item>
        </div>
      </el-form>

      <template #footer>
        <el-button @click="formOpen = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">
          {{ editing ? '保存修改' : '确认发布' }}
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.notice-title-cell {
  display: flex;
  align-items: center;
  gap: 6px;

  .notice-title-text {
    font-weight: 500;
    color: var(--x-text);
  }
}

.md-editor-split {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
  margin-top: 4px;
  background: var(--x-fill-1, #f8fafc);
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 12px;
}

.md-pane {
  display: flex;
  flex-direction: column;
  min-width: 0;

  .md-pane-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding-bottom: 6px;
    margin-bottom: 6px;
    border-bottom: 1px dashed var(--x-border);

    .md-pane-title {
      font-size: 13px;
      font-weight: 600;
      color: var(--x-text);
    }
  }
}

// 独立的紧凑工整工具栏条
.md-toolbar-strip {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  padding: 5px 8px;
  background: var(--x-card, #ffffff);
  border: 1px solid var(--x-border);
  border-radius: 6px;
  margin-bottom: 8px;

  .btn-group {
    display: flex;
    align-items: center;
    gap: 3px;
    padding-right: 6px;
    border-right: 1px solid var(--x-border-light, #e2e8f0);

    &:last-child {
      border-right: none;
      padding-right: 0;
    }
  }

  .tool-btn {
    height: 25px;
    padding: 0 6px;
    border: 1px solid transparent;
    background: transparent;
    border-radius: 4px;
    font-size: 12px;
    font-weight: 500;
    color: var(--x-text-2, #475569);
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transition: all 0.15s ease;

    &:hover {
      background: var(--x-fill-2, rgba(99, 102, 241, 0.08));
      color: var(--x-primary, #6366f1);
      border-color: rgba(99, 102, 241, 0.2);
    }

    &.font-bold {
      font-weight: 700;
    }

    &.font-italic {
      font-style: italic;
    }

    &.highlight-btn {
      color: var(--x-primary, #6366f1);
      font-weight: 600;
    }
  }
}

// 4x4 网格选择器样式
.table-picker-card {
  padding: 4px;

  .table-picker-label {
    font-size: 12px;
    color: var(--x-text-2);
    margin-bottom: 8px;
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .table-grid-4x4 {
    display: flex;
    flex-direction: column;
    gap: 4px;

    .grid-row {
      display: flex;
      gap: 4px;
    }

    .grid-cell {
      width: 26px;
      height: 26px;
      border: 1px solid var(--x-border, #cbd5e1);
      background: var(--x-fill-1, #f8fafc);
      border-radius: 4px;
      cursor: pointer;
      transition: all 0.12s ease;

      &.cell-active {
        background: rgba(99, 102, 241, 0.25);
        border-color: var(--x-primary, #6366f1);
      }
    }
  }
}

.text-primary {
  color: var(--x-primary, #6366f1);
}

.md-preview-body {
  height: 350px;
  overflow-y: auto;
  padding: 12px 14px;
  background: var(--x-card, #ffffff);
  border: 1px solid var(--x-border);
  border-radius: 6px;

  .preview-empty-hint {
    color: var(--x-text-3);
    font-size: 13px;
    padding-top: 50px;
    text-align: center;
  }
}

.form-row-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 16px;
}

.switch-field {
  display: flex;
  align-items: center;
  gap: 10px;

  .switch-hint {
    font-size: 12px;
    color: var(--x-text-3);
  }
}

.text-muted {
  color: var(--x-text-3);
}

.text-xs {
  font-size: 12px;
}

.font-mono {
  font-family: var(--x-font-mono, monospace);
}

.mr-1 {
  margin-right: 4px;
}

.mt-4 {
  margin-top: 16px;
}

@media (max-width: 768px) {
  .md-editor-split {
    grid-template-columns: 1fr;
  }
  .form-row-grid {
    grid-template-columns: 1fr;
  }
}
</style>
