<script setup lang="ts">
import { computed } from 'vue'
import { CS } from './classes'
import { cx } from './utils'

type Spacing = 'xxxs' | 'xxs' | 'xs' | 's' | 'm' | 'l' | 'xl' | 'xxl' | 'xxxl' | 'n'
type BoxVariant =
  | 'div'
  | 'span'
  | 'p'
  | 'small'
  | 'b'
  | 'strong'
  | 'code'
  | 'pre'
  | 'samp'
  | 'h1'
  | 'h2'
  | 'h3'
  | 'h4'
  | 'h5'
  | 'awsui-key-label'
  | 'awsui-value-large'
  | 'awsui-gen-ai-label'
  | 'awsui-inline-code'

const props = withDefaults(
  defineProps<{
    variant?: BoxVariant
    tag?: string
    margin?: Spacing | Partial<Record<'top' | 'right' | 'bottom' | 'left' | 'horizontal' | 'vertical', Spacing>>
    padding?: Spacing | Partial<Record<'top' | 'right' | 'bottom' | 'left' | 'horizontal' | 'vertical', Spacing>>
    display?: 'block' | 'inline' | 'inline-block' | 'none'
    textAlign?: 'left' | 'right' | 'center'
    float?: 'left' | 'right'
    fontSize?: 'body-s' | 'body-m' | 'heading-xs' | 'heading-s' | 'heading-m' | 'heading-l' | 'heading-xl' | 'display-l' | 'display-xl'
    fontWeight?: 'light' | 'normal' | 'bold' | 'heavy'
    color?:
      | 'default'
      | 'inverted'
      | 'text-label'
      | 'text-body-secondary'
      | 'text-status-error'
      | 'text-status-success'
      | 'text-status-info'
      | 'text-status-inactive'
      | 'text-status-warning'
      | 'inherit'
  }>(),
  {
    variant: 'div',
    tag: undefined,
    margin: undefined,
    padding: undefined,
    display: undefined,
    textAlign: undefined,
    float: undefined,
    fontSize: undefined,
    fontWeight: undefined,
    color: undefined,
  }
)

const sides = ['top', 'right', 'bottom', 'left', 'horizontal', 'vertical'] as const

function spacingSuffixes(value: typeof props.margin): string[] {
  if (!value) return []
  if (typeof value === 'string') return [value]
  return sides.filter((side) => Boolean(value[side])).map((side) => `${side}-${value[side]}`)
}

const tagName = computed(() => {
  if (props.tag) return props.tag
  if (props.variant === 'awsui-value-large') return 'span'
  if (props.variant === 'awsui-key-label' || props.variant === 'awsui-gen-ai-label') return 'div'
  if (props.variant === 'awsui-inline-code') return 'code'
  return props.variant
})

const variantClass = computed(() => props.variant.replace(/^awsui-/, ''))

const classes = computed(() =>
  cx(
    CS.box.root,
    CS.box.box,
    CS.box[`${variantClass.value}-variant`],
    spacingSuffixes(props.margin).map((s) => CS.box[`m-${s}`]),
    spacingSuffixes(props.padding).map((s) => CS.box[`p-${s}`]),
    props.display && CS.box[`d-${props.display}`],
    props.float && CS.box[`f-${props.float}`],
    CS.box[`color-${props.color ?? 'default'}`],
    CS.box[`font-size-${props.fontSize ?? 'default'}`],
    CS.box[`font-weight-${props.fontWeight ?? 'default'}`],
    props.textAlign && CS.box[`t-${props.textAlign}`]
  )
)
</script>

<template>
  <component :is="tagName" :class="classes"><slot /></component>
</template>
