<script setup lang="ts">
withDefaults(
  defineProps<{
    columns?: 1 | 2
  }>(),
  {
    columns: 2,
  }
)
</script>

<template>
  <dl class="cs-card-facts" :class="`cs-card-facts--cols-${columns}`">
    <slot />
  </dl>
</template>

<style scoped lang="scss">
.cs-card-facts {
  display: grid;
  gap: 12px 20px;
  margin: 0;
}

.cs-card-facts--cols-1 {
  grid-template-columns: minmax(0, 1fr);
}

.cs-card-facts--cols-2 {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

@media (max-width: $bp-small) {
  .cs-card-facts--cols-2 {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
