import { reactive } from 'vue'

export type MessageBoxType = 'success' | 'warning' | 'info' | 'error'
/** 与 Element Plus 的拒绝值一致：区分三态时才可能出现 'close' */
export type MessageBoxAction = 'confirm' | 'cancel' | 'close'
export type MessageBoxKind = 'confirm' | 'alert' | 'prompt'

export interface MessageBoxOptions {
  confirmButtonText?: string
  cancelButtonText?: string
  type?: MessageBoxType
  /** 为 true 时「取消」与「关闭（X / 遮罩 / Esc）」分别拒绝成 'cancel' 与 'close' */
  distinguishCancelAndClose?: boolean
  showCancelButton?: boolean
  showClose?: boolean
  /**
   * 破坏性配色（红色实心确认 + 红描边取消）。
   * 默认只在 `type: 'error'`（本仓库用它标注删除类操作）或
   * `confirmButtonClass` 含 danger 时启用；`type: 'warning'` 语义混杂
   * （既有删除也有「退出登录」「保存模板」），需要红色时显式传 true。
   */
  danger?: boolean
  /** prompt 专用：确认时校验，不匹配则留在弹窗内并显示 inputErrorMessage */
  inputPattern?: RegExp
  inputErrorMessage?: string
  inputValue?: string
  inputPlaceholder?: string
  /** 兼容 Element Plus 的 confirmButtonClass，只识别 danger 语义 */
  confirmButtonClass?: string
}

export interface MessageBoxRequest {
  id: number
  kind: MessageBoxKind
  title: string
  message: string
  type: MessageBoxType
  danger: boolean
  confirmText: string
  cancelText: string
  showCancel: boolean
  showClose: boolean
  distinguishCancelAndClose: boolean
  inputPattern?: RegExp
  inputErrorMessage: string
  inputValue: string
  inputPlaceholder?: string
  resolve: (value: unknown) => void
  reject: (reason: MessageBoxAction) => void
}

/** 同一时刻只渲染一个弹窗；并发的 confirm 排队，避免互相覆盖 */
const state = reactive<{ current: MessageBoxRequest | null }>({ current: null })
const queue: MessageBoxRequest[] = []
let seq = 0

export const messageBoxState = state

function open(request: MessageBoxRequest) {
  if (state.current) {
    queue.push(request)
    return
  }
  state.current = request
}

function settle(id: number, action: MessageBoxAction, value?: string) {
  const current = state.current
  if (!current || current.id !== id) return
  state.current = null

  if (action === 'confirm') {
    current.resolve(current.kind === 'prompt' ? { value: value ?? '', action } : action)
  } else {
    // Element Plus 默认把「取消」与「关闭」都报成 'cancel'，只有显式要求区分时才给 'close'
    current.reject(current.distinguishCancelAndClose ? action : 'cancel')
  }

  const next = queue.shift()
  if (next) state.current = next
}

function build(
  kind: MessageBoxKind,
  message: string,
  title: string,
  options: MessageBoxOptions,
  resolve: (value: unknown) => void,
  reject: (reason: MessageBoxAction) => void
): MessageBoxRequest {
  return {
    id: ++seq,
    kind,
    title,
    message,
    type: options.type ?? 'info',
    danger: options.danger ?? (options.type === 'error' || Boolean(options.confirmButtonClass?.includes('danger'))),
    confirmText: options.confirmButtonText ?? '确定',
    cancelText: options.cancelButtonText ?? '取消',
    showCancel: kind === 'alert' ? false : (options.showCancelButton ?? true),
    showClose: options.showClose ?? true,
    distinguishCancelAndClose: options.distinguishCancelAndClose ?? false,
    inputPattern: options.inputPattern,
    inputErrorMessage: options.inputErrorMessage ?? '输入内容不合法',
    inputValue: options.inputValue ?? '',
    inputPlaceholder: options.inputPlaceholder,
    resolve,
    reject,
  }
}

/** 确认框：确认 resolve('confirm')；取消/关闭 reject('cancel' | 'close') */
function confirm(message: string, title = '提示', options: MessageBoxOptions = {}): Promise<MessageBoxAction> {
  return new Promise((resolve, reject) => {
    open(build('confirm', message, title, options, (v) => resolve(v as MessageBoxAction), reject))
  })
}

/** 提示框：只有确认按钮 */
function alert(message: string, title = '提示', options: MessageBoxOptions = {}): Promise<MessageBoxAction> {
  return new Promise((resolve, reject) => {
    open(build('alert', message, title, options, (v) => resolve(v as MessageBoxAction), reject))
  })
}

/** 输入框：确认 resolve({ value, action })；取消/关闭 reject('cancel' | 'close') */
function prompt(
  message: string,
  title = '提示',
  options: MessageBoxOptions = {}
): Promise<{ value: string; action: MessageBoxAction }> {
  return new Promise((resolve, reject) => {
    open(build('prompt', message, title, options, (v) => resolve(v as { value: string; action: MessageBoxAction }), reject))
  })
}

/**
 * `ElMessageBox` 的兼容壳：调用点保持 `ElMessageBox.confirm(...)` 原样，
 * 渲染换成 Cloudscape Modal + 官方按钮，视觉与 `CsConfirmDialog` 一致。
 *
 * 支持本仓库实际用到的选项：confirmButtonText / cancelButtonText / type /
 * distinguishCancelAndClose / showClose / inputPattern / inputErrorMessage /
 * inputValue / inputPlaceholder / confirmButtonClass（仅危险语义）。
 * 不支持的选项（beforeClose / dangerouslyUseHTMLString / customClass 等）会静默失效。
 *
 * 注意：不把正文首段自动改写成红色「Warning:」行——调用点的 message 多是问句
 * （「确认删除证书「x」？」），而 AWS 的 Warning 行陈述的是不可逆后果，两者语义不同。
 * 需要那句红字时请直接用 `CsConfirmDialog` 的 `warning` 属性。
 */
export const messageBox = { confirm, alert, prompt }

/** 供宿主组件回写用户操作 */
export function resolveMessageBox(id: number, action: MessageBoxAction, value?: string) {
  settle(id, action, value)
}

export const ElMessageBox = messageBox
