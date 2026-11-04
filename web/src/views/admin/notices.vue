<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from '@/components/cloudscape'
import {
  CsBadge,
  CsButton,
  CsCard,
  CsCardGrid,
  CsContainer,
  CsEmpty,
  CsFormField,
  CsIcon,
  CsInput,
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
  reorderNotices,
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
      orderSnapshot = [...data.data]
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

// 编辑与新建弹窗（置顶/弹窗/排序都在列表卡片与拖拽里维护，不再进表单）
const formOpen = ref(false)
const editing = ref(false)
const saving = ref(false)
const form = reactive({
  id: 0,
  title: '',
  content: '',
  status: 1,
})

function openCreate() {
  editing.value = false
  Object.assign(form, { id: 0, title: '', content: '', status: 1 })
  formOpen.value = true
}

function openEdit(row: NoticeItem) {
  editing.value = true
  Object.assign(form, {
    id: row.id,
    title: row.title,
    content: row.content,
    status: row.status,
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
    // 只提交表单里有的字段：置顶/弹窗/排序由卡片与拖拽维护，后端缺省即不改。
    const payload: NoticePayload = {
      title: form.title.trim(),
      content: form.content,
      status: form.status,
    }

    const { data } = editing.value
      ? await updateAdminNotice(form.id, payload)
      : await createAdminNotice(payload)

    if (data.code === 0) {
      ElMessage.success(editing.value ? '公告已保存' : '公告已创建')
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
      // 置顶会改变分组顺序（置顶始终排在最前），必须重新取列表
      if (field === 'is_pinned') {
        load()
        return
      }
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

// ---- 排序（拖拽）----
// 唯一写入点：拖完一次性提交全量顺序（数组下标即 sort_order）。
// 搜索/筛选时禁用——那时「插到哪」没有确定语义。
const canReorder = computed(
  () => !keyword.value.trim() && statusFilter.value === undefined && list.value.length > 1,
)

const draggingId = ref<number | null>(null)
let orderSnapshot: NoticeItem[] = []
let savingOrder = false

function cardElOf(target: EventTarget | null): HTMLElement | null {
  return (target as HTMLElement | null)?.closest('[data-notice-id]') as HTMLElement | null
}

/**
 * 置顶公告在管理端与用户端都固定排在最前，拖拽只能在自己的分组内换位。
 * rest 是「已摘掉被拖项」的列表，pinnedCount 即置顶组可用下标的上界。
 */
function clampToPinnedGroup(rest: NoticeItem[], insertAt: number, moved: NoticeItem): number {
  const pinnedCount = rest.filter((n) => n.is_pinned).length
  return moved.is_pinned ? Math.min(insertAt, pinnedCount) : Math.max(insertAt, pinnedCount)
}

function onDragStart(e: DragEvent) {
  if (!canReorder.value || !(e.target as HTMLElement | null)?.closest('[data-drag-handle]')) {
    e.preventDefault()
    return
  }
  const card = cardElOf(e.target)
  if (!card) return
  draggingId.value = Number(card.getAttribute('data-notice-id'))
  orderSnapshot = [...list.value]
  if (e.dataTransfer) {
    e.dataTransfer.effectAllowed = 'move'
    e.dataTransfer.setData('text/plain', String(draggingId.value))
    // 整卡跟随鼠标，而不是只拖一个小抓手
    e.dataTransfer.setDragImage(card, 24, 24)
  }
}

function onDragOver(e: DragEvent) {
  if (draggingId.value === null) return
  e.preventDefault()
  if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
  const card = cardElOf(e.target)
  if (!card) return
  const targetId = Number(card.getAttribute('data-notice-id'))
  if (!targetId || targetId === draggingId.value) return
  const from = list.value.findIndex((n) => n.id === draggingId.value)
  if (from < 0) return
  const rect = card.getBoundingClientRect()
  const after = e.clientX > rect.left + rect.width / 2
  const rest = [...list.value]
  const [moved] = rest.splice(from, 1)
  let insertAt = rest.findIndex((n) => n.id === targetId)
  if (insertAt < 0) return
  if (after) insertAt += 1
  insertAt = clampToPinnedGroup(rest, insertAt, moved)
  rest.splice(insertAt, 0, moved)
  // 只在顺序真的变了才赋值，避免每次 dragover 都触发整树渲染
  if (rest.some((n, i) => n.id !== list.value[i]?.id)) list.value = rest
}

function onDrop(e: DragEvent) {
  if (draggingId.value === null) return
  e.preventDefault()
  void persistOrder()
}

function onDragEnd() {
  draggingId.value = null
  void persistOrder()
}

async function persistOrder() {
  if (savingOrder) return
  const ids = list.value.map((n) => n.id)
  if (ids.join(',') === orderSnapshot.map((n) => n.id).join(',')) return
  savingOrder = true
  try {
    const { data } = await reorderNotices(ids)
    if (data.code !== 0) throw new Error(data.message || '保存排序失败')
  } catch (e) {
    list.value = orderSnapshot
    ElMessage.error(errMsg(e, '保存排序失败，已恢复原顺序'))
  } finally {
    savingOrder = false
    orderSnapshot = [...list.value]
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
      <CsSelect v-model="statusFilter" placeholder="全部状态" clearable style="width: 120px" @change="load">
        <CsOption label="已发布" :value="1" />
        <CsOption label="草稿" :value="0" />
      </CsSelect>
      <CsButton icon-name="refresh" :loading="loading" @click="load">刷新</CsButton>
      <template #actions>
        <CsButton variant="primary" icon-name="add-plus" @click="openCreate">新建公告</CsButton>
      </template>
    </CsToolbar>

    <!-- 卡片列表（拖拽排序，与服务器卡片同一套交互） -->
    <CsContainer header="全站公告列表">
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <CsSpinner size="large" />
      </div>

      <CsEmpty v-else-if="list.length === 0" icon-name="announcement" description="暂无公告，点击右上角「新建公告」" />

      <CsCardGrid
        v-else
        :min-width="380"
        @dragstart="onDragStart"
        @dragover="onDragOver"
        @drop="onDrop"
        @dragend="onDragEnd"
      >
        <CsCard
          v-for="row in list"
          :key="row.id"
          class="notice-card"
          :class="{
            'notice-card--off': row.status !== 1,
            'notice-card--dragging': draggingId === row.id,
          }"
          :data-notice-id="row.id"
        >
          <template #header>
            <div class="notice-head">
              <span
                v-if="canReorder"
                class="notice-drag-handle"
                draggable="true"
                data-drag-handle
                title="拖拽调整公告顺序"
              >
                <CsIcon name="drag-indicator" />
              </span>
              <button type="button" class="notice-name" title="点击编辑公告" @click="openEdit(row)">
                {{ row.title }}
              </button>
              <CsBadge v-if="row.is_pinned" color="red">置顶</CsBadge>
              <CsBadge v-if="row.is_popup" color="severity-medium">强制弹窗</CsBadge>
              <CsBadge :color="row.status === 1 ? 'green' : 'grey'">
                {{ row.status === 1 ? '已发布' : '草稿' }}
              </CsBadge>
            </div>
          </template>

          <template #actions>
            <CsTooltip
              :content="row.status === 1 ? '已发布（用户可见），点击转为草稿' : '草稿（仅管理员可见），点击发布'"
              placement="top"
            >
              <CsToggle
                :model-value="row.status === 1"
                :aria-label="`切换公告 ${row.title} 的发布状态`"
                @change="() => handleToggle(row, 'status')"
              />
            </CsTooltip>
          </template>

          <!-- 渲染后的正文预览（3 行裁切），不是 Markdown 原文 -->
          <div class="notice-content-preview markdown-content" v-html="renderMarkdown(row.content)" />

          <div class="notice-switches">
            <div class="notice-switch-row">
              <span class="switch-label">置顶显示</span>
              <CsToggle
                :model-value="row.is_pinned"
                aria-label="切换置顶显示"
                @change="() => handleToggle(row, 'is_pinned')"
              />
            </div>
            <div class="notice-switch-row">
              <span class="switch-label">登录强制弹窗</span>
              <CsToggle
                :model-value="row.is_popup"
                aria-label="切换登录强制弹窗"
                @change="() => handleToggle(row, 'is_popup')"
              />
            </div>
          </div>

          <template #footer>
            <span class="notice-updated">#{{ row.id }} · {{ formatDate(row.updated_at) }} 更新</span>
            <CsButton icon-name="edit" @click="openEdit(row)">编辑</CsButton>
            <CsButton icon-name="remove" @click="remove(row)">删除</CsButton>
          </template>
        </CsCard>
      </CsCardGrid>
    </CsContainer>

    <!-- 编辑公告：基础信息 + 双栏 Markdown（左侧源码、右侧渲染预览） -->
    <CsModal
      v-model:visible="formOpen"
      :header="editing ? '编辑公告' : '新建公告'"
      size="x-large"
      scrollable
      :close-on-click-overlay="false"
    >
      <div class="notice-form">
        <!-- 基础信息不套容器：一张卡只放一个输入框会撑出一大块空白 -->
        <div class="notice-base-bar">
          <CsFormField label="公告标题" required class="notice-title-field">
            <CsInput
              v-model="form.title"
              class="notice-title-input"
              placeholder="如：系统升级维护通知 / 节点新增上线"
              :maxlength="100"
            />
          </CsFormField>

          <CsFormField label="状态" class="notice-status-field">
            <CsRadioGroup v-model="form.status" direction="horizontal">
              <CsRadio :value="1">发布</CsRadio>
              <CsRadio :value="0">草稿</CsRadio>
            </CsRadioGroup>
          </CsFormField>
        </div>

        <CsContainer header="公告正文">
          <!-- 快捷工具栏 -->
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
            <CsPopover v-model:visible="tablePopoverOpen" placement="bottom-start" :width="170">
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
                  <div v-for="r in 4" :key="'r-' + r" class="grid-row">
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

          <div class="md-editor-split">
            <div class="md-pane">
              <span class="md-pane-label">Markdown 源码</span>
              <CsInput
                v-model="form.content"
                type="textarea"
                :rows="15"
                placeholder="在此输入 Markdown 源码..."
                class="md-textarea"
              />
            </div>

            <div class="md-pane">
              <span class="md-pane-label">预览</span>
              <div
                class="md-preview-body markdown-content"
                v-html="
                  renderMarkdown(form.content) ||
                  '<div class=\'preview-empty-hint\'>在左侧输入内容，此处将实时渲染 Markdown 视觉效果...</div>'
                "
              />
            </div>
          </div>
        </CsContainer>
      </div>

      <template #footer>
        <CsButton @click="formOpen = false">取消</CsButton>
        <CsButton variant="primary" :loading="saving" @click="save">
          {{ editing ? '保存修改' : '创建公告' }}
        </CsButton>
      </template>
    </CsModal>
  </CsPage>
</template>

<style scoped lang="scss">
/* ================= 列表卡片 ================= */
.notice-head {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 8px 10px;
  min-inline-size: 0;
}

.notice-drag-handle {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  padding: 2px;
  border-radius: 4px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  cursor: grab;

  &:hover {
    color: var(--color-text-body-default-b2us04, #000716);
    background: var(--color-background-button-normal-hover-1ajrtx, rgba(0, 7, 22, 0.06));
  }

  &:active {
    cursor: grabbing;
  }
}

.notice-name {
  appearance: none;
  border: 0;
  background: transparent;
  margin: 0;
  min-inline-size: 0;
  padding: 0;
  font: inherit;
  font-size: 20px;
  line-height: 28px;
  font-weight: 700;
  color: inherit;
  text-align: start;
  cursor: pointer;
  overflow-wrap: anywhere;

  &:hover {
    color: var(--color-text-link-default-qabnuk, #006ce0);
  }
}

.notice-content-preview {
  min-height: 84px;
  max-height: 120px;
  overflow: hidden;
  padding: 10px 12px;
  border: 1px solid var(--color-border-divider-default-diwd4w, #dedee3);
  border-radius: var(--border-radius-input-k1q34z, 8px);
  background: var(--color-background-container-content-mjfil9, #ffffff);
  font-size: 13px;
  line-height: 19px;

  /* 卡片里的预览只要「看起来像正文」，标题/代码块按正文尺寸收一档，避免一行标题吃掉整块裁切高度 */
  :deep(h1),
  :deep(h2),
  :deep(h3),
  :deep(h4) {
    margin: 0 0 3px;
    padding: 0;
    border: none;
    font-size: 14px;
    line-height: 19px;
  }

  :deep(p) {
    margin: 0 0 3px;
  }

  :deep(ul),
  :deep(ol) {
    margin: 0;
    padding-left: 18px;
  }

  :deep(pre) {
    margin: 3px 0;
    padding: 6px 8px;
  }
}

.notice-switches {
  margin-block-start: 14px;
  display: flex;
  flex-wrap: wrap;
  gap: 10px 24px;
}

.notice-switch-row {
  display: flex;
  align-items: center;
  gap: 8px;
}

.switch-label {
  font-size: 13px;
  line-height: 18px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.notice-updated {
  margin-inline-end: auto;
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: var(--font-size-body-s, 12px);
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

/*
 * 官方 item-card footer 会给按钮 `flex: 1 1 0`，两个按钮 + 时间戳挤在一行时
 * 文字会溢出到相邻按钮上（截图里的「编辑公……」叠字）。这里让按钮按内容宽度排布。
 */
.notice-card :deep(.cs-card__footer) {
  flex-wrap: wrap;
  row-gap: 8px;

  button {
    flex: 0 0 auto;
    padding-inline: 12px !important;
  }
}

.notice-card--off {
  opacity: 0.85;
}

.notice-card--dragging {
  opacity: 0.45;
}

/* ================= 编辑弹窗 ================= */
.notice-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/*
 * 基础信息条：不套容器（一张卡只放一个输入框会撑出一大块空白），
 * 左右内缩 20px 对齐下方卡片内容的起始列。
 */
.notice-base-bar {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  gap: 24px;
  align-items: end;
  padding-inline: 20px;
  padding-block-start: 4px;
}

.notice-title-input :deep(input) {
  block-size: 40px;
  font-size: 15px;
}

.notice-status-field :deep([role='radiogroup']) {
  display: flex;
  align-items: center;
  min-block-size: 40px;
}

.md-toolbar-strip {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  padding: 5px 8px;
  background: var(--color-background-container-content-mjfil9);
  border: 1px solid var(--color-border-divider-default-diwd4w);
  border-radius: 6px;
  margin-bottom: 10px;

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

/* 两栏等高：右栏预览跟随左侧 textarea 的高度，避免出现一长一短 */
.md-editor-split {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
  align-items: stretch;
}

.md-pane {
  display: flex;
  flex-direction: column;
  min-inline-size: 0;
  min-block-size: 0;
}

.md-pane-label {
  margin-bottom: 4px;
  font-size: 12px;
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb);
}

.md-preview-body {
  flex: 1 1 auto;
  overflow-y: auto;
  padding: 10px 12px;
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

/* 4x4 网格选择器样式 */
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

@media (max-width: $bp-tablet-max) {
  .notice-base-bar {
    grid-template-columns: 1fr;
    gap: 12px;
  }

  .md-editor-split {
    grid-template-columns: 1fr;
  }
}
</style>
