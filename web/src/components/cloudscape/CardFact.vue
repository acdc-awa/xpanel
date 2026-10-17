<script setup lang="ts">
withDefaults(
  defineProps<{
    label: string
    value?: string | number
    fullWidth?: boolean
    mono?: boolean
  }>(),
  {
    value: undefined,
    fullWidth: false,
    mono: false,
  }
)
</script>

<template>
  <div class="cs-card-fact" :class="{ 'cs-card-fact--full': fullWidth }">
    <dt class="cs-card-fact__label">{{ label }}</dt>
    <dd class="cs-card-fact__value" :class="{ 'cs-card-fact__value--mono': mono }">
      <slot>{{ value }}</slot>
    </dd>
  </div>
</template>

<style scoped lang="scss">
.cs-card-fact {
  display: flex;
  min-inline-size: 0;
  flex-direction: column;
  gap: 2px;
}

.cs-card-fact--full {
  grid-column: 1 / -1;
}

.cs-card-fact__label {
  margin: 0;
  font-size: var(--font-size-body-s, 12px);
  line-height: var(--line-height-body-s, 16px);
  font-weight: 500;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.cs-card-fact__value {
  margin: 0;
  overflow-wrap: anywhere;
  font-size: var(--font-size-body-m, 14px);
  line-height: var(--line-height-body-m, 20px);
  font-weight: 400;
  color: var(--color-text-body-default-b2us04, #0f141a);
}

.cs-card-fact__value--mono {
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-variant-numeric: tabular-nums;
}
</style>
