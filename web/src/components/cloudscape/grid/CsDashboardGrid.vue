<script setup lang="ts">
import { onMounted, onUnmounted, provide, ref, shallowRef } from 'vue'
import { GridStack } from 'gridstack'
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

function registerItem(el: HTMLElement) {
  if (gridInstance.value) {
    gridInstance.value.makeWidget(el)
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

function updateResponsiveColumns(width: number) {
  let targetCol = 6
  if (width < 768) targetCol = 2
  else if (width < 1200) targetCol = 4
  if (props.column > 0 && targetCol > props.column) targetCol = props.column

  if (targetCol !== currentColumn.value) {
    currentColumn.value = targetCol
    if (gridInstance.value) {
      gridInstance.value.column(targetCol, 'moveScale')
      emit('layout-change', extractLayout())
    }
  }
}

onMounted(() => {
  if (!gridContainerRef.value) return

  updateResponsiveColumns(rootContainerRef.value?.clientWidth || window.innerWidth)

  const grid = GridStack.init(
    {
      column: currentColumn.value,
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

  grid.on('dragstart', () => {
    isDragging.value = true
  })

  grid.on('dragstop', () => {
    isDragging.value = false
    emit('layout-change', extractLayout())
  })

  grid.on('resizestart', () => {
    isDragging.value = true
  })

  grid.on('resizestop', (_event: Event, el: HTMLElement) => {
    isDragging.value = false
    const id = el.getAttribute('gs-id') || ''
    if (id) emit('widget-resize', id)
    window.dispatchEvent(new Event('resize'))
    emit('layout-change', extractLayout())
  })

  grid.on('change', () => {
    emit('layout-change', extractLayout())
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
    :class="cx(CSB.board.root, 'awsui-board-system', isDragging && 'is-dragging')"
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

/* Official resize handle glyph, bottom-right. */
:deep(.grid-stack-item > .ui-resizable-se) {
  right: 6px;
  bottom: 6px;
  width: 18px;
  height: 18px;
  background-image: none !important;
  cursor: nwse-resize;

  &::after {
    content: '';
    position: absolute;
    inset: 0;
    background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 16 16' fill='none' stroke='%23545b64' stroke-width='2' stroke-linecap='round'%3E%3Cpath d='M14 4 4 14M10 14l4-4'/%3E%3C/svg%3E");
    background-repeat: no-repeat;
    background-size: 14px 14px;
    background-position: center;
    opacity: 0.7;
  }

  &:hover::after {
    opacity: 1;
  }
}
</style>
