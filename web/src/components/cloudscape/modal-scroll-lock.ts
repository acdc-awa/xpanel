/**
 * 页面滚动锁：按「当前打开的弹窗数」记账。
 *
 * 弹窗会叠加（入站编辑器内还能再开「新建对外接入层」）。如果每个 CsModal 各自
 * 无条件写 `body.style.overflow`，后挂载的隐藏弹窗（visible=false 的 immediate
 * watcher）会立刻把外层弹窗刚加上的 hidden 清掉。这里让关闭方只释放自己持有的
 * 那一份，最后一份释放时才恢复滚动与焦点。
 */
let lockCount = 0
let previousActive: HTMLElement | null = null

export function lockBodyScroll() {
  if (lockCount === 0) {
    previousActive = document.activeElement as HTMLElement | null
    document.body.style.overflow = 'hidden'
  }
  lockCount += 1
}

export function unlockBodyScroll() {
  if (lockCount === 0) return
  lockCount -= 1
  if (lockCount > 0) return
  document.body.style.overflow = ''
  previousActive?.focus?.()
  previousActive = null
}
