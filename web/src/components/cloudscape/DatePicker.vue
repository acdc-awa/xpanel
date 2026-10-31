<script setup lang="ts">
import { computed } from 'vue'
import Input from './Input.vue'

// 过渡方案：用 Cloudscape Input 承载原生 date / datetime-local。
// 外观与官方输入框一致；日历面板尚未移植（后续批次可替换为官方 DatePicker 日历）。
const props = withDefaults(
  defineProps<{
    modelValue?: string | Date | null
    type?: 'date' | 'datetime' | 'datetimerange'
    valueFormat?: string
    format?: string
    placeholder?: string
    disabled?: boolean
    clearable?: boolean
    ariaLabel?: string
    style?: string | Record<string, string>
    disabledDate?: (date: Date) => boolean
  }>(),
  {
    modelValue: undefined,
    type: 'date',
    valueFormat: undefined,
    format: undefined,
    placeholder: undefined,
    disabled: false,
    clearable: true,
    ariaLabel: undefined,
    style: undefined,
    disabledDate: undefined,
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string | null): void
  (e: 'change', value: string | null): void
}>()

const pad = (value: number) => String(value).padStart(2, '0')

function toDate(value: string | Date | null | undefined): Date | null {
  if (!value) return null
  const date = value instanceof Date ? value : new Date(value)
  return Number.isNaN(date.getTime()) ? null : date
}

function formatWithTokens(date: Date, format: string): string {
  const offset = -date.getTimezoneOffset()
  const sign = offset >= 0 ? '+' : '-'
  const offsetHours = pad(Math.floor(Math.abs(offset) / 60))
  const offsetMinutes = pad(Math.abs(offset) % 60)
  return format
    .replace(/YYYY/g, String(date.getFullYear()))
    .replace(/MM/g, pad(date.getMonth() + 1))
    .replace(/DD/g, pad(date.getDate()))
    .replace(/HH/g, pad(date.getHours()))
    .replace(/mm/g, pad(date.getMinutes()))
    .replace(/ss/g, pad(date.getSeconds()))
    .replace(/Z/g, `${sign}${offsetHours}:${offsetMinutes}`)
}

const nativeType = computed(() => (props.type === 'date' ? 'date' : 'datetime-local'))

const nativeValue = computed(() => {
  const date = toDate(props.modelValue)
  if (!date) return ''
  if (props.type === 'date') {
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
  }
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(
    date.getHours()
  )}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
})

const effectivePlaceholder = computed(() => {
  if (props.placeholder) return props.placeholder
  return props.type === 'date' ? '选择日期' : '选择日期时间'
})

/**
 * 原生 input 无法按日期逐格禁用，只能把 disabledDate 收窄成 min/max。
 * 做法是从今天出发，往两个方向找「第一个被禁的日期」，再取它内侧的那一天
 * 作为边界：往前找到 d 被禁 ⇒ 下界 = d+1；往后找到 d 被禁 ⇒ 上界 = d-1。
 * 这覆盖两类实际用法（只禁未来 / 只禁过去）；整段方向都未被禁则不加该侧边界。
 * 若今天本身就被禁，说明谓词不是区间型，退化为不加边界，交给业务校验兜底。
 */
const dateRangeBounds = computed(() => {
  if (!props.disabledDate || props.type !== 'date') return { min: undefined, max: undefined }
  const probe = props.disabledDate
  const today = new Date()
  today.setHours(0, 0, 0, 0)
  if (probe(today)) return { min: undefined, max: undefined }

  const toInput = (d: Date) =>
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`

  // 最多扫两年，避免病态谓词把页面拖死
  const LIMIT = 730
  let min: string | undefined
  for (let i = 1; i <= LIMIT; i++) {
    const d = new Date(today)
    d.setDate(today.getDate() - i)
    if (probe(d)) {
      const bound = new Date(d)
      bound.setDate(d.getDate() + 1)
      min = toInput(bound)
      break
    }
  }
  let max: string | undefined
  for (let i = 1; i <= LIMIT; i++) {
    const d = new Date(today)
    d.setDate(today.getDate() + i)
    if (probe(d)) {
      const bound = new Date(d)
      bound.setDate(d.getDate() - 1)
      max = toInput(bound)
      break
    }
  }
  return { min, max }
})

function onUpdate(raw: string) {
  if (!raw) {
    emit('update:modelValue', null)
    emit('change', null)
    return
  }
  // 键盘输入可绕过原生 min/max，这里再挡一次，避免把越界日期送进业务逻辑。
  if (props.disabledDate) {
    const picked = new Date(raw)
    if (!Number.isNaN(picked.getTime()) && props.disabledDate(picked)) return
  }
  const date = new Date(raw)
  if (Number.isNaN(date.getTime())) return
  let output: string
  if (props.type === 'date') {
    output = props.valueFormat
      ? formatWithTokens(date, props.valueFormat)
      : `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
  } else {
    output = formatWithTokens(date, props.valueFormat ?? 'YYYY-MM-DDTHH:mm:ssZ')
  }
  emit('update:modelValue', output)
  emit('change', output)
}
</script>

<template>
  <div class="cs-date-picker-wrap" :class="{ 'is-empty': !nativeValue }" :style="style">
    <Input
      :type="nativeType"
      :model-value="nativeValue"
      :disabled="disabled"
      :clearable="clearable"
      :step="type === 'date' ? undefined : 1"
      :min="dateRangeBounds.min"
      :max="dateRangeBounds.max"
      :aria-label="ariaLabel"
      @update:model-value="onUpdate"
    />
    <span v-if="!nativeValue" class="cs-date-picker-placeholder">{{ effectivePlaceholder }}</span>
  </div>
</template>

<style scoped>
.cs-date-picker-wrap {
  position: relative;
  inline-size: 100%;
}

/* 原生 date/datetime-local 会忽略 placeholder，这里用覆盖层统一成 Cloudscape 输入框观感；
   聚焦后恢复原生分段文本，保证仍可正常编辑。
   两个坑：
   1) vendor 的 input 颜色规则带 :not(#\9) 的 id 级特异性，scoped 的 color 压不过它，
      空值时原生 yyyy/mm/dd 会一直露出来和覆盖层重叠；-webkit-text-fill-color 优先级更高
      且 vendor 未使用，用它把分段文本藏掉（日历图标是图片，不受影响）。
   2) Chromium 的日期输入把焦点交给内部 shadow 编辑器，宿主 input 自身不匹配 :focus，
      只有祖先的 :focus-within 能感知，所以这里一律用 :focus-within 判断。 */
.cs-date-picker-wrap.is-empty:not(:focus-within) :deep(input) {
  -webkit-text-fill-color: transparent;
  color: transparent;
}

.cs-date-picker-wrap.is-empty:not(:focus-within) :deep(input::-webkit-datetime-edit) {
  -webkit-text-fill-color: transparent;
  color: transparent;
}

.cs-date-picker-placeholder {
  position: absolute;
  inset-inline-start: 12px;
  inset-block-start: 50%;
  transform: translateY(-50%);
  font-size: 14px;
  color: var(--color-text-form-default-8bb22s, #687078);
  pointer-events: none;
}

/* 聚焦时原生分段文本（yy/mm/dd）会重新显形，覆盖层必须同时让位，否则两段字叠在一起 */
.cs-date-picker-wrap:focus-within .cs-date-picker-placeholder {
  display: none;
}
</style>
