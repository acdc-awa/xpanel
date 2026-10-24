import { onBeforeUnmount, onMounted, type Ref } from 'vue'
import { GRID } from './model'

/**
 * 把卡片外框高度吸附到 GRID 的整数倍。
 *
 * 卡片内部的行高/间距已按 8pt 排布，但层胶囊头部会换行、接入点 chip 数量不定，
 * 单靠 CSS 无法保证总高恰好是 8 的倍数；而「边界落在网格上」看的是外框。
 *
 * 高度一旦被固定，元素自身不会再因内容变化触发 ResizeObserver，所以同时观察
 * 直接子节点，并用「最后一个子节点的底边 + 下内边距」计算自然高度。
 * offsetTop/offsetHeight 是布局尺寸，不受画布缩放影响。
 */
export function useGridHeight(el: Ref<HTMLElement | null>) {
  let observer: ResizeObserver | null = null
  let mutations: MutationObserver | null = null
  let frame = 0

  function measure(node: HTMLElement): number {
    const cs = getComputedStyle(node)
    const border = (parseFloat(cs.borderTopWidth) || 0) + (parseFloat(cs.borderBottomWidth) || 0)
    const padTop = parseFloat(cs.paddingTop) || 0
    const padBottom = parseFloat(cs.paddingBottom) || 0
    let bottom = 0
    for (const child of Array.from(node.children)) {
      const h = child as HTMLElement
      if (getComputedStyle(h).position === 'absolute') continue
      bottom = Math.max(bottom, h.offsetTop + h.offsetHeight)
    }
    // 没有任何流内子节点时退回内容高度
    const content = bottom > 0 ? bottom + padBottom + border : node.scrollHeight + border
    // 保证下内边距被计入一次（offsetTop 从边框盒顶部算起）
    return Math.max(content, padTop + padBottom + border)
  }

  /** 同步量一次：首帧就要落在网格上，不能依赖 rAF（后台标签页会推迟）。 */
  function applyNow() {
    const node = el.value
    if (!node) return
    const snapped = Math.ceil(measure(node) / GRID) * GRID
    if (parseFloat(node.style.height || '0') !== snapped) node.style.height = `${snapped}px`
  }

  /** ResizeObserver 回调里合并到下一帧，避免多次读写造成抖动。 */
  function schedule() {
    cancelAnimationFrame(frame)
    frame = requestAnimationFrame(applyNow)
  }

  function observe(node: HTMLElement) {
    observer?.disconnect()
    observer = new ResizeObserver(schedule)
    observer.observe(node)
    for (const child of Array.from(node.children)) observer.observe(child)
  }

  onMounted(() => {
    const node = el.value
    if (!node) return
    observe(node)
    applyNow()
    // 子节点增删（权限 chip、描述行等）不会改变已固定的外框高度，RO 收不到通知
    mutations = new MutationObserver(() => {
      observe(node)
      schedule()
    })
    mutations.observe(node, { childList: true, subtree: true, characterData: true })
  })

  onBeforeUnmount(() => {
    observer?.disconnect()
    mutations?.disconnect()
    cancelAnimationFrame(frame)
  })
}
