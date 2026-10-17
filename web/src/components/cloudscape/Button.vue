<script setup lang="ts">
import { computed, useSlots } from 'vue'
import Icon from './Icon.vue'
import Spinner from './Spinner.vue'
import { CS } from './classes'
import { cx } from './utils'

type ButtonVariant =
  | 'normal'
  | 'primary'
  | 'link'
  | 'icon'
  | 'inline-icon'
  | 'inline-link'
  | 'modal-dismiss'
  | 'menu-trigger'

const props = withDefaults(
  defineProps<{
    variant?: ButtonVariant
    disabled?: boolean
    loading?: boolean
    iconName?: string
    iconAlign?: 'left' | 'right'
    iconSize?: 'x-small' | 'small' | 'normal' | 'medium' | 'big' | 'large'
    fullWidth?: boolean
    wrapText?: boolean
    formAction?: 'none' | 'submit' | 'reset'
    href?: string
    target?: string
    rel?: string
    ariaLabel?: string
    ariaExpanded?: boolean
    ariaHaspopup?: boolean
    ariaControls?: string
    loadingText?: string
  }>(),
  {
    variant: 'normal',
    disabled: false,
    loading: false,
    iconAlign: 'left',
    iconSize: 'normal',
    fullWidth: false,
    wrapText: true,
    formAction: 'none',
    href: undefined,
    target: undefined,
    rel: undefined,
    ariaLabel: undefined,
    ariaExpanded: undefined,
    ariaHaspopup: undefined,
    ariaControls: undefined,
    loadingText: undefined,
  }
)

const emit = defineEmits<{
  (e: 'click', event: MouseEvent): void
}>()

const slots = useSlots()

const isStandalone = computed(() => props.variant === 'icon' || props.variant === 'inline-icon')
const effectiveIconAlign = computed(() => (isStandalone.value ? 'left' : props.iconAlign))
const hasContent = computed(() => Boolean(slots.default))

const classes = computed(() =>
  cx(
    CS.button.button,
    CS.button[`variant-${props.variant}`],
    props.disabled && CS.button.disabled,
    !props.wrapText && CS.button['button-no-wrap'],
    !hasContent.value && CS.button['button-no-text'],
    props.fullWidth && hasContent.value && CS.button['full-width'],
    Boolean(props.href) && CS.button.link
  )
)

const buttonType = computed(() => (props.formAction === 'none' ? 'button' : props.formAction))

function onClick(event: MouseEvent) {
  if (props.disabled || props.loading) {
    event.preventDefault()
    return
  }
  emit('click', event)
}
</script>

<template>
  <component
    :is="href ? 'a' : 'button'"
    :class="classes"
    :type="href ? undefined : buttonType"
    :href="href && !disabled ? href : undefined"
    :target="target"
    :rel="rel ?? (target === '_blank' ? 'noopener noreferrer' : undefined)"
    :disabled="href ? undefined : disabled"
    :aria-disabled="href && disabled ? true : undefined"
    :aria-label="ariaLabel"
    :aria-expanded="ariaExpanded"
    :aria-haspopup="ariaHaspopup"
    :aria-controls="ariaControls"
    :title="ariaLabel"
    @click="onClick"
  >
    <span
      v-if="$slots.icon && effectiveIconAlign === 'left'"
      :class="cx(CS.button.icon, CS.button['icon-left'])"
    >
      <slot name="icon" />
    </span>
    <Spinner
      v-else-if="loading && effectiveIconAlign === 'left'"
      size="normal"
      :class="cx(CS.button.icon, CS.button['icon-left'])"
    />
    <Icon
      v-else-if="iconName && effectiveIconAlign === 'left'"
      :name="iconName"
      :size="iconSize"
      :class="cx(CS.button.icon, CS.button['icon-left'])"
    />

    <span v-if="hasContent" :class="CS.button.content">
      <slot />
    </span>

    <span
      v-if="$slots.icon && effectiveIconAlign === 'right'"
      :class="cx(CS.button.icon, CS.button['icon-right'])"
    >
      <slot name="icon" />
    </span>
    <Icon
      v-else-if="iconName && effectiveIconAlign === 'right' && !loading"
      :name="iconName"
      :size="iconSize"
      :class="cx(CS.button.icon, CS.button['icon-right'])"
    />
  </component>
</template>
