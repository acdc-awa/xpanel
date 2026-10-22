<script setup lang="ts">
import { computed } from 'vue'
import Button from './Button.vue'
import Icon from './Icon.vue'
import { CS } from './classes'
import { cx } from './utils'
import type { TopNavigationUtility } from './top-navigation-utils'

const props = withDefaults(
  defineProps<{
    item: TopNavigationUtility
    hideText?: boolean
    offsetRight?: 'none' | 'l' | 'xxl'
  }>(),
  {
    hideText: false,
    offsetRight: undefined,
  }
)

const cs = CS['top-navigation']

const isPrimary = computed(() => props.item.variant === 'primary-button')
const showText = computed(() => !props.hideText || props.item.disableTextCollapse)
const ariaLabel = computed(() => props.item.ariaLabel ?? props.item.text ?? props.item.title)

const wrapperClasses = computed(() =>
  cx(
    cs['utility-wrapper'],
    isPrimary.value ? cs['utility-type-button-primary-button'] : cs['utility-type-button-link'],
    props.offsetRight && cs[`offset-right-${props.offsetRight}`]
  )
)

const linkClasses = computed(() => cx(CS.link.link, CS.link['variant-top-navigation'], 'cs-topnav-utility-link'))

function onClick() {
  props.item.onClick?.()
}
</script>

<template>
  <span :class="wrapperClasses">
    <Button v-if="isPrimary" variant="primary" :icon-name="item.iconName" @click="onClick">
      <span v-if="showText && item.text">{{ item.text }}</span>
    </Button>
    <button
      v-else
      type="button"
      :class="linkClasses"
      :aria-label="ariaLabel"
      :title="item.title"
      @click="onClick"
    >
      <Icon v-if="item.iconName" :name="item.iconName" />
      <span v-if="showText && item.text" :class="cs['utility-link-icon']">{{ item.text }}</span>
    </button>
  </span>
</template>

<style scoped lang="scss">
.cs-topnav-utility-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border: 0;
  background: transparent;
  padding: 0;
  margin: 0;
  font: inherit;
  line-height: inherit;
  cursor: pointer;
}

.cs-topnav-utility-link:focus-visible {
  outline: 2px solid var(--color-border-item-focused-sewkzn, #006ce0);
  outline-offset: 2px;
  border-radius: var(--border-radius-input-k1q34z, 8px);
}
</style>
