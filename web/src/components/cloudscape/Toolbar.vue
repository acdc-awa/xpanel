<script setup lang="ts">
/**
 * 页面工具栏：左侧是筛选/搜索控件组，右侧 `#actions` 是主操作。
 * 官方没有同名组件（AppLayout 的工具栏属于外壳层），这里按 AWS 控制台
 * 表格页的通用排布收拢：单行优先、放不下时换行、左右两端对齐。
 */
</script>

<template>
  <div class="cs-toolbar">
    <div class="cs-toolbar-main">
      <slot />
    </div>
    <div v-if="$slots.actions" class="cs-toolbar-actions">
      <slot name="actions" />
    </div>
  </div>
</template>

<style scoped>
.cs-toolbar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-s-kll7oj, 12px);
  min-inline-size: 0;
}

.cs-toolbar-main,
.cs-toolbar-actions {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-xs-m9b7jf, 8px);
  min-inline-size: 0;
}

.cs-toolbar-actions {
  margin-inline-start: auto;
}
</style>
