<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import Button from './Button.vue'
import { CS } from './classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    visible: boolean
    header?: string
    /** 抽屉宽度；数字按 px 处理，字符串原样使用。 */
    size?: number | string
    placement?: 'start' | 'end'
    hideCloseAction?: boolean
    disableContentPaddings?: boolean
    closeAriaLabel?: string
  }>(),
  {
    header: undefined,
    size: 460,
    placement: 'end',
    hideCloseAction: false,
    disableContentPaddings: false,
    closeAriaLabel: '关闭抽屉',
  }
)

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'close'): void
}>()

const drawerRef = ref<HTMLElement | null>(null)
const headerId = `drawer-header-${Math.random().toString(36).slice(2)}`
let previousActive: HTMLElement | null = null

const width = computed(() => (typeof props.size === 'number' ? `${props.size}px` : props.size))

const rootClasses = computed(() => CS.drawer.root)
const drawerClasses = computed(() =>
  cx(
    CS.drawer.drawer,
    CS.drawer['position-fixed'],
    CS.drawer[`placement-${props.placement}`],
    props.hideCloseAction ? CS.drawer['hide-close-action'] : CS.drawer['has-close-action'],
    'cs-drawer'
  )
)
const contentClasses = computed(() =>
  cx(CS.drawer.content, !props.disableContentPaddings && CS.drawer['content-with-paddings'])
)

function close() {
  emit('update:visible', false)
  emit('close')
}

function onKeydown(event: KeyboardEvent) {
  if (!props.visible) return
  if (event.key === 'Escape') {
    event.stopPropagation()
    close()
    return
  }
  if (event.key !== 'Tab' || !drawerRef.value) return
  const focusables = drawerRef.value.querySelectorAll<HTMLElement>(
    'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])'
  )
  if (!focusables.length) return
  const first = focusables[0]
  const last = focusables[focusables.length - 1]
  if (event.shiftKey && document.activeElement === first) {
    event.preventDefault()
    last.focus()
  } else if (!event.shiftKey && document.activeElement === last) {
    event.preventDefault()
    first.focus()
  }
}

function lockScroll() {
  previousActive = document.activeElement as HTMLElement | null
  document.body.style.overflow = 'hidden'
  document.addEventListener('keydown', onKeydown, true)
}

function unlockScroll() {
  document.body.style.overflow = ''
  document.removeEventListener('keydown', onKeydown, true)
  previousActive?.focus?.()
  previousActive = null
}

watch(
  () => props.visible,
  async (visible) => {
    if (visible) {
      lockScroll()
      await nextTick()
      const focusables = drawerRef.value?.querySelectorAll<HTMLElement>(
        'a[href], button:not([disabled]), textarea:not([disabled]), input:not([disabled]), select:not([disabled]), [tabindex]:not([tabindex="-1"])'
      )
      focusables?.[0]?.focus()
    } else {
      unlockScroll()
    }
  },
  { immediate: true }
)

onBeforeUnmount(unlockScroll)
</script>

<template>
  <Teleport to="body">
    <div v-if="visible" :class="rootClasses">
      <div :class="cx(CS.drawer.backdrop, CS.drawer['backdrop-fixed'])" @click="close" />
      <div
        ref="drawerRef"
        :class="drawerClasses"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="headerId"
        tabindex="-1"
        :style="{ inlineSize: width, maxInlineSize: '100vw' }"
      >
        <div :class="CS.drawer.header">
          <span :id="headerId">{{ header }}</span>
          <div :class="CS.drawer['header-actions']">
            <slot name="actions" />
            <Button
              v-if="!hideCloseAction"
              variant="icon"
              icon-name="close"
              :aria-label="closeAriaLabel"
              @click="close"
            />
          </div>
        </div>
        <div :class="contentClasses">
          <slot />
        </div>
        <div v-if="$slots.footer" :class="cx(CS.drawer.footer, CS.drawer['is-sticky'])">
          <slot name="footer" />
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped lang="scss">
/* 官方 Drawer 的几何由 React 组件内联给出，CSS 只声明边框方向；
   而 has-close-action 规则会强制 position:relative（带 :not(#\9) 的 id 级特异性），
   所以这里的定位必须 !important 才能压住。 */
.cs-drawer {
  position: fixed !important;
  inset-block: 0 !important;
  display: flex;
  flex-direction: column;
  z-index: 6000;
  overflow: hidden auto;
  box-shadow: var(--shadow-panel-toggle-side, 0 1px 8px 1px rgba(0, 28, 36, 0.15));
}

.cs-drawer[class*='placement-end'] {
  inset-inline: auto 0 !important;
}

.cs-drawer[class*='placement-start'] {
  inset-inline: 0 auto !important;
}
</style>
