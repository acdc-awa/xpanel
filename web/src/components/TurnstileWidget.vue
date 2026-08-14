<script setup lang="ts">
// TurnstileWidget Cloudflare Turnstile 人机验证组件（方向②）。
// 由父组件传入 siteKey；验证通过后 emit token，失效/过期时 emit 空串。
import { onBeforeUnmount, onMounted, ref } from 'vue'

const props = defineProps<{ siteKey: string }>()
const emit = defineEmits<{ (e: 'token', token: string): void }>()

const container = ref<HTMLDivElement>()
let widgetId: string | null = null

declare global {
  interface Window {
    turnstile?: {
      render: (el: HTMLElement, opts: Record<string, any>) => string
      getResponse: (widgetId: string) => string
      reset: (widgetId: string) => void
      remove: (widgetId: string) => void
    }
  }
}

function loadScript(): Promise<void> {
  return new Promise((resolve, reject) => {
    if (document.querySelector('script[data-turnstile]')) {
      resolve()
      return
    }
    const s = document.createElement('script')
    s.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js'
    s.async = true
    s.defer = true
    s.dataset.turnstile = '1'
    s.onload = () => resolve()
    s.onerror = () => reject(new Error('Turnstile 脚本加载失败'))
    document.head.appendChild(s)
  })
}

onMounted(async () => {
  if (!props.siteKey || !container.value) return
  try {
    await loadScript()
    if (!window.turnstile) return
    widgetId = window.turnstile.render(container.value, {
      sitekey: props.siteKey,
      callback: (token: string) => emit('token', token),
      'expired-callback': () => emit('token', ''),
      'error-callback': () => emit('token', ''),
    })
  } catch {
    // 脚本加载失败：不阻断表单，由后端在校验开启时兜底拒绝
    emit('token', '')
  }
})

onBeforeUnmount(() => {
  if (widgetId && window.turnstile) {
    try {
      window.turnstile.remove(widgetId)
    } catch {
      /* noop */
    }
  }
})
</script>

<template>
  <div ref="container" class="turnstile-wrap" />
</template>

<style scoped lang="scss">
.turnstile-wrap {
  display: flex;
  justify-content: center;
  margin-bottom: 4px;
  :deep(iframe) {
    border-radius: 8px;
  }
}
</style>
