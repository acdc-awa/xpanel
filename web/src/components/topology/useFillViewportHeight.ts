import { onBeforeUnmount, onMounted, type Ref } from 'vue'

interface FillViewportHeightOptions {
  /** 可用高度不足时的下限，此时页面改为纵向滚动 */
  min?: number
  /** 底部留白（AppLayout 的 main 容器默认 `padding-block-end: 40px`） */
  bottom?: number
}

/**
 * 让元素高度吃掉「视口剩余高度」。
 *
 * 不能写死 `calc(100vh - 260px)`：画布上方依次是站点顶栏、应用栏、页头、页签与画布工具条，
 * 其中页签/提示条会随浏览器宽度换行，导航开合又会改变内容宽度，固定偏移在窄屏或收起导航时会算错。
 * 这里直接量元素在文档中的纵向位置，宽度变化导致的回流由 ResizeObserver 兜住。
 */
export function useFillViewportHeight(
  el: Ref<HTMLElement | null>,
  options: FillViewportHeightOptions = {},
) {
  const min = options.min ?? 480
  const bottom = options.bottom ?? 40

  let frame = 0
  let lastHeight = 0
  let observers: ResizeObserver[] = []

  function apply() {
    const node = el.value
    if (!node) return
    // 全屏态是 position:fixed inset:0，位置与高度由 CSS 接管；此时测量会得到错误值
    if (node.classList.contains('is-fullscreen')) return
    const docTop = node.getBoundingClientRect().top + window.scrollY
    const viewportHeight = document.documentElement.clientHeight || window.innerHeight
    const height = Math.max(min, Math.round(viewportHeight - docTop - bottom))
    if (Math.abs(height - lastHeight) < 1) return
    lastHeight = height
    node.style.setProperty('--topo-fill-h', `${height}px`)
  }

  function schedule() {
    cancelAnimationFrame(frame)
    frame = requestAnimationFrame(apply)
  }

  function observe() {
    observers.forEach((ro) => ro.disconnect())
    const targets = new Set<Element>()
    const node = el.value
    if (node) {
      targets.add(node)
      if (node.parentElement) targets.add(node.parentElement)
      const page = node.closest('.cs-page')
      if (page) targets.add(page)
    }
    targets.add(document.documentElement)
    observers = Array.from(targets).map((target) => {
      const ro = new ResizeObserver(schedule)
      ro.observe(target)
      return ro
    })
  }

  onMounted(() => {
    // 同步量一次：要在 Vue Flow 的 nodes-initialized 回调（60ms 后 fitView）之前把高度定下来
    apply()
    observe()
    window.addEventListener('resize', schedule)
  })

  onBeforeUnmount(() => {
    window.removeEventListener('resize', schedule)
    observers.forEach((ro) => ro.disconnect())
    observers = []
    cancelAnimationFrame(frame)
  })
}
