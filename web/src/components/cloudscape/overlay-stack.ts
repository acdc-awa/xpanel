/**
 * 浮层 Esc 栈：Modal / Drawer / Popover / Tooltip 共用一个 document keydown 监听器，
 * 按 Esc 时只触发「最后入栈」（视觉最上层）的回调，避免一次 Esc 同时关掉多层浮层。
 */
const stack: Array<{ onEscape: () => void }> = []
let listening = false

function onKeydown(event: KeyboardEvent) {
  if (event.key !== 'Escape') return
  const top = stack[stack.length - 1]
  if (!top) return
  event.preventDefault()
  event.stopPropagation()
  top.onEscape()
}

function syncListener() {
  if (typeof document === 'undefined') return
  if (stack.length > 0 && !listening) {
    document.addEventListener('keydown', onKeydown, true)
    listening = true
  } else if (stack.length === 0 && listening) {
    document.removeEventListener('keydown', onKeydown, true)
    listening = false
  }
}

/** 入栈并返回出栈函数（幂等，可重复调用）。 */
export function pushOverlay(onEscape: () => void): () => void {
  const entry = { onEscape }
  stack.push(entry)
  syncListener()
  let removed = false
  return () => {
    if (removed) return
    removed = true
    const index = stack.indexOf(entry)
    if (index >= 0) stack.splice(index, 1)
    syncListener()
  }
}
