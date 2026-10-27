import { onUnmounted } from 'vue'

/**
 * 挂载后连续若干帧测量一次，用来越过 admin 外壳「先桌面后窄屏」与 GridStack
 * 过渡动画的中间态。项目的 ResizeObserver 在部分切换路径里只回调第一次，
 * 单次 mount 测量会停在过渡中的宽度上（表现为列数/卡片形态落后一档）。
 *
 * 默认只在前 1.5s 内每帧测一次，之后由元素上的 ResizeObserver + window.resize 兜底。
 * 窗口被遮挡（最小化/后台标签页）时 Chrome 会暂停 requestAnimationFrame，
 * 所以再挂几条定时器测量：定时器照常触发，避免首屏停在过渡中的宽度上。
 */
export function useSettleMeasure(measure: () => void, durationMs = 1500) {
  let raf = 0
  let until = 0
  const timers = new Set<number>()

  function tick() {
    measure()
    if (performance.now() < until) raf = requestAnimationFrame(tick)
  }

  function clearTimers() {
    for (const id of timers) clearTimeout(id)
    timers.clear()
  }

  function start() {
    cancelAnimationFrame(raf)
    clearTimers()
    until = performance.now() + durationMs
    raf = requestAnimationFrame(tick)
    for (const delay of [80, 200, 400, 800, 1400]) {
      if (delay <= durationMs) timers.add(window.setTimeout(measure, delay))
    }
  }

  onUnmounted(() => {
    cancelAnimationFrame(raf)
    clearTimers()
  })

  return { start }
}
