<script setup lang="ts">
import CsTooltip from '@/components/cloudscape/Tooltip.vue'
import CsIcon from '@/components/cloudscape/Icon.vue'

/**
 * 说明性小字收纳图标：常驻小字说明统一挂到图标上，悬停展示。
 * info = 圆形信息图标（解释机制）；warn = 橙色警告图标（提醒注意）。
 */
withDefaults(
  defineProps<{
    content: string
    type?: 'info' | 'warn'
    placement?: 'top' | 'top-start' | 'top-end' | 'bottom' | 'left' | 'right'
  }>(),
  { type: 'info', placement: 'top' },
)
</script>

<template>
  <CsTooltip :content="content" :placement="placement" :show-delay="150" class="tip-icon" :class="type">
    <CsIcon
      :name="type === 'warn' ? 'status-warning' : 'status-info'"
      size="small"
      :variant="type === 'warn' ? 'warning' : 'subtle'"
    />
  </CsTooltip>
</template>

<style scoped lang="scss">
.tip-icon {
  cursor: help;
  vertical-align: middle;
  flex: none;
  transition: transform 0.15s ease;

  &:hover {
    transform: scale(1.15);
  }
}
</style>
