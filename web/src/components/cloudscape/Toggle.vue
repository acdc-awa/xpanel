<script setup lang="ts">
import { computed, useSlots } from 'vue'
import { CS } from './classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    modelValue?: boolean
    checked?: boolean
    label?: string
    description?: string
    disabled?: boolean
    readOnly?: boolean
    ariaLabel?: string
  }>(),
  {
    modelValue: undefined,
    checked: undefined,
    label: undefined,
    description: undefined,
    disabled: false,
    readOnly: false,
    ariaLabel: undefined,
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'change', detail: { checked: boolean }): void
}>()

const slots = useSlots()
const isChecked = computed(() => props.checked ?? props.modelValue ?? false)

function onChange(event: Event) {
  if (props.disabled || props.readOnly) {
    event.preventDefault()
    return
  }
  const next = (event.target as HTMLInputElement).checked
  emit('update:modelValue', next)
  emit('change', { checked: next })
}

const abstract = computed(() => CS['internal/components/abstract-switch'])
const controlClasses = computed(() =>
  cx(
    abstract.value.control,
    CS.toggle['toggle-control'],
    isChecked.value && CS.toggle['toggle-control-checked'],
    props.disabled && CS.toggle['toggle-control-disabled'],
    props.readOnly && CS.toggle['toggle-control-readonly']
  )
)
const handleClasses = computed(() =>
  cx(
    CS.toggle['toggle-handle'],
    isChecked.value && CS.toggle['toggle-handle-checked'],
    props.disabled && CS.toggle['toggle-handle-disabled'],
    props.readOnly && CS.toggle['toggle-handle-readonly']
  )
)
</script>

<template>
  <span :class="cx(abstract.wrapper, CS.toggle.root)">
    <span :class="abstract['label-wrapper']" :aria-disabled="disabled ? 'true' : undefined">
      <span :class="controlClasses">
        <input
          type="checkbox"
          :class="abstract['native-input']"
          :checked="isChecked"
          :disabled="disabled"
          :readonly="readOnly"
          :aria-label="ariaLabel"
          @change="onChange"
        />
        <span :class="handleClasses"></span>
        <span :class="cx(abstract.outline, CS.toggle.outline)"></span>
      </span>
      <span :class="cx(abstract.content, !label && !description && !slots.default && abstract['empty-content'])">
        <span v-if="label || slots.default" :class="cx(abstract.label, disabled && abstract['label-disabled'])">
          <slot>{{ label }}</slot>
        </span>
        <span
          v-if="description"
          :class="cx(abstract.description, disabled && abstract['description-disabled'], abstract['description-bottom-padding'])"
        >
          {{ description }}
        </span>
      </span>
    </span>
  </span>
</template>
