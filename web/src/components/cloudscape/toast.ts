import { reactive, readonly } from 'vue'
import type { FlashType } from './FlashItem.vue'

export type ToastType = FlashType

export interface ToastItem {
  id: string
  type: ToastType
  header?: string
  content: string
  /** 毫秒；0 = 不自动消失（用于「处理中」这类需要人工收尾的提示） */
  duration: number
  dismissible: boolean
}

export interface ToastOptions {
  header?: string
  /** 毫秒，默认 3000；0 表示不自动消失 */
  duration?: number
  /** 是否显示关闭按钮，默认 true */
  dismissible?: boolean
}

const DEFAULT_DURATION = 3000
/** 同时最多展示的条数：超出的按先进先出淘汰，避免刷屏堆满屏幕 */
const MAX_VISIBLE = 4

const state = reactive<{ items: ToastItem[] }>({ items: [] })
let seq = 0

export const toasts = readonly(state)

function push(type: ToastType, content: string, options: ToastOptions = {}) {
  state.items.push({
    id: `cs-toast-${++seq}`,
    type,
    header: options.header,
    content,
    duration: options.duration ?? DEFAULT_DURATION,
    dismissible: options.dismissible ?? true,
  })
  if (state.items.length > MAX_VISIBLE) {
    state.items.splice(0, state.items.length - MAX_VISIBLE)
  }
}

/** 由宿主在条目退出动画结束后调用，或调用方手动关闭「处理中」提示 */
function remove(id: string) {
  const index = state.items.findIndex((t) => t.id === id)
  if (index !== -1) state.items.splice(index, 1)
}

function clear() {
  state.items.splice(0, state.items.length)
}

/**
 * 页面上方的瞬时提示（Cloudscape 闪条视觉 + 自动消失）。
 *
 * 与 CsFlashbar 的区别只在生命周期：Flashbar 是常驻通知列表，需要用户关闭；
 * toast 会自己走完倒计时。悬停时倒计时暂停，移开后继续。
 */
export const toast = {
  success: (content: string, options?: ToastOptions) => push('success', content, options),
  error: (content: string, options?: ToastOptions) => push('error', content, options),
  warning: (content: string, options?: ToastOptions) => push('warning', content, options),
  info: (content: string, options?: ToastOptions) => push('info', content, options),
  /** 「处理中」默认不自动消失（duration=0），结束时由调用方 toast.remove(id) */
  loading: (content: string, options?: ToastOptions) => push('in-progress', content, { duration: 0, ...options }),
  remove,
  clear,
}
