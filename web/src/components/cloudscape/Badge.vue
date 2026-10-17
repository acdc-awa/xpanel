<script setup lang="ts">
import { computed } from 'vue'
import { CS } from './classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    color?: 'grey' | 'green' | 'blue' | 'red' | 'severity-critical' | 'severity-high' | 'severity-medium' | 'severity-low' | 'severity-neutral'
  }>(),
  { color: 'grey' }
)

const classes = computed(() => cx(CS.badge.badge, CS.badge[`badge-color-${props.color}`]))
</script>

<template>
  <span :class="classes"><slot /></span>
</template>
