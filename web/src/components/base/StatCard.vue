<script setup lang="ts">
import type { Component } from 'vue'

defineProps<{
  icon: Component
  value: string | number
  label: string
  sub?: string
  tone?: 'purple' | 'green' | 'blue' | 'orange' | 'cyan' | 'amber' | 'emerald' | 'indigo'
}>()
</script>

<template>
  <div class="x-stat-card">
    <div class="x-stat-card__top">
      <span class="x-stat-label">{{ label }}</span>
      <div class="x-stat-icon" :class="tone ?? 'purple'">
        <el-icon :size="18"><component :is="icon" /></el-icon>
      </div>
    </div>
    <div class="x-stat-value font-mono-num">{{ value }}</div>
    <div v-if="sub" class="x-stat-sub font-mono-num">{{ sub }}</div>
  </div>
</template>
