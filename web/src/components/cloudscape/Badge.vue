<script setup lang="ts">
import { computed } from 'vue'
import { CS } from './classes'
import { cx } from './utils'

export type BadgeColor =
  | 'grey'
  | 'green'
  | 'blue'
  | 'red'
  | 'severity-critical'
  | 'severity-high'
  | 'severity-medium'
  | 'severity-low'
  | 'severity-neutral'

const props = withDefaults(
  defineProps<{
    color?: BadgeColor
  }>(),
  { color: 'grey' }
)

const classes = computed(() => cx(CS.badge.badge, CS.badge[`badge-color-${props.color}`]))
</script>

<template>
  <span :class="classes"><slot /></span>
</template>
