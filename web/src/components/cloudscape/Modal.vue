<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import Header from './Header.vue'
import Button from './Button.vue'
import { CS } from './classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    visible: boolean
    header?: string
    size?: 'small' | 'medium' | 'large' | 'x-large' | 'xx-large' | 'max'
    hideDismissButton?: boolean
    disableContentPaddings?: boolean
    closeAriaLabel?: string
  }>(),
  {
    header: undefined,
    size: 'medium',
    hideDismissButton: false,
    disableContentPaddings: false,
    closeAriaLabel: 'Close modal',
  }
)

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'close'): void
}>()

const dialogRef = ref<HTMLElement | null>(null)
const headerId = `modal-header-${Math.random().toString(36).slice(2)}`
let previousActive: HTMLElement | null = null

const rootClasses = computed(() => cx(CS.modal.root, CS.modal.refresh))
const focusLockClasses = computed(() => cx(CS.modal['focus-lock'], CS.modal['position-center']))
const dialogClasses = computed(() => cx(CS.modal.dialog, CS.modal.refresh, CS.modal[props.size]))
const contentClasses = computed(() => cx(CS.modal.content, props.disableContentPaddings && CS.modal['no-paddings']))

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
      @click.self="close"
    >
      <div :class="focusLockClasses">
        <div ref="dialogRef" :class="dialogClasses">
          <div :class="CS.modal.container">
            <div :class="CS.modal.header">
              <Header variant="h2">
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
            <div v-if="$slots.footer" :class="CS.modal.footer">
              <slot name="footer" />
            </div>
          </div>
        </div>
      </div>
    </div>
  </Teleport>
</template>
