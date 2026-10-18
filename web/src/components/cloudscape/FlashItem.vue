<script setup lang="ts">
import { computed, useId } from 'vue'
import Icon from './Icon.vue'
import Button from './Button.vue'
import Spinner from './Spinner.vue'
import { CS } from './classes'
import { cx } from './utils'

export type FlashType = 'success' | 'error' | 'warning' | 'info' | 'in-progress'

export interface FlashItemModel {
  id: string
  type?: FlashType
  header?: string
  content?: string
  dismissible?: boolean
  /** 处理中：按官方规则把类型降级为 info 并把图标换成 spinner */
  loading?: boolean
}

const props = withDefaults(
  defineProps<{
    item: FlashItemModel
    /** ariaRole="status" 时由读屏在内容更新时播报 */
    ariaRole?: 'status'
  }>(),
  { ariaRole: undefined }
)

const emit = defineEmits<{
  (e: 'dismiss', id: string): void
}>()

/*
 * 官方 flash.js 的两条硬约束（改这里之前先读源码）：
 * 1. DOM 必须是 flash > flash-body > flash-focus-container > (flash-icon + flash-message)，
 *    配色规则写的是 `.flash-type-* > .flash-body > .flash-focus-container > .flash-icon`，
 *    图标一挪位置整条规则就失效。
 * 2. 文字色不能用默认值：`.flash-text` 取 --color-text-body-default（浅色模式是深色），
 *    必须靠 flash 根上的 `awsui-context-flashbar` / `awsui-context-flashbar-warning`
 *    把该 token 在局部重映射成通知色，否则彩色底上会出现深色文字。
 */
const effectiveType = computed<FlashType>(() => (props.item.loading ? 'info' : (props.item.type ?? 'info')))

const contextClass = computed(() =>
  effectiveType.value === 'warning' ? 'awsui-context-flashbar-warning' : 'awsui-context-flashbar'
)

const iconName = computed(() => {
  switch (effectiveType.value) {
    case 'error':
      return 'status-negative'
    case 'warning':
      return 'status-warning'
    case 'success':
      return 'status-positive'
    case 'in-progress':
      return 'status-in-progress'
    default:
      return 'status-info'
  }
})

const iconId = useId()
const messageId = useId()
const labelledBy = `${iconId} ${messageId}`
</script>

<template>
  <div
    :data-itemid="item.id"
    :class="cx(CS.flashbar.flash, CS.flashbar[`flash-type-${effectiveType}`], contextClass)"
    :role="ariaRole"
    :aria-live="ariaRole ? 'polite' : undefined"
  >
    <div :class="CS.flashbar['flash-body']">
      <div :class="CS.flashbar['flash-focus-container']" tabindex="-1" role="group" :aria-labelledby="labelledBy">
        <div :id="iconId" :class="cx(CS.flashbar['flash-icon'], CS.flashbar['flash-text'])">
          <Spinner v-if="item.loading" size="normal" variant="inverted" />
          <Icon v-else :name="iconName" size="normal" />
        </div>
        <div :id="messageId" :class="cx(CS.flashbar['flash-message'], CS.flashbar['flash-text'])">
          <div v-if="item.header" :class="CS.flashbar['flash-header']">{{ item.header }}</div>
          <div :class="CS.flashbar['flash-content']">
            <slot>{{ item.content }}</slot>
          </div>
        </div>
      </div>

      <div v-if="$slots.action" :class="CS.flashbar['action-button-wrapper']">
        <slot name="action" />
      </div>
    </div>

    <div v-if="item.dismissible" :class="CS.flashbar['dismiss-button-wrapper']">
      <Button
        variant="flashbar-icon"
        icon-name="close"
        aria-label="关闭通知"
        :class="CS.flashbar['dismiss-button']"
        @click="emit('dismiss', item.id)"
      />
    </div>
  </div>
</template>
