<script setup lang="ts">
import { CS } from './classes'
import { cx } from './utils'

withDefaults(
  defineProps<{
    label?: string
    description?: string
    errorText?: string
    warningText?: string
    constraintText?: string
    stretch?: boolean
    labelHidden?: boolean
  }>(),
  {
    label: undefined,
    description: undefined,
    errorText: undefined,
    warningText: undefined,
    constraintText: undefined,
    stretch: false,
    labelHidden: false,
  }
)
</script>

<template>
  <div :class="cx(CS['form-field'].root, stretch && CS['form-field'].stretch)">
    <div :class="cx(CS['form-field']['label-wrapper'], labelHidden && CS['form-field']['label-hidden'])">
      <label v-if="label" :class="CS['form-field'].label">{{ label }}</label>
      <span v-if="$slots.info" :class="CS['form-field'].info"><slot name="info" /></span>
    </div>
    <div v-if="description" :class="CS['form-field'].description">{{ description }}</div>
    <div :class="CS['form-field'].controls"><slot /></div>
    <div
      :class="
        cx(
          CS['form-field'].constraint,
          (errorText || warningText) && CS['form-field']['constraint-has-validation-text']
        )
      "
    >
      <span v-if="errorText" :class="CS['form-field']['error__message']">{{ errorText }}</span>
      <span v-else-if="warningText" :class="CS['form-field']['warning__message']">{{ warningText }}</span>
      <span v-else>{{ constraintText }}</span>
    </div>
  </div>
</template>
