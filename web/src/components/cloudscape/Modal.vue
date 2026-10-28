<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import Header from './Header.vue'
import Button from './Button.vue'
import { CS } from './classes'
import { cx } from './utils'
import { lockBodyScroll, unlockBodyScroll } from './modal-scroll-lock'

const props = withDefaults(
  defineProps<{
    visible: boolean
    header?: string
    /** 标题下方的一行说明（官方 Modal 的 header 接受 ReactNode，这里只留纯文本摘要位）。 */
    description?: string
    size?: 'small' | 'medium' | 'large' | 'x-large' | 'xx-large' | 'max'
    /** 内容超出视口时固定弹窗高度，让内容区自身滚动（折叠表单类长弹窗用）。 */
    scrollable?: boolean
    hideDismissButton?: boolean
    disableContentPaddings?: boolean
    /** 点击遮罩是否关闭。长流程（更新/升级监控）必须为 false，误点会丢进度。 */
    closeOnClickOverlay?: boolean
    /** Esc 是否关闭。与遮罩一致地用于长流程弹窗。 */
    closeOnEscape?: boolean
    closeAriaLabel?: string
    /**
     * 打开时优先聚焦的元素选择器（在弹窗内查找）。
     * 默认聚焦第一个可聚焦元素——对输入类弹窗那会是右上角关闭按钮，
     * 所以要直接落焦到输入框时传它。
     */
    initialFocus?: string
  }>(),
  {
    header: undefined,
    description: undefined,
    size: 'medium',
    scrollable: false,
    hideDismissButton: false,
    disableContentPaddings: false,
    closeOnClickOverlay: true,
    closeOnEscape: true,
    closeAriaLabel: 'Close modal',
    initialFocus: undefined,
  }
)

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'close'): void
}>()

const dialogRef = ref<HTMLElement | null>(null)
const headerId = `modal-header-${Math.random().toString(36).slice(2)}`
/** 本实例是否持有滚动锁；隐藏实例（visible=false）不得释放别人的锁。 */
let scrollLocked = false

const rootClasses = computed(() => cx(CS.modal.root, CS.modal.refresh))
const focusLockClasses = computed(() => cx(CS.modal['focus-lock'], CS.modal['position-center']))
const dialogClasses = computed(() =>
  cx(
    CS.modal.dialog,
    CS.modal.refresh,
    CS.modal[props.size],
    props.scrollable && 'cs-modal-dialog-fit'
  )
)
const containerClasses = computed(() =>
  cx(CS.modal.container, props.scrollable && 'cs-modal-container-fit')
)
const contentClasses = computed(() =>
  cx(
    CS.modal.content,
    props.disableContentPaddings && CS.modal['no-paddings'],
    props.scrollable && 'cs-modal-content-fit'
  )
)
const footerClasses = computed(() => cx(CS.modal.footer, CS.modal['footer--rounded'], 'cs-modal-footer'))

function close() {
  emit('update:visible', false)
  emit('close')
}

function onKeydown(event: KeyboardEvent) {
  if (!props.visible) return
  if (event.key === 'Escape') {
    event.stopPropagation()
    if (props.closeOnEscape) close()
    return
  }
  if (event.key !== 'Tab' || !dialogRef.value) return
  const focusables = dialogRef.value.querySelectorAll<HTMLElement>(
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
  if (scrollLocked) return
  scrollLocked = true
  lockBodyScroll()
  document.addEventListener('keydown', onKeydown, true)
}

function unlockScroll() {
  if (!scrollLocked) return
  scrollLocked = false
  unlockBodyScroll()
  document.removeEventListener('keydown', onKeydown, true)
}

watch(
  () => props.visible,
  async (visible) => {
    if (visible) {
      lockScroll()
      await nextTick()
      const preferred = props.initialFocus ? dialogRef.value?.querySelector<HTMLElement>(props.initialFocus) : null
      if (preferred) {
        preferred.focus()
        return
      }
      const focusables = dialogRef.value?.querySelectorAll<HTMLElement>(
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
    <div
      v-if="visible"
      :class="rootClasses"
      role="dialog"
      aria-modal="true"
      :aria-labelledby="headerId"
      @click.self="closeOnClickOverlay && close()"
    >
      <div :class="focusLockClasses">
        <div ref="dialogRef" :class="dialogClasses">
          <div :class="containerClasses">
            <div :class="CS.modal.header">
              <Header variant="h2" :description="description">
                <span :id="headerId" :class="CS.modal['header--text']">{{ header }}</span>
                <template v-if="!hideDismissButton" #actions>
                  <Button
                    variant="modal-dismiss"
                    icon-name="close"
                    :aria-label="closeAriaLabel"
                    :class="CS.modal['dismiss-control']"
                    @click="close"
                  />
                </template>
              </Header>
            </div>
            <div :class="contentClasses">
              <slot />
            </div>
            <div v-if="$slots.footer" :class="footerClasses">
              <slot name="footer" />
            </div>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
/* 官方 footer 只负责分隔线与 sticky，按钮排布由 React 侧的 SpaceBetween 提供，
   这里补上等价的右对齐排布。 */
.cs-modal-footer {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

/*
 * scrollable = 「内容超高时内部滚动」，不是把弹窗高度钉死在 88vh。
 * 官方 custom-height 用的是 block-size: 88vh，短表单会拖出一大块空白；
 * 这里改成高度自适应 + 上限 88vh，只有超过上限才由内容区滚动。
 * 官方类都带 :not(#\9)（ID 级特异性），所以容器那条必须 !important。
 */
.cs-modal-dialog-fit {
  display: flex;
  flex-direction: column;
  block-size: auto;
  max-block-size: 88vh;
}

.cs-modal-container-fit {
  display: flex !important;
  flex-direction: column;
  flex: 1 1 auto;
  min-block-size: 0;
  block-size: auto;
}

.cs-modal-content-fit {
  flex: 1 1 auto;
  min-block-size: 0;
  overflow-y: auto;
}
</style>
