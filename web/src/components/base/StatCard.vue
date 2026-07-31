<script setup lang="ts">
import type { Component } from 'vue'

defineProps<{
  icon: Component
  value: string
  label: string
  tone?: 'purple' | 'green' | 'blue' | 'orange'
}>()
</script>

<template>
  <div class="x-stat-card">
    <div class="x-stat-icon" :class="tone ?? 'purple'">
      <el-icon :size="20"><component :is="icon" /></el-icon>
    </div>
    <div class="x-stat-value">{{ value }}</div>
    <div class="x-stat-label">{{ label }}</div>
  </div>
</template>
