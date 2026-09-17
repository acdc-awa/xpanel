<script setup lang="ts">
import { InfoFilled, WarningFilled } from '@element-plus/icons-vue'

/**
 * 说明性小字收纳图标：常驻小字说明统一挂到图标上，悬停展示。
 * info = 圆点问号（解释机制）；warn = 橙色叹号（提醒注意）。
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
  <el-tooltip :placement="placement" :show-after="150" :content="content">
    <el-icon class="tip-icon" :class="type">
      <WarningFilled v-if="type === 'warn'" />
      <InfoFilled v-else />
    </el-icon>
  </el-tooltip>
</template>

<style scoped lang="scss">
.tip-icon {
  font-size: 13.5px;
  color: var(--x-text-3);
  cursor: help;
  vertical-align: middle;
  flex: none;
  transition: color 0.15s ease, transform 0.15s ease;

  &:hover {
    color: var(--x-primary);
    transform: scale(1.15);
  }

  &.warn {
    color: var(--x-warning);

    &:hover {
      color: var(--x-warning);
      transform: scale(1.15);
    }
  }
}
</style>
