<script setup lang="ts">
import { computed } from 'vue'
import Icon from './Icon.vue'
import Select from './Select.vue'
import { CS } from './classes'
import { cx } from './utils'

export interface SegmentedOption {
  id: string | number
  text: string
  iconName?: string
  disabled?: boolean
}

const props = withDefaults(
  defineProps<{
    selectedId?: string | number
    modelValue?: string | number
    options: SegmentedOption[]
    label?: string
    fullWidth?: boolean
  }>(),
  { selectedId: undefined, modelValue: undefined, label: undefined, fullWidth: false }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string | number): void
  (e: 'change', detail: { selectedId: string | number }): void
}>()

const activeId = computed(() => props.selectedId ?? props.modelValue)
const isActive = (option: SegmentedOption) => activeId.value !== undefined && String(option.id) === String(activeId.value)

const selectOptions = computed(() =>
  props.options.map((option) => ({ label: option.text, value: option.id, disabled: option.disabled }))
)

function select(option: SegmentedOption) {
  if (option.disabled || isActive(option)) return
  emit('update:modelValue', option.id)
  emit('change', { selectedId: option.id })
}

/** 下拉降级路径：窄屏时官方用 select 承载同一组选项，事件口径保持一致。 */
function onSelectChange(value: string | number | (string | number)[] | '') {
  if (value === '') return
  emit('update:modelValue', value as string | number)
  emit('change', { selectedId: value as string | number })
}
</script>

<template>
  <div :class="CS['segmented-control'].root">
    <div
      :class="
        cx(
          CS['segmented-control']['segment-part'],
          CS['segmented-control'].refresh,
          CS['segmented-control'][`segment-count-${Math.min(options.length, 6)}`],
          fullWidth && CS['segmented-control']['full-width']
        )
      "
      role="group"
      :aria-label="label"
    >
      <button
        v-for="option in options"
        :key="option.id"
        type="button"
        :class="
          cx(
            CS['segmented-control'].segment,
            CS['segmented-control'].refresh,
            isActive(option) && CS['segmented-control'].selected,
            option.disabled && CS['segmented-control'].disabled,
            fullWidth && CS['segmented-control']['full-width']
          )
        "
        :disabled="option.disabled"
        :aria-pressed="isActive(option) ? 'true' : 'false'"
        @click="select(option)"
      >
        <Icon
          v-if="option.iconName"
          :name="option.iconName"
          :variant="option.disabled ? 'disabled' : 'normal'"
          :class="cx(CS['segmented-control'].icon, option.text ? CS['segmented-control']['with-text'] : CS['segmented-control']['with-no-text'])"
        />
        <span>{{ option.text }}</span>
      </button>
    </div>
    <div :class="CS['segmented-control'].select" class="cs-segmented-select">
      <Select
        :model-value="activeId"
        :options="selectOptions"
        :aria-label="label"
        @change="onSelectChange"
      />
    </div>
  </div>
</template>

<style scoped lang="scss">
/* 官方实现同时渲染分段条与下拉框，由 CSS 在 ≤688px 切换：
   分段条 display:none、下拉框 display:block。分段条的隐藏已由官方
   styles.scoped.css 内的同名媒体查询完成，这里只需管下拉框的显隐。 */
.cs-segmented-select {
  display: none;
}

@media (max-width: $bp-cloudscape-narrow) {
  .cs-segmented-select {
    display: block;
  }
}
</style>
