<script setup lang="ts">
import { computed, useSlots } from 'vue'
import { CS } from './classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    variant?: 'h1' | 'h2' | 'h3'
    description?: string
    counter?: string | number
    headingTagOverride?: string
  }>(),
  {
    variant: 'h2',
    description: undefined,
    counter: undefined,
    headingTagOverride: undefined,
  }
)

const slots = useSlots()
const hasActions = computed(() => Boolean(slots.actions))

const rootClasses = computed(() =>
  cx(
    CS.header.root,
    CS.header.refresh,
    CS.header[`root-variant-${props.variant}`],
    !hasActions.value && CS.header['root-no-actions'],
    Boolean(props.description) && CS.header['root-has-description']
  )
)
const mainClasses = computed(() => cx(CS.header.main, CS.header.refresh, CS.header[`main-variant-${props.variant}`]))
const titleClasses = computed(() => cx(CS.header.title, CS.header.refresh, CS.header[`title-variant-${props.variant}`]))
const headingClasses = computed(() =>
  cx(CS.header.heading, CS.header.refresh, CS.header[`heading-variant-${props.variant}`])
)
const headingTextClasses = computed(() =>
  cx(CS.header['heading-text'], CS.header[`heading-text-variant-${props.variant}`])
)
const actionsClasses = computed(() =>
  cx(
    CS.header.actions,
    CS.header.refresh,
    CS.header[`actions-variant-${props.variant}`],
    CS.header['actions-centered']
  )
)
const descriptionClasses = computed(() =>
  cx(CS.header.description, CS.header.refresh, CS.header[`description-variant-${props.variant}`])
)
</script>

<template>
  <div :class="rootClasses">
    <div :class="mainClasses">
      <div :class="titleClasses">
        <component :is="headingTagOverride ?? variant" :class="headingClasses">
          <span :class="headingTextClasses"><slot /></span>
          <span v-if="counter !== undefined" :class="CS.header.counter">&nbsp;{{ counter }}</span>
          <template v-if="$slots.info">
            <span :class="CS.header['virtual-space']"> &nbsp;</span>
            <span :class="CS.header.info"><slot name="info" /></span>
          </template>
        </component>
      </div>
      <div v-if="hasActions" :class="actionsClasses">
        <slot name="actions" />
      </div>
    </div>
    <p v-if="description || $slots.description" :class="descriptionClasses">
      <slot name="description">{{ description }}</slot>
    </p>
  </div>
</template>
