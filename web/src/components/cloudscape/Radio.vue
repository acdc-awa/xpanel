<script setup lang="ts">
// CsRadioGroup 的选项声明载体。
defineProps<{
  value?: string | number | boolean
  label?: string
  disabled?: boolean
}>()
</script>

<template>
  <span hidden><slot /></span>
</template>
