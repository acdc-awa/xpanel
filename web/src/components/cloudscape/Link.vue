<script setup lang="ts">
import { computed } from 'vue'
import { CS } from './classes'
import { cx } from './utils'
import Icon from './Icon.vue'

export interface LinkFollowDetail {
  href?: string
  external: boolean
  target?: string
}

type LinkVariant = 'primary' | 'secondary' | 'info'
type LinkFontSize =
  | 'body-s'
  | 'body-m'
  | 'heading-xs'
  | 'heading-s'
  | 'heading-m'
  | 'heading-l'
  | 'heading-xl'
  | 'display-l'
  | 'inherit'

const props = withDefaults(
  defineProps<{
    href?: string
    variant?: LinkVariant
    fontSize?: LinkFontSize
    color?: 'normal' | 'inverted'
    external?: boolean
    target?: string
    rel?: string
    ariaLabel?: string
    externalIconAriaLabel?: string
    disabled?: boolean
  }>(),
  {
    href: undefined,
    variant: 'secondary',
    fontSize: 'body-m',
    color: 'normal',
    external: false,
    target: undefined,
    rel: undefined,
    ariaLabel: undefined,
    externalIconAriaLabel: undefined,
    disabled: false,
  }
)

const emit = defineEmits<{
  (e: 'follow', detail: LinkFollowDetail, event: Event): void
}>()

// 无 href 时官方渲染为 role="button" 的 <a>，并补 Enter/Space 触发。
const isButton = computed(() => !props.href)
const anchorTarget = computed(() => props.target ?? (props.external ? '_blank' : undefined))
const anchorRel = computed(() => props.rel ?? (anchorTarget.value === '_blank' ? 'noopener noreferrer' : undefined))
const applyButtonStyles = computed(() => isButton.value && props.variant !== 'info')

const classes = computed(() =>
  cx(
    CS.link.link,
    CS.link[`variant-${props.variant}`],
    CS.link[`font-size-${props.fontSize}`],
    props.color === 'inverted' && CS.link['color-inverted'],
    applyButtonStyles.value && CS.link.button
  )
)

function onClick(event: MouseEvent) {
  if (props.disabled) {
    event.preventDefault()
    return
  }
  const plainLeft = event.button === 0 && !event.altKey && !event.ctrlKey && !event.metaKey && !event.shiftKey
  if (isButton.value || plainLeft) {
    emit('follow', { href: props.href, external: props.external, target: anchorTarget.value }, event)
  }
}

function onKeydown(event: KeyboardEvent) {
  if (!isButton.value || props.disabled) return
  if (event.key === 'Enter' || event.key === ' ') {
    event.preventDefault()
    emit('follow', { href: props.href, external: props.external, target: anchorTarget.value }, event)
  }
}
</script>

<template>
  <a
    v-if="!disabled"
    :class="classes"
    :href="href"
    :target="isButton ? undefined : anchorTarget"
    :rel="isButton ? undefined : anchorRel"
    :role="isButton ? 'button' : undefined"
    :tabindex="isButton ? 0 : undefined"
    :aria-label="ariaLabel"
    @click="onClick"
    @keydown="onKeydown"
  >
    <span><slot /></span>
    <span v-if="external" :class="CS.link['icon-wrapper']">
      &nbsp;
      <span
        :class="CS.link.icon"
        :role="externalIconAriaLabel ? 'img' : undefined"
        :aria-label="externalIconAriaLabel"
      >
        <Icon name="external" size="small" />
      </span>
    </span>
  </a>
  <span v-else :class="classes" role="link" aria-disabled="true">
    <span><slot /></span>
  </span>
</template>
