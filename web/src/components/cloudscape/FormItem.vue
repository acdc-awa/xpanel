<script setup lang="ts">
import { computed, inject, useSlots } from 'vue'
import FormField from './FormField.vue'

const props = withDefaults(
  defineProps<{
    label?: string
    required?: boolean
    error?: string
    warning?: string
    description?: string
    constraintText?: string
    labelPosition?: 'top' | 'left'
    labelWidth?: string | number
    /** Element Plus 兼容占位：Cloudscape 不使用 rules 引擎 */
    prop?: string
    rules?: unknown
  }>(),
  {
    label: undefined,
    required: false,
    error: undefined,
    warning: undefined,
    description: undefined,
    constraintText: undefined,
    labelPosition: undefined,
    labelWidth: undefined,
    prop: undefined,
    rules: undefined,
  }
)

const inheritedPosition = inject<'top' | 'left'>('csFormLabelPosition', 'top')
const position = computed(() => props.labelPosition ?? inheritedPosition)
const width = computed(() =>
  typeof props.labelWidth === 'number' ? `${props.labelWidth}px` : props.labelWidth
)
const slots = useSlots()
</script>

<template>
  <div
    class="cs-form-item"
    :class="position === 'left' && 'cs-form-item-horizontal'"
    :style="width ? { '--cs-form-label-width': width } : undefined"
  >
    <FormField
      :label="label"
      :required="required"
      :error-text="error"
      :warning-text="warning"
      :description="description"
      :constraint-text="constraintText"
    >
      <template #label><slot name="label">{{ label }}</slot></template>
      <template v-if="slots.info" #info><slot name="info" /></template>
      <slot />
    </FormField>
  </div>
</template>

<style scoped>
.cs-form-item {
  margin-block-end: 16px;
}

.cs-form-item-horizontal {
  display: grid;
  grid-template-columns: var(--cs-form-label-width, 120px) minmax(0, 1fr);
  gap: 12px;
  align-items: start;
}

.cs-form-item-horizontal :deep([class*='label-wrapper']) {
  padding-block: 6px;
  margin-block-end: 0;
}
</style>
