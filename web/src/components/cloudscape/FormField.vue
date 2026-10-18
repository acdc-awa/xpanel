<script setup lang="ts">
import { computed, useSlots } from 'vue'
import Icon from './Icon.vue'
import { CS } from './classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    label?: string
    description?: string
    errorText?: string
    warningText?: string
    constraintText?: string
    required?: boolean
    controlId?: string
    stretch?: boolean
    labelHidden?: boolean
  }>(),
  {
    label: undefined,
    description: undefined,
    errorText: undefined,
    warningText: undefined,
    constraintText: undefined,
    required: false,
    controlId: undefined,
    stretch: false,
    labelHidden: false,
  }
)

const slots = useSlots()
const hasHints = computed(() => Boolean(props.constraintText || props.errorText || props.warningText))
const showWarning = computed(() => Boolean(props.warningText && !props.errorText))
</script>

<template>
  <div :class="cx(CS['form-field'].root, stretch && CS['form-field'].stretch)">
    <div
      :class="
        cx(
          CS['form-field']['label-wrapper'],
          labelHidden && CS['form-field']['visually-hidden']
        )
      "
    >
      <label v-if="label || $slots.label" :class="CS['form-field'].label" :for="controlId">
        <slot name="label">{{ label }}</slot>
        <span v-if="required" class="cs-form-field-required" aria-hidden="true"> *</span>
      </label>
      <span v-if="slots.info" :class="CS['form-field'].info"><slot name="info" /></span>
    </div>
    <div v-if="description" :class="CS['form-field'].description">{{ description }}</div>
    <div :class="cx(CS['form-field'].controls, labelHidden && CS['form-field']['label-hidden'])">
      <div :class="CS['form-field'].control"><slot /></div>
      <div v-if="slots.secondaryControl" :class="CS['form-field']['secondary-control']">
        <slot name="secondaryControl" />
      </div>
    </div>
    <div v-if="hasHints" :class="CS['form-field'].hints">
      <div v-if="errorText" :class="[CS['form-field'].error, 'cs-form-field-validation']">
        <div :class="CS['form-field']['error-icon-shake-wrapper']">
          <div :class="CS['form-field']['error-icon-scale-wrapper']">
            <Icon name="status-negative" size="small" />
          </div>
        </div>
        <span :class="CS['form-field']['error__message']">{{ errorText }}</span>
      </div>
      <div v-else-if="showWarning" :class="[CS['form-field'].warning, 'cs-form-field-validation']">
        <div :class="CS['form-field']['warning-icon-shake-wrapper']">
          <div :class="CS['form-field']['warning-icon-scale-wrapper']">
            <Icon name="status-warning" size="small" />
          </div>
        </div>
        <span :class="CS['form-field']['warning__message']">{{ warningText }}</span>
      </div>
      <div
        v-if="constraintText"
        :class="
          cx(
            CS['form-field'].constraint,
            (errorText || showWarning) && CS['form-field']['constraint-has-validation-text']
          )
        "
      >
        <span>{{ constraintText }}</span>
      </div>
    </div>
  </div>
</template>

<style scoped>
.cs-form-field-required {
  color: var(--color-text-status-error-ri38gt, #d91515);
}
</style>
