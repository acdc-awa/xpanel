<script setup lang="ts">
import { computed } from 'vue'
import Icon from './Icon.vue'
import Button from './Button.vue'
import { CS } from './classes'
import { cx } from './utils'

export interface FlashbarItem {
  id: string
  type?: 'success' | 'error' | 'warning' | 'info' | 'in-progress'
  header?: string
  content?: string
  dismissible?: boolean
}

const props = withDefaults(
  defineProps<{
    items: FlashbarItem[]
  }>(),
  {}
)

const emit = defineEmits<{
  (e: 'dismiss', id: string): void
}>()

const visibleItems = computed(() => props.items)

function iconName(type?: string) {
  switch (type) {
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
}
</script>

<template>
  <div :class="cx(CS.flashbar.flashbar, CS.flashbar['visual-refresh'])">
    <ul :class="CS.flashbar['flash-list']" role="list">
      <li v-for="item in visibleItems" :key="item.id" :class="CS.flashbar['flash-list-item']">
        <div :class="cx(CS.flashbar.flash, CS.flashbar[`flash-type-${item.type ?? 'info'}`])">
          <div :class="CS.flashbar['flash-body']">
            <div :class="CS.flashbar['flash-message']">
              <div :class="CS.flashbar['flash-icon']">
                <Icon :name="iconName(item.type)" size="normal" />
              </div>
              <div :class="CS.flashbar['flash-content']">
                <div v-if="item.header" :class="CS.flashbar['flash-header']">{{ item.header }}</div>
                <div :class="CS.flashbar['flash-text']">{{ item.content }}</div>
              </div>
            </div>
            <div v-if="item.dismissible" :class="CS.flashbar['dismiss-button-wrapper']">
              <Button
                variant="icon"
                icon-name="close"
                aria-label="Dismiss"
                :class="CS.flashbar['dismiss-button']"
                @click="emit('dismiss', item.id)"
              />
            </div>
          </div>
        </div>
      </li>
    </ul>
  </div>
</template>
