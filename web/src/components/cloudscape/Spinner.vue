<script setup lang="ts">
import { computed } from 'vue'
import { CS } from './classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    size?: 'x-small' | 'normal' | 'big' | 'large'
    variant?: 'normal' | 'disabled' | 'inverted'
    ariaLabel?: string
  }>(),
  {
    size: 'normal',
    variant: 'normal',
    ariaLabel: undefined,
  }
)

const classes = computed(() => cx(CS.spinner.root, CS.spinner[`size-${props.size}`], CS.spinner[`variant-${props.variant}`]))
</script>

<template>
  <span :class="classes" :role="ariaLabel ? 'img' : undefined" :aria-label="ariaLabel">
    <span :class="cx(CS.spinner.circle, CS.spinner['circle-left'])"></span>
    <span :class="cx(CS.spinner.circle, CS.spinner['circle-right'])"></span>
  </span>
</template>
