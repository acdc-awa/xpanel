<script setup lang="ts">
import { computed } from 'vue'
import Icon from './Icon.vue'
import { CS } from './classes'
import { cx } from './utils'

export interface SegmentedOption {
  id: string | number
  text: string
  iconName?: string
  disabled?: boolean
}

const props = withDefaults(
  defineProps<{
    selectedId?: string | number
    modelValue?: string | number
    options: SegmentedOption[]
    label?: string
    fullWidth?: boolean
  }>(),
  { selectedId: undefined, modelValue: undefined, label: undefined, fullWidth: false }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string | number): void
  (e: 'change', detail: { selectedId: string | number }): void
}>()

const activeId = computed(() => props.selectedId ?? props.modelValue)
const isActive = (option: SegmentedOption) => activeId.value !== undefined && String(option.id) === String(activeId.value)

function select(option: SegmentedOption) {
  if (option.disabled || isActive(option)) return
  emit('update:modelValue', option.id)
  emit('change', { selectedId: option.id })
}
</script>

<template>
  <div :class="CS['segmented-control'].root">
    <div
      :class="
        cx(
          CS['segmented-control']['segment-part'],
          CS['segmented-control'].refresh,
          CS['segmented-control'][`segment-count-${Math.min(options.length, 6)}`],
          fullWidth && CS['segmented-control']['full-width']
        )
      "
      role="group"
      :aria-label="label"
    >
      <button
        v-for="option in options"
        :key="option.id"
        type="button"
        :class="
          cx(
            CS['segmented-control'].segment,
            CS['segmented-control'].refresh,
            isActive(option) && CS['segmented-control'].selected,
            option.disabled && CS['segmented-control'].disabled,
            fullWidth && CS['segmented-control']['full-width']
          )
        "
        :disabled="option.disabled"
        :aria-pressed="isActive(option) ? 'true' : 'false'"
        @click="select(option)"
      >
        <Icon
          v-if="option.iconName"
          :name="option.iconName"
          :variant="option.disabled ? 'disabled' : 'normal'"
          :class="cx(CS['segmented-control'].icon, option.text ? CS['segmented-control']['with-text'] : CS['segmented-control']['with-no-text'])"
        />
        <span>{{ option.text }}</span>
      </button>
    </div>
  </div>
</template>
