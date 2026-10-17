<script setup lang="ts">
import { computed, inject, onMounted, onUnmounted, ref } from 'vue'
import Icon from '../Icon.vue'
import Button from '../Button.vue'
import Container from '../Container.vue'
import Header from '../Header.vue'
import { CSB } from '../board-classes'
import { cx } from '../utils'
import { ElDropdown, ElDropdownItem, ElDropdownMenu } from 'element-plus'

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

function handleCommand(cmd: string) {
  if (cmd === 'remove') {
    gridContext?.requestWidgetRemove(props.id)
    emit('remove', props.id)
  } else if (cmd === 'refresh') {
    emit('refresh', props.id)
  }
}

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
                <el-dropdown trigger="click" @command="handleCommand">
                  <Button variant="icon" icon-name="menu" aria-label="小组件选项" />
                  <template #dropdown>
                    <el-dropdown-menu>
                      <el-dropdown-item command="refresh">刷新此卡片</el-dropdown-item>
                      <el-dropdown-item command="remove" divided>移除小组件</el-dropdown-item>
                    </el-dropdown-menu>
                  </template>
                </el-dropdown>
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
  overflow: hidden;
}

/* Container fit-height 的内容区默认 overflow:auto，会在卡片右下角叠出一条滚动条，
   与缩放手柄挤在一起；widget 内部（表格/图表）各自管理滚动，这里直接裁掉外层滚动。 */
.dashboard-widget-container :deep([class*='content-fit-height']) {
  overflow: hidden !important;
}
</style>
