<script setup lang="ts">
// Port of the official Cloudscape ChartSeriesMarker.
import { computed } from 'vue'
import { CHART } from './classes'
import { cx } from '../utils'

const props = withDefaults(
  defineProps<{
    type?: 'line' | 'rectangle' | 'hollow-rectangle' | 'dashed'
    color: string
  }>(),
  { type: 'line' }
)

const classes = computed(() => cx(CHART.marker.marker, CHART.marker[`marker--${props.type}`]))
</script>

<template>
  <span :class="classes" :style="{ backgroundColor: color }" aria-hidden="true" />
</template>
