<script setup lang="ts">
import TipIcon from './TipIcon.vue'

defineProps<{
  title?: string
  /** 标题旁的说明文案，悬停展示（替代常驻小字） */
  titleTip?: string
  titleTipType?: 'info' | 'warn'
  /** 紧凑模式，用于高密度信息展示 */
  compact?: boolean
}>()
</script>

<template>
  <section class="base-card" :class="{ 'is-compact': compact }">
    <header v-if="title || $slots.extra" class="base-card__head">
      <h3 class="base-card__title">
        {{ title }}
        <TipIcon v-if="titleTip" :content="titleTip" :type="titleTipType ?? 'info'" />
      </h3>
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
  box-shadow: var(--x-shadow-xs, var(--x-shadow));
  margin-bottom: 16px;
  transition: background-color 0.2s ease, border-color 0.2s ease, box-shadow 0.2s ease;

  &__head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 18px;
    border-bottom: 1px solid var(--x-border);
    min-height: 48px;

    @media (max-width: $bp-tablet-max) {
      padding: 10px 14px;
      min-height: 42px;
    }
  }

  &__title {
    font-size: var(--x-font-lg, 15px);
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
    padding: 16px 18px;

    @media (max-width: $bp-tablet-max) {
      padding: 12px 14px;
    }
  }

  &.is-compact {
    margin-bottom: 12px;

    .base-card__head {
      padding: 10px 14px;
      min-height: 40px;
    }

    .base-card__body {
      padding: 12px 14px;
    }
  }
}
</style>
