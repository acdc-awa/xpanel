<script setup lang="ts">
import { computed } from 'vue'
import { CsModal, CsButton, CsBadge, CsIcon } from '@/components/cloudscape'
import { WIDGET_DEFINITIONS } from '@/components/cloudscape'
import type { WidgetLayoutItem } from '@/components/cloudscape'

const props = defineProps<{
  visible: boolean
  currentLayout: WidgetLayoutItem[]
}>()

const emit = defineEmits<{
  (e: 'update:visible', val: boolean): void
  (e: 'add-widget', id: string): void
}>()

const activeIds = computed(() => new Set(props.currentLayout.map((i) => i.id)))

const widgetList = computed(() => {
  return Object.values(WIDGET_DEFINITIONS).map((def) => ({
    ...def,
    isAdded: activeIds.value.has(def.id),
  }))
})

function handleAdd(id: string) {
  emit('add-widget', id)
}
</script>

<template>
  <CsModal
    :visible="visible"
    header="添加小组件至大盘 (Add Widgets)"
    size="medium"
    @update:visible="(val) => emit('update:visible', val)"
    @close="emit('update:visible', false)"
  >
    <div class="add-widgets-modal-body">
      <p class="modal-subtext">
        选择想要展示在运维大盘上的监控与分析组件。添加后可直接在页面中拖拽左上角把手或拉伸右下角调整排版。
      </p>

      <div class="widget-selection-list">
        <div
          v-for="item in widgetList"
          :key="item.id"
          class="widget-selection-card"
          :class="{ 'is-active': item.isAdded }"
        >
          <div class="card-left">
            <div class="card-title-row">
              <span class="card-title">{{ item.title }}</span>
              <CsBadge color="blue">{{ item.defaultW }} × {{ item.defaultH }} 格</CsBadge>
            </div>
            <p class="card-desc">{{ item.description }}</p>
          </div>

          <div class="card-right">
            <CsButton
              v-if="!item.isAdded"
              variant="primary"
              icon-name="add-plus"
              @click="handleAdd(item.id)"
            >
              添加
            </CsButton>
            <div v-else class="already-added-tag">
              <CsIcon name="status-positive" size="small" />
              <span>已添加</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <template #footer>
      <CsButton variant="normal" @click="emit('update:visible', false)">完成</CsButton>
    </template>
  </CsModal>
</template>

<style scoped lang="scss">
.add-widgets-modal-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.modal-subtext {
  font-size: 13px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  margin: 0;
  line-height: 1.5;
}

.widget-selection-list {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.widget-selection-card {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 16px;
  background-color: var(--color-background-container-content-mjfil9, #ffffff);
  border: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
  border-radius: 8px;
  transition: all 0.15s ease-in-out;

  &:hover {
    border-color: var(--color-text-link-default-qabnuk, #006ce0);
    background-color: var(--color-background-container-content-mjfil9, #ffffff);
    box-shadow: var(--shadow-container-quglkl, 0 2px 6px rgba(0, 28, 36, 0.08));
  }

  &.is-active {
    background-color: var(--color-background-container-content-mjfil9, #ffffff);
    border-color: var(--color-border-divider-default-diwd4w, #e9ebed);
  }
}

.card-left {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
  flex: 1;
}

.card-title-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.card-title {
  font-size: 14px;
  font-weight: 700;
  color: var(--color-text-body-default-b2us04, #0f141a);
}

.card-desc {
  font-size: 12px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  margin: 0;
  line-height: 1.4;
}

.card-right {
  flex-shrink: 0;
}

.already-added-tag {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  font-weight: 600;
  color: var(--color-text-status-success-1i2jf8, #1d8102);
  background-color: var(--color-background-status-success-1woffs, #f2f8f0);
  padding: 4px 10px;
  border-radius: 6px;
}
</style>
