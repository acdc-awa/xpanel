<script setup lang="ts">
import { provide } from 'vue'
import Alert from './Alert.vue'
import { CS } from './classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    variant?: 'full-page' | 'embedded'
    header?: string
    errorText?: string
    /** 与 Element Plus 对齐：Cloudscape 官方表单标签在控件上方，left 仅作为过渡兼容 */
    labelPosition?: 'top' | 'left'
  }>(),
  {
    variant: 'embedded',
    header: undefined,
    errorText: undefined,
    labelPosition: 'top',
  }
)

provide('csFormLabelPosition', props.labelPosition)
</script>

<template>
  <form :class="cx(CS.form.root, variant === 'embedded' && 'cs-form-embedded')" @submit.prevent>
    <div v-if="header || $slots.header" :class="CS.form.header">
      <slot name="header">{{ header }}</slot>
    </div>
    <div :class="CS.form.content"><slot /></div>
    <div v-if="errorText" class="cs-form-error">
      <Alert type="error">{{ errorText }}</Alert>
    </div>
    <div v-if="$slots.actions || $slots.secondaryActions" :class="CS.form.footer">
      <div :class="CS.form['actions-section']">
        <div v-if="$slots.actions" :class="CS.form.actions"><slot name="actions" /></div>
        <div v-if="$slots.secondaryActions" :class="CS.form['secondary-actions']">
          <slot name="secondaryActions" />
        </div>
      </div>
    </div>
  </form>
</template>

<style scoped>
.cs-form-error {
  margin-top: 16px;
}
</style>
