<script setup lang="ts">
/**
 * 无边框图标按钮：输入框尾部（清除 / 显隐密码）、下拉清除等场景共用。
 *
 * 官方 `button` 的 `variant-icon` 依赖 React 内联注入的 `--awsui-style-*-ka4djm`
 * 变量，纯 CSS 环境下它们没有定义；这里按该实际观感复刻，页面里不要再直接写
 * 裸 <button> 手搓一遍——裸按钮会拿到浏览器默认的 2px outset 灰底方框。
 */
import { computed } from 'vue'
import Icon from './Icon.vue'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    /** 官方图标名；需要自定义图形时用默认插槽替代 */
    iconName?: string
    /** 必填：图标按钮没有可见文本，必须给无障碍名称 */
    label: string
    /** 仅用于开合类按钮（如密码显隐） */
    pressed?: boolean
    disabled?: boolean
    /** small=20px（嵌在输入框内），normal=24px */
    size?: 'small' | 'normal'
  }>(),
  {
    iconName: undefined,
    pressed: undefined,
    disabled: false,
    size: 'small',
  }
)

const emit = defineEmits<{
  (e: 'click', event: MouseEvent): void
}>()

const classes = computed(() =>
  cx('cs-icon-button', `cs-icon-button-${props.size}`, props.disabled && 'cs-icon-button-disabled')
)
</script>

<template>
  <button
    type="button"
    :class="classes"
    :disabled="disabled"
    :aria-label="label"
    :aria-pressed="pressed === undefined ? undefined : pressed ? 'true' : 'false'"
    :title="label"
    @click="emit('click', $event)"
  >
    <slot><Icon v-if="iconName" :name="iconName" size="x-small" /></slot>
  </button>
</template>

<style scoped>
.cs-icon-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 0;
  border: 0;
  border-radius: 4px;
  background: transparent;
  color: var(--color-text-interactive-default-pauydh, #424650);
  cursor: pointer;
  appearance: none;
  line-height: 0;
  transition: color 0.15s ease;
}

.cs-icon-button-small {
  inline-size: 20px;
  block-size: 20px;
}

.cs-icon-button-normal {
  inline-size: 24px;
  block-size: 24px;
}

.cs-icon-button:hover:not(.cs-icon-button-disabled) {
  color: var(--color-text-interactive-hover-zj1911, #0f141a);
}

.cs-icon-button:focus-visible {
  outline: 2px solid var(--color-border-item-focused-sewkzn, #006ce0);
  outline-offset: 1px;
}

.cs-icon-button-disabled {
  color: var(--color-text-interactive-disabled-tuxgxq, #8c8c94);
  cursor: auto;
}
</style>
