import { toast } from './toast'
import type { ToastOptions } from './toast'

export { toast }
export type { ToastItem, ToastOptions, ToastType } from './toast'

/**
 * `ElMessage` 的兼容壳：调用点保持原样（`ElMessage.success('已保存')`），
 * 实际渲染换成 Cloudscape 闪条样式的顶部提示。
 *
 * 只覆盖本仓库实际用到的调用形态：
 * - `ElMessage.success('文本')` / `error` / `warning` / `info`
 * - `ElMessage.success({ message, duration })`
 * 不支持 `dangerouslyUseHTMLString` / `grouping` / `showClose: false` 等 Element 专有选项
 * —— 这些一旦被使用会静默失效，所以需要时请改用 `toast` 显式传参。
 */
type MessageKind = 'success' | 'error' | 'warning' | 'info'

interface ElementLikeOptions {
  message?: string
  duration?: number
}

function normalize(content: string | ElementLikeOptions): { text: string; options: ToastOptions } {
  if (typeof content === 'string') return { text: content, options: {} }
  return {
    text: content.message ?? '',
    options: content.duration === undefined ? {} : { duration: content.duration },
  }
}

function create(kind: MessageKind) {
  return (content: string | ElementLikeOptions) => {
    const { text, options } = normalize(content)
    return toast[kind](text, options)
  }
}

export const message = {
  success: create('success'),
  error: create('error'),
  warning: create('warning'),
  info: create('info'),
  /** 对应 ElMessage.closeAll()：清空当前所有提示 */
  closeAll: () => toast.clear(),
}

/** 以 `ElMessage` 的名字导出，便于把 `import { ElMessage } from 'element-plus'` 定向替换过来。 */
export const ElMessage = message
