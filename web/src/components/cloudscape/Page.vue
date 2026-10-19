<script setup lang="ts">
/**
 * 页面内容骨架。Cloudscape 的 ContentLayout 负责“页面级最大宽度 + 内容列居中”，
 * 本项目没有 app-layout 外壳（侧栏与顶栏在 AdminLayout 里），所以这里只保留
 * 同一职责的子集：限宽居中 + 官方 page 级纵向间距（scaled-l = 20px）。
 * 横向内边距由 .admin-content 统一提供，这里不再叠加。
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
  /* 内容最大宽度是应用级布局常量，与 AdminLayout 共享同一个 token */
  max-inline-size: var(--x-content-max, 1320px);
  margin-inline: auto;
  display: flex;
  flex-direction: column;
  row-gap: var(--space-scaled-l-33ruz6, 20px);
  min-inline-size: 0;
}
</style>
