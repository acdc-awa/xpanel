<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { adminMenuGroups, type MenuGroup } from '@/config/menu'
import { useAuthStore } from '@/stores/auth'
import { useSiteStore } from '@/stores/site'
import { useThemeStore } from '@/stores/theme'
import { getSystemStatus } from '@/api/admin'
import UserAvatarDropdown from '@/components/UserAvatarDropdown.vue'
import {
  CsBreadcrumbGroup,
  CsDrawer,
  CsIconButton,
  CsInput,
  CsSideNavigation,
  CsTopNavigation,
  type BreadcrumbItem,
  type SideNavigationChangeDetail,
  type SideNavigationFollowDetail,
  type SideNavigationItem,
  type TopNavigationUtility,
} from '@/components/cloudscape'

const auth = useAuthStore()
const site = useSiteStore()
const theme = useThemeStore()
const router = useRouter()
const route = useRoute()
const pageTitle = computed(() => (route.meta.title as string) ?? '')

// 折叠侧边栏状态（true = 折叠/靠近展开远离收起模式；false = 固定展开模式）
const isCollapsed = ref(localStorage.getItem('admin_sidebar_collapsed') === '1')

// 悬停展开状态（靠近展开，远离收起）
const isHovered = ref(false)
let hoverLeaveTimer: ReturnType<typeof setTimeout> | null = null

// 是否当前处于视觉展开状态（固定展开 OR 悬停展开）
const isEffectiveExpanded = computed(() => !isCollapsed.value || isHovered.value)

function toggleCollapse() {
  isCollapsed.value = !isCollapsed.value
  isHovered.value = false
  localStorage.setItem('admin_sidebar_collapsed', isCollapsed.value ? '1' : '0')
}

function handleMouseEnter() {
  if (hoverLeaveTimer) {
    clearTimeout(hoverLeaveTimer)
    hoverLeaveTimer = null
  }
  if (isCollapsed.value) {
    isHovered.value = true
  }
}

function handleMouseLeave() {
  if (isCollapsed.value) {
    hoverLeaveTimer = setTimeout(() => {
      isHovered.value = false
    }, 140)
  }
}

// 侧边栏搜索过滤
const searchQuery = ref('')
const searchInputRef = ref<{ focus: () => void; select: () => void; blur: () => void } | null>(null)

const filteredMenuGroups = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  if (!query) return adminMenuGroups

  return adminMenuGroups
    .map((group) => {
      const groupTitleMatch = group.title.toLowerCase().includes(query)
      if (!group.children || group.children.length === 0) {
        return groupTitleMatch ? group : null
      }
      const filteredChildren = group.children.filter(
        (child) => child.title.toLowerCase().includes(query) || groupTitleMatch
      )
      if (filteredChildren.length > 0) {
        return {
          ...group,
          children: filteredChildren,
        }
      }
      return null
    })
    .filter(Boolean) as typeof adminMenuGroups
})

function groupForPath(path: string): MenuGroup | undefined {
  return adminMenuGroups.find((group) => group.children?.some((child) => child.path === path))
}

const initialGroup = groupForPath(route.path)
const expandedGroups = ref<Set<string>>(new Set(initialGroup ? [initialGroup.title] : []))
let preSearchExpanded: Set<string> | null = null

watch(searchQuery, (query) => {
  if (query.trim()) {
    if (!preSearchExpanded) preSearchExpanded = new Set(expandedGroups.value)
    expandedGroups.value = new Set([
      ...expandedGroups.value,
      ...adminMenuGroups.filter((group) => group.children?.length).map((group) => group.title),
    ])
  } else if (preSearchExpanded) {
    expandedGroups.value = preSearchExpanded
    preSearchExpanded = null
  }
})

function toNavItems(groups: MenuGroup[]): SideNavigationItem[] {
  return groups.map((group) => {
    if (!group.children || group.children.length === 0) {
      return {
        type: 'link',
        text: group.title,
        href: group.path ?? '/admin/dashboard',
        icon: group.iconName,
      }
    }
    return {
      type: 'expandable-link-group',
      text: group.title,
      icon: group.iconName,
      expanded: expandedGroups.value.has(group.title),
      items: group.children.map((child) => ({
        type: 'link' as const,
        text: child.title,
        href: child.path,
        icon: child.iconName,
      })),
    }
  })
}

const navItems = computed(() => toNavItems(filteredMenuGroups.value))
const mobileNavItems = computed(() => toNavItems(adminMenuGroups))

function onNavChange(detail: SideNavigationChangeDetail) {
  if (detail.item.type !== 'expandable-link-group') return
  const next = new Set(expandedGroups.value)
  if (detail.expanded) next.add(detail.item.text)
  else next.delete(detail.item.text)
  expandedGroups.value = next
}

function onNavFollow(detail: SideNavigationFollowDetail, event: MouseEvent) {
  event.preventDefault()
  if (!detail.href || detail.href === '#') return
  void router.push(detail.href)
  if (isMobile.value) drawerOpen.value = false
  if (isCollapsed.value) isHovered.value = false
}

function handleSearchKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter') {
    const firstGroup = filteredMenuGroups.value[0]
    if (!firstGroup) return
    if (!firstGroup.children || !firstGroup.children.length) {
      if (firstGroup.path) {
        router.push(firstGroup.path)
        searchQuery.value = ''
        if (isCollapsed.value) isHovered.value = false
        searchInputRef.value?.blur()
      }
    } else if (firstGroup.children[0]?.path) {
      router.push(firstGroup.children[0].path)
      searchQuery.value = ''
      if (isCollapsed.value) isHovered.value = false
      searchInputRef.value?.blur()
    }
  } else if (e.key === 'Escape') {
    searchQuery.value = ''
    searchInputRef.value?.blur()
  }
}

function handleCollapsedSearchClick() {
  if (isCollapsed.value) {
    isHovered.value = true
  }
  nextTick(() => {
    searchInputRef.value?.focus()
  })
}

// 全局快捷键监听 (Ctrl/Cmd + B 切换固定/折叠, Ctrl/Cmd + K 聚焦搜索)
function handleGlobalKeydown(e: KeyboardEvent) {
  const isMac = navigator.platform.toUpperCase().indexOf('MAC') >= 0
  const isCtrlOrCmd = isMac ? e.metaKey : e.ctrlKey

  if (isCtrlOrCmd && e.key.toLowerCase() === 'b') {
    e.preventDefault()
    toggleCollapse()
  } else if (isCtrlOrCmd && e.key.toLowerCase() === 'k') {
    e.preventDefault()
    if (isCollapsed.value) {
      isHovered.value = true
    }
    nextTick(() => {
      searchInputRef.value?.focus()
      searchInputRef.value?.select()
    })
  }
}

// 路由切换：收起悬停浮层，并展开当前页所在的菜单组
watch(
  () => route.path,
  (path) => {
    if (isCollapsed.value) isHovered.value = false
    const group = groupForPath(path)
    if (group) expandedGroups.value = new Set([...expandedGroups.value, group.title])
  }
)

const breadcrumbs = computed<BreadcrumbItem[]>(() => {
  const path = route.path
  for (const group of adminMenuGroups) {
    if (group.path === path) return []
    const child = group.children?.find((item) => item.path === path)
    if (child) return [{ text: group.title }, { text: child.title }]
  }
  return []
})

const topbarUtilities = computed<TopNavigationUtility[]>(() => [
  {
    type: 'button',
    iconName: 'light-dark',
    ariaLabel: theme.modeTooltip,
    title: theme.modeTooltip,
    onClick: () => theme.toggle(),
  },
  {
    type: 'button',
    iconName: 'user-profile',
    text: '切换至用户端视图',
    title: '切换至用户端视图',
    collapseAt: 'narrow',
    onClick: () => void router.push('/dashboard'),
  },
])

function goDashboard() {
  void router.push('/admin/dashboard')
}

// 响应式分级：<768px 手机端进入抽屉；768px~1024px (Pad端) 自动折叠侧栏为 Mini Dock
// 媒体查询对象与初值都在 setup 阶段同步建立：原实现把赋值放在 onMounted，
// 手机端首帧 isMobile 仍是 false，会先画一帧 236px 固定侧栏再消失（首屏位置跳一次）
const mobileMq = typeof window !== 'undefined' ? window.matchMedia('(max-width: 767.98px)') : null
const tabletMq = typeof window !== 'undefined' ? window.matchMedia('(max-width: 1023.98px)') : null

const isMobile = ref(mobileMq?.matches ?? false)
const drawerOpen = ref(false)

const onMobileMq = (e: MediaQueryListEvent | MediaQueryList) => {
  isMobile.value = e.matches
  if (!e.matches) drawerOpen.value = false
}

const onTabletMq = (e: MediaQueryListEvent | MediaQueryList) => {
  // 如果是 Pad 端且用户未手动固定展开，则默认保持折叠模式
  if (e.matches && localStorage.getItem('admin_sidebar_collapsed') === null) {
    isCollapsed.value = true
  }
}

// 页脚版本号：取后端 ldflags 注入的真实 panel_version
const panelVersion = ref('')
onMounted(async () => {
  // 初值已同步求值过，这里只订阅变化并补一次 Pad 端默认折叠策略
  if (tabletMq) onTabletMq(tabletMq)
  mobileMq?.addEventListener('change', onMobileMq)
  tabletMq?.addEventListener('change', onTabletMq)
  window.addEventListener('keydown', handleGlobalKeydown)
  try {
    const { data } = await getSystemStatus()
    if (data.code === 0) panelVersion.value = data.data.panel_version
  } catch {
    /* 版本获取失败静默，页脚仅显示用户名 */
  }
})
onUnmounted(() => {
  if (hoverLeaveTimer) clearTimeout(hoverLeaveTimer)
  mobileMq?.removeEventListener('change', onMobileMq)
  tabletMq?.removeEventListener('change', onTabletMq)
  window.removeEventListener('keydown', handleGlobalKeydown)
})
</script>

<template>
  <div class="admin-layout">
    <!-- 桌面端侧栏：固定展开 / Mini Dock 悬停展开 -->
    <aside
      v-if="!isMobile"
      class="admin-aside"
      :class="{
        collapsed: isCollapsed,
        'is-hovered': isHovered,
        'is-expanded': isEffectiveExpanded,
      }"
      @mouseenter="handleMouseEnter"
      @mouseleave="handleMouseLeave"
    >
      <div class="admin-aside-header">
        <div
          v-if="isEffectiveExpanded"
          class="admin-logo"
          :title="site.appName || 'Xray 管理面板'"
          @click="goDashboard"
        >
          <div class="admin-logo-icon-wrap">
            <img v-if="site.logo" :src="site.logo" class="admin-logo-img" alt="logo" />
            <span v-else class="admin-logo-mark">X</span>
          </div>
          <span class="admin-logo-text">{{ site.appName || 'Xray 管理面板' }}</span>
        </div>

        <CsIconButton
          class="sidebar-toggle-btn"
          icon-name="side-bar"
          size="normal"
          :label="isCollapsed ? '固定侧边栏 (Ctrl+B)' : '折叠侧边栏，靠近可自动展开 (Ctrl+B)'"
          @click.stop="toggleCollapse"
        />
      </div>

      <div class="admin-menu-search">
        <div v-if="isEffectiveExpanded" class="admin-search-box">
          <CsInput
            ref="searchInputRef"
            v-model="searchQuery"
            type="search"
            prefix-icon="search"
            clearable
            placeholder="搜索管理菜单..."
            aria-label="搜索管理菜单"
            @keydown="handleSearchKeydown"
          />
          <kbd v-if="!searchQuery" class="shortcut-badge">Ctrl K</kbd>
        </div>
        <CsIconButton
          v-else
          icon-name="search"
          size="normal"
          label="搜索菜单 (Ctrl+K)"
          @click="handleCollapsedSearchClick"
        />
      </div>

      <div class="admin-menu">
        <CsSideNavigation
          :items="navItems"
          :active-href="route.path"
          :collapsed="!isEffectiveExpanded"
          @follow="onNavFollow"
          @change="onNavChange"
        />
      </div>

      <div class="admin-aside-foot">
        <span class="status-dot-wrap" :title="`${auth.username} · 在线`">
          <span class="status-pulse-dot" />
        </span>
        <span class="foot-text">
          {{ auth.username }}<template v-if="panelVersion"> · {{ panelVersion }}</template>
        </span>
      </div>
    </aside>

    <!-- 移动端抽屉 -->
    <CsDrawer
      v-model:visible="drawerOpen"
      placement="start"
      :size="280"
      :header="site.appName || '导航'"
      disable-content-paddings
    >
      <div class="admin-drawer-body">
        <div class="admin-drawer-nav">
          <CsSideNavigation
            :items="mobileNavItems"
            :active-href="route.path"
            @follow="onNavFollow"
            @change="onNavChange"
          />
        </div>
        <div class="admin-aside-foot">
          <span class="status-dot-wrap">
            <span class="status-pulse-dot" />
          </span>
          <span class="foot-text">
            {{ auth.username }}<template v-if="panelVersion"> · {{ panelVersion }}</template>
          </span>
        </div>
      </div>
    </CsDrawer>

    <!-- 主工作区 -->
    <div class="admin-main" :class="{ collapsed: isCollapsed && !isMobile }">
      <CsTopNavigation class="admin-topbar" :utilities="topbarUtilities" visual-context="none">
        <template #left>
          <div class="admin-topbar-left">
            <CsIconButton
              v-if="isMobile"
              class="admin-hamburger"
              icon-name="menu"
              size="normal"
              label="打开导航菜单"
              @click="drawerOpen = true"
            />
            <span class="admin-topbar-title">{{ pageTitle }}</span>
          </div>
        </template>
        <template #utility-extra>
          <UserAvatarDropdown current-portal="admin" />
        </template>
      </CsTopNavigation>

      <main class="admin-content">
        <div v-if="breadcrumbs.length > 0" class="admin-breadcrumbs">
          <CsBreadcrumbGroup :items="breadcrumbs" />
        </div>
        <router-view />
      </main>
    </div>
  </div>
</template>

<style scoped lang="scss">
.admin-layout {
  display: flex;
  min-height: 100vh;
  background: var(--color-background-layout-main-y4565g, #ffffff);
}

/* 侧边栏主体：宽度/过渡/层级沿用与 ClientLayout 共享的布局 token。
   颜色、边框、字号一律使用 Cloudscape 语义 token。 */
.admin-aside {
  width: var(--x-sidebar-w, 236px);
  background: var(--color-background-container-content-mjfil9, #ffffff);
  border-inline-end: 1px solid var(--color-border-divider-default-diwd4w, #c6c6cd);
  position: fixed;
  inset-block: 0;
  inset-inline-start: 0;
  block-size: 100vh;
  display: flex;
  flex-direction: column;
  z-index: var(--x-z-sidebar, 80);
  transition:
    width var(--x-collapse-dur, 0.24s) var(--x-collapse-ease, cubic-bezier(0.2, 0, 0, 1)),
    box-shadow 0.24s ease;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.02);
  /* 官方 SideNavigation collapsed 模式下用这个变量计算列表内边距 */
  --awsui-navigation-collapsed-width-ka4djm: var(--x-sidebar-w-mini, 58px);

  &.collapsed {
    width: var(--x-sidebar-w-mini, 58px);

    &.is-hovered {
      width: var(--x-sidebar-w, 236px);
      z-index: var(--x-z-sidebar-hover, 200);
      box-shadow: 8px 0 32px rgba(0, 0, 0, 0.12);
    }

    &:not(.is-hovered) {
      .admin-aside-header {
        padding-inline: 0;
        justify-content: center;
      }

      .admin-menu-search {
        justify-content: center;
        padding-inline: 0;
      }

      .admin-aside-foot {
        padding-inline: 0;
        justify-content: center;
      }

      .status-dot-wrap {
        margin-inline-end: 0;
      }

      .foot-text {
        display: none;
      }
    }
  }
}

/* 顶部品牌与折叠按钮栏 */
.admin-aside-header {
  block-size: var(--x-topbar-h, 56px);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-inline: 14px 12px;
  border-block-end: 1px solid var(--color-border-divider-default-diwd4w, #c6c6cd);
  flex: none;
  overflow: hidden;
  transition: padding 0.24s var(--x-collapse-ease, cubic-bezier(0.2, 0, 0, 1));
}

.admin-logo {
  display: flex;
  align-items: center;
  cursor: pointer;
  overflow: hidden;
  user-select: none;
  min-inline-size: 0;
  flex: 1;
}

.admin-logo-icon-wrap {
  inline-size: 30px;
  block-size: 30px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex: none;
  margin-inline-end: 10px;
}

.admin-logo-mark {
  inline-size: 28px;
  block-size: 28px;
  border-radius: var(--border-radius-input-k1q34z, 8px);
  background: linear-gradient(135deg, #6366f1, #a855f7);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  font-weight: 800;
  box-shadow: 0 2px 6px rgba(99, 102, 241, 0.3);
}

.admin-logo-img {
  inline-size: 28px;
  block-size: 28px;
  border-radius: var(--border-radius-input-k1q34z, 8px);
  object-fit: contain;
  background: var(--color-background-container-header-irktp5, #ffffff);
}

.admin-logo-text {
  font-weight: 700;
  font-size: var(--font-size-body-m-tq5a14, 14px);
  line-height: var(--line-height-body-m-5xw6xo, 20px);
  color: var(--color-text-body-default-b2us04, #0f141a);
  white-space: nowrap;
  letter-spacing: -0.2px;
  max-inline-size: 140px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.sidebar-toggle-btn {
  flex: none;
  inline-size: 30px;
  block-size: 30px;
  color: var(--color-text-body-secondary-66ygbb, #424650);
}

/* 侧边快捷搜索栏 */
.admin-menu-search {
  padding: 8px 10px 4px;
  flex: none;
  display: flex;
}

.admin-search-box {
  position: relative;
  inline-size: 100%;

  :deep(.cs-input-root) {
    inline-size: 100%;
  }
}

.shortcut-badge {
  position: absolute;
  inset-inline-end: 8px;
  inset-block-start: 50%;
  transform: translateY(-50%);
  font-size: var(--font-size-body-s-89y7qg, 12px);
  font-family: var(--font-family-monospace-842lr2, monospace);
  color: var(--color-text-body-secondary-66ygbb, #424650);
  background: var(--color-background-container-content-mjfil9, #ffffff);
  border: 1px solid var(--color-border-divider-default-diwd4w, #c6c6cd);
  border-radius: 4px;
  padding: 1px 4px;
  line-height: 1.2;
  user-select: none;
  pointer-events: none;
}

/* 侧边菜单区 */
.admin-menu {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  padding: 6px 0;
  scrollbar-width: thin;
  scrollbar-color: var(--color-border-divider-default-diwd4w, #c6c6cd) transparent;
  // 常驻预留滚动条槽位，避免折叠/展开切换时文字右边缘跳动
  scrollbar-gutter: stable;

  &::-webkit-scrollbar {
    width: 8px;
  }

  &::-webkit-scrollbar-thumb {
    background: var(--color-border-divider-default-diwd4w, #c6c6cd);
    border-radius: 4px;
  }
}

/* 底部状态栏 */
.admin-aside-foot {
  block-size: 42px;
  padding-inline: 14px;
  border-block-start: 1px solid var(--color-border-divider-default-diwd4w, #c6c6cd);
  color: var(--color-text-body-secondary-66ygbb, #424650);
  font-size: var(--font-size-body-s-89y7qg, 12px);
  display: flex;
  align-items: center;
  flex: none;
  background: var(--color-background-container-header-irktp5, #ffffff);
  user-select: none;
  overflow: hidden;
  transition: padding 0.24s var(--x-collapse-ease, cubic-bezier(0.2, 0, 0, 1));
}

.status-dot-wrap {
  inline-size: 24px;
  block-size: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex: none;
  margin-inline-end: 8px;
}

.status-pulse-dot {
  inline-size: 7px;
  block-size: 7px;
  border-radius: 50%;
  background: var(--color-background-status-success-1woffs, #10b981);
  box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.2);
  flex: none;
}

.foot-text {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-inline-size: 0;
}

/* 主内容区自适应过渡 */
.admin-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  min-height: 100vh;
  transition: margin-left var(--x-collapse-dur, 0.24s) var(--x-collapse-ease, cubic-bezier(0.2, 0, 0, 1));

  @media (min-width: 768px) {
    margin-inline-start: var(--x-sidebar-w, 236px);
  }

  @media (max-width: 767.98px) {
    margin-inline-start: 0;
  }

  &.collapsed {
    @media (min-width: 768px) {
      margin-inline-start: var(--x-sidebar-w-mini, 58px);
    }
  }
}

.admin-topbar {
  position: sticky;
  inset-block-start: 0;
  z-index: var(--x-z-sticky, 10);
  flex: none;
}

.admin-topbar-title {
  font-size: var(--font-size-heading-xs-yrvp79, 14px);
  font-weight: 700;
  color: var(--color-text-body-default-b2us04, #0f141a);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  min-inline-size: 0;
}

.admin-topbar-left {
  display: flex;
  align-items: center;
  min-inline-size: 0;
}

.admin-hamburger {
  margin-inline-end: 8px;
}

.admin-content {
  flex: 1;
  // 正文内边距的唯一来源：.x-page 已不再叠加 padding。
  // 三档单调递减，拖拽窗口跨断点时不出现「左右跳」
  padding: 20px 24px 32px;

  @media (max-width: 900px) {
    padding: 18px 20px 28px;
  }

  @media (max-width: 767.98px) {
    padding: 14px 12px 24px;
  }
}

.admin-breadcrumbs {
  max-inline-size: var(--x-content-max, 1320px);
  margin-inline: auto;
  padding-block-end: 12px;

  @media (max-width: 767.98px) {
    display: none;
  }
}

.admin-drawer-body {
  display: flex;
  flex-direction: column;
  min-block-size: 100%;
  padding-block: 8px;
}

.admin-drawer-nav {
  flex: 1;
  min-block-size: 0;
  overflow-y: auto;
}
</style>
