<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'

type Placement = 'top' | 'top-start' | 'top-end' | 'bottom' | 'bottom-start' | 'bottom-end' | 'left' | 'right'

const props = withDefaults(
  defineProps<{
    visible: boolean
    placement?: Placement
    width?: number | string
    trigger?: 'click' | 'manual'
    offset?: number
  }>(),
  {
    placement: 'bottom-start',
    width: undefined,
    trigger: 'click',
    offset: 4,
  }
)

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
}>()

const triggerRef = ref<HTMLElement | null>(null)
const panelRef = ref<HTMLElement | null>(null)
const panelStyle = ref<Record<string, string>>({})

const panelWidth = computed(() =>
  props.width === undefined ? undefined : typeof props.width === 'number' ? `${props.width}px` : props.width
)

function updatePosition() {
  const trigger = triggerRef.value
  const panel = panelRef.value
  if (!trigger || !panel) return

  const rect = trigger.getBoundingClientRect()
  const width = panel.offsetWidth
  const height = panel.offsetHeight
  const gap = props.offset
  const isTop = props.placement.startsWith('top')
  const isCenter = props.placement === 'left' || props.placement === 'right'
  let left = rect.left
  let top = isTop ? rect.top - height - gap : rect.bottom + gap

  if (props.placement === 'bottom-end' || props.placement === 'top-end') left = rect.right - width
  else if (isCenter) {
    left = props.placement === 'left' ? rect.left - width - gap : rect.right + gap
    top = rect.top + rect.height / 2 - height / 2
  }

  const margin = 8
  left = Math.min(Math.max(margin, left), Math.max(margin, window.innerWidth - width - margin))
  top = Math.min(Math.max(margin, top), Math.max(margin, window.innerHeight - height - margin))

  panelStyle.value = {
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

function setVisible(value: boolean) {
  emit('update:visible', value)
}

function toggle() {
  if (props.trigger !== 'click') return
  setVisible(!props.visible)
}

function onPointerDown(event: PointerEvent) {
  const target = event.target as Node
  if (triggerRef.value?.contains(target) || panelRef.value?.contains(target)) return
  setVisible(false)
}

function onKeydown(event: KeyboardEvent) {
  if (event.key === 'Escape' && props.visible) setVisible(false)
}

watch(
  () => props.visible,
  async (value) => {
    if (value) {
      document.addEventListener('pointerdown', onPointerDown, true)
      document.addEventListener('keydown', onKeydown, true)
      await nextTick()
      updatePosition()
      bindPositionListeners()
    } else {
      document.removeEventListener('pointerdown', onPointerDown, true)
      document.removeEventListener('keydown', onKeydown, true)
      unbindPositionListeners()
    }
  }
)

onBeforeUnmount(() => {
  document.removeEventListener('pointerdown', onPointerDown, true)
  document.removeEventListener('keydown', onKeydown, true)
  unbindPositionListeners()
})
</script>

<template>
  <span ref="triggerRef" class="cs-popover-trigger" @click="toggle">
    <slot name="reference" />
  </span>
  <Teleport to="body">
    <div
      v-if="visible"
      ref="panelRef"
      class="cs-popover"
      :style="{ ...panelStyle, inlineSize: panelWidth }"
      role="dialog"
    >
      <slot />
    </div>
  </Teleport>
</template>

<style scoped lang="scss">
.cs-popover-trigger {
  display: inline-flex;
}

.cs-popover {
  box-sizing: border-box;
  padding: 12px;
  border: 1px solid var(--color-border-dropdown-container-zmsfa8, #c6c6cd);
  border-radius: 8px;
  background: var(--color-background-container-content-mjfil9, #ffffff);
  color: var(--color-text-body-default-b2us04, #0f141a);
  box-shadow: 0 4px 16px rgba(0, 0, 0, 0.16);
  font-size: 14px;
  line-height: 20px;
}
</style>
