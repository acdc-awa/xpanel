<script setup lang="ts">
import { computed, ref, useSlots } from 'vue'
import Icon from './Icon.vue'
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
    clearable?: boolean
    showPassword?: boolean
    showWordLimit?: boolean
    rows?: number
    maxlength?: number
    prefixIcon?: string
    min?: string | number
    max?: string | number
    step?: string | number
    size?: string
    ariaLabel?: string
    ariaLabelledby?: string
    ariaDescribedby?: string
    autocomplete?: string
  }>(),
  {
    modelValue: '',
    type: 'text',
    placeholder: undefined,
    disabled: false,
    readOnly: false,
    invalid: false,
    warning: false,
    clearable: false,
    showPassword: false,
    showWordLimit: false,
    rows: 3,
    maxlength: undefined,
    prefixIcon: undefined,
    min: undefined,
    max: undefined,
    step: undefined,
    size: undefined,
    ariaLabel: undefined,
    ariaLabelledby: undefined,
    ariaDescribedby: undefined,
    autocomplete: undefined,
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'change', detail: { value: string }): void
  (e: 'input', event: Event): void
  (e: 'keydown', event: KeyboardEvent): void
  (e: 'keyup', event: KeyboardEvent): void
  (e: 'clear'): void
  (e: 'blur', event: FocusEvent): void
  (e: 'focus', event: FocusEvent): void
}>()

const slots = useSlots()
const passwordVisible = ref(false)
const isTextarea = computed(() => props.type === 'textarea')
const hasValue = computed(() => props.modelValue !== '' && props.modelValue !== null && props.modelValue !== undefined)
const showClear = computed(() => props.clearable && hasValue.value && !props.disabled && !props.readOnly)
const showPasswordToggle = computed(() => props.showPassword && !isTextarea.value)
const hasEndIcon = computed(() => showClear.value || showPasswordToggle.value)
const hasStartIcon = computed(() => Boolean(props.prefixIcon || slots.icon))
const charCount = computed(() => String(props.modelValue ?? '').length)

const effectiveType = computed(() => {
  if (props.type === 'visualSearch') return 'text'
  if (props.type === 'password' && props.showPassword) {
    return passwordVisible.value ? 'text' : 'password'
  }
  return props.type
})

const inputClasses = computed(() =>
  cx(
    CS.input.input,
    props.type && CS.input[`input-type-${props.type}`],
    props.readOnly && CS.input['input-readonly'],
    props.invalid && CS.input['input-invalid'],
    props.warning && !props.invalid && CS.input['input-warning'],
    hasStartIcon.value && CS.input['input-has-icon-start'],
    hasEndIcon.value && CS.input['input-has-icon-end']
  )
)

function onInput(event: Event) {
  const value = (event.target as HTMLInputElement | HTMLTextAreaElement).value
  emit('update:modelValue', value)
  emit('change', { value })
  emit('input', event)
}

function clear() {
  emit('update:modelValue', '')
  emit('change', { value: '' })
  emit('clear')
}
</script>

<template>
  <div v-if="isTextarea" :class="[CS.textarea.root, 'cs-input-root']">
    <textarea
      :class="
        cx(
          CS.textarea.textarea,
          readOnly && CS.textarea['textarea-readonly'],
          invalid && CS.textarea['textarea-invalid'],
          warning && !invalid && CS.textarea['textarea-warning']
        )
      "
      :value="modelValue"
      :rows="rows"
      :placeholder="placeholder"
      :disabled="disabled"
      :readonly="readOnly"
      :maxlength="maxlength"
      :aria-label="ariaLabel"
      :aria-invalid="invalid ? 'true' : undefined"
      @input="onInput"
      @blur="emit('blur', $event)"
      @focus="emit('focus', $event)"
      @keydown="emit('keydown', $event)"
      @keyup="emit('keyup', $event)"
    />
    <span v-if="showWordLimit && maxlength" class="cs-input-word-count">{{ charCount }} / {{ maxlength }}</span>
  </div>
  <div v-else :class="[CS.input['input-container'], 'cs-input-root']">
    <span v-if="hasStartIcon" :class="CS.input['input-icon-start']">
      <slot name="icon"><Icon v-if="prefixIcon" :name="prefixIcon" /></slot>
    </span>
    <input
      :class="inputClasses"
      :type="effectiveType"
      :value="modelValue"
      :placeholder="placeholder"
      :disabled="disabled"
      :readonly="readOnly"
      :aria-label="ariaLabel"
      :aria-labelledby="ariaLabelledby"
      :aria-describedby="ariaDescribedby"
      :aria-invalid="invalid ? 'true' : undefined"
      :autocomplete="autocomplete"
      :maxlength="maxlength"
      :min="min"
      :max="max"
      :step="step"
      @input="onInput"
      @keydown="emit('keydown', $event)"
      @keyup="emit('keyup', $event)"
      @blur="emit('blur', $event)"
      @focus="emit('focus', $event)"
    />
    <span v-if="hasEndIcon" :class="CS.input['input-icon-end']">
      <button
        v-if="showClear"
        type="button"
        :class="CS.input['input-button-right']"
        aria-label="清除"
        @click="clear"
      >
        <Icon name="close" size="x-small" />
      </button>
      <button
        v-if="showPasswordToggle"
        type="button"
        :class="CS.input['input-button-right']"
        :aria-label="passwordVisible ? '隐藏密码' : '显示密码'"
        @click="passwordVisible = !passwordVisible"
      >
        <svg viewBox="0 0 16 16" width="14" height="14" aria-hidden="true" fill="none" stroke="currentColor">
          <path v-if="passwordVisible" d="M1 8s2.5-4.5 7-4.5S15 8 15 8s-2.5 4.5-7 4.5S1 8 1 8Z" />
          <circle cx="8" cy="8" r="2.2" />
          <path v-if="!passwordVisible" d="M2 14 14 2" />
        </svg>
      </button>
    </span>
    <span v-if="showWordLimit && maxlength" class="cs-input-word-count">{{ charCount }} / {{ maxlength }}</span>
  </div>
</template>

<style scoped lang="scss">
.cs-input-root {
  position: relative;
}

.cs-input-word-count {
  position: absolute;
  inset-inline-end: 0;
  top: 100%;
  margin-top: 2px;
  font-size: 11px;
  line-height: 14px;
  color: var(--color-text-body-secondary, #656871);
  pointer-events: none;
}
</style>
