<script setup lang="ts">
withDefaults(
  defineProps<{
    minWidth?: number
    gap?: number
  }>(),
  {
    minWidth: 320,
    gap: 20,
  }
)
</script>

<template>
  <div
    class="cs-card-grid"
    :style="{ gridTemplateColumns: `repeat(auto-fill, minmax(min(100%, ${minWidth}px), 1fr))`, gap: `${gap}px` }"
  >
    <slot />
  </div>
</template>

<style scoped lang="scss">
.cs-card-grid {
  display: grid;
  align-items: stretch;
}
</style>
