<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Icon from './Icon.vue'
import { CS } from './classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    header?: string
    defaultExpanded?: boolean
    expanded?: boolean
    variant?: 'default' | 'container' | 'footer' | 'inline' | 'navigation' | 'compact'
    headerDescription?: string
    headerCounter?: string | number
    disableContentPaddings?: boolean
  }>(),
  {
    header: undefined,
    defaultExpanded: false,
    expanded: undefined,
    variant: 'default',
    headerDescription: undefined,
    headerCounter: undefined,
    disableContentPaddings: false,
  }
)

const emit = defineEmits<{
  (e: 'update:expanded', value: boolean): void
  (e: 'change', detail: { expanded: boolean }): void
}>()

const innerExpanded = ref(props.defaultExpanded)
const isControlled = computed(() => props.expanded !== undefined)
const isExpanded = computed(() => (isControlled.value ? Boolean(props.expanded) : innerExpanded.value))

watch(
  () => props.expanded,
  (value) => {
    if (value !== undefined) innerExpanded.value = value
  }
)

function toggle() {
  const next = !isExpanded.value
  if (!isControlled.value) innerExpanded.value = next
  emit('update:expanded', next)
  emit('change', { expanded: next })
}

const wrapperClasses = computed(() =>
  cx(
    CS['expandable-section'].wrapper,
    CS['expandable-section'][`wrapper-${props.variant}`],
    isExpanded.value && CS['expandable-section']['wrapper-expanded'],
    !isExpanded.value && !props.headerDescription && CS['expandable-section']['wrapper-not-expanded-without-actions']
  )
)
</script>

<template>
  <div :class="CS['expandable-section'].root">
    <div :class="wrapperClasses">
      <div :class="CS['expandable-section'].header">
        <div :class="CS['expandable-section']['header-wrapper']">
          <button
            type="button"
            :class="cx(CS['expandable-section']['expand-button'], CS['expandable-section']['click-target'], CS['expandable-section'].focusable)"
            :aria-expanded="isExpanded ? 'true' : 'false'"
            @click="toggle"
          >
            <span :class="CS['expandable-section']['icon-container']">
              <Icon
                name="angle-down"
                :class="cx(CS['expandable-section'].icon, isExpanded && CS['expandable-section'].expanded)"
              />
            </span>
            <span :class="CS['expandable-section']['header-text']">{{ header }}</span>
            <span v-if="headerCounter !== undefined" :class="CS['expandable-section']['header-counter']">{{ headerCounter }}</span>
          </button>
        </div>
      </div>
      <div
        v-show="isExpanded"
        :class="
          cx(
            CS['expandable-section'].content,
            CS['expandable-section'][`content-${variant}`],
            isExpanded && CS['expandable-section']['content-expanded'],
            disableContentPaddings && CS['expandable-section']['disable-content-paddings']
          )
        "
        role="group"
      >
        <div
          :class="
            cx(
              CS['expandable-section']['content-inner'],
              isExpanded && CS['expandable-section']['content-inner-expanded'],
              isExpanded && CS['expandable-section']['content-inner-settled']
            )
          "
        >
          <div
            :class="
              cx(
                CS['expandable-section']['content-inner-body'],
                CS['expandable-section'][`content-inner-body-${variant}`],
                disableContentPaddings && CS['expandable-section']['disable-content-paddings']
              )
            "
          >
            <slot />
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
/* 官方组件把标题渲染为无默认样式的元素；这里用原生 button，需要显式清掉浏览器外观。 */
button {
  display: flex;
  align-items: center;
  inline-size: 100%;
  margin: 0;
  padding: 0;
  border: 0;
  background: transparent;
  color: inherit;
  font: inherit;
  text-align: start;
  cursor: pointer;
}
</style>
