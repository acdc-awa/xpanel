<script setup lang="ts">
import { computed } from 'vue'
import FlashItem from './FlashItem.vue'
import type { FlashItemModel } from './FlashItem.vue'
import { CS } from './classes'

export type { FlashItemModel }

/**
 * 官方 Flashbar 的「常驻通知列表」形态：消息留在页面上，由用户点关闭。
 * 需要「弹出后自动消失」的瞬时提示请用 CsToast。
 */
const props = withDefaults(
  defineProps<{
    items: FlashItemModel[]
    ariaLabel?: string
    /** ariaRole="status" 时由读屏在内容更新时播报 */
    ariaRole?: 'status'
  }>(),
  {
    ariaLabel: '通知',
    ariaRole: undefined,
  }
)

const emit = defineEmits<{
  (e: 'dismiss', id: string): void
  (e: 'action', id: string): void
}>()

const visibleItems = computed(() => props.items)
</script>

<template>
  <div :class="CS.flashbar.flashbar">
    <ul :class="CS.flashbar['flash-list']" :aria-label="ariaLabel">
      <li v-for="item in visibleItems" :key="item.id" :class="CS.flashbar['flash-list-item']">
        <FlashItem :item="item" :aria-role="ariaRole" @dismiss="emit('dismiss', $event)">
          <template v-if="$slots.action" #action>
            <slot name="action" :item="item" />
          </template>
        </FlashItem>
      </li>
    </ul>
  </div>
</template>
