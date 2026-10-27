import { onUnmounted } from 'vue'

/**
 * 挂载后连续若干帧测量一次，用来越过 admin 外壳「先桌面后窄屏」与 GridStack
 * 过渡动画的中间态。项目的 ResizeObserver 在部分切换路径里只回调第一次，
 * 单次 mount 测量会停在过渡中的宽度上（表现为列数/卡片形态落后一档）。
 *
 * 默认只在前 1.5s 内每帧测一次，之后由元素上的 ResizeObserver + window.resize 兜底。
 */
export function useSettleMeasure(measure: () => void, durationMs = 1500) {
  let raf = 0
  let until = 0

  function tick() {
    measure()
    if (performance.now() < until) raf = requestAnimationFrame(tick)
  }

  function start() {
    cancelAnimationFrame(raf)
    until = performance.now() + durationMs
    raf = requestAnimationFrame(tick)
  }

  onUnmounted(() => cancelAnimationFrame(raf))

  return { start }
}
