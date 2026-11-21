<script setup lang="ts">
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { pushOverlay } from './overlay-stack'

type Placement = 'top' | 'top-start' | 'top-end' | 'bottom' | 'left' | 'right'

const props = withDefaults(
  defineProps<{
    content: string
    placement?: Placement
    showDelay?: number
    hideDelay?: number
    disabled?: boolean
  }>(),
  {
    placement: 'top',
    showDelay: 150,
    hideDelay: 0,
    disabled: false,
  }
)

defineOptions({ inheritAttrs: false })

const triggerRef = ref<HTMLElement | null>(null)
const tooltipRef = ref<HTMLElement | null>(null)
const open = ref(false)
const tooltipStyle = ref<Record<string, string>>({})
let showTimer: ReturnType<typeof setTimeout> | undefined
let hideTimer: ReturnType<typeof setTimeout> | undefined

function updatePosition() {
  const trigger = triggerRef.value
  const tooltip = tooltipRef.value
  if (!trigger || !tooltip) return
  const rect = trigger.getBoundingClientRect()
  const width = tooltip.offsetWidth
  const height = tooltip.offsetHeight
  const gap = 6
  let left = rect.left + rect.width / 2 - width / 2
  let top = rect.top - height - gap

  if (props.placement === 'top-start') left = rect.left
  else if (props.placement === 'top-end') left = rect.right - width
  else if (props.placement === 'bottom') top = rect.bottom + gap
  else if (props.placement === 'left') {
    left = rect.left - width - gap
    top = rect.top + rect.height / 2 - height / 2
  } else if (props.placement === 'right') {
    left = rect.right + gap
    top = rect.top + rect.height / 2 - height / 2
  }

  const margin = 8
  left = Math.min(Math.max(margin, left), Math.max(margin, window.innerWidth - width - margin))
  top = Math.min(Math.max(margin, top), Math.max(margin, window.innerHeight - height - margin))
  tooltipStyle.value = {
    position: 'fixed',
    left: `${left}px`,
    top: `${top}px`,
    zIndex: '9000',
  }
}

function bindPositionListeners() {
  window.addEventListener('scroll', updatePosition, true)
  window.addEventListener('resize', updatePosition)
}

function unbindPositionListeners() {
  window.removeEventListener('scroll', updatePosition, true)
  window.removeEventListener('resize', updatePosition)
}

function show() {
  if (props.disabled || !props.content) return
  clearTimeout(hideTimer)
  clearTimeout(showTimer)
  showTimer = setTimeout(async () => {
    open.value = true
    await nextTick()
    updatePosition()
    bindPositionListeners()
  }, props.showDelay)
}

function hide() {
  clearTimeout(showTimer)
  clearTimeout(hideTimer)
  hideTimer = setTimeout(() => {
    open.value = false
  }, props.hideDelay)
}

let popOverlay: (() => void) | null = null

watch(open, (value) => {
  if (value) {
    if (!popOverlay) popOverlay = pushOverlay(hide)
  } else {
    popOverlay?.()
    popOverlay = null
    unbindPositionListeners()
  }
})

onBeforeUnmount(() => {
  clearTimeout(showTimer)
  clearTimeout(hideTimer)
  popOverlay?.()
  popOverlay = null
  unbindPositionListeners()
})
</script>

<template>
  <span
    ref="triggerRef"
    class="cs-tooltip-trigger"
    v-bind="$attrs"
    @mouseenter="show"
    @mouseleave="hide"
    @focusin="show"
    @focusout="hide"
  >
    <slot />
    <!-- Teleport 必须留在 trigger 内部：模板一旦出现第二个根节点，Vue 就不再把这个组件
         当单根处理，父组件传进来的作用域 id 不会落到 trigger 上，父级所有
         `.xxx[data-v-…]` 规则（间距、字号、justify-content…）都会静默失效。
         Teleport 会把节点搬到 body，嵌套在 span 里不影响布局。 -->
    <Teleport to="body">
      <div
        v-if="open"
        ref="tooltipRef"
        class="cs-tooltip"
        :class="`cs-tooltip--${placement}`"
        :style="tooltipStyle"
        role="tooltip"
      >
        {{ content }}
      </div>
    </Teleport>
  </span>
</template>

<style scoped lang="scss">
.cs-tooltip-trigger {
  display: inline-flex;
  align-items: center;
}

.cs-tooltip {
  max-width: min(360px, calc(100vw - 16px));
  padding: 6px 10px;
  border-radius: 8px;
  background: #0f141a;
  color: #ffffff;
  font-size: 12px;
  line-height: 1.5;
  white-space: normal;
  overflow-wrap: anywhere;
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.24);
  pointer-events: none;
}

:global(html.dark) .cs-tooltip,
:global(.awsui-dark-mode) .cs-tooltip {
  border: 1px solid var(--color-border-divider-default-diwd4w, #414d5c);
}
</style>
