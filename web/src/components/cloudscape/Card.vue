<script setup lang="ts">
import { computed, useSlots } from 'vue'
import { CS } from './classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    header?: string
    variant?: 'default' | 'embedded'
    fullHeight?: boolean
  }>(),
  {
    header: undefined,
    variant: 'default',
    fullHeight: true,
  }
)

const slots = useSlots()

const rootClasses = computed(() =>
  cx(
    CS['item-card'].root,
    CS['item-card'].refresh,
    CS['item-card'][`variant-${props.variant}`],
    props.fullHeight && CS['item-card']['full-height'],
    'cs-card'
  )
)
</script>

<template>
  <article :class="rootClasses">
    <div :class="CS['item-card']['inner-card']">
      <header
        v-if="header || slots.header || slots.actions"
        :class="[
          CS['item-card'].header,
          slots.actions && CS['item-card']['with-actions'],
          'cs-card__header',
        ]"
      >
        <div :class="[CS['item-card']['header-inner'], 'cs-card__title']">
          <slot name="header">{{ header }}</slot>
        </div>
        <div v-if="slots.actions" class="cs-card__actions">
          <slot name="actions" />
        </div>
      </header>
      <div :class="[CS['item-card'].body, 'cs-card__body']">
        <slot />
      </div>
      <footer v-if="slots.footer" :class="[CS['item-card'].footer, 'cs-card__footer']">
        <slot name="footer" />
      </footer>
    </div>
  </article>
</template>

<style scoped lang="scss">
.cs-card {
  min-inline-size: 0;
  transition: box-shadow 0.18s ease, border-color 0.18s ease;
}

.cs-card:hover {
  box-shadow: var(--shadow-item-card-hover, 0 2px 8px rgba(0, 0, 0, 0.08));
}

.cs-card__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
}

.cs-card__title {
  min-inline-size: 0;
  overflow-wrap: anywhere;
}

.cs-card__actions {
  display: flex;
  flex: none;
  align-items: center;
  gap: 8px;
}

.cs-card__body {
  min-inline-size: 0;
}

.cs-card__footer {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;

  :deep(button) {
    flex: 1 1 0;
    margin: 0;
    padding-inline: 6px !important;
    font-size: 12px !important;
    white-space: nowrap !important;
    text-align: center !important;
  }
}
</style>
