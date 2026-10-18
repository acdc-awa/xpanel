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
    /** 隐藏右侧百分比文本，用于表格内的紧凑条形。 */
    hideText?: boolean
    /** 条形高度（px），覆盖官方默认的 4px。 */
    barHeight?: number
    /** 条形颜色，覆盖官方默认主色。 */
    color?: string
    /** 条形轨道颜色，覆盖官方默认底色。 */
    trackColor?: string
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
    hideText: false,
    barHeight: undefined,
    color: undefined,
    trackColor: undefined,
  }
)

const clamped = computed(() => Math.max(0, Math.min(100, Math.round(props.value))))
const isComplete = computed(() => clamped.value >= 100)

// 官方样式通过这两个自定义属性读取尺寸与配色，内联覆盖即可生效。
const barVars = computed<Record<string, string>>(() => {
  const vars: Record<string, string> = {}
  if (props.barHeight) vars['--awsui-progress-bar-height-ka4djm'] = `${props.barHeight}px`
  if (props.color) vars['--awsui-progress-value-background-color-ka4djm'] = props.color
  if (props.trackColor) vars['--awsui-progress-bar-background-color-ka4djm'] = props.trackColor
  return vars
})
</script>

<template>
  <div :class="cx(CS['progress-bar'].root, CS['progress-bar'][variant])" :style="barVars">
    <div>
      <div v-if="label" :class="cx(CS['progress-bar']['word-wrap'], CS['progress-bar'][`label-${variant}`])">{{ label }}</div>
      <Box v-if="description" variant="small" display="block">{{ description }}</Box>
      <div>
        <template v-if="status === 'in-progress'">
          <div :class="[CS['progress-bar']['progress-container'], hideText && 'cs-progress-compact']">
            <progress
              :class="cx(CS['progress-bar'].progress, isComplete && CS['progress-bar'].complete, variant === 'flash' && CS['progress-bar']['progress-flash'])"
              max="100"
              :value="clamped"
              :aria-label="ariaLabel"
            />
            <span v-if="!hideText" aria-hidden="true" :class="CS['progress-bar']['percentage-container']">
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

<style scoped lang="scss">
/* 无文本时去掉官方给百分比预留的右侧外边距，条形占满可用宽度。
   官方类名带 :not(#\9) 高特异性，这里需要 !important 才能覆盖。 */
.cs-progress-compact {
  block-size: auto !important;
}

.cs-progress-compact [class*='awsui_progress_'] {
  margin-inline-end: 0 !important;
}
</style>
