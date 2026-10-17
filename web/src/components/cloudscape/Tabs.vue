<script setup lang="ts">
import { computed } from 'vue'
import { CS } from './classes'
import { cx } from './utils'

export interface TabItem {
  id: string
  label: string
  disabled?: boolean
}

const props = withDefaults(
  defineProps<{
    activeTabId?: string
    modelValue?: string
    tabs: TabItem[]
    ariaLabel?: string
    disableContentPaddings?: boolean
  }>(),
  { activeTabId: undefined, modelValue: undefined, ariaLabel: undefined, disableContentPaddings: false }
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'change', detail: { activeTabId: string }): void
}>()

const activeId = computed(() => props.activeTabId ?? props.modelValue ?? props.tabs[0]?.id ?? '')
const activeIndex = computed(() => Math.max(0, props.tabs.findIndex((tab) => tab.id === activeId.value)))

function select(tab: TabItem) {
  if (tab.disabled || tab.id === activeId.value) return
  emit('update:modelValue', tab.id)
  emit('change', { activeTabId: tab.id })
}

function onKeydown(event: KeyboardEvent) {
  const enabled = props.tabs.filter((tab) => !tab.disabled)
  if (!enabled.length) return
  if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight' && event.key !== 'Home' && event.key !== 'End') return
  event.preventDefault()
  const current = enabled.findIndex((tab) => tab.id === activeId.value)
  let next = current
  if (event.key === 'ArrowLeft') next = (current - 1 + enabled.length) % enabled.length
  if (event.key === 'ArrowRight') next = (current + 1) % enabled.length
  if (event.key === 'Home') next = 0
  if (event.key === 'End') next = enabled.length - 1
  select(enabled[next])
}
</script>

<template>
  <div :class="CS.tabs.root">
    <div :class="CS.tabs.tabs">
      <div :class="cx(CS.tabs['tabs-header'], CS.tabs.refresh, CS.tabs['tabs-header-with-divider'])">
        <div :class="CS.tabs['tab-header-scroll-container']">
          <ul :class="CS.tabs['tabs-header-list']" role="tablist" :aria-label="ariaLabel" @keydown="onKeydown">
            <li v-for="(tab, index) in tabs" :key="tab.id" :class="CS.tabs['tabs-tab']" role="presentation">
              <div
                :class="
                  cx(
                    CS.tabs['tabs-tab-header-container'],
                    CS.tabs.refresh,
                    tab.id === activeId && CS.tabs['tabs-tab-active'],
                    tab.disabled && CS.tabs['tabs-tab-disabled'],
                    tab.id === activeId && CS.tabs['tabs-tab-focusable']
                  )
                "
              >
                <button
                  type="button"
                  role="tab"
                  :class="
                    cx(
                      CS.tabs['tabs-tab-link'],
                      CS.tabs.refresh,
                      tab.id === activeId && CS.tabs['tabs-tab-active'],
                      tab.disabled && CS.tabs['tabs-tab-disabled'],
                      tab.id === activeId && CS.tabs['tabs-tab-focusable']
                    )
                  "
                  :aria-selected="tab.id === activeId ? 'true' : 'false'"
                  :aria-controls="`tabpanel-${tab.id}`"
                  :tabindex="index === activeIndex ? 0 : -1"
                  :disabled="tab.disabled"
                  @click="select(tab)"
                >
                  <span :class="CS.tabs['tabs-tab-label']"><span>{{ tab.label }}</span></span>
                </button>
              </div>
            </li>
          </ul>
        </div>
        <div v-if="$slots.actions" :class="CS.tabs['actions-container']">
          <slot name="actions" />
        </div>
      </div>
      <div :class="cx(CS.tabs['tabs-content'], CS.tabs['tabs-content-active'])">
        <div
          :class="cx(CS.tabs['tabs-content-wrapper'], !disableContentPaddings && CS.tabs['with-paddings'])"
          :id="`tabpanel-${activeId}`"
          role="tabpanel"
        >
          <slot />
        </div>
      </div>
    </div>
  </div>
</template>
