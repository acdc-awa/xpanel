<script setup lang="ts">
import Checkbox from './Checkbox.vue'

export interface CheckboxGroupOption {
  value: string | number
  label?: string
  description?: string
  disabled?: boolean
}

const props = withDefaults(
  defineProps<{
    modelValue?: Array<string | number>
    options?: CheckboxGroupOption[]
    disabled?: boolean
    ariaLabel?: string
  }>(),
  {
    modelValue: () => [],
    options: () => [],
    disabled: false,
    ariaLabel: undefined,
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: Array<string | number>): void
  (e: 'change', value: Array<string | number>): void
}>()

function toggle(value: string | number, checked: boolean) {
  const current = props.modelValue ?? []
  const next = checked ? [...current.filter((item) => item !== value), value] : current.filter((item) => item !== value)
  emit('update:modelValue', next)
  emit('change', next)
}
</script>

<template>
  <!-- 官方没有 CheckboxGroup 组件；标准做法是 FormField/FormItem 承担标签，内部纵向排列 Checkbox。 -->
  <div role="group" :aria-label="ariaLabel">
    <Checkbox
      v-for="option in options"
      :key="String(option.value)"
      :checked="(modelValue ?? []).includes(option.value)"
      :label="option.label"
      :description="option.description"
      :disabled="disabled || option.disabled"
      :value="option.value"
      @change="(checked: boolean) => toggle(option.value, checked)"
    />
  </div>
</template>
