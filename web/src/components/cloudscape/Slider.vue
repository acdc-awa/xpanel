<script setup lang="ts">
/**
 * Vue port of the official Cloudscape Slider.
 * 官方实现就是一个 input[type=range] + 轨道/进度层，这里按同一 markup 复刻，
 * 沿用 vendored 的 slider/styles.scoped.css，不另写视觉。
 */
import { computed, ref } from 'vue'
import { SLIDER } from './slider-classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    modelValue?: number
    min?: number
    max?: number
    step?: number
    disabled?: boolean
    readOnly?: boolean
    ariaLabel?: string
    valueFormatter?: (value: number) => string
    hideFillLine?: boolean
  }>(),
  {
    modelValue: undefined,
    min: 0,
    max: 100,
    step: undefined,
    disabled: false,
    readOnly: false,
    ariaLabel: undefined,
    valueFormatter: undefined,
    hideFillLine: false,
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: number): void
  (e: 'change', detail: { value: number }): void
}>()

const active = ref(false)
const rootRef = ref<HTMLElement | null>(null)

const current = computed(() => {
  const v = props.modelValue ?? props.min
  return Math.max(props.min, Math.min(props.max, v))
})

const percent = computed(() => {
  const span = props.max - props.min
  if (span <= 0) return 0
  return ((current.value - props.min) / span) * 100
})

function onInput(event: Event) {
  if (props.readOnly || props.disabled) return
  const value = Number((event.target as HTMLInputElement).value)
  emit('update:modelValue', value)
  emit('change', { value })
}

/**
 * 官方滑杆把可见轨道做成装饰层，可交互的 input[type=range] 高度是 0，
 * 因此点击/拖动轨道本身没有反应（只有拖滑块有效）。这里把轨道层的指针事件
 * 换算成数值，补齐「点哪跳哪」的原生手感。
 */
function onTrackPointer(event: PointerEvent) {
  if (props.disabled || props.readOnly) return
  const root = rootRef.value
  if (!root) return
  const track = root.querySelector<HTMLElement>('[class*="awsui_slider-track"]')
  const rect = (track ?? root).getBoundingClientRect()
  if (!rect.width) return
  const ratio = Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width))
  const raw = props.min + ratio * (props.max - props.min)
  const step = props.step || (props.max - props.min) / 100
  const snapped = props.min + Math.round((raw - props.min) / step) * step
  const value = Number(Math.max(props.min, Math.min(props.max, snapped)).toFixed(6))
  emit('update:modelValue', value)
  emit('change', { value })
}
</script>

<template>
  <div ref="rootRef" :class="SLIDER.root">
    <div
      :class="SLIDER['tooltip-thumb']"
      :style="{ '--awsui-slider-tooltip-position-ka4djm': `calc(${percent}% - 10px)` }"
    />
    <div :class="SLIDER.slider">
      <div
        :class="cx(SLIDER['slider-track'], disabled && SLIDER.disabled)"
        @pointerdown="onTrackPointer"
      />
      <div
        v-if="!hideFillLine"
        :class="cx(SLIDER['slider-range'], active && SLIDER.active, disabled && SLIDER.disabled)"
        :style="{ '--awsui-slider-range-inline-size-ka4djm': `${percent}%` }"
      />
    </div>
    <input
      type="range"
      tabindex="0"
      :class="cx(SLIDER.thumb, current <= min && SLIDER.min, current >= max && min < max && SLIDER.max)"
      :aria-label="ariaLabel"
      :aria-valuetext="valueFormatter ? valueFormatter(current) : undefined"
      :min="min"
      :max="max"
      :step="step"
      :value="current"
      :disabled="disabled"
      @input="onInput"
      @pointerdown="active = true"
      @pointerup="active = false"
      @focus="active = true"
      @blur="active = false"
    />
  </div>
</template>
