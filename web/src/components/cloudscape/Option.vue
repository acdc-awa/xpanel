<script setup lang="ts">
// 仅作为 CsSelect 的选项声明载体，本身不渲染任何可见内容。
// 这样 Element Plus 的 <el-option label value /> 可以近乎机械地替换为 <CsOption />。
defineProps<{
  label?: string
  value?: string | number
  disabled?: boolean
}>()
</script>

<template>
  <span hidden><slot /></span>
</template>
