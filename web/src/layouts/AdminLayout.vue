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
  CsAppLayout,
  CsAppToolbar,
  CsBreadcrumbGroup,
  CsDrawer,
  CsInput,
  CsSideNavigation,
  CsTopNavigation,
  type BreadcrumbItem,
  type SideNavigationChangeDetail,
  type SideNavigationFollowDetail,
  type SideNavigationItem,
  type TopNavigationIdentity,
  type TopNavigationUtility,
} from '@/components/cloudscape'

const auth = useAuthStore()
const site = useSiteStore()
const theme = useThemeStore()
const router = useRouter()
const route = useRoute()

// 导航开合（官方 AppLayout 语义：开 = 固定列，关 = 完全收起 + 左侧圆钮）。
// 持久化沿用旧 key，值为 '1' 表示收起。
const storedCollapsed = localStorage.getItem('admin_sidebar_collapsed')
const navigationOpen = ref(storedCollapsed !== '1')

function setNavigationOpen(value: boolean) {
  navigationOpen.value = value
  localStorage.setItem('admin_sidebar_collapsed', value ? '0' : '1')
}

function toggleNavigation() {
  setNavigationOpen(!navigationOpen.value)
}

// 顶栏汉堡：桌面切换侧栏展开/收起，移动端打开抽屉（与官方控制台一致）
function onNavToggleClick() {
  if (isMobile.value) {
    drawerOpen.value = true
    return
  }
  toggleNavigation()
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

// 分组用官方 section：表头是纯文本折叠节，不再把组头当成链接。
// 与官方控制台一致，导航项不带图标——SideNavigation 的 `list--with-icons`
// 会按「图标在行首」重算内边距，和 section 表头的负边距叠加后会撑出容器。
function toNavItems(groups: MenuGroup[]): SideNavigationItem[] {
  return groups.map((group) => {
    if (!group.children || group.children.length === 0) {
      return {
        type: 'link',
        text: group.title,
        href: group.path ?? '/admin/dashboard',
      }
    }
    return {
      type: 'section',
      text: group.title,
      expanded: expandedGroups.value.has(group.title),
      items: group.children.map((child) => ({
        type: 'link' as const,
        text: child.title,
        href: child.path,
      })),
    }
  })
}

const navItems = computed(() => toNavItems(filteredMenuGroups.value))
const mobileNavItems = computed(() => toNavItems(adminMenuGroups))

function onNavChange(detail: SideNavigationChangeDetail) {
  if (detail.item.type !== 'section') return
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
}

function handleSearchKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter') {
    const firstGroup = filteredMenuGroups.value[0]
    if (!firstGroup) return
    if (!firstGroup.children || !firstGroup.children.length) {
      if (firstGroup.path) {
        router.push(firstGroup.path)
        searchQuery.value = ''
        searchInputRef.value?.blur()
      }
    } else if (firstGroup.children[0]?.path) {
      router.push(firstGroup.children[0].path)
      searchQuery.value = ''
      searchInputRef.value?.blur()
    }
  } else if (e.key === 'Escape') {
    searchQuery.value = ''
    searchInputRef.value?.blur()
  }
}

// 全局快捷键：Ctrl/Cmd+B 开合导航；Ctrl/Cmd+K 聚焦菜单搜索
function handleGlobalKeydown(e: KeyboardEvent) {
  const isMac = navigator.platform.toUpperCase().indexOf('MAC') >= 0
  const isCtrlOrCmd = isMac ? e.metaKey : e.ctrlKey

  if (isCtrlOrCmd && e.key.toLowerCase() === 'b') {
    e.preventDefault()
    toggleNavigation()
  } else if (isCtrlOrCmd && e.key.toLowerCase() === 'k') {
    e.preventDefault()
    // 桌面端搜索框常驻顶栏；移动端搜索在抽屉里，需要先打开抽屉
    if (isMobile.value) drawerOpen.value = true
    nextTick(() => {
      searchInputRef.value?.focus()
      searchInputRef.value?.select()
    })
  }
}

// 路由切换：上报页面变化后展开所在分组
watch(
  () => route.path,
  (path) => {
    if (isMobile.value) drawerOpen.value = false
    const group = groupForPath(path)
    if (group) expandedGroups.value = new Set([...expandedGroups.value, group.title])
  }
)

const breadcrumbs = computed<BreadcrumbItem[]>(() => {
  const path = route.path
  // 根级固定为「管理页面」；分组有独立页面时是二级，否则是三级的中间级。
  const root: BreadcrumbItem = { text: '管理页面', href: '/admin/dashboard' }
  for (const group of adminMenuGroups) {
    if (group.path === path) return [root, { text: group.title }]
    const child = group.children?.find((item) => item.path === path)
    if (child) return [root, { text: group.title }, { text: child.title }]
  }
  return [root]
})

const topbarIdentity = computed<TopNavigationIdentity>(() => ({
  href: '/admin/dashboard',
  title: site.appName || 'Xray 管理面板',
  logo: site.logo ? { src: site.logo, alt: site.appName || '' } : undefined,
  mark: site.logo ? undefined : 'X',
}))

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

const contentType = computed(() => (route.path === '/admin/dashboard' ? 'dashboard' : 'default'))

// 响应式分级：<768px 手机端走抽屉；768px~1024px (Pad端) 默认收起导航
const mobileMq = typeof window !== 'undefined' ? window.matchMedia('(max-width: 767.98px)') : null
const tabletMq = typeof window !== 'undefined' ? window.matchMedia('(max-width: 1023.98px)') : null

const isMobile = ref(mobileMq?.matches ?? false)
const drawerOpen = ref(false)

const onMobileMq = (e: MediaQueryListEvent | MediaQueryList) => {
  isMobile.value = e.matches
  if (!e.matches) drawerOpen.value = false
}

const onTabletMq = (e: MediaQueryListEvent | MediaQueryList) => {
  if (e.matches && localStorage.getItem('admin_sidebar_collapsed') === null) {
    navigationOpen.value = false
  }
}

// 页脚版本号：取后端 ldflags 注入的真实 panel_version
const panelVersion = ref('')
const accountFootnote = computed(() =>
  panelVersion.value ? `${auth.username} · ${panelVersion.value}` : auth.username
)

onMounted(async () => {
  if (tabletMq) onTabletMq(tabletMq)
  mobileMq?.addEventListener('change', onMobileMq)
  tabletMq?.addEventListener('change', onTabletMq)
  window.addEventListener('keydown', handleGlobalKeydown)
  try {
    const { data } = await getSystemStatus()
    if (data.code === 0) panelVersion.value = data.data.panel_version
  } catch {
    /* 版本获取失败静默，菜单只显示用户名 */
  }
})
onUnmounted(() => {
  mobileMq?.removeEventListener('change', onMobileMq)
  tabletMq?.removeEventListener('change', onTabletMq)
  window.removeEventListener('keydown', handleGlobalKeydown)
})
</script>

<template>
  <div class="admin-shell">
    <!-- 全宽顶栏：品牌 + 菜单搜索 + 全局工具 -->
    <CsTopNavigation
      class="admin-topbar"
      :identity="topbarIdentity"
      :utilities="topbarUtilities"
      visual-context="top-navigation"
      @identity-follow="goDashboard"
    >
      <template #search>
        <!-- 窄屏顶栏放不下搜索框，改用抽屉内的搜索（Ctrl+K 会打开抽屉） -->
        <div v-if="!isMobile" class="admin-nav-search admin-topbar-search">
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
      </template>
      <template #utility-extra>
        <UserAvatarDropdown current-portal="admin" :footnote="accountFootnote" />
      </template>
    </CsTopNavigation>

    <!-- 应用栏：汉堡 + 面包屑，横跨全宽（官方 universal toolbar） -->
    <CsAppToolbar
      :height="40"
      :sticky-top="57"
      :navigation-label="isMobile ? '打开导航菜单' : navigationOpen ? '收起导航' : '展开导航'"
      @navigation-click="onNavToggleClick"
    >
      <CsBreadcrumbGroup v-if="!isMobile && breadcrumbs.length" :items="breadcrumbs" />
    </CsAppToolbar>

    <CsAppLayout
      :navigation-open="navigationOpen"
      :navigation-width="236"
      :header-height="56"
      :navigation-hide="isMobile"
      :content-type="contentType"
      navigation-toggle="external"
      @update:navigation-open="setNavigationOpen"
    >
      <template #navigation>
        <CsSideNavigation
          v-if="!isMobile"
          :items="navItems"
          :active-href="route.path"
          @follow="onNavFollow"
          @change="onNavChange"
        />
      </template>

      <router-view />
    </CsAppLayout>

    <!-- 移动端抽屉 -->
    <CsDrawer
      v-model:visible="drawerOpen"
      placement="start"
      :size="280"
      :header="site.appName || '导航'"
      disable-content-paddings
    >
      <div class="admin-drawer-body">
        <div class="admin-nav-search admin-drawer-search">
          <CsInput
            v-model="searchQuery"
            type="search"
            prefix-icon="search"
            clearable
            placeholder="搜索管理菜单..."
            aria-label="搜索管理菜单"
            @keydown="handleSearchKeydown"
          />
        </div>
        <div class="admin-drawer-nav">
          <CsSideNavigation
            :items="mobileNavItems"
            :active-href="route.path"
            @follow="onNavFollow"
            @change="onNavChange"
          />
        </div>
      </div>
    </CsDrawer>
  </div>
</template>

<style scoped lang="scss">
.admin-shell {
  display: flex;
  flex-direction: column;
  min-block-size: 100vh;
  background: var(--color-background-layout-main-y4565g, #ffffff);
}

/* 顶栏横跨全宽并常驻；AppLayout 以 --awsui-header-height 为下边界 */
.admin-topbar {
  position: sticky;
  inset-block-start: 0;
  z-index: 1100;
  flex: none;
}

.admin-nav-search {
  position: relative;
  inline-size: 100%;
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

.admin-drawer-body {
  display: flex;
  flex-direction: column;
  min-block-size: 100%;
  padding-block: 8px;
}

.admin-drawer-search {
  padding-inline: var(--space-m-n8iq8o, 16px);
  padding-block-end: var(--space-xs-m9b7jf, 8px);
}

.admin-drawer-nav {
  flex: 1;
  min-block-size: 0;
  overflow-y: auto;
}
</style>
