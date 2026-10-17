<script setup lang="ts">
import { Fragment, computed, defineComponent, useSlots } from 'vue'
import Radio from './Radio.vue'
import { CS } from './classes'
import { cx } from './utils'

interface RadioItem {
  label: string
  value: string | number | boolean
  disabled?: boolean
  render?: () => unknown
}

// 直接渲染 CsRadio 的默认插槽内容，避免从 VNode 结构里硬猜文本。
const SlotContent = defineComponent({
  props: { render: { type: Function, required: true } },
  setup(props) {
    return () => props.render()
  },
})

const props = withDefaults(
  defineProps<{
    modelValue?: string | number | boolean | null
    items?: RadioItem[]
    name?: string
    direction?: 'vertical' | 'horizontal'
    disabled?: boolean
    readOnly?: boolean
    ariaLabel?: string
  }>(),
  {
    modelValue: undefined,
    items: undefined,
    name: undefined,
    direction: 'vertical',
    disabled: false,
    readOnly: false,
    ariaLabel: undefined,
  }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string | number | boolean): void
  (e: 'change', value: string | number | boolean): void
}>()

const slots = useSlots()
const groupName = props.name ?? `cs-radio-${Math.random().toString(36).slice(2)}`

function textOf(node: unknown): string {
  if (typeof node === 'string' || typeof node === 'number') return String(node)
  if (Array.isArray(node)) return node.map(textOf).join('')
  if (!node || typeof node !== 'object') return ''
  const vnode = node as { type?: unknown; children?: unknown }
  // 单个 VNode（含 Text vnode）：继续读它的 children
  if ('type' in vnode) return textOf(vnode.children)
  if (vnode.children === undefined || vnode.children === null) return ''
  if (typeof vnode.children === 'string') return vnode.children
  if (Array.isArray(vnode.children)) return vnode.children.map(textOf).join('')
  if (typeof vnode.children === 'object') {
    const slot = (vnode.children as { default?: unknown }).default
    if (typeof slot === 'function') return textOf((slot as () => unknown)())
  }
  return ''
}

function collectSlotItems(): RadioItem[] {
  const result: RadioItem[] = []
  const walk = (nodes: unknown[]) => {
    for (const node of nodes) {
      if (!node) continue
      if (Array.isArray(node)) {
        walk(node)
        continue
      }
      const vnode = node as {
        type?: unknown
        props?: Record<string, unknown>
        children?: unknown
      }
      if (vnode.type === Radio) {
        const value = vnode.props?.value as string | number | boolean | undefined
        const children = vnode.children as
          | { default?: () => unknown }
          | (() => unknown)
          | undefined
        const render =
          typeof children === 'function'
            ? (children as () => unknown)
            : typeof children === 'object' && typeof children?.default === 'function'
              ? children.default
              : undefined
        result.push({
          label: (vnode.props?.label as string | undefined) ?? textOf(vnode.children),
          value: value ?? '',
          disabled: Boolean(vnode.props?.disabled),
          render,
        })
      } else if (vnode.type === Fragment && Array.isArray(vnode.children)) {
        walk(vnode.children)
      }
    }
  }
  walk(slots.default?.() ?? [])
  return result
}

const options = computed<RadioItem[]>(() => props.items ?? collectSlotItems())
const isHorizontal = computed(() => props.direction === 'horizontal')

function select(item: RadioItem) {
  if (props.disabled || props.readOnly || item.disabled) return
  emit('update:modelValue', item.value)
  emit('change', item.value)
}

const abstract = computed(() => CS['internal/components/abstract-switch'])
const radioClasses = computed(() => CS['internal/components/radio-button'])
</script>

<template>
  <div
    role="radiogroup"
    :aria-label="ariaLabel"
    :class="cx(CS['radio-group']['radio-group'], isHorizontal && CS['radio-group']['horizontal-group'])"
  >
    <label
      v-for="item in options"
      :key="`${item.value}`"
      :class="cx(CS['radio-group'].radio, isHorizontal && CS['radio-group'].horizontal)"
    >
      <span :class="abstract.wrapper">
        <span :class="abstract['label-wrapper']" :aria-disabled="disabled || item.disabled ? 'true' : undefined">
          <span :class="cx(abstract.control, radioClasses['radio-control'])">
            <input
              type="radio"
              :class="abstract['native-input']"
              :name="groupName"
              :value="item.value"
              :checked="item.value === modelValue"
              :disabled="disabled || item.disabled"
              @change="select(item)"
            />
            <svg viewBox="0 0 100 100" focusable="false" aria-hidden="true">
              <circle
                :class="cx(radioClasses['styled-circle-border'], item.disabled && radioClasses['styled-circle-disabled'])"
                stroke-width="6.25"
                cx="50"
                cy="50"
                r="46"
              />
              <circle
                :class="
                  cx(
                    radioClasses['styled-circle-fill'],
                    item.value === modelValue && radioClasses['styled-circle-checked'],
                    item.disabled && radioClasses['styled-circle-disabled']
                  )
                "
                stroke-width="30"
                cx="50"
                cy="50"
                r="35"
              />
            </svg>
            <span :class="cx(abstract.outline, radioClasses.outline)"></span>
          </span>
          <span :class="abstract.content">
            <span :class="abstract.label">
              <SlotContent v-if="item.render" :render="item.render" />
              <template v-else>{{ item.label }}</template>
            </span>
          </span>
        </span>
      </span>
    </label>
  </div>
</template>
