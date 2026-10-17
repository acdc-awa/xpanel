<script setup lang="ts">
import { computed } from 'vue'
import { CS } from './classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    modelValue?: string | number
    type?: string
    placeholder?: string
    disabled?: boolean
    readOnly?: boolean
    invalid?: boolean
    warning?: boolean
    ariaLabel?: string
    ariaLabelledby?: string
    ariaDescribedby?: string
    autocomplete?: string
    step?: string | number
  }>(),
  {
    modelValue: '',
    type: 'text',
    placeholder: undefined,
    disabled: false,
    readOnly: false,
    invalid: false,
    warning: false,
    ariaLabel: undefined,
    ariaLabelledby: undefined,
    ariaDescribedby: undefined,
    autocomplete: undefined,
    step: undefined,
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'change', detail: { value: string }): void
  (e: 'input', event: Event): void
  (e: 'keydown', event: KeyboardEvent): void
}>()

const classes = computed(() =>
  cx(
    CS.input.input,
    props.type && CS.input[`input-type-${props.type}`],
    props.readOnly && CS.input['input-readonly'],
    props.invalid && CS.input['input-invalid'],
    props.warning && !props.invalid && CS.input['input-warning']
  )
)

function onInput(event: Event) {
  const value = (event.target as HTMLInputElement).value
  emit('update:modelValue', value)
  emit('change', { value })
  emit('input', event)
}
</script>

<template>
  <input
    :class="classes"
    :type="type === 'visualSearch' ? 'text' : type"
    :value="modelValue"
    :placeholder="placeholder"
    :disabled="disabled"
    :readonly="readOnly"
    :aria-label="ariaLabel"
    :aria-labelledby="ariaLabelledby"
    :aria-describedby="ariaDescribedby"
    :aria-invalid="invalid ? 'true' : undefined"
    :autocomplete="autocomplete"
    :step="step"
    @input="onInput"
    @keydown="emit('keydown', $event)"
  />
</template>
