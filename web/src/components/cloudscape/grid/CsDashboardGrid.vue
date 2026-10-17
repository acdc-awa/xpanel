<script setup lang="ts">
import { computed, onMounted, onUnmounted, provide, ref, shallowRef } from 'vue'
import { GridStack } from 'gridstack'
import type { GridItemHTMLElement } from 'gridstack'
import 'gridstack/dist/gridstack.min.css'
import { CSB } from '../board-classes'
import { cx } from '../utils'
import type { WidgetLayoutItem } from './types'

const props = withDefaults(
  defineProps<{
    /** 6-column Cloudscape board layout. */
    column?: number
    /** Cloudscape Board compact row height (76px) / standard (96px). */
    cellHeight?: number
    /** Cloudscape Board gap: 16px compact, 20px standard. */
    margin?: number
    disabled?: boolean
    layout?: WidgetLayoutItem[]
  }>(),
  {
    column: 6,
    cellHeight: 76,
    margin: 16,
    disabled: false,
    layout: () => [],
  }
)

const emit = defineEmits<{
  (e: 'layout-change', layout: WidgetLayoutItem[]): void
  (e: 'widget-resize', id: string): void
  (e: 'widget-remove', id: string): void
}>()

const rootContainerRef = ref<HTMLDivElement | null>(null)
const gridContainerRef = ref<HTMLDivElement | null>(null)
const gridInstance = shallowRef<GridStack | null>(null)
const pendingWidgets = new Set<HTMLElement>()
const isDragging = ref(false)
const currentColumn = ref<number>(props.column || 6)
let resizeObserver: ResizeObserver | null = null
let responsiveAdjusting = false

/** 单列（移动端）走整页纵向流：卡片按内容展开，不提供拖拽/缩放。 */
const isMobileStack = computed(() => currentColumn.value === 2 && props.column > 2)

/**
 * 桌面/平板维持原有交互（拖拽 + 右下角缩放）；单列移动端完全静态，
 * 由 CSS 把卡片还原成文档流并让内容自然撑高。
 */
function applyInteractionState(el?: HTMLElement) {
  const grid = gridInstance.value
  if (!grid) return
  const mobile = isMobileStack.value
  grid.setStatic(props.disabled || mobile)
  if (props.disabled || mobile) return

  // 平板按比例缩列后拖拽/缩放坐标需要换算回基准列数，见 extractCommitLayout。
  grid.opts.disableDrag = false
  grid.opts.resizable = { ...(grid.opts.resizable || {}), handles: 'se' }
  const targets: GridItemHTMLElement[] = el
    ? [el as GridItemHTMLElement]
    : grid.engine.nodes
        .map((node) => node.el)
        .filter((nodeEl): nodeEl is GridItemHTMLElement => Boolean(nodeEl))
  targets.forEach((target) => grid.prepareDragDrop(target, true))
}

function extractLayout(): WidgetLayoutItem[] {
  if (!gridInstance.value) return []
  return gridInstance.value.engine.nodes
    .map((node) => ({
      id: String(node.id || node.el?.getAttribute('gs-id') || ''),
      x: Number(node.x ?? 0),
      y: Number(node.y ?? 0),
      w: Number(node.w ?? 1),
      h: Number(node.h ?? 1),
      minW: node.minW,
      minH: node.minH,
    }))
    .filter((item) => item.id)
}

/**
 * 平板缩列后引擎里的 x/w 是 4 列坐标，按比例换算回基准列数再保存；
 * y/h 与列数无关，直接沿用用户当前值。
 */
function extractCommitLayout(): WidgetLayoutItem[] {
  const current = extractLayout()
  const baseColumn = props.column || 6
  if (currentColumn.value === baseColumn) return current
  const ratio = baseColumn / currentColumn.value
  const baseById = new Map((props.layout || []).map((item) => [item.id, item]))
  const commit = current.map((item) => {
    const base = baseById.get(item.id)
    const w = Math.max(1, Math.min(baseColumn, Math.round(item.w * ratio)))
    const x = Math.max(0, Math.min(baseColumn - w, Math.round(item.x * ratio)))
    return { ...(base || item), x, y: item.y, w, h: item.h }
  })
  // 同步 GridStack 的基准列缓存，避免回到宽屏时先回退到缩列前的位置。
  const scaled = gridInstance.value?.engine.nodes.map((node) => {
    const n = node as unknown as { _id?: number; x?: number; y?: number; w?: number }
    const w = Math.max(1, Math.min(baseColumn, Math.round((n.w ?? 1) * ratio)))
    const x = Math.max(0, Math.min(baseColumn - w, Math.round((n.x ?? 0) * ratio)))
    return { _id: n._id, x, y: Number(n.y ?? 0), w }
  })
  if (scaled) gridInstance.value?.engine.cacheLayout(scaled as never[], baseColumn)
  return commit
}

function registerItem(el: HTMLElement) {
  if (gridInstance.value) {
    gridInstance.value.makeWidget(el)
    applyInteractionState(el)
  } else {
    pendingWidgets.add(el)
  }
}

function unregisterItem(el: HTMLElement) {
  if (gridInstance.value) {
    gridInstance.value.removeWidget(el, false, false)
  } else {
    pendingWidgets.delete(el)
  }
}

function requestWidgetRemove(id: string) {
  emit('widget-remove', id)
}

provide('dashboardGridContext', {
  grid: gridInstance,
  registerItem,
  unregisterItem,
  requestWidgetRemove,
})

function resolveColumn(width: number): number {
  let targetCol = 6
  if (width < 768) targetCol = 2
  else if (width < 1200) targetCol = 4
  if (props.column > 0 && targetCol > props.column) targetCol = props.column
  return targetCol
}

function updateResponsiveColumns(width: number) {
  const targetCol = resolveColumn(width)

  if (targetCol !== currentColumn.value) {
    currentColumn.value = targetCol
    if (gridInstance.value) {
      // column() 会同步触发 change 事件；响应式缩列不是用户编辑，不能落盘。
      responsiveAdjusting = true
      gridInstance.value.column(targetCol, 'moveScale')
      responsiveAdjusting = false
      applyInteractionState()
    }
  }
}

onMounted(() => {
  if (!gridContainerRef.value) return

  const initialColumn = resolveColumn(rootContainerRef.value?.clientWidth || window.innerWidth)
  // 引擎始终以基准列数创建，再把初始视口按比例缩列；避免平板首屏被强制 clamp 成非等比布局。
  currentColumn.value = props.column || 6

  const grid = GridStack.init(
    {
      column: props.column,
      cellHeight: props.cellHeight,
      margin: props.margin,
      animate: true,
      staticGrid: props.disabled,
      draggable: { handle: '.awsui-drag-handle', scroll: false },
      resizable: { handles: 'se' },
    },
    gridContainerRef.value
  )

  if (!grid) return
  gridInstance.value = grid

  pendingWidgets.forEach((el) => grid.makeWidget(el))
  pendingWidgets.clear()

  if (initialColumn !== currentColumn.value) {
    responsiveAdjusting = true
    grid.column(initialColumn, 'moveScale')
    responsiveAdjusting = false
    currentColumn.value = initialColumn
  }
  applyInteractionState()

  grid.on('dragstart', () => {
    isDragging.value = true
  })

  grid.on('dragstop', () => {
    isDragging.value = false
    emit('layout-change', extractCommitLayout())
  })

  grid.on('resizestart', () => {
    isDragging.value = true
  })

  grid.on('resizestop', (_event: Event, el: HTMLElement) => {
    isDragging.value = false
    const id = el.getAttribute('gs-id') || ''
    if (id) emit('widget-resize', id)
    window.dispatchEvent(new Event('resize'))
    emit('layout-change', extractCommitLayout())
  })

  grid.on('change', () => {
    if (responsiveAdjusting) return
    emit('layout-change', extractCommitLayout())
  })

  if (rootContainerRef.value && typeof ResizeObserver !== 'undefined') {
    resizeObserver = new ResizeObserver((entries) => {
      for (const entry of entries) {
        if (entry.contentRect.width > 0) updateResponsiveColumns(entry.contentRect.width)
      }
    })
    resizeObserver.observe(rootContainerRef.value)
  }
})

onUnmounted(() => {
  if (resizeObserver) {
    resizeObserver.disconnect()
    resizeObserver = null
  }
  if (gridInstance.value) {
    gridInstance.value.destroy(false)
    gridInstance.value = null
  }
})

defineExpose({
  getGrid: () => gridInstance.value,
  extractLayout,
  currentColumn,
})
</script>

<template>
  <div
    ref="rootContainerRef"
    :class="cx(CSB.board.root, 'awsui-board-system', isDragging && 'is-dragging', isMobileStack && 'is-mobile-stack')"
  >
    <div ref="gridContainerRef" :class="cx('grid-stack', CSB['internal/grid'].grid)">
      <slot />
    </div>
  </div>
</template>

<style scoped lang="scss">
.awsui-board-system {
  width: 100%;
  min-height: 480px;
}

/* 单列移动端：脱离 GridStack 绝对定位，回到普通文档流。
   卡片按内容自然撑高，滚动交给页面（移动端信息流逻辑），不做卡片内折叠。 */
.awsui-board-system.is-mobile-stack {
  min-height: 0;
}

.awsui-board-system.is-mobile-stack :deep(.grid-stack) {
  display: block !important;
  grid-template-rows: none !important;
  gap: 0 !important;
  height: auto !important;
}

.awsui-board-system.is-mobile-stack :deep(.grid-stack > .grid-stack-item) {
  position: static !important;
  width: auto !important;
  height: auto !important;
  min-height: 0 !important;
  margin-block-end: var(--gs-item-margin-bottom, 16px);
}

.awsui-board-system.is-mobile-stack
  :deep(.grid-stack > .grid-stack-item > .grid-stack-item-content) {
  position: static !important;
  height: auto !important;
  margin: 0 !important;
  overflow: visible !important;
}

/* 单列移动端取消卡片内滚动，内容自然展开，滚动交给页面。 */
.awsui-board-system.is-mobile-stack :deep(.widget-body) {
  height: auto;
  overflow: visible;
}

.awsui-board-system.is-mobile-stack :deep([class*='content-wrapper-fit-height']) {
  block-size: auto !important;
  overflow: visible !important;
}

.awsui-board-system.is-mobile-stack :deep([class*='content-fit-height']) {
  overflow: visible !important;
}

/* 依赖固定高度/内部滚动的容器改为按内容展开；图表在 auto 高度下需要显式高度。 */
.awsui-board-system.is-mobile-stack :deep(.table-container),
.awsui-board-system.is-mobile-stack :deep(.ranking-tab-container),
.awsui-board-system.is-mobile-stack :deep(.ranking-tab-content) {
  flex: none;
  height: auto;
}

.awsui-board-system.is-mobile-stack :deep(.chart-container) {
  flex: none;
  height: 320px;
}

/* GridStack's own children are rendered through the default slot by Vue;
   keep the placeholder visually aligned with official Cloudscape Board. */
:deep(.grid-stack-placeholder > .placeholder-content) {
  background-color: var(--color-board-placeholder-active-y53hbb, #ebebf0) !important;
  border-radius: var(--border-radius-container-33hhdh, 16px) !important;
  border: none !important;
  opacity: 1 !important;
}

:deep(.grid-stack-item.ui-draggable-dragging > .grid-stack-item-content),
:deep(.grid-stack-item.ui-resizable-resizing > .grid-stack-item-content) {
  z-index: 100 !important;
}

/* 官方 resize-area 图标；GridStack 默认图标是竖直箭头并带 rotate(-45deg)，
   若不解除旋转，换成的对角图标会变成竖线。 */
:deep(.grid-stack-item > .ui-resizable-se) {
  /* 手柄是 grid-stack-item 的子元素，坐标含 16px 卡片间距；
     加上 margin 才能落在卡片圆角内侧，而不是浮在卡片外的间隙里。 */
  right: calc(var(--gs-item-margin-right, 16px) + 6px);
  bottom: calc(var(--gs-item-margin-bottom, 16px) + 6px);
  width: 20px;
  height: 20px;
  background-image: none !important;
  transform: none !important;
  cursor: nwse-resize;
  z-index: 102;
  opacity: 0.7;
  transition: opacity 0.12s ease;

  &::after {
    content: '';
    position: absolute;
    inset: 2px;
    background-color: var(--color-text-interactive-default-pauydh, #545b64);
    mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 16 16' fill='none' stroke='%23fff' stroke-width='1.5' stroke-linecap='round'%3E%3Cpath d='M14 4 4 14M10 14l4-4'/%3E%3C/svg%3E") center / 14px 14px no-repeat;
    -webkit-mask: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 16 16' fill='none' stroke='%23fff' stroke-width='1.5' stroke-linecap='round'%3E%3Cpath d='M14 4 4 14M10 14l4-4'/%3E%3C/svg%3E") center / 14px 14px no-repeat;
  }

  &:hover {
    opacity: 1;
  }
}
</style>
