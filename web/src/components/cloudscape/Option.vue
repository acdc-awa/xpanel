<script setup lang="ts">
// 仅作为 CsSelect 的选项声明载体，本身不渲染任何可见内容。
defineProps<{
  label?: string
  value?: string | number
  disabled?: boolean
}>()
</script>

<template>
  <span hidden><slot /></span>
</template>
