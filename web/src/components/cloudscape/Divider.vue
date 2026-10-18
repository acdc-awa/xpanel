<script setup lang="ts">
/**
 * Vue port of the official Cloudscape Divider component.
 * Mirrors divider/internal.js: <hr> for a plain line, a flex row of
 * line / label / line when a label is supplied.
 */
import { computed } from 'vue'
import { DIVIDER } from './steps-classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    orientation?: 'horizontal' | 'vertical'
  }>(),
  { orientation: 'horizontal' }
)

const hasLabel = computed(() => props.orientation !== 'vertical')
</script>

<template>
  <div
    v-if="$slots.default && hasLabel"
    :class="cx(DIVIDER.divider, DIVIDER.horizontal, DIVIDER['has-label'])"
  >
    <span :class="DIVIDER.line" />
    <span :class="DIVIDER.label"><slot /></span>
    <span :class="DIVIDER.line" />
  </div>
  <hr
    v-else
    role="presentation"
    :class="
      cx(DIVIDER.divider, orientation === 'vertical' ? DIVIDER.vertical : DIVIDER.horizontal)
    "
  />
</template>
