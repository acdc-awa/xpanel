<script setup lang="ts">
import { ref } from 'vue'
import Button from './Button.vue'
import { CS } from './classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    textToCopy: string
    textToDisplay?: string
    copyButtonText?: string
    copySuccessText?: string
    copyErrorText?: string
    copyButtonAriaLabel?: string
    variant?: 'normal' | 'icon' | 'inline-icon' | 'link'
    inline?: boolean
    wrapText?: boolean
    disabled?: boolean
  }>(),
  {
    textToDisplay: undefined,
    copyButtonText: 'Copy',
    copySuccessText: 'Copied',
    copyErrorText: 'Copy failed',
    copyButtonAriaLabel: undefined,
    variant: 'normal',
    inline: false,
    wrapText: true,
    disabled: false,
  }
)

const status = ref<'idle' | 'success' | 'error'>('idle')
let resetTimer: number | undefined

async function copy() {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(props.textToCopy)
    } else {
      const textarea = document.createElement('textarea')
      textarea.value = props.textToCopy
      textarea.style.position = 'fixed'
      textarea.style.opacity = '0'
      document.body.appendChild(textarea)
      textarea.select()
      document.execCommand('copy')
      document.body.removeChild(textarea)
    }
    status.value = 'success'
  } catch {
    status.value = 'error'
  }
  if (resetTimer) window.clearTimeout(resetTimer)
  resetTimer = window.setTimeout(() => {
    status.value = 'idle'
  }, 2000)
}
</script>

<template>
  <span :class="CS['copy-to-clipboard'].root">
    <span v-if="inline" :class="cx(CS['copy-to-clipboard']['inline-container'], !wrapText && CS['copy-to-clipboard']['inline-container-no-wrap'])">
      <span :class="CS['copy-to-clipboard']['inline-container-trigger']">
        <Button
          :variant="variant"
          icon-name="copy"
          :wrap-text="false"
          :disabled="disabled"
          :aria-label="copyButtonAriaLabel ?? copyButtonText"
          :title="status === 'success' ? copySuccessText : status === 'error' ? copyErrorText : undefined"
          @click="copy"
        >
          {{ copyButtonText }}
        </Button>
      </span>
      <span :class="CS['copy-to-clipboard']['text-to-display']">{{ textToDisplay ?? textToCopy }}</span>
    </span>
    <Button
      v-else
      :variant="variant"
      icon-name="copy"
      :disabled="disabled"
      :aria-label="copyButtonAriaLabel ?? copyButtonText"
      :title="status === 'success' ? copySuccessText : status === 'error' ? copyErrorText : undefined"
      @click="copy"
    >
      {{ copyButtonText }}
    </Button>
  </span>
</template>
