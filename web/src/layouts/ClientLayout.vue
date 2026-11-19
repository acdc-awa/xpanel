<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { MQ } from '@/utils/media'
import { clientMenus } from '@/config/menu'
import { useAuthStore } from '@/stores/auth'
import { useSiteStore } from '@/stores/site'
import { useThemeStore } from '@/stores/theme'
import UserAvatarDropdown from '@/components/UserAvatarDropdown.vue'
import {
  CsAppLayout,
  CsAppToolbar,
  CsBreadcrumbGroup,
  CsIcon,
  CsSideNavigation,
  CsTopNavigation,
  type BreadcrumbItem,
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

const pageTitle = computed(() => (route.meta.title as string) || '用户中心')

// 侧边栏菜单项（桌面/平板端）
const navItems = computed<SideNavigationItem[]>(() =>
  clientMenus.map((item) => ({
    type: 'link',
    text: item.title,
    href: item.path,
    icon: item.iconName,
  })),
)

function onNavFollow(detail: SideNavigationFollowDetail, event?: MouseEvent) {
  if (event && (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0)) {
    return
  }
  event?.preventDefault()
  if (detail.href && detail.href !== route.path) {
    router.push(detail.href)
  }
}

// 顶栏品牌与全局工具
const topbarIdentity = computed<TopNavigationIdentity>(() => ({
  href: '/dashboard',
  title: site.appName || 'XrayPanel',
  logo: site.logo
    ? {
        src: site.logo,
        alt: site.appName || 'XrayPanel',
      }
    : undefined,
}))

const themeUtilityIcon = computed(() => {
  if (theme.mode === 'light') return 'sun'
  if (theme.mode === 'dark') return 'moon'
  return 'settings'
})

const topbarUtilities = computed<TopNavigationUtility[]>(() => {
  const list: TopNavigationUtility[] = [
    {
      type: 'button',
      iconName: themeUtilityIcon.value,
      ariaLabel: theme.modeTooltip,
      title: theme.modeTooltip,
      onClick: () => theme.toggle(),
    },
  ]
  if (auth.role === 'admin') {
    list.push({
      type: 'button',
      text: '管理后台',
      iconName: 'settings',
      title: '进入管理后台',
      onClick: () => router.push('/admin/dashboard'),
    })
  }
  return list
})

function goDashboard(_detail: { href: string }, event?: MouseEvent) {
  if (event && (event.metaKey || event.ctrlKey || event.shiftKey || event.altKey || event.button !== 0)) {
    return
  }
  event?.preventDefault()
  if (route.path !== '/dashboard') {
    router.push('/dashboard')
  }
}

// 面包屑
const breadcrumbs = computed<BreadcrumbItem[]>(() => {
  if (route.path === '/dashboard') {
    return [{ text: '用户中心', href: '/dashboard' }]
  }
  return [
    { text: '用户中心', href: '/dashboard' },
    { text: pageTitle.value, href: route.path },
  ]
})

// 顶栏高度同步（向下取整避免高分屏/缩放下与工具栏之间出现 1px 缝隙）
const TOOLBAR_HEIGHT = 42
const topbarRef = ref<{ $el?: HTMLElement } | null>(null)
const topbarHeight = ref(57)
let topbarObserver: ResizeObserver | null = null

const appHeaderHeight = computed(() =>
  isMobile.value ? topbarHeight.value : topbarHeight.value + TOOLBAR_HEIGHT,
)

function syncTopbarHeight() {
  const el = topbarRef.value?.$el as HTMLElement | undefined
  if (!el) return
  const rectHeight = Math.floor(el.getBoundingClientRect().height)
  if (rectHeight > 0) topbarHeight.value = rectHeight
}

// 响应式分级：< 768px 手机端走底部 Tab Bar
const mobileMq = typeof window !== 'undefined' ? window.matchMedia(MQ.tablet) : null

const isMobile = ref(mobileMq?.matches ?? false)

function onMobileMq(e: MediaQueryListEvent | MediaQueryList) {
  isMobile.value = e.matches
}

onMounted(() => {
  mobileMq?.addEventListener('change', onMobileMq)
  syncTopbarHeight()
  const topbarEl = topbarRef.value?.$el as HTMLElement | undefined
  if (topbarEl && typeof ResizeObserver !== 'undefined') {
    topbarObserver = new ResizeObserver(syncTopbarHeight)
    topbarObserver.observe(topbarEl)
  }
})

onUnmounted(() => {
  mobileMq?.removeEventListener('change', onMobileMq)
  topbarObserver?.disconnect()
  topbarObserver = null
})
</script>

<template>
  <div class="client-shell" :class="{ 'is-mobile': isMobile }">
    <CsTopNavigation
      ref="topbarRef"
      class="client-topbar"
      :identity="topbarIdentity"
      :utilities="topbarUtilities"
      @identity-follow="goDashboard"
    >
      <template #utility-extra>
        <UserAvatarDropdown current-portal="client" />
      </template>
    </CsTopNavigation>

    <CsAppToolbar
      v-if="!isMobile"
      :height="TOOLBAR_HEIGHT"
      :sticky-top="topbarHeight"
    >
      <CsBreadcrumbGroup v-if="breadcrumbs.length" :items="breadcrumbs" />
    </CsAppToolbar>

    <CsAppLayout
      :navigation-open="true"
      :navigation-width="236"
      :header-height="appHeaderHeight"
      :navigation-hide="isMobile"
      content-type="default"
      :max-content-width="1280"
      navigation-toggle="none"
      :reserve-navigation-toggle-row="false"
    >
      <template #navigation>
        <CsSideNavigation
          v-if="!isMobile"
          class="client-side-nav"
          :items="navItems"
          :active-href="route.path"
          @follow="onNavFollow"
        />
      </template>

      <router-view v-slot="{ Component }">
        <Transition name="fade" mode="out-in">
          <component :is="Component" />
        </Transition>
      </router-view>
    </CsAppLayout>

    <!-- 移动端底部固定导航栏 -->
    <nav v-if="isMobile" class="client-bottom-nav" aria-label="主导航">
      <router-link
        v-for="item in clientMenus"
        :key="item.path"
        :to="item.path"
        class="bottom-nav-item"
        :class="{ 'is-active': route.path === item.path }"
      >
        <CsIcon :name="item.iconName ?? 'ellipsis'" size="normal" />
        <span class="bottom-nav-label">{{ item.title }}</span>
      </router-link>
    </nav>
  </div>
</template>

<style lang="scss" scoped>
.client-shell {
  min-height: 100vh;
  background-color: var(--x-bg-page);

  &.is-mobile {
    padding-bottom: calc(60px + env(safe-area-inset-bottom, 0px));
  }
}

.client-topbar {
  position: sticky;
  top: 0;
  z-index: 900;
}

.client-bottom-nav {
  position: fixed;
  left: 0;
  right: 0;
  bottom: 0;
  z-index: 900;
  display: flex;
  align-items: stretch;
  justify-content: space-around;
  min-height: 56px;
  padding-bottom: env(safe-area-inset-bottom, 0px);
  background-color: var(--color-background-container-content-mjfil9);
  border-top: 1px solid var(--x-border);
}

.bottom-nav-item {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 3px;
  padding: 6px 4px;
  color: var(--x-text-secondary);
  text-decoration: none;
  font-size: 12px;
  font-weight: 500;
  transition: color 0.15s ease;

  &.is-active {
    color: var(--x-primary);
    font-weight: 600;
  }
}

.bottom-nav-label {
  line-height: 1.2;
}

.client-side-nav {
  --size-side-navigation-item-height-4tugsp: 40px;
  --space-side-navigation-item-icon-gap-0fty3a: 8px;
  --space-panel-content-top-ehvp15: 26px;
  --color-background-side-navigation-item-active-vz7d6m: var(--color-background-item-selected-dotkup);

  :deep(a) {
    align-items: center !important;
    padding-inline: 12px !important;
  }

  :deep([class*='item-icon']) {
    margin-inline-end: 10px !important;
  }

  :deep(a .cs-nav-root-link-text) {
    font-size: 14px;
    font-weight: 500;
    line-height: 20px;
  }

  :deep(a[aria-current='page'] .cs-nav-root-link-text) {
    font-weight: 700;
  }

  :deep(a .cs-nav-root-link-text--idle) {
    color: inherit;
  }
}

.fade-enter-active,
.fade-leave-active {
  transition: opacity 0.16s ease;
}

.fade-enter-from,
.fade-leave-to {
  opacity: 0;
}
</style>
