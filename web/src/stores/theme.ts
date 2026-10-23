import { ref, computed } from 'vue'
import { defineStore } from 'pinia'

/** 只有亮/暗两态；「跟随系统」已移除（三态没有对应的图标，也容易让人误以为没切换成功）。 */
export type ThemeMode = 'light' | 'dark'

/**
 * 首次进入（或历史值还是 `auto`）时按系统偏好定一次，之后就是显式两态，
 * 系统主题变化不再影响已保存的选择。
 */
function resolveInitialMode(): ThemeMode {
  const stored = localStorage.getItem('theme_mode')
  if (stored === 'light' || stored === 'dark') return stored
  const prefersDark =
    typeof window !== 'undefined' && window.matchMedia
      ? window.matchMedia('(prefers-color-scheme: dark)').matches
      : false
  return prefersDark ? 'dark' : 'light'
}

export const useThemeStore = defineStore('theme', () => {
  const mode = ref<ThemeMode>(resolveInitialMode())

  const isDark = computed(() => mode.value === 'dark')

  const modeLabel = computed(() => (isDark.value ? '深色模式' : '浅色模式'))

  const modeTooltip = computed(() =>
    isDark.value ? '当前：深色模式（点击切换为浅色）' : '当前：浅色模式（点击切换为深色）'
  )

  function applyTheme() {
    if (typeof document === 'undefined') return
    const html = document.documentElement
    // Cloudscape 官方暗色 token 定义在 .awsui-dark-mode 上。挂在 body 而不是页面容器上，
    // 才能覆盖 body 自身的亮色 token，并让 Teleport 到 body 的 Modal/下拉层继承暗色。
    const body = document.body
    if (isDark.value) {
      html.classList.add('dark')
      body?.classList.add('awsui-dark-mode')
    } else {
      html.classList.remove('dark')
      body?.classList.remove('awsui-dark-mode')
    }
  }

  function setMode(newMode: ThemeMode) {
    mode.value = newMode
    localStorage.setItem('theme_mode', newMode)
    applyTheme()
  }

  // 浅色 <-> 深色
  function toggle() {
    setMode(isDark.value ? 'light' : 'dark')
  }

  // 初始化应用主题（同时把历史的 'auto' 落地成具体值）
  applyTheme()
  localStorage.setItem('theme_mode', mode.value)

  return {
    mode,
    isDark,
    modeLabel,
    modeTooltip,
    setMode,
    toggle,
  }
})
