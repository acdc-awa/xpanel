<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import Icon from './Icon.vue'
import ExpandableSection from './ExpandableSection.vue'
import { CS } from './classes'
import { cx } from './utils'
import {
  hasActiveLink,
  isPlainLeftClick,
  sideNavLinkClasses,
  sideNavListClasses,
  type SideNavigationChangeDetail,
  type SideNavigationFollowDetail,
  type SideNavigationItem,
} from './side-navigation-utils'

defineOptions({ name: 'SideNavigationItem' })

const props = withDefaults(
  defineProps<{
    item: SideNavigationItem
    activeHref?: string
    collapsed?: boolean
    variant?: 'root' | 'section' | 'section-group' | 'link-group' | 'expandable-link-group'
    withIcons?: boolean
  }>(),
  {
    activeHref: undefined,
    collapsed: false,
    variant: 'root',
    withIcons: false,
  }
)

const emit = defineEmits<{
  (e: 'follow', detail: SideNavigationFollowDetail, event: MouseEvent): void
  (e: 'change', detail: SideNavigationChangeDetail): void
}>()

const cs = CS['side-navigation']

const innerExpanded = ref(false)

const isControlled = computed(() => {
  const item = props.item
  return (item.type === 'section' || item.type === 'expandable-link-group') && item.expanded !== undefined
})

const isExpanded = computed(() => {
  const item = props.item
  if (item.type === 'section') return item.expanded ?? innerExpanded.value
  if (item.type === 'expandable-link-group') return item.expanded ?? innerExpanded.value
  return false
})

function resetExpanded() {
  const item = props.item
  if (item.type === 'section') {
    innerExpanded.value = item.defaultExpanded ?? true
  } else if (item.type === 'expandable-link-group') {
    innerExpanded.value =
      item.defaultExpanded ?? (item.href === props.activeHref || hasActiveLink(item.items, props.activeHref))
  }
}

resetExpanded()

watch(
  () => [props.item, props.activeHref],
  () => {
    if (!isControlled.value) resetExpanded()
  },
  { deep: true }
)

const isActiveLink = computed(() => props.item.type === 'link' && props.item.href === props.activeHref)
const containsActiveLink = computed(() => {
  if (props.item.type === 'link') return false
  if (props.item.type === 'divider') return false
  return hasActiveLink(props.item.items, props.activeHref)
})

const showCollapsedLink = computed(() => !props.collapsed || (props.item.type === 'link' && Boolean(props.item.icon)))
const showCollapsedGroup = computed(
  () => !props.collapsed || (props.item.type === 'expandable-link-group' && Boolean(props.item.icon))
)

const listItemClasses = computed(() => cx(cs['list-item'], props.collapsed && cs['list-item--collapsed']))

function linkClasses(item: { href: string; icon?: string }) {
  return sideNavLinkClasses(item, props.activeHref, props.collapsed)
}

function onLinkClick(event: MouseEvent) {
  if (props.item.type !== 'link' || !isPlainLeftClick(event)) return
  emit(
    'follow',
    { text: props.item.text, href: props.item.href, type: 'link', info: props.item.info },
    event
  )
}

function onGroupFollow(event: MouseEvent) {
  if (props.item.type !== 'link-group' || !isPlainLeftClick(event)) return
  emit('follow', { text: props.item.text, href: props.item.href, type: 'link-group', info: props.item.info }, event)
}

function toggle() {
  if (props.item.type !== 'section' && props.item.type !== 'expandable-link-group') return
  const next = !isExpanded.value
  if (!isControlled.value) innerExpanded.value = next
  emit('change', { item: props.item, expanded: next, expandableParents: [] })
}
</script>

<template>
  <template v-if="item.type === 'link'">
    <li v-if="showCollapsedLink" :class="[listItemClasses, item.info && cs['list-item--link-with-info']]">
      <a
        :class="linkClasses(item)"
        :href="item.href"
        :aria-current="isActiveLink ? 'page' : undefined"
        :title="collapsed ? item.text : undefined"
        @click="onLinkClick"
      >
        <span v-if="item.icon" :class="cx(cs['item-icon'], collapsed && cs['item-icon--collapsed'])" aria-hidden="true">
          <Icon :name="item.icon" />
        </span>
        <span :class="cx(cs['link-text-wrapper'], collapsed && cs['link-text-wrapper--collapsed'])">
          <span
            :class="
              cx(
                cs['link-text-wrapper-content'],
                variant === 'root' && 'cs-nav-root-link-text',
                variant === 'root' && !isActiveLink && 'cs-nav-root-link-text--idle'
              )
            "
          >
            {{ item.text }}
            <span v-if="item.external" :class="cs['external-icon']" aria-hidden="true">
              <Icon name="external" />
            </span>
          </span>
        </span>
      </a>
      <span v-if="item.info" :class="cs.info">{{ item.info }}</span>
    </li>
  </template>

  <template v-else-if="item.type === 'divider'">
    <li :class="listItemClasses">
      <hr :class="cx(cs.divider, cs['divider-default'], collapsed && cs['divider--collapsed'])" />
    </li>
  </template>

  <template v-else-if="item.type === 'section'">
    <!-- 折叠轨里 section 表头隐藏，只把子项提升成图标列表（官方 collapsed 语义）。 -->
    <li v-if="collapsed" :class="[listItemClasses, cs['list-item--group']]">
      <ul :class="sideNavListClasses('section', { collapsed: true, withIcons })">
        <SideNavigationItem
          v-for="(child, index) in item.items"
          :key="index"
          :item="child"
          :active-href="activeHref"
          :collapsed="true"
          :with-icons="withIcons"
          variant="section"
          @follow="(detail, event) => emit('follow', detail, event)"
          @change="(detail) => emit('change', detail)"
        />
      </ul>
    </li>
    <li v-else :class="[listItemClasses, cs['list-item--group']]">
      <ExpandableSection
        :class="cx(cs.section, cs.refresh)"
        variant="footer"
        disable-content-paddings
        :expanded="isExpanded"
        expand-icon-position="start"
        @change="toggle"
      >
        <template #header>
          <span :class="cx(cs['section-header'], cs['label-text'])">{{ item.text }}</span>
        </template>
        <ul :class="sideNavListClasses('section', { withIcons })">
          <SideNavigationItem
            v-for="(child, index) in item.items"
            :key="index"
            :item="child"
            :active-href="activeHref"
            :collapsed="collapsed"
            :with-icons="withIcons"
            variant="section"
            @follow="(detail, event) => emit('follow', detail, event)"
            @change="(detail) => emit('change', detail)"
          />
        </ul>
      </ExpandableSection>
    </li>
  </template>

  <template v-else-if="item.type === 'section-group'">
    <li :class="[listItemClasses, cs['list-item--group']]">
      <div :class="cs['section-group']">
        <div
          v-if="!collapsed"
          :class="cx(cs['section-group-title'], cs['label-text'])"
        >
          {{ item.title }}
        </div>
        <ul :class="sideNavListClasses('section-group', { collapsed, withIcons })">
          <SideNavigationItem
            v-for="(child, index) in item.items"
            :key="index"
            :item="child"
            :active-href="activeHref"
            :collapsed="collapsed"
            :with-icons="withIcons"
            variant="section-group"
            @follow="(detail, event) => emit('follow', detail, event)"
            @change="(detail) => emit('change', detail)"
          />
        </ul>
      </div>
    </li>
  </template>

  <template v-else-if="item.type === 'link-group'">
    <li :class="listItemClasses">
      <a
        :class="linkClasses(item)"
        :href="item.href"
        :aria-current="item.href === activeHref ? 'page' : undefined"
        :title="collapsed ? item.text : undefined"
        @click="onGroupFollow"
      >
        <span v-if="item.icon" :class="cx(cs['item-icon'], collapsed && cs['item-icon--collapsed'])" aria-hidden="true">
          <Icon :name="item.icon" />
        </span>
        <span :class="cx(cs['link-text-wrapper'], collapsed && cs['link-text-wrapper--collapsed'])">
          <span :class="cs['link-text-wrapper-content']">{{ item.text }}</span>
        </span>
      </a>
      <div v-if="!collapsed" :class="cs['link-group-children']">
        <div
          :class="
            cx(
              cs['link-group-children-inner'],
              cs['link-group-children-inner--expanded'],
              cs['link-group-children-inner--settled']
            )
          "
        >
          <ul :class="sideNavListClasses('link-group', { withIcons })">
            <SideNavigationItem
              v-for="(child, index) in item.items"
              :key="index"
              :item="child"
              :active-href="activeHref"
              :collapsed="collapsed"
              :with-icons="withIcons"
              variant="link-group"
              @follow="(detail, event) => emit('follow', detail, event)"
              @change="(detail) => emit('change', detail)"
            />
          </ul>
        </div>
      </div>
    </li>
  </template>

  <template v-else-if="item.type === 'expandable-link-group'">
    <li v-if="showCollapsedGroup" :class="[listItemClasses, cs['list-item--group']]">
      <div
        :class="
          cx(
            cs['expandable-link-group'],
            collapsed && cs['expandable-link-group--collapsed'],
            containsActiveLink && cs['expandable-link-group--active'],
            withIcons && cs['expandable-link-group--expand-icon-end']
          )
        "
      >
        <div class="cs-sidenav-group-header">
          <button
            type="button"
            :class="cx(cs.link, collapsed && cs['link--collapsed'], 'cs-sidenav-group-trigger')"
            :aria-expanded="isExpanded"
            :title="collapsed ? item.text : undefined"
            @click="toggle"
          >
            <span v-if="item.icon" :class="cx(cs['item-icon'], collapsed && cs['item-icon--collapsed'])" aria-hidden="true">
              <Icon :name="item.icon" />
            </span>
            <span :class="cx(cs['link-text-wrapper'], collapsed && cs['link-text-wrapper--collapsed'])">
              <span :class="cs['link-text-wrapper-content']">{{ item.text }}</span>
            </span>
            <span v-if="!collapsed" class="cs-sidenav-chevron" :class="{ 'is-expanded': isExpanded }">
              <Icon name="angle-down" />
            </span>
          </button>
        </div>
        <ul
          v-if="isExpanded && !collapsed"
          :class="sideNavListClasses('expandable-link-group', { withIcons })"
        >
          <SideNavigationItem
            v-for="(child, index) in item.items"
            :key="index"
            :item="child"
            :active-href="activeHref"
            :collapsed="collapsed"
            :with-icons="withIcons"
            variant="expandable-link-group"
            @follow="(detail, event) => emit('follow', detail, event)"
            @change="(detail) => emit('change', detail)"
          />
        </ul>
      </div>
    </li>
  </template>
</template>

<style scoped lang="scss">
.cs-sidenav-group-header {
  display: flex;
  align-items: center;
  inline-size: 100%;
  min-inline-size: 0;
  border: 0;
  background: transparent;
  padding: 0;
  font: inherit;
  text-align: start;
  cursor: pointer;
  color: inherit;
}

.cs-sidenav-group-trigger {
  flex: 1 1 auto;
}

.cs-sidenav-chevron {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  margin-inline-start: auto;
  padding-inline-start: 4px;
  color: var(--color-text-body-secondary-66ygbb, #424650);
  transition: transform 0.15s ease;
}

.cs-sidenav-chevron.is-expanded {
  transform: rotate(180deg);
}

.cs-sidenav-group-header:focus-within {
  outline: 2px solid var(--color-border-item-focused-sewkzn, #006ce0);
  outline-offset: -2px;
  border-radius: var(--border-radius-side-navigation-item-umkj1e, 8px);
}

/* 根级单项（如「仪表盘」）与分组标题同级：用官方 expandable heading 的字号/字重，
   避免单项看起来比分类标题小一号。 */
.cs-nav-root-link-text {
  font-size: var(--font-size-expandable-heading-04ofyn, 16px);
  font-weight: var(--font-weight-heading-s-mu75ca, 700);
  letter-spacing: var(--letter-spacing-heading-s-kfu48f, -0.005em);
}

/* 侧栏链接默认色是 #424650（次要灰），分组标题是 heading 黑；根级单项要跟标题同色，
   但激活态仍走链接蓝，所以只在非激活时覆盖。 */
.cs-nav-root-link-text--idle {
  color: var(--color-text-heading-default-e3tuze, #0f141a);
}
</style>
