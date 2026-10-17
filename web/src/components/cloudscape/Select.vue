<script setup lang="ts">
import { Fragment, computed, nextTick, onBeforeUnmount, ref, useSlots, watch } from 'vue'
import Icon from './Icon.vue'
import Input from './Input.vue'
import Option from './Option.vue'
import { CS } from './classes'
import { cx } from './utils'

export interface SelectOption {
  label?: string
  value: string | number
  disabled?: boolean
}

const props = withDefaults(
  defineProps<{
    modelValue?: string | number | (string | number)[] | null
    options?: SelectOption[]
    placeholder?: string
    disabled?: boolean
    clearable?: boolean
    filterable?: boolean
    allowCreate?: boolean
    multiple?: boolean
    collapseTags?: boolean
    loading?: boolean
    size?: string
  }>(),
  {
    modelValue: undefined,
    options: undefined,
    placeholder: '请选择',
    disabled: false,
    clearable: false,
    filterable: false,
    allowCreate: false,
    multiple: false,
    collapseTags: false,
    loading: false,
    size: undefined,
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string | number | (string | number)[] | ''): void
  (e: 'change', value: string | number | (string | number)[] | ''): void
  (e: 'visible-change', visible: boolean): void
  (e: 'clear'): void
}>()

const slots = useSlots()
const rootRef = ref<HTMLElement | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const filterRef = ref<InstanceType<typeof Input> | null>(null)
const open = ref(false)
const filterText = ref('')
const highlighted = ref(0)
const dropdownStyle = ref<Record<string, string>>({})

function collectSlotOptions(): SelectOption[] {
  const result: SelectOption[] = []
  const walk = (nodes: unknown[]) => {
    for (const node of nodes) {
      if (!node) continue
      if (Array.isArray(node)) {
        walk(node)
        continue
      }
      const vnode = node as { type?: unknown; props?: Record<string, unknown>; children?: unknown }
      if (vnode.type === Option) {
        const value = vnode.props?.value as string | number | undefined
        result.push({
          label: (vnode.props?.label as string | undefined) ?? String(value ?? ''),
          value: value ?? '',
          disabled: Boolean(vnode.props?.disabled),
        })
      } else if (vnode.type === Fragment && Array.isArray(vnode.children)) {
        walk(vnode.children)
      }
    }
  }
  walk(slots.default?.() ?? [])
  return result
}

const allOptions = computed<SelectOption[]>(() =>
  props.options ? props.options : collectSlotOptions()
)

const selectedValues = computed<(string | number)[]>(() => {
  if (props.multiple) return Array.isArray(props.modelValue) ? props.modelValue : []
  const value = props.modelValue
  if (value === undefined || value === null || value === '') return []
  return [value as string | number]
})

function labelOf(value: string | number): string {
  const found = allOptions.value.find((option) => option.value === value)
  if (found) return found.label ?? String(found.value)
  // 与 Element Plus 一致：值不在选项里时显示占位符；allow-create 的临时值才回退为原文
  return props.allowCreate ? String(value) : ''
}

const triggerText = computed(() => {
  if (!selectedValues.value.length) return ''
  if (!props.multiple) return labelOf(selectedValues.value[0])
  const labels = selectedValues.value.map(labelOf).filter(Boolean)
  if (!props.collapseTags || labels.length <= 2) return labels.join('、')
  return `${labels.slice(0, 2).join('、')} +${labels.length - 2}`
})

const filteredOptions = computed(() => {
  const query = filterText.value.trim().toLowerCase()
  if (!query) return allOptions.value
  return allOptions.value.filter((option) =>
    (option.label ?? String(option.value)).toLowerCase().includes(query)
  )
})

const creatableOption = computed<SelectOption | null>(() => {
  if (!props.allowCreate) return null
  const query = filterText.value.trim()
  if (!query) return null
  const exists = allOptions.value.some(
    (option) => (option.label ?? String(option.value)) === query
  )
  return exists ? null : { label: query, value: query }
})

const visibleOptions = computed<SelectOption[]>(() => {
  const list = [...filteredOptions.value]
  if (creatableOption.value) list.push(creatableOption.value)
  return list
})

const showClear = computed(
  () => props.clearable && selectedValues.value.length > 0 && !props.disabled
)

const triggerButtonClasses = computed(() =>
  cx(
    CS['internal/components/button-trigger']['button-trigger'],
    CS['internal/components/button-trigger']['has-caret'],
    open.value && CS['internal/components/button-trigger'].pressed,
    props.disabled && CS['internal/components/button-trigger'].disabled
  )
)

function isSelected(value: string | number) {
  return selectedValues.value.includes(value)
}

function updatePosition() {
  const trigger = rootRef.value?.querySelector('button')
  if (!trigger) return
  const rect = trigger.getBoundingClientRect()
  const panel = dropdownRef.value
  const panelHeight = panel?.offsetHeight ?? 240
  const gap = 4
  let top = rect.bottom + gap
  if (top + panelHeight > window.innerHeight && rect.top - panelHeight - gap > 0) {
    top = rect.top - panelHeight - gap
  }
  dropdownStyle.value = {
    position: 'fixed',
    left: `${rect.left}px`,
    top: `${top}px`,
    minWidth: `${rect.width}px`,
    maxWidth: 'min(520px, calc(100vw - 16px))',
  }
}

function openDropdown() {
  if (props.disabled || open.value) return
  open.value = true
  filterText.value = ''
  highlighted.value = Math.max(
    0,
    visibleOptions.value.findIndex((option) => isSelected(option.value))
  )
  emit('visible-change', true)
  nextTick(() => {
    updatePosition()
    if (props.filterable) filterRef.value?.$el?.querySelector('input')?.focus()
    window.addEventListener('scroll', updatePosition, true)
    window.addEventListener('resize', updatePosition)
  })
}

function closeDropdown() {
  if (!open.value) return
  open.value = false
  emit('visible-change', false)
  window.removeEventListener('scroll', updatePosition, true)
  window.removeEventListener('resize', updatePosition)
}

function toggleDropdown() {
  if (open.value) closeDropdown()
  else openDropdown()
}

function commit(values: (string | number)[], scalar?: string | number) {
  if (props.multiple) {
    emit('update:modelValue', values)
    emit('change', values)
  } else {
    const value = scalar ?? values[0] ?? ''
    emit('update:modelValue', value)
    emit('change', value)
    closeDropdown()
  }
}

function choose(option: SelectOption) {
  if (option.disabled) return
  if (props.multiple) {
    const next = isSelected(option.value)
      ? selectedValues.value.filter((value) => value !== option.value)
      : [...selectedValues.value, option.value]
    filterText.value = ''
    highlighted.value = 0
    commit(next)
    nextTick(() => {
      updatePosition()
      if (props.filterable) filterRef.value?.$el?.querySelector('input')?.focus()
    })
  } else {
    commit([option.value], option.value)
  }
}

function clearSelection() {
  emit('update:modelValue', props.multiple ? [] : '')
  emit('change', props.multiple ? [] : '')
  emit('clear')
}

function moveHighlight(delta: number) {
  const count = visibleOptions.value.length
  if (!count) return
  highlighted.value = (highlighted.value + delta + count) % count
}

function onTriggerKeydown(event: KeyboardEvent) {
  if (props.disabled) return
  switch (event.key) {
    case 'ArrowDown':
    case 'ArrowUp':
      event.preventDefault()
      if (!open.value) openDropdown()
      else moveHighlight(event.key === 'ArrowDown' ? 1 : -1)
      break
    case 'Enter':
    case ' ':
      event.preventDefault()
      if (!open.value) openDropdown()
      else {
        const option = visibleOptions.value[highlighted.value]
        if (option) choose(option)
      }
      break
    case 'Escape':
      closeDropdown()
      break
    case 'Tab':
      closeDropdown()
      break
  }
}

function onFilterKeydown(event: KeyboardEvent) {
  switch (event.key) {
    case 'ArrowDown':
      event.preventDefault()
      moveHighlight(1)
      break
    case 'ArrowUp':
      event.preventDefault()
      moveHighlight(-1)
      break
    case 'Enter':
      event.preventDefault()
      if (visibleOptions.value[highlighted.value]) choose(visibleOptions.value[highlighted.value])
      break
    case 'Escape':
      event.preventDefault()
      closeDropdown()
      break
  }
}

function onDocumentPointerDown(event: PointerEvent) {
  const target = event.target as Node
  if (rootRef.value?.contains(target) || dropdownRef.value?.contains(target)) return
  closeDropdown()
}

watch(open, (value) => {
  if (value) document.addEventListener('pointerdown', onDocumentPointerDown, true)
  else document.removeEventListener('pointerdown', onDocumentPointerDown, true)
})

watch(filterText, () => {
  highlighted.value = 0
})

watch(
  () => props.modelValue,
  () => {
    if (!open.value) return
    nextTick(updatePosition)
  }
)

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onDocumentPointerDown, true)
  window.removeEventListener('scroll', updatePosition, true)
  window.removeEventListener('resize', updatePosition)
})
</script>

<template>
  <div ref="rootRef" :class="CS.select.root" class="cs-select">
    <button
      type="button"
      :class="triggerButtonClasses"
      :disabled="disabled"
      aria-haspopup="listbox"
      :aria-expanded="open"
      @click="toggleDropdown"
      @keydown="onTriggerKeydown"
    >
      <span
        v-if="!selectedValues.length || !triggerText"
        :class="[CS['select/parts'].placeholder, CS['select/parts'].trigger]"
      >
        {{ placeholder }}
      </span>
      <span v-else :class="CS['select/parts'].trigger">{{ triggerText }}</span>
      <span :class="CS['internal/components/button-trigger'].arrow">
        <Icon name="angle-down" size="x-small" />
      </span>
    </button>

    <button
      v-if="showClear"
      type="button"
      class="cs-select-clear"
      aria-label="清除选择"
      @click.stop="clearSelection"
    >
      <Icon name="close" size="x-small" />
    </button>

    <Teleport to="body">
      <div
        v-if="open"
        ref="dropdownRef"
        :class="[
          CS.dropdown.dropdown,
          CS.dropdown.refresh,
          CS.dropdown.open,
          CS.dropdown['use-portal'],
          'cs-select-dropdown',
        ]"
        :style="dropdownStyle"
      >
        <div :class="[CS.dropdown['dropdown-content-wrapper'], CS.dropdown.refresh]">
          <div :class="CS.dropdown['dropdown-content']">
            <div v-if="filterable" class="cs-select-filter">
              <Input
                ref="filterRef"
                :model-value="filterText"
                placeholder="筛选选项"
                @update:model-value="filterText = $event"
                @keydown="onFilterKeydown"
              />
            </div>
            <ul
              :class="CS['internal/components/options-list']['options-list']"
              role="listbox"
              :aria-multiselectable="multiple || undefined"
            >
              <li
                v-for="(option, index) in visibleOptions"
                :key="`${option.value}`"
                role="option"
                :aria-selected="isSelected(option.value)"
                :aria-disabled="option.disabled || undefined"
                :class="[
                  CS['internal/components/selectable-item']['selectable-item'],
                  CS['internal/components/selectable-item']['option-content'],
                  isSelected(option.value) && CS['internal/components/selectable-item'].selected,
                  index === highlighted && CS['internal/components/selectable-item'].highlighted,
                  option.disabled && CS['internal/components/selectable-item'].disabled,
                ]"
                @mouseenter="highlighted = index"
                @click="choose(option)"
              >
                <div :class="CS['internal/components/selectable-item']['selectable-item-content']">
                  <div :class="CS['select/parts'].item">
                    <span
                      v-if="multiple"
                      :class="CS['select/parts'].checkbox"
                      :data-checked="isSelected(option.value) || undefined"
                    >
                      <Icon v-if="isSelected(option.value)" name="check" size="x-small" />
                    </span>
                    <span
                      :class="[
                        CS['internal/components/option'].option,
                        CS['internal/components/option'].content,
                      ]"
                    >
                      <span :class="CS['internal/components/option']['label-content']">
                        <span :class="CS['internal/components/option'].label">
                          {{ option.label ?? option.value }}
                        </span>
                      </span>
                    </span>
                    <span
                      v-if="!multiple && isSelected(option.value)"
                      :class="CS['select/parts']['selected-icon']"
                    >
                      <Icon name="check" />
                    </span>
                  </div>
                </div>
              </li>
              <li v-if="!visibleOptions.length" class="cs-select-empty">
                {{ loading ? '加载中…' : '无匹配选项' }}
              </li>
            </ul>
          </div>
        </div>
      </div>
    </Teleport>
  </div>
</template>

<style>
.cs-select {
  position: relative;
  inline-size: 100%;
}

.cs-select-clear {
  position: absolute;
  inset-block-start: 50%;
  inset-inline-end: 30px;
  transform: translateY(-50%);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  inline-size: 20px;
  block-size: 20px;
  padding: 0;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-interactive-default-pauydh, #545b64);
  cursor: pointer;
}

.cs-select-clear:hover {
  background: var(--color-background-button-normal-hover, rgba(0, 0, 0, 0.06));
}

.cs-select-dropdown {
  z-index: 9000;
}

.cs-select-filter {
  padding-block: 6px;
  padding-inline: 6px;
  border-block-end: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
}

.cs-select-empty {
  padding-block: 12px;
  padding-inline: 12px;
  font-size: 13px;
  color: var(--color-text-body-secondary, #5f6b7a);
}
</style>
