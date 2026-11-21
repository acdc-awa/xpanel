import { computed, nextTick, onUnmounted, ref, type Ref } from 'vue'
import { toast } from '@/components/cloudscape'
import { errMsg } from '@/api/http'

export interface TableRowDragOptions<T extends { id: number }> {
  /** 列表唯一数据源：拖拽只改这个 ref 的顺序，松手后持久化。 */
  list: Ref<T[]>
  /** 保存全量顺序；返回非 0 code 或抛错都会自动回滚到拖拽前。 */
  persist: (ids: number[]) => Promise<{ data: { code: number; message?: string } }>
  /** 是否允许拖拽（筛选/搜索/勾选时禁用）；list 少于 2 项时内部自动禁用。 */
  enabled?: Ref<boolean>
  /** 保存失败提示，默认「保存排序失败，已恢复原顺序」。 */
  errorMessage?: string
}

/**
 * 表格行指针拖拽排序（带 FLIP 动画），提取自 notices.vue。
 * 契约：tbody 内每行带 `data-sort-id="<id>"`，抓手带 `data-sort-handle`（`<SortHandle :draggable="false" />`）。
 */
export function useTableRowDrag<T extends { id: number }>(options: TableRowDragOptions<T>) {
  const {
    list,
    persist,
    enabled,
    errorMessage = '保存排序失败，已恢复原顺序',
  } = options

  const HYSTERESIS = 4
  const FLIP_MS = 180
  const SETTLE_MS = 150

  const canReorder = computed(() => (enabled ? enabled.value : true) && list.value.length > 1)
  const draggingId = ref<number | null>(null)

  let snapshot: T[] = []
  let saving = false
  let tbodyEl: HTMLElement | null = null
  let dragEl: HTMLElement | null = null
  let pointerId: number | null = null
  let grabOffsetY = 0
  let lastClientY = 0
  let lastOffsetY = 0
  let minTopInTbody = 0
  let maxTopInTbody = 0
  const heightById = new Map<number, number>()
  let pendingOldTops: Map<number, number> | null = null
  let scrollTimer: number | null = null
  let scrollStep = 0
  let settleCleanup: (() => void) | null = null
  const siblingCleanups = new WeakMap<HTMLElement, () => void>()

  function rowHeightOf(id: number): number {
    return heightById.get(id) ?? 48
  }

  function slotTopOf(items: T[], idx: number): number {
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
    const idx = list.value.findIndex((item) => item.id === draggingId.value)
    if (idx < 0) return
    const desiredTop = lastClientY - tbodyEl.getBoundingClientRect().top - grabOffsetY
    const clampedTop = Math.max(minTopInTbody, Math.min(maxTopInTbody, desiredTop))
    const offsetY = clampedTop - slotTopOf(list.value, idx)
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
      const timer = window.setTimeout(() => cleanup(), FLIP_MS + 50)
      const onEnd = (ev: TransitionEvent) => {
        if (ev.target !== tr || ev.propertyName !== 'transform') return
        cleanup()
      }
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
    const from = list.value.findIndex((item) => item.id === draggingId.value)
    if (from < 0) return

    const desiredTop = clientY - tbodyEl.getBoundingClientRect().top - grabOffsetY
    const clampedTop = Math.max(minTopInTbody, Math.min(maxTopInTbody, desiredTop))

    const next = [...list.value]
    const maxIdx = next.length - 1
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

    while (curIdx > 0) {
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
      const rest = [...list.value]
      const [moved] = rest.splice(from, 1)
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
      rest.splice(curIdx, 0, moved)
      list.value = rest
      if (scheduleFlip) void nextTick(flushSwapFlip)
      return
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
        const timer = window.setTimeout(() => finishSettle(), SETTLE_MS + 50)
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
        settleCleanup = finishSettle
        el.addEventListener('transitionend', onEnd)
      } else {
        el.style.transition = ''
        el.style.transform = ''
      }
    }

    const changed = list.value.some((item, i) => snapshot[i]?.id !== item.id)
    if (!changed) return

    saving = true
    try {
      const { data } = await persist(list.value.map((item) => item.id))
      if (data.code === 0) {
        snapshot = [...list.value]
      } else {
        list.value = [...snapshot]
        if (activeTbody) clearRowInlineStyles(activeTbody)
        toast.error(data.message || errorMessage)
      }
    } catch (err) {
      list.value = [...snapshot]
      if (activeTbody) clearRowInlineStyles(activeTbody)
      toast.error(errMsg(err, errorMessage))
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
    const idx = list.value.findIndex((item) => item.id === id)
    if (idx < 0 || list.value.length < 2) return

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

    const maxIdx = list.value.length - 1
    minTopInTbody = 0
    maxTopInTbody = slotTopOf(list.value, maxIdx)

    snapshot = [...list.value]
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

  return { canReorder, draggingId, onPointerDown }
}
