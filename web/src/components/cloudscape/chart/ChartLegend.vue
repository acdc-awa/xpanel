<script setup lang="ts">
import { CHART } from './classes'
import ChartSeriesMarker from './ChartSeriesMarker.vue'

export interface ChartLegendItem {
  title: string
  color: string
  type: 'line' | 'rectangle' | 'dashed'
}

defineProps<{
  items: ChartLegendItem[]
  legendTitle?: string
  /** start = 官方行为（靠左）；center = 整行居中。 */
  align?: 'start' | 'center'
  highlightedTitle?: string | null
}>()

const emit = defineEmits<{
  (e: 'hover', title: string | null): void
}>()
</script>

<template>
  <div :class="CHART.legend.root">
    <div v-if="legendTitle" :class="CHART.legend.title">{{ legendTitle }}</div>
    <div
      :class="CHART.legend.list"
      :style="align === 'center' ? { justifyContent: 'center' } : undefined"
      role="toolbar"
      :aria-label="legendTitle || '图例'"
    >
      <div
        v-for="item in items"
        :key="item.title"
        role="button"
        tabindex="0"
        :aria-pressed="highlightedTitle === item.title"
        :class="[
          CHART.legend.marker,
          highlightedTitle && highlightedTitle !== item.title ? CHART.legend['marker--dimmed'] : '',
        ]"
        @mouseenter="emit('hover', item.title)"
        @mouseleave="emit('hover', null)"
        @focus="emit('hover', item.title)"
        @blur="emit('hover', null)"
      >
        <ChartSeriesMarker :type="item.type" :color="item.color" />
        <span>{{ item.title }}</span>
      </div>
    </div>
  </div>
</template>
