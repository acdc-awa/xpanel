<script setup lang="ts">
import { computed } from 'vue'
import { APP_ICON_DEFINITIONS } from './app-icons'
import { CS } from './classes'
import { ICON_DEFINITIONS } from './icons'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    name: string
    size?: 'x-small' | 'small' | 'normal' | 'medium' | 'big' | 'large'
    variant?: 'normal' | 'disabled' | 'inverted' | 'subtle' | 'warning' | 'error' | 'success' | 'link' | 'info'
    ariaLabel?: string
    badge?: boolean
  }>(),
  {
    size: 'normal',
    variant: 'normal',
    badge: false,
    ariaLabel: undefined,
  }
)

const definition = computed(() => APP_ICON_DEFINITIONS[props.name] ?? ICON_DEFINITIONS[props.name])

const classes = computed(() =>
  cx(
    CS.icon.icon,
    CS.icon[`size-${props.size}`],
    CS.icon[`size-${props.size}-mapped-height`],
    CS.icon[`variant-${props.variant}`],
    props.badge && CS.icon.badge,
    CS.icon[`name-${props.name}`]
  )
)
</script>

<template>
  <span
    v-if="definition"
    :class="classes"
    :aria-hidden="ariaLabel ? undefined : 'true'"
    :aria-label="ariaLabel"
    :role="ariaLabel ? 'img' : undefined"
  >
    <svg
      :viewBox="definition.viewBox"
      xmlns="http://www.w3.org/2000/svg"
      focusable="false"
      aria-hidden="true"
      data-awsui-icon-animated="true"
      v-html="definition.inner"
    />
  </span>
</template>
