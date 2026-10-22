<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, useSlots } from 'vue'
import TopNavigationUtility from './TopNavigationUtility.vue'
import { CS } from './classes'
import { cx } from './utils'
import type { TopNavigationIdentity, TopNavigationUtility as TopNavigationUtilityItem } from './top-navigation-utils'

defineOptions({ name: 'TopNavigation' })

const props = withDefaults(
  defineProps<{
    identity?: TopNavigationIdentity
    utilities?: TopNavigationUtilityItem[]
    visualContext?: 'top-navigation' | 'none'
    searchMaxWidth?: string
  }>(),
  {
    identity: undefined,
    utilities: () => [],
    visualContext: 'none',
    searchMaxWidth: undefined,
  }
)

const emit = defineEmits<{
  (e: 'identityFollow', detail: { href: string }, event: MouseEvent): void
}>()

const slots = useSlots()
const cs = CS['top-navigation']

const rootRef = ref<HTMLElement | null>(null)
const sizeMode = ref<'wide' | 'medium' | 'narrow'>('wide')
let resizeObserver: ResizeObserver | null = null

function updateSizeMode() {
  const width = rootRef.value?.getBoundingClientRect().width ?? 0
  if (width === 0) return
  sizeMode.value = width < 560 ? 'narrow' : width < 900 ? 'medium' : 'wide'
}

onMounted(() => {
  const el = rootRef.value
  if (!el) return
  updateSizeMode()
  resizeObserver = new ResizeObserver(updateSizeMode)
  resizeObserver.observe(el)
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  resizeObserver = null
})

const rootClasses = computed(() =>
  cx(
    cs['top-navigation'],
    sizeMode.value === 'medium' && cs.medium,
    sizeMode.value === 'narrow' && cs.narrow
  )
)

const visibleUtilities = computed(() =>
  props.utilities.filter((item) => !(item.collapseAt === 'narrow' && sizeMode.value === 'narrow'))
)

const hideText = computed(() => sizeMode.value !== 'wide')

const hasExtraSlot = computed(() => Boolean(slots['utility-extra']))

function offsetRightFor(index: number): 'none' | 'l' | 'xxl' | undefined {
  if (index !== visibleUtilities.value.length - 1) return undefined
  if (sizeMode.value !== 'wide') return 'l'
  return hasExtraSlot.value ? 'l' : 'xxl'
}

function onIdentityClick(event: MouseEvent) {
  if (!props.identity) return
  if (event.button !== 0 || event.ctrlKey || event.metaKey || event.shiftKey || event.altKey) return
  event.preventDefault()
  emit('identityFollow', { href: props.identity.href }, event)
}
</script>

<template>
  <div
    ref="rootRef"
    :class="visualContext === 'top-navigation' ? 'awsui-context-top-navigation' : undefined"
  >
    <header :class="rootClasses">
      <div :class="cs['padding-box']">
        <div
          v-if="$slots.left || identity"
          :class="cx(cs.identity, !identity?.logo && cs['no-logo'])"
        >
          <slot name="left">
            <a
              v-if="identity"
              :class="cs['identity-link']"
              :href="identity.href"
              @click="onIdentityClick"
            >
              <img
                v-if="identity.logo"
                :class="cx(cs.logo, sizeMode === 'narrow' && cs.narrow)"
                :src="identity.logo.src"
                :alt="identity.logo.alt || ''"
              />
              <span v-if="identity.title" :class="cs.title">{{ identity.title }}</span>
            </a>
          </slot>
        </div>

        <div
          v-if="$slots.search"
          :class="cs.inputs"
          :style="searchMaxWidth ? { maxInlineSize: searchMaxWidth } : undefined"
        >
          <div :class="cs.search">
            <slot name="search" />
          </div>
        </div>

        <div :class="cs.utilities">
          <TopNavigationUtility
            v-for="(item, index) in visibleUtilities"
            :key="index"
            :item="item"
            :hide-text="hideText"
            :offset-right="offsetRightFor(index)"
          />
          <span v-if="hasExtraSlot" class="cs-topnav-extra">
            <slot name="utility-extra" />
          </span>
        </div>
      </div>
    </header>
  </div>
</template>

<style scoped lang="scss">
.cs-topnav-extra {
  display: flex;
  align-items: center;
  padding-inline: var(--space-m-n8iq8o, 16px);
}
</style>
