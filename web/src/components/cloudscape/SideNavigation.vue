<script setup lang="ts">
import { computed } from 'vue'
import SideNavigationItem from './SideNavigationItem.vue'
import { CS } from './classes'
import { cx } from './utils'
import {
  hasNavigationIcons,
  sideNavListClasses,
  type SideNavigationChangeDetail,
  type SideNavigationFollowDetail,
  type SideNavigationItem as SideNavigationEntry,
} from './side-navigation-utils'

defineOptions({ name: 'SideNavigation' })

const props = withDefaults(
  defineProps<{
    header?: { text?: string; href: string; logo?: { src?: string; alt?: string } }
    items?: SideNavigationEntry[]
    activeHref?: string
    collapsed?: boolean
  }>(),
  {
    header: undefined,
    items: () => [],
    activeHref: undefined,
    collapsed: false,
  }
)

const emit = defineEmits<{
  (e: 'follow', detail: SideNavigationFollowDetail, event: MouseEvent): void
  (e: 'change', detail: SideNavigationChangeDetail): void
}>()

const cs = CS['side-navigation']

const withIcons = computed(() => hasNavigationIcons(props.items))

function flattenCollapsed(items: SideNavigationEntry[]): SideNavigationEntry[] {
  const out: SideNavigationEntry[] = []
  for (const item of items) {
    if (item.type === 'link' && item.icon) {
      out.push(item)
    } else if (item.type === 'divider') {
      out.push(item)
    } else if ((item.type === 'expandable-link-group' || item.type === 'link-group') && item.icon) {
      out.push(item)
    } else if (item.type === 'section' || item.type === 'section-group') {
      out.push(...flattenCollapsed(item.items))
    }
  }
  return out
}

const visibleItems = computed(() => (props.collapsed ? flattenCollapsed(props.items) : props.items))

const rootListClasses = computed(() =>
  sideNavListClasses('root', {
    collapsed: props.collapsed,
    withIcons: withIcons.value,
    first: true,
  })
)

function onHeaderFollow(event: MouseEvent) {
  if (!props.header) return
  emit('follow', { text: props.header.text, href: props.header.href, type: 'link' }, event)
}
</script>

<template>
  <div :class="cx(cs.root, collapsed && cs['root--collapsed'])">
    <template v-if="header && !collapsed">
      <h2 :class="cs.header">
        <a
          :class="cx(
            cs['header-link'],
            header.logo?.src && cs['header-link--has-logo'],
            header.href === activeHref && cs['header-link-active']
          )"
          :href="header.href"
          :aria-current="header.href === activeHref ? 'page' : undefined"
          @click="onHeaderFollow"
        >
          <img
            v-if="header.logo?.src"
            :class="cx(cs['header-logo'], !header.text && cs['header-logo--stretched'])"
            :src="header.logo.src"
            :alt="header.logo.alt || ''"
          />
          <span v-if="header.text" :class="cs['header-link-text']">{{ header.text }}</span>
        </a>
      </h2>
      <hr :class="cx(cs.divider, cs['divider-header'])" role="presentation" />
    </template>

    <div v-if="!collapsed && $slots['items-control']" :class="cs['items-control']">
      <slot name="items-control" />
    </div>

    <div :class="cs['list-container']">
      <ul v-if="visibleItems.length > 0" :class="rootListClasses">
        <SideNavigationItem
          v-for="(item, index) in visibleItems"
          :key="index"
          :item="item"
          :active-href="activeHref"
          :collapsed="collapsed"
          :with-icons="withIcons"
          variant="root"
          @follow="(detail, event) => emit('follow', detail, event)"
          @change="(detail) => emit('change', detail)"
        />
      </ul>
    </div>
  </div>
</template>
