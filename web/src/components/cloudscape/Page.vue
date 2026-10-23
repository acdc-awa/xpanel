<script setup lang="ts">
/**
 * 页面内容骨架。限宽与左右 gutter 由外层 CsAppLayout 的 grid 负责
 * （官方断点 1280 / 1440 / 1620），这里只提供 page 级纵向节奏
 * （scaled-l = 20px），避免和外壳重复约束宽度。
 */
</script>

<template>
  <div class="cs-page">
    <slot />
  </div>
</template>

<style scoped>
.cs-page {
  inline-size: 100%;
  display: flex;
  flex-direction: column;
  row-gap: var(--space-scaled-l-33ruz6, 20px);
  min-inline-size: 0;
}
</style>
