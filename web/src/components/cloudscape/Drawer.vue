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
    /**
     * 距视口顶部的偏移（px）：抽屉与遮罩都从这条线往下铺，
     * 用于让抽屉停在应用栏（toolbar）下方、不遮挡站点顶栏。
     */
    topOffset?: number
    /**
     * 不渲染整行标题栏（标题 + 关闭按钮）。用于「上沿本身是应用栏、关闭靠汉堡/遮罩」
     * 的移动端导航：此时 `header` 只作为无障碍名称。
     */
    hideHeader?: boolean
  }>(),
  {
    header: undefined,
    size: 460,
    placement: 'end',
    hideCloseAction: false,
    disableContentPaddings: false,
    closeAriaLabel: '关闭抽屉',
    topOffset: 0,
    hideHeader: false,
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
const offsetTop = computed(() => `${props.topOffset}px`)

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
    <div v-if="visible" :class="rootClasses" :style="{ '--cs-drawer-top-offset': offsetTop }">
      <div
        :class="cx(CS.drawer.backdrop, CS.drawer['backdrop-fixed'], 'cs-drawer-backdrop')"
        @click="close"
      />
      <div
        ref="drawerRef"
        :class="drawerClasses"
        role="dialog"
        aria-modal="true"
        :aria-labelledby="hideHeader ? undefined : headerId"
        :aria-label="hideHeader ? header || '抽屉' : undefined"
        tabindex="-1"
        :style="{ inlineSize: width, maxInlineSize: '100vw' }"
      >
        <div v-if="!hideHeader" :class="CS.drawer.header">
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
  /* 顶部偏移来自 topOffset（默认 0）：抽屉从应用栏下沿开始，不遮站点顶栏 */
  inset-block: var(--cs-drawer-top-offset, 0px) 0 !important;
  display: flex;
  flex-direction: column;
  z-index: 6000;
  overflow: hidden auto;
  box-shadow: var(--shadow-panel-toggle-side, 0 1px 8px 1px rgba(0, 28, 36, 0.15));
}

/* 遮罩同样从顶栏下沿开始，否则顶栏虽可见却点不到（被遮罩吃掉点击） */
.cs-drawer-backdrop {
  inset-block-start: var(--cs-drawer-top-offset, 0px) !important;
}

.cs-drawer[class*='placement-end'] {
  inset-inline: auto 0 !important;
}

.cs-drawer[class*='placement-start'] {
  inset-inline: 0 auto !important;
}
</style>
