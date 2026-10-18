<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import FlashItem from './FlashItem.vue'
import type { ToastItem } from './toast'
import { CS } from './classes'
import { cx } from './utils'

const props = defineProps<{
  toast: ToastItem
}>()

const emit = defineEmits<{
  (e: 'dismiss', id: string): void
  (e: 'expired', id: string): void
}>()

/*
 * 官方 flashbar 的动效是「基础类 flash-with-motion + 状态类 enter/entering/entered/exiting」
 * 两级组合，状态类只提供 transform/opacity 与 transition 时长（250ms 进 / 115ms 出）。
 * 这里手动推进状态，复用同一套 CSS，不另写关键帧。
 */
type Phase = 'enter' | 'entering' | 'entered' | 'exiting'
const phase = ref<Phase>('enter')

const ENTER_MS = 250
const EXIT_MS = 115

let enterTimer: ReturnType<typeof setTimeout> | null = null
let exitTimer: ReturnType<typeof setTimeout> | null = null
/** 剩余可停留时长：悬停时冻结，移开后续跑 */
let remaining = props.toast.duration
let startedAt = 0
let paused = false

const motionClass = computed(() => cx(CS.flashbar['flash-with-motion'], CS.flashbar[phase.value]))

function beginExit() {
  if (phase.value === 'exiting') return
  if (exitTimer) clearTimeout(exitTimer)
  phase.value = 'exiting'
  exitTimer = setTimeout(() => emit('expired', props.toast.id), EXIT_MS)
}

function startCountdown() {
  if (props.toast.duration <= 0) return
  startedAt = Date.now()
  exitTimer = setTimeout(beginExit, remaining)
}

function pauseCountdown() {
  if (paused || props.toast.duration <= 0 || phase.value === 'exiting') return
  paused = true
  if (exitTimer) {
    clearTimeout(exitTimer)
    exitTimer = null
  }
  remaining = Math.max(0, remaining - (Date.now() - startedAt))
}

function resumeCountdown() {
  if (!paused) return
  paused = false
  startCountdown()
}

onMounted(() => {
  // 先渲染 enter 态（opacity:0），下一帧切 entering 才会真正触发过渡
  requestAnimationFrame(() => {
    phase.value = 'entering'
    enterTimer = setTimeout(() => {
      phase.value = 'entered'
    }, ENTER_MS)
  })
  startCountdown()
})

onBeforeUnmount(() => {
  if (enterTimer) clearTimeout(enterTimer)
  if (exitTimer) clearTimeout(exitTimer)
})

/** 用户点关闭：走完退出动画再通知宿主移除 */
function onDismiss() {
  beginExit()
}
</script>

<template>
  <div class="cs-toast-entry" @mouseenter="pauseCountdown" @mouseleave="resumeCountdown">
    <FlashItem
      :class="motionClass"
      :item="{
        id: toast.id,
        type: toast.type,
        header: toast.header,
        content: toast.content,
        dismissible: toast.dismissible,
      }"
      aria-role="status"
      @dismiss="onDismiss"
    />
  </div>
</template>
