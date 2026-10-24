<script setup lang="ts">
import ToastEntry from './ToastEntry.vue'
import { toasts, toast } from './toast'

/*
 * 顶部提示栏的宿主：挂在布局里一次，所有页面通过 toast.success(...) 调用。
 * 进出场动画由 ToastEntry 内部的官方 flash-with-motion 过渡类负责，
 * 所以这里不加 TransitionGroup（两套动画会互相打架）。
 */
function dismiss(id: string) {
  toast.remove(id)
}
</script>

<template>
  <div class="cs-toast-host" role="region" aria-label="通知">
    <ToastEntry v-for="item in toasts.items" :key="item.id" :toast="item" @dismiss="dismiss" @expired="dismiss" />
  </div>
</template>

<style scoped>
/*
 * 官方 AppLayout 的 notifications 槽是「内容区顶部留白里的常驻区」；
 * 这里要的是浮在页面之上、滚走也不消失的 toast，所以用 fixed + 顶栏下方偏移。
 * 层级必须在 Modal(5000) / Drawer(6000) 之上：弹窗里的保存、复制、生成都会弹 toast，
 * 压在遮罩下面等于没有反馈（实测 elementFromPoint 命中遮罩而不是 toast）。
 * 仍低于 Select / ButtonDropdown 的 9000，避免下拉选项被 toast 盖住。
 */
.cs-toast-host {
  position: fixed;
  inset-block-start: calc(var(--x-topbar-h, 56px) + 12px);
  inset-inline: 0;
  z-index: 7000;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding-inline: 16px;
  pointer-events: none;
}

.cs-toast-host > * {
  inline-size: min(720px, 100%);
  pointer-events: auto;
}
</style>
