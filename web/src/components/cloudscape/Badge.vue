<script setup lang="ts">
import { computed } from 'vue'
import { CS } from './classes'
import { cx } from './utils'

export type BadgeColor =
  | 'grey'
  | 'green'
  | 'blue'
  | 'red'
  | 'severity-critical'
  | 'severity-high'
  | 'severity-medium'
  | 'severity-low'
  | 'severity-neutral'

const props = withDefaults(
  defineProps<{
    color?: BadgeColor
  }>(),
  { color: 'grey' }
)

const classes = computed(() => cx(CS.badge.badge, CS.badge[`badge-color-${props.color}`], 'cs-badge'))
</script>

<template>
  <span :class="classes" :data-badge-color="props.color"><slot /></span>
</template>

<style>
/* 覆盖 Cloudscape vendor 的 :not(#\9) 高特异性规则，注入官方 Style API 变量 */
span.cs-badge[data-badge-color='red']:not(#\9) {
  --awsui-style-background-color: #9e0000;
  --awsui-style-color: #ffffff;
}

span.cs-badge[data-badge-color='green']:not(#\9) {
  --awsui-style-background-color: #028a00;
  --awsui-style-color: #ffffff;
}
</style>
