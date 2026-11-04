import { computed, onUnmounted, ref, type Ref } from 'vue'
import { ElMessage } from '@/components/cloudscape'
import { errMsg } from '@/api/http'

export interface CardDragSortOptions<T> {
  /** 列表唯一数据源：拖拽只改这个 ref 的顺序，不持有第二份状态。 */
  list: Ref<T[]>
  /** 从行数据取排序 ID（后端 order 接口用这个 ID 序列）。 */
  idOf: (item: T) => number
  /** 保存全量顺序；返回非 0 code 或抛错都会自动回滚到拖拽前。 */
  persist: (ids: number[]) => Promise<{ data: { code: number; message?: string } }>
  /** 页面自己的门控（筛选/搜索/多选/归档开关等）；list 少于 2 项时内部自动禁用。 */
  enabled?: Ref<boolean>
  /** 可选分组约束：返回被拖项允许插入的下标（如公告置顶组只能组内换位）。 */
  clamp?: (rest: T[], insertAt: number, moved: T) => number
  /** 拖到视口上下边缘时自动滚动，默认开启。 */
  autoScroll?: boolean
  /** 卡片定位属性的选择器；卡片需带 `data-sort-id`（与 SortHandle 的 `data-sort-handle` 配套）。 */
  cardSelector?: string
  handleSelector?: string
  /** 保存失败提示，默认「保存排序失败，已恢复原顺序」。 */
  errorMessage?: string
}

/**
 * 卡片网格拖拽排序的唯一实现（服务器/公告/套餐/权限组共用）。
 *
 * 契约：
 * - 卡片根节点带 `data-sort-id="<id>"`，抓手上挂 `SortHandle`（内部带 `data-sort-handle`）；
 * - 只有从抓手发起的拖动才生效，整卡跟随鼠标；
 * - 拖动过程只重排内存列表（dragover 时按左右半区插入），松手一次性提交全量顺序；
 * - 提交失败回滚到拖拽前快照，避免界面与库不一致；
 * - 页面只提供列表、ID 与 persist，不再各写一份顺序算法。
 */
export function useCardDragSort<T>(options: CardDragSortOptions<T>) {
  const {
    list,
    idOf,
    persist,
    enabled,
    clamp,
    autoScroll = true,
    cardSelector = '[data-sort-id]',
    handleSelector = '[data-sort-handle]',
    errorMessage = '保存排序失败，已恢复原顺序',
  } = options

  const canReorder = computed(() => (enabled ? enabled.value : true) && list.value.length > 1)
  const draggingId = ref<number | null>(null)
  let orderSnapshot: T[] = []
  let saving = false
  let scrollTimer: number | null = null
  let scrollStep = 0

  function cardElOf(target: EventTarget | null): HTMLElement | null {
    return (target as HTMLElement | null)?.closest(cardSelector) as HTMLElement | null
  }

  function onDragStart(e: DragEvent) {
    if (!canReorder.value || !(e.target as HTMLElement | null)?.closest(handleSelector)) {
      e.preventDefault()
      return
    }
    const card = cardElOf(e.target)
    if (!card) return
    draggingId.value = Number(card.getAttribute('data-sort-id'))
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
    if (autoScroll) updateAutoScroll(e.clientY)
    const card = cardElOf(e.target)
    if (!card) return
    const targetId = Number(card.getAttribute('data-sort-id'))
    if (!targetId || targetId === draggingId.value) return
    const from = list.value.findIndex((x) => idOf(x) === draggingId.value)
    if (from < 0) return
    const rect = card.getBoundingClientRect()
    // 卡片网格按左右半区判定插入点（拖过中线即换位）
    const after = e.clientX > rect.left + rect.width / 2
    const next = [...list.value]
    const [moved] = next.splice(from, 1)
    let insertAt = next.findIndex((x) => idOf(x) === targetId)
    if (insertAt < 0) return
    if (after) insertAt += 1
    if (clamp) insertAt = clamp(next, insertAt, moved)
    next.splice(insertAt, 0, moved)
    // 只在顺序真的变了才赋值，避免每次 dragover 都触发整树渲染
    if (next.some((x, i) => idOf(x) !== prevIdAt(i))) list.value = next
  }

  function prevIdAt(i: number): number | undefined {
    const prev = list.value[i]
    return prev ? idOf(prev) : undefined
  }

  function onDrop(e: DragEvent) {
    if (draggingId.value === null) return
    e.preventDefault()
    void persistOrder()
  }

  function onDragEnd() {
    stopAutoScroll()
    draggingId.value = null
    void persistOrder()
  }

  async function persistOrder() {
    if (saving) return
    const ids = list.value.map(idOf)
    if (ids.join(',') === orderSnapshot.map(idOf).join(',')) return
    saving = true
    try {
      const { data } = await persist(ids)
      if (data.code !== 0) throw new Error(data.message || '保存排序失败')
    } catch (e) {
      list.value = orderSnapshot
      ElMessage.error(errMsg(e, errorMessage))
    } finally {
      saving = false
      orderSnapshot = [...list.value]
    }
  }

  /** 重新取数后同步基线：避免把「加载导致的顺序变化」误判为用户拖拽。 */
  function syncSnapshot() {
    orderSnapshot = [...list.value]
  }

  function updateAutoScroll(clientY: number) {
    const edge = 24
    scrollStep = clientY < edge ? -14 : clientY > window.innerHeight - edge ? 14 : 0
    if (scrollStep && scrollTimer === null) {
      scrollTimer = window.setInterval(() => window.scrollBy(0, scrollStep), 16)
    } else if (!scrollStep) {
      stopAutoScroll()
    }
  }

  function stopAutoScroll() {
    if (scrollTimer !== null) {
      clearInterval(scrollTimer)
      scrollTimer = null
    }
    scrollStep = 0
  }

  onUnmounted(stopAutoScroll)

  return { canReorder, draggingId, onDragStart, onDragOver, onDrop, onDragEnd, syncSnapshot }
}
