import { onMounted, onUnmounted, watch, type Ref } from 'vue'

type ResizeHandler = () => void

/**
 * 监听一组容器元素的尺寸变化并触发回调（典型用途：ECharts 的 resize）。
 *
 * 只挂 window.resize 是不够的：侧栏折叠只是改了 .admin-main 的 margin-left，
 * 不会触发 window resize，图表的 canvas 像素宽会停在旧值上，图表按旧坐标绘制、
 * 右侧留出一块空白，直到用户手动改一次窗口大小才恢复。
 *
 * 弹窗里的图表还会在开合之间销毁重建（destroy-on-close），所以除了挂载时同步一次，
 * 还要监听 ref 自身的变化来重新挂载观察目标。
 *
 * 回调统一走 requestAnimationFrame：侧栏折叠的 0.24s 过渡期间容器宽度每帧都在变，
 * 不节流会把 resize/setOption 打满整个动画过程。
 */
export function useContainerResize(
  containers: Array<Ref<HTMLElement | null>>,
  handler: ResizeHandler
) {
  if (typeof ResizeObserver === 'undefined') return

  let frame = 0
  const schedule = () => {
    if (frame) return
    frame = requestAnimationFrame(() => {
      frame = 0
      handler()
    })
  }

  const observer = new ResizeObserver(schedule)
  const observed = new Set<HTMLElement>()

  // 幂等：挂载钩子与 watch 可能都触发，重复 observe 同一个元素没有额外意义
  function sync(els: Array<HTMLElement | null>) {
    observed.forEach((el) => {
      if (!els.includes(el)) {
        observer.unobserve(el)
        observed.delete(el)
      }
    })
    els.forEach((el) => {
      if (el && !observed.has(el)) {
        observer.observe(el)
        observed.add(el)
      }
    })
  }

  onMounted(() => sync(containers.map((c) => c.value)))
  watch(containers, (els) => sync(els), { flush: 'post' })

  onUnmounted(() => {
    if (frame) cancelAnimationFrame(frame)
    observer.disconnect()
    observed.clear()
  })
}
