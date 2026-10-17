<script setup lang="ts">
import { computed } from 'vue'
import Box from './Box.vue'
import { CS } from './classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    value?: number
    status?: 'in-progress' | 'success' | 'error'
    variant?: 'standalone' | 'flash' | 'key-value'
    label?: string
    description?: string
    additionalInfo?: string
    resultText?: string
    ariaLabel?: string
  }>(),
  {
    value: 0,
    status: 'in-progress',
    variant: 'standalone',
    label: undefined,
    description: undefined,
    additionalInfo: undefined,
    resultText: undefined,
    ariaLabel: undefined,
  }
)

const clamped = computed(() => Math.max(0, Math.min(100, Math.round(props.value))))
const isComplete = computed(() => clamped.value >= 100)
</script>

<template>
  <div :class="cx(CS['progress-bar'].root, CS['progress-bar'][variant])">
    <div>
      <div v-if="label" :class="cx(CS['progress-bar']['word-wrap'], CS['progress-bar'][`label-${variant}`])">{{ label }}</div>
      <Box v-if="description" variant="small" display="block">{{ description }}</Box>
      <div>
        <template v-if="status === 'in-progress'">
          <div :class="CS['progress-bar']['progress-container']">
            <progress
              :class="cx(CS['progress-bar'].progress, isComplete && CS['progress-bar'].complete, variant === 'flash' && CS['progress-bar']['progress-flash'])"
              max="100"
              :value="clamped"
              :aria-label="ariaLabel"
            />
            <span aria-hidden="true" :class="CS['progress-bar']['percentage-container']">
              <span :class="CS['progress-bar'].percentage">{{ clamped }}%</span>
            </span>
          </div>
        </template>
        <div v-else :class="CS['progress-bar'][`result-container-${status}`]">
          <span :class="CS['progress-bar']['result-text']">{{ resultText }}</span>
        </div>
      </div>
      <div v-if="additionalInfo" :class="cx(CS['progress-bar']['word-wrap'], CS['progress-bar']['additional-info'])">
        {{ additionalInfo }}
      </div>
    </div>
  </div>
</template>
