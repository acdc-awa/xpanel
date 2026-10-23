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
    /** 展开图标在标题前还是标题后（官方 __expandIconPosition） */
    expandIconPosition?: 'start' | 'end'
    /** 只保留标题、不渲染展开图标（官方 __hideExpandIcon） */
    hideExpandIcon?: boolean
  }>(),
  {
    header: undefined,
    defaultExpanded: false,
    expanded: undefined,
    variant: 'default',
    headerDescription: undefined,
    headerCounter: undefined,
    disableContentPaddings: false,
    expandIconPosition: 'start',
    hideExpandIcon: false,
  }
)

const emit = defineEmits<{
  (e: 'update:expanded', value: boolean): void
  (e: 'change', detail: { expanded: boolean }): void
}>()

const innerExpanded = ref(props.defaultExpanded)
const isControlled = computed(() => props.expanded !== undefined)
const isExpanded = computed(() => (isControlled.value ? Boolean(props.expanded) : innerExpanded.value))
const iconAtEnd = computed(() => props.expandIconPosition === 'end')
const cs = CS['expandable-section']

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
    <!-- 官方把 wrapper-* 类挂在 header 上，content 是其兄弟节点。
         若把两者包进同一个 wrapper，side-navigation 的「表头 hover 底色」会染整块内容。 -->
    <div :class="cx(cs.header, wrapperClasses)">
      <div :class="cs['header-content']">
        <div :class="cx(cs['header-wrapper'], iconAtEnd && cs['header-icon-end'])">
          <button
            type="button"
            :class="
              cx(
                cs['expand-button'],
                cs['header-button'],
                cs['click-target'],
                cs.focusable,
                iconAtEnd && cs['header-button-icon-end']
              )
            "
            :aria-expanded="isExpanded ? 'true' : 'false'"
            @click="toggle"
          >
            <span v-if="!hideExpandIcon && !iconAtEnd" :class="cs['icon-container']">
              <Icon name="angle-down" :class="cx(cs.icon, isExpanded && cs.expanded)" />
            </span>
            <span :class="cs['header-text']">
              <slot name="header">{{ header }}</slot>
            </span>
            <span v-if="headerCounter !== undefined" :class="cs['header-counter']">{{ headerCounter }}</span>
            <span v-if="!hideExpandIcon && iconAtEnd" :class="cx(cs['icon-container'], cs['icon-container-end'])">
              <Icon name="angle-down" :class="cx(cs.icon, isExpanded && cs.expanded)" />
            </span>
          </button>
        </div>
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
