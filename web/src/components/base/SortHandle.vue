<script setup lang="ts">
import { CsIcon } from '@/components/cloudscape'

// 卡片拖拽排序的抓手（唯一实现）：所有列表页共用同一图标/尺寸/hover 与 data 属性。
// 拖拽逻辑在 useCardDragSort；这里只把 dragstart/dragend 抛给卡片自身（用于拖动中降透明度）。
withDefaults(
  defineProps<{
    title?: string
  }>(),
  { title: '拖拽调整顺序' },
)

const emit = defineEmits<{
  (e: 'dragstart', event: DragEvent): void
  (e: 'dragend', event: DragEvent): void
}>()
</script>

<template>
  <span
    class="cs-sort-handle"
    draggable="true"
    data-sort-handle
    :title="title"
    @dragstart="emit('dragstart', $event)"
    @dragend="emit('dragend', $event)"
  >
    <CsIcon name="drag-indicator" />
  </span>
</template>

<style scoped>
.cs-sort-handle {
  display: inline-flex;
  flex: none;
  align-items: center;
  justify-content: center;
  padding: 2px;
  border-radius: 4px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  cursor: grab;
}

.cs-sort-handle:hover {
  color: var(--color-text-body-default-b2us04, #000716);
  background: var(--color-background-button-normal-hover-1ajrtx, rgba(0, 7, 22, 0.06));
}

.cs-sort-handle:active {
  cursor: grabbing;
}
</style>
