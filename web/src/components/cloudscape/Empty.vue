<script setup lang="ts">
/**
 * 轻量空态组件。Cloudscape 官方没有 Empty 组件（各集合组件内部自带空态文案），
 * 这里把各页重复的「大图标 + 一句说明」收拢成一个组件，视觉沿用已迁移页面的
 * CsIcon + 次要文本组合，不引入新的设计语义。
 */
import Icon from './Icon.vue'

withDefaults(
  defineProps<{
    description?: string
    iconName?: string
    /** 空态与加载态不是一回事：加载中请用 CsSpinner。 */
    minHeight?: string
  }>(),
  { description: '暂无数据', iconName: 'folder-open', minHeight: '160px' }
)
</script>

<template>
  <div class="cs-empty" :style="{ minHeight }">
    <Icon :name="iconName" size="large" variant="subtle" />
    <p class="cs-empty-text">{{ description }}</p>
    <slot />
  </div>
</template>

<style scoped>
.cs-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  padding: 32px 16px;
  text-align: center;
}

.cs-empty-text {
  margin: 0;
  font-size: 13.5px;
  line-height: 20px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}
</style>
