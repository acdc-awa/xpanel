<script setup lang="ts">
/**
 * Vue 移植的 Cloudscape AppLayout（visual-refresh）。
 *
 * 官方实现是单个 CSS grid：`main.layout` 平铺 navigation / breadcrumbs / header / main，
 * 各区域靠 `grid-area` 落位，尺寸全部来自 `--awsui-*` 变量，不依赖 JS 测量。
 * 本项目只渲染 navigation + breadcrumbs + main，不引入 drawers / tools / split-panel
 * （对应 CSS 也没有 import）。
 *
 * 导航只有两态：展开（第一列固定宽度）与完全收起（`display:none` + 左侧 36px 圆形按钮）。
 * 这是官方语义，不是「折叠成图标轨」。
 */
import { computed, useSlots } from 'vue'
import Button from './Button.vue'
import Icon from './Icon.vue'
import { CS } from './classes'
import { cx } from './utils'

defineOptions({ name: 'AppLayout' })

const props = withDefaults(
  defineProps<{
    /** 导航是否展开（受控） */
    navigationOpen?: boolean
    /** 导航列宽度 */
    navigationWidth?: number
    /** 全局顶栏高度：AppLayout 位于其下方，sticky 与内容高度都以此为偏移 */
    headerHeight?: number
    /** 不渲染导航（移动端走 Drawer 时传 true） */
    navigationHide?: boolean
    contentType?: 'default' | 'dashboard' | 'table' | 'cards'
    maxContentWidth?: number
    disableContentPaddings?: boolean
    /**
     * 导航开合按钮由谁提供：
     * - `panel`（默认）：官方语义，收起时在左侧留出 36px 圆形按钮；
     * - `external`：调用方在自己的顶栏里放了开合按钮（如 AWS 控制台的汉堡），
     *   这里不再渲染圆形按钮，也不预留左侧 gutter。
     */
    navigationToggle?: 'panel' | 'external'
    /**
     * 是否在导航内容顶部预留 44px 给悬浮的收起按钮。
     * 调用方若给 SideNavigation 传了 header（收起按钮会与 header 同行），传 false。
     */
    reserveNavigationToggleRow?: boolean
    navigationLabel?: string
    navigationCloseLabel?: string
    navigationToggleLabel?: string
  }>(),
  {
    navigationOpen: true,
    navigationWidth: 236,
    headerHeight: 56,
    navigationHide: false,
    contentType: 'default',
    maxContentWidth: undefined,
    disableContentPaddings: false,
    navigationToggle: 'panel',
    reserveNavigationToggleRow: true,
    navigationLabel: '主导航',
    navigationCloseLabel: '收起导航',
    navigationToggleLabel: '打开导航',
  }
)

const emit = defineEmits<{
  (e: 'update:navigationOpen', value: boolean): void
}>()

const slots = useSlots()
const cs = CS['app-layout']

const hasNavigation = computed(() => !props.navigationHide && Boolean(slots.navigation))
const hasBreadcrumbs = computed(() => Boolean(slots.breadcrumbs))
const hasHeader = computed(() => Boolean(slots.header))
const hasNotifications = computed(() => Boolean(slots.notifications))

/** 官方 getContentFirstChild：决定内容区顶部间距该按谁对齐。 */
const contentFirstChild = computed(() => {
  if (hasNotifications.value) return 'notifications'
  if (hasBreadcrumbs.value) return 'breadcrumbs'
  if (hasHeader.value) return 'header'
  return 'main'
})

const hasContentGapLeft = computed(() => props.navigationOpen || props.navigationHide)
const hasRightTogglesGutter = computed(() => false)
/** 左侧 36px 圆形开合按钮只在「面板自管」且收起时出现。 */
const showPanelToggle = computed(() => props.navigationToggle === 'panel' && !props.navigationOpen)
const hasLeftTogglesGutter = computed(
  () => showPanelToggle.value && !props.navigationHide && Boolean(slots.navigation)
)

const layoutClasses = computed(() =>
  cx(
    cs.layout,
    cs[`content-first-child-${contentFirstChild.value}`],
    props.contentType !== 'default' && cs[`content-type-${props.contentType}`],
    hasBreadcrumbs.value && cs['has-breadcrumbs'],
    hasHeader.value && cs['has-header'],
    hasContentGapLeft.value && cs['has-content-gap-left'],
    cs['has-content-gap-right'],
    Boolean(props.maxContentWidth && props.maxContentWidth > 0) && cs['has-max-content-width'],
    hasLeftTogglesGutter.value && cs['has-left-toggles-gutter'],
    hasRightTogglesGutter.value && cs['has-right-toggles-gutter'],
    props.disableContentPaddings && cs['disable-content-paddings']
  )
)

const layoutStyle = computed(() => ({
  '--awsui-header-height-ka4djm': `${props.headerHeight}px`,
  '--awsui-footer-height-ka4djm': '0px',
  '--awsui-notifications-height-ka4djm': '0px',
  '--awsui-layout-width-ka4djm': '0px',
  '--awsui-main-offset-left-ka4djm': '0px',
  ...(props.maxContentWidth
    ? { '--awsui-max-content-width-ka4djm': `${props.maxContentWidth}px` }
    : {}),
}))

const navigationStyle = computed(() => ({
  '--awsui-navigation-width-ka4djm': `${props.navigationWidth}px`,
}))

function openNavigation() {
  emit('update:navigationOpen', true)
}

function closeNavigation() {
  emit('update:navigationOpen', false)
}
</script>

<template>
  <main :class="layoutClasses" :style="layoutStyle">
    <div :class="cs.background">
      <div :class="cs['scrolling-background']" />
    </div>

    <div v-if="hasNavigation" :class="[cs['navigation-container'], 'cs-app-nav-container']" :style="navigationStyle">
      <nav
        v-if="navigationToggle === 'panel'"
        :class="cx(cs['show-navigation'], navigationOpen && cs['is-navigation-open'])"
        :aria-hidden="navigationOpen"
      >
        <span :class="cs['trigger-wrapper']">
          <button
            type="button"
            :class="cx(cs['trigger-button-styles'], cs.trigger)"
            :aria-label="navigationToggleLabel"
            :title="navigationToggleLabel"
            @click="openNavigation"
          >
            <Icon name="menu" />
          </button>
        </span>
      </nav>

      <nav
        :class="
          cx(
            cs.navigation,
            cs['animated-content'],
            'cs-app-nav',
            navigationOpen && cs['is-navigation-open']
          )
        "
        :aria-label="navigationLabel"
        :aria-hidden="!navigationOpen"
      >
        <div
          :class="cx(cs['content-container'], cs['animated-content'], 'cs-nav-content')"
          :style="{ paddingBlockStart: reserveNavigationToggleRow ? '44px' : '0px' }"
        >
          <div :class="[cs['hide-navigation'], 'cs-nav-close']">
            <Button
              variant="icon"
              icon-name="angle-left"
              :aria-label="navigationCloseLabel"
              @click="closeNavigation"
            />
          </div>
          <slot name="navigation" />
        </div>
      </nav>
    </div>

    <div v-if="hasBreadcrumbs" :class="cs.breadcrumbs">
      <slot name="breadcrumbs" />
    </div>

    <header v-if="hasHeader" :class="cs.content">
      <slot name="header" />
    </header>

    <div v-if="hasNotifications" :class="cs.notifications">
      <slot name="notifications" />
    </div>

    <div :class="cs.container">
      <slot />
    </div>
  </main>
</template>

<style scoped>
/* 官方 grid 第一列是 min-content，展开时由 --awsui-navigation-width 决定。
   AppLayout 的 inline-size 动画由官方 JS 触发，这里补上纯 CSS 的过渡以复用项目节奏。 */
.cs-app-nav {
  transition: inline-size var(--x-collapse-dur, 0.24s) var(--x-collapse-ease, cubic-bezier(0.2, 0, 0, 1));
  scrollbar-width: thin;
  scrollbar-color: var(--color-border-divider-default-diwd4w, #c6c6cd) transparent;
}

.cs-app-nav::-webkit-scrollbar {
  inline-size: 8px;
}

.cs-app-nav::-webkit-scrollbar-thumb {
  background: var(--color-border-divider-default-diwd4w, #c6c6cd);
  border-radius: 4px;
}

/* 收起按钮是绝对定位（官方 top 15px / right 16px），导航内容若从面板顶部开始，
   z-index 为 auto 时会被后出现的定位元素压住（搜索框的 relative 容器）。
   这里把按钮抬到内容之上；顶部留白由 reserveNavigationToggleRow 决定。 */
.cs-nav-close {
  z-index: 2;
}
</style>
