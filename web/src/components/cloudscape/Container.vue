<script setup lang="ts">
import { computed, useSlots } from 'vue'
import Header from './Header.vue'
import { CS } from './classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    header?: string
    variant?: 'default' | 'stacked'
    fitHeight?: boolean
    disableContentPaddings?: boolean
    disableHeaderPaddings?: boolean
  }>(),
  {
    header: undefined,
    variant: 'default',
    fitHeight: false,
    disableContentPaddings: false,
    disableHeaderPaddings: false,
  }
)

const slots = useSlots()

const rootClasses = computed(() =>
  cx(
    CS.container.root,
    CS.container.refresh,
    CS.container[`variant-${props.variant}`],
    props.fitHeight && CS.container['fit-height']
  )
)
const contentWrapperClasses = computed(() =>
  cx(CS.container['content-wrapper'], props.fitHeight && CS.container['content-wrapper-fit-height'])
)
const headerClasses = computed(() =>
  cx(
    CS.container.refresh,
    CS.container.header,
    !props.disableHeaderPaddings && CS.container['with-paddings'],
    props.variant === 'stacked' && CS.container['header-variant-embedded']
  )
)
const contentClasses = computed(() => cx(CS.container.content, props.fitHeight && CS.container['content-fit-height']))
const contentInnerClasses = computed(() =>
  cx(
    CS.container['content-inner'],
    !props.disableContentPaddings && CS.container['with-paddings'],
    !props.disableContentPaddings && (Boolean(slots.header) || Boolean(props.header)) && CS.container['with-header']
  )
)
const footerClasses = computed(() => cx(CS.container.footer, CS.container['with-paddings'], CS.container['with-divider']))
</script>

<template>
  <div :class="rootClasses">
    <div :class="contentWrapperClasses">
      <div v-if="slots.header || header" :class="headerClasses">
        <slot name="header">
          <Header variant="h2">{{ header }}</Header>
        </slot>
      </div>
      <div :class="contentClasses">
        <div :class="contentInnerClasses">
          <slot />
        </div>
      </div>
      <div v-if="slots.footer" :class="footerClasses">
        <slot name="footer" />
      </div>
    </div>
  </div>
</template>
