import { onUnmounted, ref, type Ref } from 'vue'

/**
 * 响应式断点（JS 侧）。
 *
 * 数字必须与 src/styles/breakpoints.scss 保持一致 —— 媒体查询字符串无法引用 CSS 变量，
 * 所以这里是第二份字面量，改一处就得改另一处。CSS 侧靠 vite 注入 `$bp-*`，JS 侧用 MQ.*。
 *
 * max 侧一律 .98 结尾、min 侧用整数，避免正好落在断点上时两侧同时命中。
 */
export const BP = {
  small: 480,
  cloudscape: 688.98,
  twoPane: 900,
  tabletMax: 767.98,
  tabletMin: 768,
  contentMax: 1023.98,
  contentMin: 1024,
  wide: 1280,
} as const

export const MQ = {
  small: `(max-width: ${BP.small}px)`,
  tablet: `(max-width: ${BP.tabletMax}px)`,
  content: `(max-width: ${BP.contentMax}px)`,
} as const

/**
 * 订阅媒体查询结果，随窗口变化自动更新，组件卸载时移除监听。
 * 无 window 的环境（SSR / 构建期）恒为 false。
 */
export function useMediaQuery(query: string): Ref<boolean> {
  const matches = ref(false)
  if (typeof window === 'undefined') return matches

  const mql = window.matchMedia(query)
  matches.value = mql.matches

  const onChange = (e: MediaQueryListEvent) => {
    matches.value = e.matches
  }
  mql.addEventListener('change', onChange)
  onUnmounted(() => mql.removeEventListener('change', onChange))

  return matches
}
