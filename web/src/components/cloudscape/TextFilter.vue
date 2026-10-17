<script setup lang="ts">
import Input from './Input.vue'
import { CS } from './classes'
import { cx } from './utils'

withDefaults(
  defineProps<{
    modelValue: string
    filteringPlaceholder?: string
    filteringAriaLabel?: string
    countText?: string
    disabled?: boolean
  }>(),
  {
    filteringPlaceholder: 'Filter',
    filteringAriaLabel: 'Filter',
    countText: undefined,
    disabled: false,
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'change', detail: { filteringText: string }): void
}>()

function onUpdate(value: string) {
  emit('update:modelValue', value)
  emit('change', { filteringText: value })
}
</script>

<template>
  <div :class="CS['text-filter'].root">
    <Input
      :model-value="modelValue"
      type="search"
      :class="cx(CS['text-filter'].input)"
      :placeholder="filteringPlaceholder"
      :aria-label="filteringAriaLabel"
      :disabled="disabled"
      @update:model-value="onUpdate"
    />
    <div v-if="countText" :class="CS['text-filter'].results">{{ countText }}</div>
  </div>
</template>
