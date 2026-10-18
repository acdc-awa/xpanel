<script setup lang="ts">
/**
 * Vue port of the official Cloudscape Steps component.
 * Markup follows steps/internal.js (horizontal + vertical variants, optional
 * annotations, optional connector lines) so the vendored scoped CSS applies.
 */
import { computed } from 'vue'
import Box from './Box.vue'
import Icon from './Icon.vue'
import Spinner from './Spinner.vue'
import { STEPS } from './steps-classes'
import { cx } from './utils'

export type StepsStatus =
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

export interface StepItem {
  status: StepsStatus
  header: string
  details?: string
  annotation?: string
  statusIconAriaLabel?: string
}

const props = withDefaults(
  defineProps<{
    steps: StepItem[]
    orientation?: 'horizontal' | 'vertical'
    connectorLines?: 'visible' | 'none'
    ariaLabel?: string
  }>(),
  { orientation: 'vertical', connectorLines: 'visible', ariaLabel: undefined }
)

const statusIconName = (status: StepsStatus) => {
  switch (status) {
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
}

/** 与官方 statusToColor 一致：头部文案跟随状态着色。 */
const statusColor = (status: StepsStatus) => {
  switch (status) {
    case 'error':
      return 'text-status-error' as const
    case 'warning':
      return 'text-status-warning' as const
    case 'success':
      return 'text-status-success' as const
    case 'info':
      return 'text-status-info' as const
    default:
      return 'text-status-inactive' as const
  }
}

const rootClasses = computed(() =>
  cx(STEPS.root, props.orientation === 'horizontal' ? STEPS.horizontal : STEPS.vertical)
)
const listClasses = computed(() =>
  cx(STEPS.list, props.orientation === 'vertical' && props.steps.some((s) => s.annotation) && STEPS['with-annotation'])
)
const connectorClass = computed(() =>
  cx(STEPS.connector, props.connectorLines === 'none' && STEPS['connector-hidden'])
)
</script>

<template>
  <div :class="rootClasses">
    <ol :class="listClasses" :aria-label="ariaLabel">
      <li v-for="(step, index) in steps" :key="index" :class="STEPS.container">
        <div v-if="step.annotation" :class="STEPS.annotation">{{ step.annotation }}</div>

        <div :class="STEPS['step-layout']">
          <template v-if="orientation === 'horizontal'">
            <Box :color="statusColor(step.status)">
              <span
                :aria-label="step.statusIconAriaLabel"
                :role="step.statusIconAriaLabel ? 'img' : undefined"
              >
                <Spinner v-if="step.status === 'loading'" size="normal" />
                <Icon v-else :name="statusIconName(step.status)" />
              </span>
            </Box>
            <hr :class="connectorClass" role="none" />
          </template>

          <template v-else>
            <div :class="STEPS.header">
              <!-- 官方垂直版用 embedded StatusIndicator（图标与文本同行），这里用
                   等价的行内 flex，避免再引入一层 status-indicator 容器样式。 -->
              <div class="cs-step-vertical-header">
                <Box :color="statusColor(step.status)" display="inline-block">
                  <span
                    :aria-label="step.statusIconAriaLabel"
                    :role="step.statusIconAriaLabel ? 'img' : undefined"
                  >
                    <Spinner v-if="step.status === 'loading'" size="normal" />
                    <Icon v-else :name="statusIconName(step.status)" />
                  </span>
                </Box>
                <Box :color="statusColor(step.status)">{{ step.header }}</Box>
              </div>
            </div>
            <div v-if="step.details" :class="STEPS.details">{{ step.details }}</div>
            <hr :class="cx(connectorClass, STEPS['connector-continuation'])" role="none" />
          </template>
        </div>

        <div v-if="orientation === 'horizontal'" :class="STEPS.content">
          <div :class="STEPS.header">
            <Box :color="statusColor(step.status)">{{ step.header }}</Box>
          </div>
          <div v-if="step.details" :class="STEPS.details">{{ step.details }}</div>
        </div>
      </li>
    </ol>
  </div>
</template>

<style scoped>
.cs-step-vertical-header {
  display: flex;
  align-items: center;
  gap: 8px;
  line-height: 20px;
}
</style>
