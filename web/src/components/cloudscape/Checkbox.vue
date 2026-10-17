<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { CS } from './classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    modelValue?: boolean
    checked?: boolean
    indeterminate?: boolean
    label?: string
    description?: string
    disabled?: boolean
    readOnly?: boolean
    value?: string | number
    ariaLabel?: string
  }>(),
  {
    modelValue: undefined,
    checked: undefined,
    indeterminate: false,
    label: undefined,
    description: undefined,
    disabled: false,
    readOnly: false,
    value: undefined,
    ariaLabel: undefined,
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'change', value: boolean): void
}>()

const inputRef = ref<HTMLInputElement | null>(null)
const isChecked = computed(() => props.checked ?? props.modelValue ?? false)

watch(
  () => props.indeterminate,
  (value) => {
    if (inputRef.value) inputRef.value.indeterminate = Boolean(value)
  },
  { immediate: true }
)

function onChange(event: Event) {
  if (props.disabled || props.readOnly) {
    event.preventDefault()
    return
  }
  const next = (event.target as HTMLInputElement).checked
  emit('update:modelValue', next)
  emit('change', next)
}

const abstract = computed(() => CS['internal/components/abstract-switch'])
const icon = computed(() => CS['internal/components/checkbox-icon'])
</script>

<template>
  <label
    :class="
      cx(
        abstract.wrapper,
        CS.checkbox.root,
        disabled && abstract['label-disabled']
      )
    "
  >
    <span :class="abstract['label-wrapper']" :aria-disabled="disabled ? 'true' : undefined">
      <span :class="cx(abstract.control, CS.checkbox['checkbox-control'])">
        <input
          ref="inputRef"
          type="checkbox"
          :class="abstract['native-input']"
          :checked="isChecked"
          :disabled="disabled"
          :value="value"
          :aria-label="ariaLabel"
          @change="onChange"
        />
        <svg :class="icon.root" viewBox="0 0 16 16" aria-hidden="true" focusable="false">
          <rect
            :class="
              cx(
                icon['styled-box'],
                isChecked && icon['styled-box-checked'],
                indeterminate && icon['styled-box-indeterminate'],
                disabled && icon['styled-box-disabled']
              )
            "
            x="0.5"
            y="0.5"
            rx="3"
            ry="3"
            width="15"
            height="15"
          />
          <polyline
            v-if="isChecked || indeterminate"
            :class="cx(icon['styled-line'], disabled && icon['styled-line-disabled'])"
            :points="indeterminate ? '3.5,8 12.5,8' : '3.5,8 7,11 12,4'"
          />
        </svg>
        <span :class="cx(abstract.outline, CS.checkbox.outline)"></span>
      </span>
      <span :class="abstract.content">
        <span v-if="label || $slots.default" :class="abstract.label">
          <slot>{{ label }}</slot>
        </span>
        <span v-if="description" :class="abstract.description">{{ description }}</span>
      </span>
    </span>
  </label>
</template>
