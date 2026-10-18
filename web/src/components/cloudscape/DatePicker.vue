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

function onUpdate(raw: string) {
  if (!raw) {
    emit('update:modelValue', null)
    emit('change', null)
    return
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
  <div class="cs-date-picker-wrap" :class="{ 'is-empty': !nativeValue }">
    <Input
      :type="nativeType"
      :model-value="nativeValue"
      :disabled="disabled"
      :clearable="clearable"
      :step="type === 'date' ? undefined : 1"
      :style="style"
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
   聚焦后恢复原生分段文本，保证仍可正常编辑。 */
.cs-date-picker-wrap.is-empty :deep(input:not(:focus)) {
  color: transparent;
}

/* Chromium 的日期输入分段由伪元素绘制，仅设置 input 的 color 不会隐藏 */
.cs-date-picker-wrap.is-empty :deep(input:not(:focus)::-webkit-datetime-edit) {
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
</style>
