<script setup lang="ts">
import { computed, inject, onMounted, onUnmounted, ref } from 'vue'
import Icon from '../Icon.vue'
import Container from '../Container.vue'
import Header from '../Header.vue'
import ButtonDropdown from '../ButtonDropdown.vue'
import { CSB } from '../board-classes'
import { cx } from '../utils'
import { useSettleMeasure } from './useSettleMeasure'

const props = withDefaults(
  defineProps<{
    id: string
    title?: string
    badge?: string | number
    description?: string
    noPadding?: boolean
    x?: number
    y?: number
    w?: number
    h?: number
    minW?: number
    minH?: number
    maxW?: number
    maxH?: number
  }>(),
  { title: '', badge: undefined, description: undefined, noPadding: false }
)

const emit = defineEmits<{
  (e: 'remove', id: string): void
  (e: 'refresh', id: string): void
}>()

const rootRef = ref<HTMLDivElement | null>(null)

/**
 * 卡片实际宽度决定是否进入窄卡片模式（标题单行省略、副标题计数让位）。
 * 窄卡片里操作区是否留在第一行，再由下面的实测宽度单独决定。
 */
const narrowHeader = ref(false)
const NARROW_HEADER_WIDTH = 500
let sizeObserver: ResizeObserver | null = null

const headerRef = ref<HTMLElement | null>(null)
const handleRef = ref<HTMLElement | null>(null)
const settingsRef = ref<HTMLElement | null>(null)
const menuRef = ref<HTMLElement | null>(null)

/**
 * 窄卡片里操作区不一定都要落到第二行：粒度选择器降级成的下拉只有 90px 左右，
 * 和标题、更多菜单排一行完全放得下。只有操作区本身很宽（服务器筛选框、
 * 未降级的分段控件）时才换行——把标题压到不足 80px 还不如让操作区独占一行。
 */
const settingsInline = ref(true)
const TITLE_MIN_WIDTH = 80
/** 超过这个宽度的操作区视为「宽控件」，窄卡片里始终独占一行。 */
const COMPACT_ACTIONS_WIDTH = 120
/** handle→标题 4 + 标题→操作区 8 + 操作区→更多菜单 8 */
const HEADER_INLINE_GAPS = 20

function syncNarrowHeader() {
  const root = rootRef.value
  const header = headerRef.value
  if (!root || !header) return
  const width = root.clientWidth
  if (width <= 0) return
  narrowHeader.value = width < NARROW_HEADER_WIDTH
  if (!narrowHeader.value) {
    settingsInline.value = true
    return
  }
  const style = getComputedStyle(header)
  const contentWidth =
    header.clientWidth -
    (Number.parseFloat(style.paddingLeft) || 0) -
    (Number.parseFloat(style.paddingRight) || 0)
  const chromeWidth =
    (handleRef.value?.offsetWidth ?? 0) +
    (settingsRef.value?.offsetWidth ?? 0) +
    (menuRef.value?.offsetWidth ?? 0)
  const actionsWidth = settingsRef.value?.offsetWidth ?? 0
  settingsInline.value =
    actionsWidth <= COMPACT_ACTIONS_WIDTH &&
    contentWidth - chromeWidth >= TITLE_MIN_WIDTH + HEADER_INLINE_GAPS
}

const settle = useSettleMeasure(syncNarrowHeader)

interface GridContext {
  registerItem: (el: HTMLElement) => void
  unregisterItem: (el: HTMLElement) => void
  requestWidgetRemove: (id: string) => void
}

const gridContext = inject<GridContext | null>('dashboardGridContext', null)

function handleCommand(cmd: string | number) {
  if (cmd === 'remove') {
    gridContext?.requestWidgetRemove(props.id)
    emit('remove', props.id)
  } else if (cmd === 'refresh') {
    emit('refresh', props.id)
  }
}

const menuItems = [
  { id: 'refresh', text: '刷新此卡片' },
  { id: 'remove', text: '移除小组件', dividerBefore: true },
]

onMounted(() => {
  if (!rootRef.value) return
  gridContext?.registerItem(rootRef.value)
  syncNarrowHeader()
  if (typeof ResizeObserver !== 'undefined') {
    sizeObserver = new ResizeObserver(syncNarrowHeader)
    sizeObserver.observe(rootRef.value)
  }
  // 外壳定宽/GridStack 排版都晚于挂载，且 ResizeObserver 在部分切换路径里只回调第一次
  settle.start()
  window.addEventListener('resize', settle.start)
})

onUnmounted(() => {
  sizeObserver?.disconnect()
  sizeObserver = null
  window.removeEventListener('resize', settle.start)
  if (rootRef.value) gridContext?.unregisterItem(rootRef.value)
})

const headerClass = computed(() => CSB['board-item'].header)
</script>

<template>
  <div
    ref="rootRef"
    class="grid-stack-item"
    :gs-id="id"
    :gs-x="x"
    :gs-y="y"
    :gs-w="w"
    :gs-h="h"
    :gs-min-w="minW"
    :gs-min-h="minH"
    :gs-max-w="maxW"
    :gs-max-h="maxH"
  >
    <div class="grid-stack-item-content">
      <div :class="CSB['board-item'].root">
        <Container
          fit-height
          disable-header-paddings
          :disable-content-paddings="noPadding"
          :class="CSB['board-item']['container-override']"
          class="dashboard-widget-container"
        >
          <template #header>
            <div
              ref="headerRef"
              :class="
                cx(
                  headerClass,
                  'cs-widget-header',
                  narrowHeader && 'cs-widget-header-narrow',
                  narrowHeader && settingsInline && 'cs-widget-header-inline'
                )
              "
            >
              <div ref="handleRef" :class="cx(CSB['board-item'].fixed, CSB['board-item'].handle)">
                <span
                  class="awsui-drag-handle"
                  :class="CSB['internal/drag-handle'].handle"
                  title="拖拽调整小组件位置"
                >
                  <Icon name="drag-indicator" />
                </span>
              </div>
              <div :class="cx(CSB['board-item'].flexible, CSB['board-item']['header-content'], 'cs-widget-title')">
                <Header variant="h2" :counter="badge">
                  {{ title }}
                </Header>
              </div>
              <div
                ref="settingsRef"
                :class="cx(CSB['board-item'].fixed, CSB['board-item'].settings, 'widget-settings')"
              >
                <slot name="actions" />
              </div>
              <div ref="menuRef" class="widget-menu">
                <ButtonDropdown
                  icon-only
                  icon-name="ellipsis"
                  aria-label="小组件选项"
                  :items="menuItems"
                  @select="handleCommand"
                />
              </div>
            </div>
          </template>

          <div class="widget-body">
            <slot />
          </div>
        </Container>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.grid-stack-item-content {
  background: transparent !important;
  overflow: visible !important;
}

:deep(.awsui-drag-handle) {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 4px;
  border-radius: 4px;
  cursor: grab;
}

.widget-settings {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: nowrap;
}

.widget-menu {
  flex: 0 0 auto;
  display: flex;
  align-items: center;
  margin-inline-start: 8px;
}

/* 官方 Board 的 header 是不换行的 flex：操作区（筛选框/粒度选择器）的固定宽度
   会把标题压到只剩一两个字宽，中文于是逐字竖排、整行高达 200px+。
   窄卡片（按实测宽度 < 500）时标题独占第一行、操作区落到第二行。
   vendor 规则带 :not(#\9)（ID 级特异性），只能 !important 覆盖。 */
.cs-widget-header-narrow {
  flex-wrap: wrap !important;
  row-gap: 2px !important;
}

.cs-widget-header-narrow .cs-widget-title {
  order: 1;
  flex: 1 1 calc(100% - 84px) !important;
  min-inline-size: 0;
}

/* 窄卡片里标题只占一行：长标题省略而不是折成两三层把 header 撑高；
   副标题计数（实时采样 / N 实例）是补充信息，窄卡片里先让位给主标题。 */
.cs-widget-header-narrow .cs-widget-title :deep(h2) {
  display: block !important;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.cs-widget-header-narrow .cs-widget-title :deep([class*='awsui_counter_']) {
  display: none;
}

.cs-widget-header-narrow .widget-menu {
  order: 2;
  margin-inline-start: 8px;
}

.cs-widget-header-narrow .widget-settings {
  order: 3;
  flex: 0 0 auto !important;
  max-inline-size: 100%;
  margin-inline-start: auto !important;
}

/* 操作区足够窄（下拉/粒度选择器）时留在标题同一行：标题让出宽度并省略，
   更多菜单仍固定在最右。只有放不下（筛选框）才走上面的第二行。 */
.cs-widget-header-narrow.cs-widget-header-inline .cs-widget-title {
  flex: 1 1 0 !important;
}

.cs-widget-header-narrow.cs-widget-header-inline .widget-settings {
  order: 2;
  margin-inline-start: 8px !important;
}

.cs-widget-header-narrow.cs-widget-header-inline .widget-menu {
  order: 3;
}

/* 筛选框按可用宽度收缩，否则它会带着输入框的固有宽度把操作区撑到卡片外 */
.widget-settings :deep([class*='compact-filter']) {
  flex: 1 1 auto;
  min-inline-size: 0;
  max-inline-size: 260px;
}

.widget-settings :deep([class*='compact-filter'] input) {
  inline-size: 100% !important;
  min-inline-size: 0;
}

.widget-body {
  display: flex;
  flex-direction: column;
  min-width: 0;
  min-height: 0;
  height: 100%;
  overflow: auto;

  /* 不要设 overscroll-behavior: contain：卡片是常驻的滚动容器，
     它会把滚轮事件锁在卡片内（内容不溢出时也一样），导致页面无法滚动。
     保留默认的链式滚动：卡片滚到底后自然交给页面。 */

  /* 滚动容器的子项不能被 flex 压缩，否则内容会缩到卡片高度而不是产生滚动。 */
  > * {
    flex-shrink: 0;
  }

  /* Chrome 在设置标准 scrollbar-width/color 后会忽略 ::-webkit-scrollbar，
     退回带箭头的原生滚动条；这里只用伪元素统一外观。 */
  &::-webkit-scrollbar {
    width: 10px;
    height: 10px;
  }

  &::-webkit-scrollbar-track {
    background: transparent;
    margin-block: 4px;
  }

  &::-webkit-scrollbar-button {
    display: none;
  }

  /* 透明边框会吃掉可见宽度：10px 滑道减 2×2px 后仍有 6px 滑块，否则只剩 4px 几乎看不见。 */
  &::-webkit-scrollbar-thumb {
    background-color: var(--color-text-status-inactive-fqjc97, #879596);
    background-clip: content-box;
    border: 2px solid transparent;
    border-radius: 8px;

    &:hover {
      background-color: var(--color-text-body-secondary-66ygbb, #656871);
    }
  }
}

/* 官方 content-inner 是 flex:1 但 min-height 默认 auto，
   内容高于卡片时会把它撑开，导致外层裁切而不是滚动。 */
.dashboard-widget-container :deep([class*='content-inner']) {
  min-block-size: 0;
}

/* Container fit-height 的内容区默认 overflow:auto，会在卡片右下角叠出一条滚动条，
   与缩放手柄挤在一起；滚动统一交给 widget-body 管理。 */
.dashboard-widget-container :deep([class*='content-fit-height']) {
  overflow: hidden !important;
}
</style>
