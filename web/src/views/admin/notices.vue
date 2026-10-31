<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from '@/components/cloudscape'
import {
  CsBadge,
  CsButton,
  CsCard,
  CsCardGrid,
  CsContainer,
  CsEmpty,
  CsForm,
  CsFormItem,
  CsInput,
  CsInputNumber,
  CsModal,
  CsOption,
  CsPage,
  CsPopover,
  CsRadio,
  CsRadioGroup,
  CsSelect,
  CsSpinner,
  CsToggle,
  CsToolbar,
  CsTooltip,
} from '@/components/cloudscape'
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
  <CsPage>
    <CsToolbar>
      <CsInput
          v-model="keyword"
          placeholder="搜索公告标题或内容..."
          clearable
          prefix-icon="search"
          style="width: 220px"
          @keyup.enter="load"
          @clear="load"
        />
        <CsSelect
          v-model="statusFilter"
          placeholder="全部状态"
          clearable
          style="width: 120px"
          @change="load"
        >
          <CsOption label="已启用" :value="1" />
          <CsOption label="已隐藏" :value="0" />
        </CsSelect>
      <CsButton icon-name="refresh" :loading="loading" @click="load">刷新</CsButton>
      <template #actions>
        <CsButton variant="primary" icon-name="add-plus" @click="openCreate">发布公告</CsButton>
      </template>
    </CsToolbar>

    <!-- 表格列表 -->
    <CsContainer header="全站公告列表">
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <CsSpinner size="large" />
      </div>

      <CsEmpty v-else-if="list.length === 0" icon-name="announcement" description="暂无公告，点击右上角「发布公告」" />

      <!-- 公告卡片网格（官方 ItemCard 口径） -->
      <CsCardGrid v-else>
        <CsCard v-for="row in list" :key="row.id" :class="{ 'notice-card--off': row.status !== 1 }">
          <template #header>
            <button
              type="button"
              class="notice-name"
              title="点击编辑公告"
              @click="openEdit(row as NoticeItem)"
            >
              {{ (row as NoticeItem).title }}
            </button>
            <div class="notice-meta">
              <span class="notice-id">#{{ row.id }}</span>
              <CsBadge v-if="(row as NoticeItem).is_pinned" color="red">置顶</CsBadge>
              <CsBadge v-if="(row as NoticeItem).is_popup" color="severity-medium">强制弹窗</CsBadge>
              <CsBadge :color="row.status === 1 ? 'green' : 'grey'">
                {{ row.status === 1 ? '已启用' : '已隐藏' }}
              </CsBadge>
            </div>
          </template>

          <template #actions>
            <CsTooltip
              :content="row.status === 1 ? '已启用，点击隐藏' : '已隐藏，点击启用'"
              placement="top"
            >
              <CsToggle
                :model-value="(row as NoticeItem).status === 1"
                :aria-label="`切换公告 ${(row as NoticeItem).title} 的启用状态`"
                @change="() => handleToggle(row as NoticeItem, 'status')"
              />
            </CsTooltip>
          </template>

          <div class="notice-content-preview">
            <span class="notice-content-text">{{ (row as NoticeItem).content }}</span>
          </div>

          <div class="notice-switches">
            <div class="notice-switch-row">
              <span class="switch-label">置顶显示</span>
              <CsToggle
                :model-value="(row as NoticeItem).is_pinned"
                aria-label="切换置顶显示"
                @change="() => handleToggle(row as NoticeItem, 'is_pinned')"
              />
            </div>
            <div class="notice-switch-row">
              <span class="switch-label">登录强制弹窗</span>
              <CsToggle
                :model-value="(row as NoticeItem).is_popup"
                aria-label="切换登录强制弹窗"
                @change="() => handleToggle(row as NoticeItem, 'is_popup')"
              />
            </div>
          </div>

          <template #footer>
            <span class="notice-updated">{{ formatDate((row as NoticeItem).updated_at) }} 更新</span>
            <CsButton icon-name="edit" @click="openEdit(row as NoticeItem)">编辑公告</CsButton>
            <CsButton icon-name="remove" @click="remove(row as NoticeItem)">删除</CsButton>
          </template>
        </CsCard>
      </CsCardGrid>
    </CsContainer>

    <!-- 发布/编辑公告对话框（支持双栏 Markdown 实时渲染预览与 4x4 表格拾取器） -->
    <CsModal
      v-model:visible="formOpen"
      :header="editing ? '编辑公告' : '发布新公告'"
      size="x-large"
    >
      <CsForm label-position="top">
        <CsFormItem label="公告标题" required>
          <CsInput v-model="form.title" placeholder="如：系统升级维护通知 / 节点新增上线" :maxlength="100" />
        </CsFormItem>

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
              <CsPopover
                v-model:visible="tablePopoverOpen"
                placement="bottom-start"
                :width="170"
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
              </CsPopover>
            </div>

            <CsInput
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
          <CsFormItem label="置顶显示">
            <div class="switch-field">
              <CsToggle v-model="form.is_pinned" />
              <span class="switch-hint">置顶公告将排在列表最前</span>
            </div>
          </CsFormItem>

          <CsFormItem label="首页强制弹窗提醒">
            <div class="switch-field">
              <CsToggle v-model="form.is_popup" />
              <span class="switch-hint">用户登录后将自动弹出该公告</span>
            </div>
          </CsFormItem>
        </div>

        <div class="form-row-grid">
          <CsFormItem label="发布状态">
            <CsRadioGroup v-model="form.status">
              <CsRadio :value="1">立即发布</CsRadio>
              <CsRadio :value="0">暂不发布</CsRadio>
            </CsRadioGroup>
          </CsFormItem>

          <CsFormItem label="排序权重">
            <CsInputNumber v-model="form.sort_order" :min="0" :max="9999" />
          </CsFormItem>
        </div>
      </CsForm>

      <template #footer>
        <CsButton @click="formOpen = false">取消</CsButton>
        <CsButton variant="primary" :loading="saving" @click="save">
          {{ editing ? '保存修改' : '确认发布' }}
        </CsButton>
      </template>
    </CsModal>
  </CsPage>
</template>

<style scoped lang="scss">
.md-editor-split {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
  margin-top: 4px;
  background: var(--color-background-cell-shaded-twxsbz);
  border: 1px solid var(--color-border-divider-default-diwd4w);
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
    border-bottom: 1px dashed var(--color-border-divider-default-diwd4w);

    .md-pane-title {
      font-size: 13px;
      font-weight: 600;
      color: var(--color-text-body-default-b2us04);
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
  background: var(--color-background-container-content-mjfil9);
  border: 1px solid var(--color-border-divider-default-diwd4w);
  border-radius: 6px;
  margin-bottom: 8px;

  .btn-group {
    display: flex;
    align-items: center;
    gap: 3px;
    padding-right: 6px;
    border-right: 1px solid var(--color-border-divider-default-diwd4w);

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
    color: var(--color-text-body-secondary-66ygbb);
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    transition: all 0.15s ease;

    &:hover {
      background: var(--color-background-cell-shaded-twxsbz);
      color: var(--color-text-link-default-qabnuk);
      border-color: rgba(99, 102, 241, 0.2);
    }

    &.font-bold {
      font-weight: 700;
    }

    &.font-italic {
      font-style: italic;
    }

    &.highlight-btn {
      color: var(--color-text-link-default-qabnuk);
      font-weight: 600;
    }
  }
}

// 4x4 网格选择器样式
.table-picker-card {
  padding: 4px;

  .table-picker-label {
    font-size: 12px;
    color: var(--color-text-body-secondary-66ygbb);
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
      border: 1px solid var(--color-border-divider-default-diwd4w);
      background: var(--color-background-cell-shaded-twxsbz);
      border-radius: 4px;
      cursor: pointer;
      transition: all 0.12s ease;

      &.cell-active {
        background: rgba(99, 102, 241, 0.25);
        border-color: var(--color-text-link-default-qabnuk);
      }
    }
  }
}

.text-primary {
  color: var(--color-text-link-default-qabnuk);
}

.md-preview-body {
  height: 350px;
  overflow-y: auto;
  padding: 12px 14px;
  background: var(--color-background-container-content-mjfil9);
  border: 1px solid var(--color-border-divider-default-diwd4w);
  border-radius: 6px;

  .preview-empty-hint {
    color: var(--color-text-body-secondary-66ygbb);
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
    color: var(--color-text-body-secondary-66ygbb);
  }
}

.text-muted {
  color: var(--color-text-body-secondary-66ygbb);
}

.text-xs {
  font-size: 12px;
}

.mt-4 {
  margin-top: 16px;
}

/* ================= 公告卡片（官方 ItemCard 口径） ================= */
.notice-name {
  appearance: none;
  border: 0;
  background: transparent;
  margin: 0;
  padding: 0;
  font: inherit;
  color: inherit;
  text-align: start;
  cursor: pointer;
  overflow-wrap: anywhere;

  &:hover {
    color: var(--color-text-link-default-qabnuk, #006ce0);
  }
}

.notice-meta {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 6px;
  margin-block-start: 4px;
}

.notice-id {
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.notice-content-preview {
  padding: 8px 10px;
  border: 1px solid var(--color-border-divider-default-diwd4w, #dedee3);
  border-radius: var(--border-radius-input-k1q34z, 8px);
  background: var(--color-background-container-content-mjfil9, #ffffff);
}

/*
 * 正文截断放在内层：外层负责内边距与边框，内层只留一个 3 行高的裁切盒。
 * 若直接在带 padding 的外层上 overflow:hidden，裁剪边界是 padding box，
 * 第 4 行会从下方内边距里露出来。
 */
.notice-content-text {
  display: block;
  max-height: 54px; /* 3 × 18px 行高 */
  overflow: hidden;
  font-size: var(--font-size-body-s, 12px);
  line-height: 18px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  white-space: pre-line;
}

.notice-switches {
  margin-block-start: 12px;
  display: flex;
  flex-wrap: wrap;
  gap: 8px 20px;
}

.notice-switch-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.switch-label {
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.notice-updated {
  margin-inline-end: auto;
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.notice-card--off {
  opacity: 0.85;
}

@media (max-width: $bp-tablet-max) {
  .md-editor-split {
    grid-template-columns: 1fr;
  }
  .form-row-grid {
    grid-template-columns: 1fr;
  }
}
</style>
