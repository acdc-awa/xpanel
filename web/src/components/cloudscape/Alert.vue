<script setup lang="ts">
import { computed } from 'vue'
import Icon from './Icon.vue'
import Button from './Button.vue'
import { CS } from './classes'
import { cx } from './utils'

const props = withDefaults(
  defineProps<{
    type?: 'error' | 'warning' | 'success' | 'info'
    header?: string
    dismissible?: boolean
    visible?: boolean
    dismissAriaLabel?: string
    action?: boolean
    actionText?: string
  }>(),
  {
    type: 'info',
    header: undefined,
    dismissible: false,
    visible: true,
    dismissAriaLabel: 'Dismiss',
    action: false,
    actionText: undefined,
  }
)

const emit = defineEmits<{
  (e: 'dismiss'): void
  (e: 'action'): void
}>()

const iconName = computed(() => {
  switch (props.type) {
    case 'error':
      return 'status-negative'
    case 'warning':
      return 'status-warning'
    case 'success':
      return 'status-positive'
    default:
      return 'status-info'
  }
})

const alertClasses = computed(() =>
  cx(CS.alert.alert, CS.alert[`type-${props.type}`], CS.alert['icon-size-normal'], props.dismissible && CS.alert['with-dismiss'])
)
</script>

<template>
  <div v-if="visible" :class="CS.alert.root" role="alert">
    <div :class="alertClasses">
      <div :class="CS.alert['alert-wrapper']">
        <div :class="CS.alert['alert-focus-wrapper']" role="group">
          <div :class="cx(CS.alert.icon, CS.alert.text)">
            <Icon :name="iconName" size="normal" />
          </div>
          <div :class="cx(CS.alert.message, CS.alert.text)">
            <div v-if="header" :class="CS.alert.header">{{ header }}</div>
            <div :class="CS.alert.content"><slot /></div>
          </div>
        </div>
      </div>
      <div v-if="$slots.action || action" :class="CS.alert.action">
        <slot name="action">
          <Button variant="link" @click="emit('action')">{{ actionText }}</Button>
        </slot>
      </div>
      <div v-if="dismissible" :class="CS.alert.dismiss">
        <Button
          variant="icon"
          icon-name="close"
          :aria-label="dismissAriaLabel"
          :class="CS.alert['dismiss-button']"
          @click="emit('dismiss')"
        />
      </div>
    </div>
  </div>
</template>
