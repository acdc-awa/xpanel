<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, reactive, ref, type Ref } from 'vue'
import { toast, messageBox } from '@/components/cloudscape'
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
import { formatDateTimeMinute } from '@/utils/timezone'
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
      noticeDrag.syncSnapshot()
      guideDrag.syncSnapshot()
    } else {
      toast.error(data.message)
    }
  } catch (e) {
    toast.error(errMsg(e, '加载公告列表失败'))
  } finally {
    loading.value = false
  }
}

onMounted(load)

// 编辑与新建弹窗（置顶/弹窗/排序都在列表卡片与拖拽里维护，不再进表单）
const formOpen = ref(false)
const editing = ref(false)
const editingName = ref('')
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
  editingName.value = ''
  Object.assign(form, { id: 0, category: 'notice', title: '', content: '', status: 1 })
  formOpen.value = true
}

function openEdit(row: NoticeItem) {
  editing.value = true
  editingName.value = row.title
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
    toast.warning('请填写公告标题与内容')
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
      toast.success(editing.value ? '公告已保存' : '公告已创建')
      formOpen.value = false
      load()
    } else {
      toast.error(data.message)
    }
  } catch (e) {
    toast.error(errMsg(e, '保存公告失败'))
  } finally {
    saving.value = false
  }
}

async function handleToggle(row: NoticeItem, field: 'status' | 'is_pinned' | 'is_popup') {
  try {
    const { data } = await toggleAdminNotice(row.id, field)
    if (data.code === 0) {
      toast.success('状态已更新')
      // 置顶会改变分组顺序（置顶始终排在最前），必须重新取列表
      if (field === 'is_pinned') {
        load()
        return
      }
      Object.assign(row, data.data)
    } else {
      toast.error(data.message)
      load()
    }
  } catch (e) {
    toast.error(errMsg(e, '切换状态失败'))
    load()
  }
}

async function remove(row: NoticeItem) {
  try {
    await messageBox.confirm(`确定要删除公告「${row.title}」吗？`, '删除公告', {
      type: 'error',
      confirmButtonText: '确定删除',
      cancelButtonText: '取消',
    })
    const { data } = await deleteAdminNotice(row.id)
    if (data.code === 0) {
      toast.success('公告已删除')
      load()
    } else {
      toast.error(data.message)
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
  const HYSTERESIS = 4
  const FLIP_MS = 180
  const SETTLE_MS = 150

  const canReorder = computed(() => enabled.value && targetList.value.length > 1)
  const draggingId = ref<number | null>(null)

  let snapshot: NoticeItem[] = []
  let saving = false
  let tbodyEl: HTMLElement | null = null
  let dragEl: HTMLElement | null = null
  let pointerId: number | null = null
  let grabOffsetY = 0
  let lastClientY = 0
  let lastOffsetY = 0
  let minIdx = 0
  let maxIdx = 0
  let minTopInTbody = 0
  let maxTopInTbody = 0
  const heightById = new Map<number, number>()
  let pendingOldTops: Map<number, number> | null = null
  let scrollTimer: number | null = null
  let scrollStep = 0
  let settleCleanup: (() => void) | null = null
  const siblingCleanups = new WeakMap<HTMLElement, () => void>()

  function syncSnapshot() {
    snapshot = [...targetList.value]
  }

  function rowHeightOf(id: number): number {
    return heightById.get(id) ?? 48
  }

  function slotTopOf(items: NoticeItem[], idx: number): number {
    let top = 0
    for (let i = 0; i < idx; i++) {
      top += rowHeightOf(items[i].id)
    }
    return top
  }

  function clearRowInlineStyles(tbody: HTMLElement) {
    tbody.querySelectorAll<HTMLElement>('tr[data-sort-id]').forEach((tr) => {
      siblingCleanups.get(tr)?.()
      tr.style.transition = ''
      tr.style.transform = ''
    })
  }

  function stopAutoScroll() {
    if (scrollTimer !== null) {
      clearInterval(scrollTimer)
      scrollTimer = null
    }
    scrollStep = 0
  }

  function updateAutoScroll(clientY: number) {
    const edge = 32
    scrollStep = clientY < edge ? -10 : clientY > window.innerHeight - edge ? 10 : 0
    if (scrollStep && scrollTimer === null) {
      scrollTimer = window.setInterval(() => {
        window.scrollBy(0, scrollStep)
        updateDragPosition(lastClientY)
      }, 16)
    } else if (!scrollStep) {
      stopAutoScroll()
    }
  }

  function applyDragTransform() {
    if (draggingId.value === null || !tbodyEl || !dragEl) return
    const idx = targetList.value.findIndex((n) => n.id === draggingId.value)
    if (idx < 0) return
    const desiredTop = lastClientY - tbodyEl.getBoundingClientRect().top - grabOffsetY
    const clampedTop = Math.max(minTopInTbody, Math.min(maxTopInTbody, desiredTop))
    const offsetY = clampedTop - slotTopOf(targetList.value, idx)
    lastOffsetY = offsetY
    dragEl.style.transform = `translate3d(0, ${offsetY}px, 0)`
  }

  function flushSwapFlip() {
    const oldTops = pendingOldTops
    const tbody = tbodyEl
    pendingOldTops = null
    if (!oldTops || !tbody) return

    if (draggingId.value !== null && dragEl) {
      applyDragTransform()
    }

    const animated: HTMLElement[] = []
    tbody.querySelectorAll<HTMLElement>('tr[data-sort-id]').forEach((tr) => {
      const id = Number(tr.getAttribute('data-sort-id'))
      if (!id || id === draggingId.value) return
      const oldTop = oldTops.get(id)
      if (oldTop === undefined) return

      siblingCleanups.get(tr)?.()
      tr.style.transition = 'none'
      tr.style.transform = ''
      const newTop = tr.getBoundingClientRect().top
      const dy = oldTop - newTop
      if (Math.abs(dy) > 0.5) {
        tr.style.transform = `translate3d(0, ${dy}px, 0)`
        animated.push(tr)
      } else {
        tr.style.transition = ''
      }
    })

    if (animated.length === 0) return
    void tbody.offsetHeight
    for (const tr of animated) {
      tr.style.transition = `transform ${FLIP_MS}ms cubic-bezier(0.2, 0, 0, 1)`
      tr.style.transform = ''
      const onEnd = (ev: TransitionEvent) => {
        if (ev.target !== tr || ev.propertyName !== 'transform') return
        cleanup()
      }
      const timer = window.setTimeout(() => cleanup(), FLIP_MS + 50)
      const cleanup = () => {
        window.clearTimeout(timer)
        tr.removeEventListener('transitionend', onEnd)
        siblingCleanups.delete(tr)
        tr.style.transition = ''
      }
      siblingCleanups.set(tr, cleanup)
      tr.addEventListener('transitionend', onEnd)
    }
  }

  function updateDragPosition(clientY: number) {
    if (draggingId.value === null || !tbodyEl || !dragEl) return
    lastClientY = clientY
    const from = targetList.value.findIndex((n) => n.id === draggingId.value)
    if (from < 0) return

    const desiredTop = clientY - tbodyEl.getBoundingClientRect().top - grabOffsetY
    const clampedTop = Math.max(minTopInTbody, Math.min(maxTopInTbody, desiredTop))

    const next = [...targetList.value]
    let curIdx = from
    let curSlotTop = slotTopOf(next, curIdx)

    while (curIdx < maxIdx) {
      const belowH = rowHeightOf(next[curIdx + 1].id)
      if (clampedTop > curSlotTop + belowH / 2 + HYSTERESIS) {
        const tmp = next[curIdx]
        next[curIdx] = next[curIdx + 1]
        next[curIdx + 1] = tmp
        curSlotTop += belowH
        curIdx += 1
      } else {
        break
      }
    }

    while (curIdx > minIdx) {
      const aboveH = rowHeightOf(next[curIdx - 1].id)
      if (clampedTop < curSlotTop - aboveH / 2 - HYSTERESIS) {
        const tmp = next[curIdx]
        next[curIdx] = next[curIdx - 1]
        next[curIdx - 1] = tmp
        curSlotTop -= aboveH
        curIdx -= 1
      } else {
        break
      }
    }

    if (curIdx !== from) {
      const rest = [...targetList.value]
      const [moved] = rest.splice(from, 1)
      const insertAt = clamp(rest, curIdx, moved)
      if (insertAt !== from) {
        const scheduleFlip = pendingOldTops === null
        if (scheduleFlip) {
          const tops = new Map<number, number>()
          tbodyEl.querySelectorAll<HTMLElement>('tr[data-sort-id]').forEach((tr) => {
            const rowId = Number(tr.getAttribute('data-sort-id'))
            if (rowId && rowId !== draggingId.value) {
              tops.set(rowId, tr.getBoundingClientRect().top)
            }
          })
          pendingOldTops = tops
        }
        rest.splice(insertAt, 0, moved)
        targetList.value = rest
        if (scheduleFlip) void nextTick(flushSwapFlip)
        return
      }
    }

    if (pendingOldTops === null) {
      lastOffsetY = clampedTop - curSlotTop
      dragEl.style.transform = `translate3d(0, ${lastOffsetY}px, 0)`
    }
  }

  function onPointerMove(e: PointerEvent) {
    if (draggingId.value === null) return
    if (pointerId !== null && e.pointerId !== pointerId) return
    updateAutoScroll(e.clientY)
    updateDragPosition(e.clientY)
  }

  function onScroll() {
    if (draggingId.value !== null) {
      updateDragPosition(lastClientY)
    }
  }

  function detachListeners() {
    window.removeEventListener('pointermove', onPointerMove)
    window.removeEventListener('pointerup', onPointerUp)
    window.removeEventListener('pointercancel', onPointerUp)
    window.removeEventListener('scroll', onScroll)
  }

  async function onPointerUp(e?: PointerEvent) {
    if (e && pointerId !== null && e.pointerId !== pointerId) return
    detachListeners()
    stopAutoScroll()

    const capturedId = pointerId
    const activeTbody = tbodyEl
    if (capturedId !== null && activeTbody) {
      try {
        if (activeTbody.hasPointerCapture(capturedId)) {
          activeTbody.releasePointerCapture(capturedId)
        }
      } catch {
        /* 忽略未捕获或已卸载 */
      }
    }

    if (draggingId.value === null) return
    const releasedId = draggingId.value
    const el = dragEl
    const settledOffset = lastOffsetY

    if (pendingOldTops !== null && el) {
      pendingOldTops.set(releasedId, el.getBoundingClientRect().top)
    }

    draggingId.value = null
    dragEl = null
    tbodyEl = null
    pointerId = null
    lastOffsetY = 0

    if (el && pendingOldTops === null) {
      if (Math.abs(settledOffset) > 0.5) {
        el.style.transition = 'none'
        el.style.transform = `translate3d(0, ${settledOffset}px, 0)`
        void el.offsetHeight
        el.style.transition = `transform ${SETTLE_MS}ms cubic-bezier(0.2, 0, 0, 1)`
        el.style.transform = ''
        const finishSettle = () => {
          window.clearTimeout(timer)
          el.removeEventListener('transitionend', onEnd)
          if (settleCleanup === finishSettle) settleCleanup = null
          el.style.transition = ''
        }
        const onEnd = (ev: TransitionEvent) => {
          if (ev.target !== el || ev.propertyName !== 'transform') return
          finishSettle()
        }
        const timer = window.setTimeout(finishSettle, SETTLE_MS + 50)
        settleCleanup = finishSettle
        el.addEventListener('transitionend', onEnd)
      } else {
        el.style.transition = ''
        el.style.transform = ''
      }
    }

    const changed = targetList.value.some((n, i) => snapshot[i]?.id !== n.id)
    if (!changed) return

    saving = true
    try {
      const { data } = await persist()
      if (data.code === 0) {
        snapshot = [...targetList.value]
      } else {
        targetList.value = [...snapshot]
        if (activeTbody) clearRowInlineStyles(activeTbody)
        toast.error(data.message || '保存排序失败，已恢复原顺序')
      }
    } catch (err) {
      targetList.value = [...snapshot]
      if (activeTbody) clearRowInlineStyles(activeTbody)
      toast.error(errMsg(err, '保存排序失败，已恢复原顺序'))
    } finally {
      saving = false
    }
  }

  function onPointerDown(e: PointerEvent) {
    if (!canReorder.value || saving || draggingId.value !== null || e.button !== 0) return
    const handle = (e.target as HTMLElement | null)?.closest<HTMLElement>('[data-sort-handle]')
    const row = handle?.closest<HTMLElement>('tr[data-sort-id]')
    const tbody = row?.closest<HTMLElement>('tbody')
    if (!handle || !row || !tbody) return

    const id = Number(row.getAttribute('data-sort-id'))
    if (!id) return
    const idx = targetList.value.findIndex((n) => n.id === id)
    if (idx < 0) return

    const item = targetList.value[idx]
    const pinnedCount = targetList.value.filter((n) => n.is_pinned).length
    minIdx = item.is_pinned ? 0 : pinnedCount
    maxIdx = item.is_pinned ? Math.max(0, pinnedCount - 1) : targetList.value.length - 1
    if (minIdx >= maxIdx) {
      e.preventDefault()
      return
    }

    e.preventDefault()
    settleCleanup?.()
    clearRowInlineStyles(tbody)
    heightById.clear()
    tbody.querySelectorAll<HTMLElement>('tr[data-sort-id]').forEach((tr) => {
      const rowId = Number(tr.getAttribute('data-sort-id'))
      if (rowId) {
        heightById.set(rowId, tr.getBoundingClientRect().height || 48)
      }
    })

    minTopInTbody = slotTopOf(targetList.value, minIdx)
    const groupBottom = slotTopOf(targetList.value, maxIdx) + rowHeightOf(targetList.value[maxIdx].id)
    maxTopInTbody = groupBottom - rowHeightOf(id)

    snapshot = [...targetList.value]
    tbodyEl = tbody
    dragEl = row
    pointerId = e.pointerId
    grabOffsetY = e.clientY - row.getBoundingClientRect().top
    lastClientY = e.clientY
    lastOffsetY = 0
    pendingOldTops = null
    draggingId.value = id

    try {
      tbody.setPointerCapture(e.pointerId)
    } catch {
      /* 降级走 window 监听 */
    }

    window.addEventListener('pointermove', onPointerMove)
    window.addEventListener('pointerup', onPointerUp)
    window.addEventListener('pointercancel', onPointerUp)
    window.addEventListener('scroll', onScroll, { passive: true })
  }

  onUnmounted(() => {
    detachListeners()
    stopAutoScroll()
    settleCleanup?.()
  })

  return { canReorder, draggingId, onPointerDown, syncSnapshot }
}

const noticeDrag = createRowDrag(noticeList, reorderEnabled, persistMergedOrder, clampPinned)
const guideDrag = createRowDrag(guideList, reorderEnabled, persistMergedOrder, clampPinned)

const sections = computed(() => [
  {
    key: 'notice',
    header: '系统公告',
    emptyText: '暂无系统公告，点击右上角「新建公告」',
    items: noticeList.value,
    canReorder: noticeDrag.canReorder.value,
    draggingId: noticeDrag.draggingId.value,
    onPointerDown: noticeDrag.onPointerDown,
  },
  {
    key: 'guide',
    header: '使用指南',
    emptyText: '暂无使用指南，点击右上角「新建公告」并选择类型「使用指南」',
    items: guideList.value,
    canReorder: guideDrag.canReorder.value,
    draggingId: guideDrag.draggingId.value,
    onPointerDown: guideDrag.onPointerDown,
  },
])
</script>

<template>
  <CsPage>
    <CsToolbar>
      <CsInput
        v-model="keyword"
        placeholder="搜索公告标题或内容..."
        clearable
        prefix-icon="search"
        style="inline-size: min(100%, 220px)"
        @keyup.enter="load"
        @clear="load"
      />
      <CsSelect v-model="statusFilter" placeholder="全部状态" clearable style="inline-size: min(100%, 120px)" @change="load">
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
      v-for="section in sections"
      :key="section.key"
      :header="section.header"
    >
      <CsEmpty v-if="!loading && section.items.length === 0" icon-name="announcement" :description="section.emptyText" />

      <div
        v-else
        class="notice-table-wrap"
        @pointerdown="section.onPointerDown"
        @dragstart.prevent
      >
        <table class="notice-table" :class="{ 'notice-table--dragging': section.draggingId !== null }">
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
          <tbody>
            <tr
              v-for="item in section.items"
              :key="item.id"
              class="notice-row"
              :class="{
                'notice-row--off': item.status !== 1,
                'notice-row--dragging': section.draggingId === item.id,
              }"
              :data-sort-id="item.id"
            >
              <td v-if="section.canReorder" class="col-sort">
                <SortHandle :draggable="false" title="拖拽调整顺序" />
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
                <span class="notice-updated-cell">{{ formatDateTimeMinute(item.updated_at) }}</span>
              </td>

              <td class="col-actions">
                <CsButton icon-name="edit" :wrap-text="false" @click="openEdit(item)">编辑</CsButton>
              </td>
            </tr>
          </tbody>
        </table>
      </div>
    </CsContainer>

    <!-- 编辑公告：基础信息 + 双栏 Markdown（左侧源码、右侧渲染预览） -->
    <CsModal
      v-model:visible="formOpen"
      :header="editing ? `编辑公告：${editingName}` : '新建公告'"
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
  grid-template-columns: minmax(0, 1fr) auto auto;
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
  border-collapse: separate;
  border-spacing: 0;
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

.notice-table--dragging {
  user-select: none;
  cursor: grabbing;

  .notice-row:not(.notice-row--dragging):hover {
    background: transparent;
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

  &--dragging {
    position: relative;
    z-index: 10;
    background: var(--color-background-container-content-mjfil9);
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.14);
    will-change: transform;
    transition: none;
  }
}

.notice-title-cell {
  display: flex;
  align-items: baseline;
  gap: 10px;
  min-width: 0;
}

.notice-name {
  appearance: none;
  border: 0;
  background: transparent;
  margin: 0;
  padding: 0;
  flex-shrink: 0;
  max-width: 45%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font: inherit;
  font-size: 14px;
  line-height: 20px;
  font-weight: 600;
  color: inherit;
  text-align: start;
  cursor: pointer;

  &:hover {
    color: var(--color-text-link-default-qabnuk, #006ce0);
  }
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
