<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, type Ref } from 'vue'
import { ElMessage, ElMessageBox } from '@/components/cloudscape'
import {
  CsButton,
  CsContainer,
  CsEmpty,
  CsFormField,
  CsInput,
  CsModal,
  CsOption,
  CsPage,
  CsPopover,
  CsRadio,
  CsRadioGroup,
  CsSelect,
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
import SortHandle from '@/components/base/SortHandle.vue'
import { formatDate } from '@/utils/format'
import { renderMarkdown } from '@/utils/markdown'

const list = ref<NoticeItem[]>([])
const noticeList = ref<NoticeItem[]>([])
const guideList = ref<NoticeItem[]>([])
const loading = ref(false)
const keyword = ref('')
const statusFilter = ref<number | undefined>(undefined)

function plainPreview(md: string): string {
  return md
    .replace(/```[\s\S]*?```/g, ' ')
    .replace(/`([^`]+)`/g, '$1')
    .replace(/!\[[^\]]*\]\([^)]*\)/g, '')
    .replace(/\[([^\]]+)\]\([^)]*\)/g, '$1')
    .replace(/[#>*_~\-]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim()
}

async function load() {
  loading.value = true
  try {
    const params: { keyword?: string; status?: number } = {}
    if (keyword.value.trim()) params.keyword = keyword.value.trim()
    if (statusFilter.value !== undefined && statusFilter.value !== null && (statusFilter.value as unknown) !== '') {
      params.status = statusFilter.value
    }

    const { data } = await getAdminNotices(params)
    if (data.code === 0) {
      list.value = data.data
      noticeList.value = data.data.filter((n) => n.category !== 'guide')
      guideList.value = data.data.filter((n) => n.category === 'guide')
      syncNoticeSnapshot()
      syncGuideSnapshot()
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
  category: 'notice' as 'notice' | 'guide',
  title: '',
  content: '',
  status: 1,
})

function openCreate() {
  editing.value = false
  Object.assign(form, { id: 0, category: 'notice', title: '', content: '', status: 1 })
  formOpen.value = true
}

function openEdit(row: NoticeItem) {
  editing.value = true
  Object.assign(form, {
    id: row.id,
    category: row.category === 'guide' ? 'guide' : 'notice',
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
      category: form.category,
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
const reorderEnabled = computed(
  () =>
    !keyword.value.trim() &&
    (statusFilter.value === undefined || statusFilter.value === null || (statusFilter.value as unknown) === ''),
)

const persistMergedOrder = () =>
  reorderNotices([...noticeList.value, ...guideList.value].map((n) => n.id))

const clampPinned = (rest: NoticeItem[], insertAt: number, moved: NoticeItem) => {
  const pinnedCount = rest.filter((n) => n.is_pinned).length
  return moved.is_pinned ? Math.min(insertAt, pinnedCount) : Math.max(insertAt, pinnedCount)
}

function createRowDrag(
  targetList: Ref<NoticeItem[]>,
  enabled: Ref<boolean>,
  persist: () => Promise<{ data: { code: number; message?: string } }>,
  clamp: (rest: NoticeItem[], insertAt: number, moved: NoticeItem) => number,
) {
  const canReorder = computed(() => enabled.value && targetList.value.length > 1)
  const draggingId = ref<number | null>(null)
  const dragOffsetY = ref(0)
  let snapshot: NoticeItem[] = []
  let startY = 0
  let startIndex = 0
  let rowHeight = 48

  function syncSnapshot() {
    snapshot = [...targetList.value]
  }

  function onPointerMove(e: PointerEvent) {
    if (draggingId.value === null) return
    const from = targetList.value.findIndex((n) => n.id === draggingId.value)
    if (from < 0) return

    const movedItem = targetList.value[from]
    const pinnedCount = targetList.value.filter((n) => n.is_pinned).length
    const minIdx = movedItem.is_pinned ? 0 : pinnedCount
    const maxIdx = movedItem.is_pinned ? Math.max(0, pinnedCount - 1) : targetList.value.length - 1

    const minDy = (minIdx - startIndex) * rowHeight
    const maxDy = (maxIdx - startIndex) * rowHeight
    const clampedDy = Math.max(minDy, Math.min(maxDy, e.clientY - startY))

    const rawTarget = startIndex + Math.round(clampedDy / rowHeight)
    const bounded = Math.max(minIdx, Math.min(maxIdx, rawTarget))

    if (bounded !== from) {
      const next = [...targetList.value]
      const [moved] = next.splice(from, 1)
      const insertAt = clamp(next, bounded, moved)
      if (insertAt !== from) {
        next.splice(insertAt, 0, moved)
        targetList.value = next
      }
    }

    const currentIndex = targetList.value.findIndex((n) => n.id === draggingId.value)
    dragOffsetY.value = clampedDy - (currentIndex - startIndex) * rowHeight
  }

  async function onPointerUp() {
    window.removeEventListener('pointermove', onPointerMove)
    window.removeEventListener('pointerup', onPointerUp)
    window.removeEventListener('pointercancel', onPointerUp)
    if (draggingId.value === null) return
    draggingId.value = null
    dragOffsetY.value = 0

    const changed = targetList.value.some((n, i) => snapshot[i]?.id !== n.id)
    if (!changed) return
    try {
      const { data } = await persist()
      if (data.code === 0) {
        snapshot = [...targetList.value]
      } else {
        targetList.value = [...snapshot]
        ElMessage.error(data.message || '保存排序失败，已恢复原顺序')
      }
    } catch (e) {
      targetList.value = [...snapshot]
      ElMessage.error(errMsg(e, '保存排序失败，已恢复原顺序'))
    }
  }

  function onPointerDown(e: PointerEvent) {
    if (!canReorder.value || e.button !== 0) return
    const handle = (e.target as HTMLElement | null)?.closest('[data-sort-handle]')
    const row = handle?.closest<HTMLElement>('tr[data-sort-id]')
    if (!handle || !row) return
    const id = Number(row.getAttribute('data-sort-id'))
    if (!id) return
    const idx = targetList.value.findIndex((n) => n.id === id)
    if (idx < 0) return

    e.preventDefault()
    snapshot = [...targetList.value]
    draggingId.value = id
    dragOffsetY.value = 0
    startY = e.clientY
    startIndex = idx
    rowHeight = row.getBoundingClientRect().height || 48
    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', onPointerUp)
  }

  onUnmounted(() => {
    window.removeEventListener('pointermove', onPointerMove)
    window.removeEventListener('pointerup', onPointerUp)
    window.removeEventListener('pointercancel', onPointerUp)
  })

  return { canReorder, draggingId, dragOffsetY, onPointerDown, syncSnapshot }
}

const {
  canReorder: canReorderNotice,
  draggingId: draggingNoticeId,
  dragOffsetY: dragNoticeOffsetY,
  onPointerDown: onNoticePointerDown,
  syncSnapshot: syncNoticeSnapshot,
} = createRowDrag(noticeList, reorderEnabled, persistMergedOrder, clampPinned)

const {
  canReorder: canReorderGuide,
  draggingId: draggingGuideId,
  dragOffsetY: dragGuideOffsetY,
  onPointerDown: onGuidePointerDown,
  syncSnapshot: syncGuideSnapshot,
} = createRowDrag(guideList, reorderEnabled, persistMergedOrder, clampPinned)
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

    <!-- 紧凑列表：分「系统公告」与「使用指南」两栏 -->
    <CsContainer
      v-for="section in [
        {
          key: 'notice',
          header: '系统公告',
          emptyText: '暂无系统公告，点击右上角「新建公告」',
          items: noticeList,
          canReorder: canReorderNotice,
          draggingId: draggingNoticeId,
          dragOffsetY: dragNoticeOffsetY,
          onPointerDown: onNoticePointerDown,
        },
        {
          key: 'guide',
          header: '使用指南',
          emptyText: '暂无使用指南，点击右上角「新建公告」并选择类型「使用指南」',
          items: guideList,
          canReorder: canReorderGuide,
          draggingId: draggingGuideId,
          dragOffsetY: dragGuideOffsetY,
          onPointerDown: onGuidePointerDown,
        },
      ]"
      :key="section.key"
      :header="section.header"
    >
      <CsEmpty v-if="!loading && section.items.length === 0" icon-name="announcement" :description="section.emptyText" />

      <div
        v-else
        class="notice-table-wrap"
        @pointerdown="section.onPointerDown"
      >
        <table class="notice-table">
          <thead>
            <tr>
              <th v-if="section.canReorder" class="col-sort" />
              <th class="col-title">标题与内容</th>
              <th class="col-toggle">发布</th>
              <th class="col-toggle">置顶</th>
              <th v-if="section.key !== 'guide'" class="col-toggle">弹窗</th>
              <th class="col-updated">更新时间</th>
              <th class="col-actions">操作</th>
            </tr>
          </thead>
          <TransitionGroup name="notice-row" tag="tbody">
            <tr
              v-for="item in section.items"
              :key="item.id"
              class="notice-row"
              :class="{
                'notice-row--off': item.status !== 1,
                'notice-row--dragging': section.draggingId === item.id,
              }"
              :style="
                section.draggingId === item.id
                  ? { transform: `translate3d(0, ${section.dragOffsetY}px, 0)` }
                  : undefined
              "
              :data-sort-id="item.id"
            >
              <td v-if="section.canReorder" class="col-sort">
                <SortHandle title="拖拽调整顺序" />
              </td>

              <td class="col-title">
                <div class="notice-title-cell">
                  <button type="button" class="notice-name" title="点击编辑" @click="openEdit(item)">
                    {{ item.title }}
                  </button>
                  <span v-if="plainPreview(item.content)" class="notice-snippet">
                    {{ plainPreview(item.content) }}
                  </span>
                </div>
              </td>

              <td class="col-toggle">
                <CsTooltip
                  :content="item.status === 1 ? '已发布（用户可见），点击转为草稿' : '草稿（仅管理员可见），点击发布'"
                  placement="top"
                >
                  <CsToggle
                    :model-value="item.status === 1"
                    :aria-label="`切换 ${item.title} 的发布状态`"
                    @change="() => handleToggle(item, 'status')"
                  />
                </CsTooltip>
              </td>

              <td class="col-toggle">
                <CsToggle
                  :model-value="item.is_pinned"
                  aria-label="切换置顶显示"
                  @change="() => handleToggle(item, 'is_pinned')"
                />
              </td>

              <td v-if="section.key !== 'guide'" class="col-toggle">
                <CsToggle
                  :model-value="item.is_popup"
                  aria-label="切换登录强制弹窗"
                  @change="() => handleToggle(item, 'is_popup')"
                />
              </td>

              <td class="col-updated">
                <span class="notice-updated-cell">{{ formatDate(item.updated_at) }}</span>
              </td>

              <td class="col-actions">
                <CsButton size="small" icon-name="edit" @click="openEdit(item)">编辑</CsButton>
              </td>
            </tr>
          </TransitionGroup>
        </table>
      </div>
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

          <CsFormField label="类型" class="notice-status-field">
            <CsRadioGroup v-model="form.category" direction="horizontal">
              <CsRadio value="notice" label="系统公告" />
              <CsRadio value="guide" label="使用指南" />
            </CsRadioGroup>
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
        <CsButton
          v-if="editing && form.id"
          variant="danger-outline"
          icon-name="remove"
          @click="
            () => {
              const target = list.find((n) => n.id === form.id)
              if (target) {
                formOpen = false
                remove(target)
              }
            }
          "
        >
          删除
        </CsButton>
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
  display: inline-flex;
  gap: 8px;
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
  font-size: 16px;
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
  border-radius: 8px;
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
      border-color: rgba(0, 108, 224, 0.2);
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
  border-radius: 8px;

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
        background: rgba(0, 108, 224, 0.25);
        border-color: var(--color-text-link-default-qabnuk);
      }
    }
  }
}

.notice-table-wrap {
  width: 100%;
  overflow-x: auto;
}

.notice-table {
  width: 100%;
  border-collapse: collapse;
  table-layout: fixed;
  font-size: 14px;

  th {
    padding: 8px 10px;
    text-align: left;
    font-size: 12px;
    font-weight: 700;
    color: var(--color-text-label-g80qnu);
    border-bottom: 1px solid var(--color-border-divider-default-diwd4w);
    white-space: nowrap;
  }

  td {
    padding: 10px;
    border-bottom: 1px solid var(--color-border-divider-default-diwd4w);
    vertical-align: middle;
  }

  .col-sort {
    width: 36px;
    padding-inline: 6px;
    text-align: center;
  }

  .col-title {
    width: auto;
    min-width: 220px;
  }

  .col-category {
    width: 68px;
    white-space: nowrap;
  }

  .col-toggle {
    width: 56px;
    text-align: center;
  }

  .col-updated {
    width: 152px;
    white-space: nowrap;
  }

  .col-actions {
    width: 115px;
    text-align: right;
    white-space: nowrap;

    :deep(button) {
      white-space: nowrap;
    }
  }
}

.notice-row {
  transition: background-color 0.15s ease, opacity 0.15s ease;

  &:hover {
    background: var(--color-background-dropdown-item-hover-pvfsrg);
  }

  &--off {
    opacity: 0.65;
  }

  &--dragging,
  &--dragging.notice-row-move {
    position: relative;
    z-index: 10;
    background: var(--color-background-container-content-mjfil9);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.18);
    transition: none !important;
  }
}

.notice-row-move {
  transition: transform 0.2s cubic-bezier(0.2, 0, 0, 1);
}

.notice-title-cell {
  display: flex;
  align-items: baseline;
  gap: 10px;
  min-width: 0;
}

.notice-name {
  flex-shrink: 0;
  max-width: 45%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.notice-snippet {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-size: 13px;
  color: var(--color-text-body-secondary-66ygbb);
}

.notice-cat-badge {
  white-space: nowrap;
}

.notice-updated-cell {
  font-size: 13px;
  color: var(--color-text-body-secondary-66ygbb);
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
