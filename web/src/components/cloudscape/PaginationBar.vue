<script setup lang="ts">
/**
 * 集合底部分页区。官方把分页放进表格/集合的 footer（Container 的 footer 槽自带
 * 分隔线与内边距），所以这里只负责对齐方式，供 `CsContainer #footer` 或独立分页区
 * 复用，替代各页手写的 `.x-pager`。
 */
withDefaults(
  defineProps<{
    align?: 'start' | 'center' | 'end'
  }>(),
  { align: 'end' }
)
</script>

<template>
  <div class="cs-pagination-bar" :class="`is-${align}`">
    <slot />
  </div>
</template>

<style scoped>
.cs-pagination-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-s-kll7oj, 12px);
}

.cs-pagination-bar.is-start {
  justify-content: flex-start;
}

.cs-pagination-bar.is-center {
  justify-content: center;
}

.cs-pagination-bar.is-end {
  justify-content: flex-end;
}
</style>
