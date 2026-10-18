<script setup lang="ts">
import Spinner from './Spinner.vue'

withDefaults(
  defineProps<{
    loading: boolean
    text?: string
    minHeight?: string
  }>(),
  {
    text: undefined,
    minHeight: undefined,
  }
)
</script>

<template>
  <div class="cs-loading" :class="{ 'is-loading': loading }" :style="minHeight ? { minHeight } : undefined">
    <slot />
    <div v-if="loading" class="cs-loading-overlay" aria-live="polite">
      <Spinner size="large" />
      <span v-if="text" class="cs-loading-text">{{ text }}</span>
    </div>
  </div>
</template>

<style scoped lang="scss">
.cs-loading {
  position: relative;
}

.cs-loading-overlay {
  position: absolute;
  inset: 0;
  z-index: 5;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-height: 64px;
  background: color-mix(in srgb, var(--color-background-container-content-mjfil9, #ffffff) 72%, transparent);
}

.cs-loading-text {
  font-size: 12px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}
</style>
