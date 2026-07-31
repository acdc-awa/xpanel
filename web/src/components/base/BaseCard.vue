<script setup lang="ts">
defineProps<{ title?: string }>()
</script>

<template>
  <section class="base-card">
    <header v-if="title || $slots.extra" class="base-card__head">
      <h3 class="base-card__title">{{ title }}</h3>
      <div class="base-card__extra"><slot name="extra" /></div>
    </header>
    <div class="base-card__body"><slot /></div>
  </section>
</template>

<style scoped lang="scss">
.base-card {
  background: var(--x-card);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);
  box-shadow: var(--x-shadow);
  margin-bottom: 20px;

  &__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 14px 20px;
    border-bottom: 1px solid var(--x-border);
  }
  &__title {
    font-size: 14.5px;
    font-weight: 600;
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0;
  }
  &__body { padding: 0; }
  &__body:has(> :only-child:not(table)) { padding: 20px; }
}
</style>
