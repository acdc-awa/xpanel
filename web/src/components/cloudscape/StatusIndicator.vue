<script setup lang="ts">
import { computed } from 'vue'
import Icon from './Icon.vue'
import Spinner from './Spinner.vue'
import { CS } from './classes'
import { cx } from './utils'

export type StatusType =
  | 'error'
  | 'warning'
  | 'success'
  | 'info'
  | 'stopped'
  | 'pending'
  | 'in-progress'
  | 'loading'
  | 'not-started'
  | 'log'
  // Convenience aliases used across the panel.
  | 'online'
  | 'offline'
  | 'healthy'
  | 'degraded'
  | 'danger'

const props = withDefaults(
  defineProps<{
    type?: StatusType
    status?: StatusType
    text?: string
    display?: 'inline' | 'inline-block'
    size?: 'small' | 'normal'
    ariaLabel?: string
    wrapText?: boolean
  }>(),
  {
    type: undefined,
    status: undefined,
    text: '',
    display: 'inline',
    size: 'normal',
    ariaLabel: undefined,
    wrapText: true,
  }
)

const aliases: Record<string, string> = {
  online: 'success',
  healthy: 'success',
  offline: 'stopped',
  degraded: 'warning',
  danger: 'error',
}

const resolvedType = computed(() => {
  const raw = props.type ?? props.status ?? 'info'
  return aliases[raw] ?? raw
})

const iconName = computed(() => {
  switch (resolvedType.value) {
    case 'error':
      return 'status-negative'
    case 'warning':
      return 'status-warning'
    case 'success':
      return 'status-positive'
    case 'stopped':
      return 'status-stopped'
    case 'pending':
      return 'status-pending'
    case 'in-progress':
      return 'status-in-progress'
    case 'not-started':
      return 'status-not-started'
    case 'log':
      return 'dot'
    default:
      return 'status-info'
  }
})

const rootClasses = computed(() => cx(CS['status-indicator'].root, CS['status-indicator'][`status-${resolvedType.value}`]))
const containerClasses = computed(() =>
  cx(
    CS['status-indicator'].container,
    CS['status-indicator'][`display-${props.display}`],
    !props.wrapText && CS['status-indicator']['overflow-ellipsis']
  )
)
</script>

<template>
  <span :class="rootClasses">
    <span :class="containerClasses">
      <span :class="CS['status-indicator'].icon" :aria-label="ariaLabel" :role="ariaLabel ? 'img' : undefined">
        <Spinner v-if="resolvedType === 'loading'" size="normal" />
        <Icon v-else :name="iconName" :size="size" />
      </span>
      <span><slot>{{ text }}</slot></span>
    </span>
  </span>
</template>
