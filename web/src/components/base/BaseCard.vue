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
  transition: background-color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;

  &__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 14px 20px;
    border-bottom: 1px solid var(--x-border);
    min-height: 52px;

    @media (max-width: 640px) {
      padding: 12px 14px;
      min-height: 46px;
    }
  }

  &__title {
    font-size: 14.5px;
    font-weight: 600;
    color: var(--x-text);
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 0;
  }

  &__extra {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  &__body {
    padding: 18px 20px;

    @media (max-width: 640px) {
      padding: 14px 12px;
    }
  }
}
</style>
