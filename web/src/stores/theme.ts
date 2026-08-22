import { ref, computed } from 'vue'
import { defineStore } from 'pinia'

export type ThemeMode = 'light' | 'dark' | 'auto'

export const useThemeStore = defineStore('theme', () => {
  const mode = ref<ThemeMode>((localStorage.getItem('theme_mode') as ThemeMode) || 'auto')
  const systemPrefersDark = ref(
    typeof window !== 'undefined' && window.matchMedia ? window.matchMedia('(prefers-color-scheme: dark)').matches : false
  )

  // 监听系统主题变化
  if (typeof window !== 'undefined' && window.matchMedia) {
    const mediaQuery = window.matchMedia('(prefers-color-scheme: dark)')
    mediaQuery.addEventListener('change', (e) => {
      systemPrefersDark.value = e.matches
      applyTheme()
    })
  }

  const isDark = computed(() => {
    if (mode.value === 'dark') return true
    if (mode.value === 'light') return false
    return systemPrefersDark.value
  })

  const modeLabel = computed(() => {
    if (mode.value === 'light') return '浅色模式'
    if (mode.value === 'dark') return '深色模式'
    return `跟随系统 (${isDark.value ? '深色' : '浅色'})`
  })

  const modeTooltip = computed(() => {
    if (mode.value === 'light') return '当前：浅色模式（点击切换为深色）'
    if (mode.value === 'dark') return '当前：深色模式（点击切换为跟随系统）'
    return `当前：跟随系统 [${isDark.value ? '深色' : '浅色'}]（点击切换为浅色）`
  })

  function applyTheme() {
    if (typeof document === 'undefined') return
    const html = document.documentElement
    if (isDark.value) {
      html.classList.add('dark')
    } else {
      html.classList.remove('dark')
    }
  }

  function setMode(newMode: ThemeMode) {
    mode.value = newMode
    localStorage.setItem('theme_mode', newMode)
    applyTheme()
  }

  // 浅色 -> 深色 -> 跟随系统 三态循环
  function toggle() {
    if (mode.value === 'light') {
      setMode('dark')
    } else if (mode.value === 'dark') {
      setMode('auto')
    } else {
      setMode('light')
    }
  }

  // 初始化应用主题
  applyTheme()

  return {
    mode,
    isDark,
    modeLabel,
    modeTooltip,
    setMode,
    toggle,
  }
})

