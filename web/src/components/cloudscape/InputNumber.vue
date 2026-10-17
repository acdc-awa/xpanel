<script setup lang="ts">
import { computed } from 'vue'
import Input from './Input.vue'

const props = withDefaults(
  defineProps<{
    modelValue?: number | null
    min?: number
    max?: number
    step?: number
    precision?: number
    placeholder?: string
    disabled?: boolean
    controls?: boolean
    size?: string
    style?: string | Record<string, string>
  }>(),
  {
    modelValue: undefined,
    min: undefined,
    max: undefined,
    step: undefined,
    precision: undefined,
    placeholder: undefined,
    disabled: false,
    controls: true,
    size: undefined,
    style: undefined,
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: number | null): void
  (e: 'change', value: number | null): void
}>()

const displayValue = computed(() =>
  props.modelValue === null || props.modelValue === undefined ? '' : String(props.modelValue)
)

function normalize(value: number): number | null {
  let next = value
  if (props.precision !== undefined) {
    const factor = 10 ** props.precision
    next = Math.round(next * factor) / factor
  }
  if (props.min !== undefined && next < props.min) next = props.min
  if (props.max !== undefined && next > props.max) next = props.max
  return next
}

function onUpdate(raw: string) {
  if (raw === '' || raw === '-') {
    emit('update:modelValue', null)
    emit('change', null)
    return
  }
  const parsed = Number(raw)
  if (Number.isNaN(parsed)) return
  emit('update:modelValue', parsed)
  emit('change', parsed)
}

function onBlur() {
  if (props.modelValue === null || props.modelValue === undefined) return
  const next = normalize(props.modelValue)
  if (next !== props.modelValue) {
    emit('update:modelValue', next)
    emit('change', next)
  }
}
</script>

<template>
  <Input
    type="number"
    :model-value="displayValue"
    :placeholder="placeholder"
    :disabled="disabled"
    :min="min"
    :max="max"
    :step="step"
    :style="style"
    @update:model-value="onUpdate"
    @blur="onBlur"
  />
</template>
