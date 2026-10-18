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
  /**
   * Flashbar 的关闭按钮：透明底 + 反色图标，颜色由外层闪条类型决定。
   * 官方的 flashbar 关闭按钮用的就是这个变体，CSS 早已随 flashbar 一起 vendor。
   */
  | 'flashbar-icon'
  /**
   * AWS 控制台破坏性操作的两态：
   * - danger-outline：红字红描边（对应确认框里的「No, cancel」）
   * - danger：红色实心（对应「Yes, delete」）
   * 官方 Cloudscape 没导出这两个变体，但按钮配色全部走 --awsui-style-* 变量，
   * 这里只覆盖颜色，不新增任何布局/尺寸规则。
   */
  | 'danger'
  | 'danger-outline'

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

const isStandalone = computed(
  () => props.variant === 'icon' || props.variant === 'inline-icon' || props.variant === 'flashbar-icon'
)
const effectiveIconAlign = computed(() => (isStandalone.value ? 'left' : props.iconAlign))
const hasContent = computed(() => Boolean(slots.default))

const classes = computed(() =>
  cx(
    CS.button.button,
    CS.button[`variant-${props.variant}`] ?? CS.button['variant-normal'],
    props.disabled && CS.button.disabled,
    !props.wrapText && CS.button['button-no-wrap'],
    !hasContent.value && CS.button['button-no-text'],
    props.fullWidth && hasContent.value && CS.button['full-width'],
    Boolean(props.href) && CS.button.link,
    props.variant === 'danger' && 'cs-btn-danger',
    props.variant === 'danger-outline' && 'cs-btn-danger-outline'
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

<style>
/*
 * 破坏性操作配色（AWS 控制台 ConfirmDialog 口径）。
 * 官方按钮的颜色/描边全部通过 --awsui-style-* 变量注入，所以只覆盖这些变量，
 * 尺寸、圆角、焦点环、禁用态仍走官方规则。用非 scoped 样式是为了盖过
 * 官方 scoped.css 里 :not(#\9) 的高特异性选择器。
 */
.awsui_button_vjswe_1ig09_70.cs-btn-danger:not(#\9) {
  --awsui-style-background-default-ka4djm: var(--color-error-600-tub94v, #db0000);
  --awsui-style-background-hover-ka4djm: var(--color-error-700-jl0s0z, #c20000);
  --awsui-style-background-active-ka4djm: var(--color-error-800-n1566o, #990000);
  --awsui-style-border-color-default-ka4djm: var(--color-error-600-tub94v, #db0000);
  --awsui-style-border-color-hover-ka4djm: var(--color-error-700-jl0s0z, #c20000);
  --awsui-style-border-color-active-ka4djm: var(--color-error-800-n1566o, #990000);
  --awsui-style-color-default-ka4djm: #fff;
  --awsui-style-color-hover-ka4djm: #fff;
  --awsui-style-color-active-ka4djm: #fff;
}

.awsui_button_vjswe_1ig09_70.cs-btn-danger-outline:not(#\9) {
  --awsui-style-background-default-ka4djm: transparent;
  --awsui-style-background-hover-ka4djm: var(--color-background-status-error-edh6bo, #fff4f4);
  --awsui-style-background-active-ka4djm: var(--color-background-status-error-edh6bo, #fce8e8);
  --awsui-style-border-color-default-ka4djm: var(--color-border-status-error-nczvil, #db0000);
  --awsui-style-border-color-hover-ka4djm: var(--color-error-700-jl0s0z, #c20000);
  --awsui-style-border-color-active-ka4djm: var(--color-error-800-n1566o, #990000);
  --awsui-style-color-default-ka4djm: var(--color-text-status-error-ri38gt, #db0000);
  --awsui-style-color-hover-ka4djm: var(--color-text-status-error-ri38gt, #db0000);
  --awsui-style-color-active-ka4djm: var(--color-text-status-error-ri38gt, #db0000);
}
</style>
