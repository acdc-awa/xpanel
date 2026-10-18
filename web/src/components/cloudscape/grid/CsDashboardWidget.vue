<script setup lang="ts">
import { computed, inject, onMounted, onUnmounted, ref } from 'vue'
import Icon from '../Icon.vue'
import Container from '../Container.vue'
import Header from '../Header.vue'
import ButtonDropdown from '../ButtonDropdown.vue'
import { CSB } from '../board-classes'
import { cx } from '../utils'

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
  if (rootRef.value) gridContext?.registerItem(rootRef.value)
})

onUnmounted(() => {
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
            <div :class="headerClass">
              <div :class="cx(CSB['board-item'].fixed, CSB['board-item'].handle)">
                <span
                  class="awsui-drag-handle"
                  :class="CSB['internal/drag-handle'].handle"
                  title="拖拽调整小组件位置"
                >
                  <Icon name="drag-indicator" />
                </span>
              </div>
              <div :class="cx(CSB['board-item'].flexible, CSB['board-item']['header-content'])">
                <Header variant="h2" :counter="badge">
                  {{ title }}
                </Header>
              </div>
              <div :class="cx(CSB['board-item'].fixed, CSB['board-item'].settings, 'widget-settings')">
                <slot name="actions" />
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
