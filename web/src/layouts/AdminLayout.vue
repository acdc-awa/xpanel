<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted, nextTick, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import { User, SwitchButton, Search, Sunny, Moon } from '@element-plus/icons-vue'
import { adminMenuGroups } from '@/config/menu'
import { useAuthStore } from '@/stores/auth'
import { useSiteStore } from '@/stores/site'
import { useThemeStore } from '@/stores/theme'

const auth = useAuthStore()
const site = useSiteStore()
const theme = useThemeStore()
const router = useRouter()
const route = useRoute()
const pageTitle = computed(() => (route.meta.title as string) ?? '')
const activeMenu = computed(() => route.path)

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

// 路由切换时，若处于悬停浮层模式，自动收起侧栏
watch(
  () => route.path,
  () => {
    if (isCollapsed.value) {
      isHovered.value = false
    }
  }
)

// 侧边栏搜索过滤 (Cloudflare 风格)
const searchQuery = ref('')
const searchInputRef = ref<HTMLInputElement | null>(null)

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

function handleSearchKeydown(e: KeyboardEvent) {
  if (e.key === 'Enter') {
    const firstGroup = filteredMenuGroups.value[0]
    if (!firstGroup) return
    if (!firstGroup.children || !firstGroup.children.length) {
      if (firstGroup.path) {
        router.push(firstGroup.path)
        searchQuery.value = ''
        if (isCollapsed.value) isHovered.value = false
      }
    } else if (firstGroup.children[0]?.path) {
      router.push(firstGroup.children[0].path)
      searchQuery.value = ''
      if (isCollapsed.value) isHovered.value = false
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

// 响应式：<900px 侧栏收进抽屉
const isMobile = ref(false)
const drawerOpen = ref(false)
let mq: MediaQueryList | null = null
const onMq = (e: MediaQueryListEvent | MediaQueryList) => {
  isMobile.value = e.matches
  if (!e.matches) drawerOpen.value = false
}
onMounted(() => {
  mq = window.matchMedia('(max-width: 900px)')
  onMq(mq)
  mq.addEventListener('change', onMq)
  window.addEventListener('keydown', handleGlobalKeydown)
})
onUnmounted(() => {
  if (hoverLeaveTimer) clearTimeout(hoverLeaveTimer)
  mq?.removeEventListener('change', onMq)
  window.removeEventListener('keydown', handleGlobalKeydown)
})

async function handleLogout() {
  try {
    await ElMessageBox.confirm('确认退出登录？', '提示', { type: 'warning' })
  } catch {
    return
  }
  await auth.logout()
  await router.replace('/login')
}
</script>

<template>
  <div class="admin-layout" :class="{ 'is-collapsed': isCollapsed && !isMobile }">
    <!-- 桌面端侧栏 (Cloudflare 风格：支持固定展开、靠近自动展开、远离自动收起) -->
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
      <!-- 顶部 Header: Logo + 品牌名 + 侧栏固定/折叠按钮 -->
      <div class="admin-aside-header">
        <div
          class="admin-logo"
          :title="!isEffectiveExpanded ? '展开侧边栏 (Ctrl+B)' : ''"
          @click="!isEffectiveExpanded ? toggleCollapse() : router.push('/admin/dashboard')"
        >
          <div class="admin-logo-icon-wrap">
            <img v-if="site.logo" :src="site.logo" class="admin-logo-img" alt="logo" />
            <span v-else class="admin-logo-mark">X</span>
          </div>
          <span class="admin-logo-text">{{ site.appName || 'Xray 管理面板' }}</span>
        </div>

        <!-- 顶部折叠/固定按钮（Cloudflare 专用面板切换图标） -->
        <button
          type="button"
          class="sidebar-toggle-btn"
          :title="isCollapsed ? '固定侧边栏 (Ctrl+B)' : '折叠侧边栏 (靠近可自动展开) (Ctrl+B)'"
          @click.stop="toggleCollapse"
        >
          <svg
            v-if="!isCollapsed"
            class="toggle-svg"
            viewBox="0 0 24 24"
            width="17"
            height="17"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <rect width="18" height="18" x="3" y="3" rx="2" />
            <path d="M9 3v18" />
            <path d="m14 9-3 3 3 3" />
          </svg>
          <svg
            v-else
            class="toggle-svg"
            viewBox="0 0 24 24"
            width="17"
            height="17"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <rect width="18" height="18" x="3" y="3" rx="2" />
            <path d="M9 3v18" />
            <path d="m12 9 3 3-3 3" />
          </svg>
        </button>
      </div>

      <!-- 侧边栏快捷搜索区 (Cloudflare 风格) -->
      <div class="sidebar-search-area">
        <div v-if="isEffectiveExpanded" class="sidebar-search-box">
          <el-icon class="search-icon"><Search /></el-icon>
          <input
            ref="searchInputRef"
            v-model="searchQuery"
            type="text"
            class="search-input"
            placeholder="搜索管理菜单..."
            @keydown="handleSearchKeydown"
          />
          <span v-if="!searchQuery" class="shortcut-badge">Ctrl K</span>
          <span v-else class="clear-search-btn" title="清除搜索" @click="searchQuery = ''">✕</span>
        </div>
        <button
          v-else
          type="button"
          class="sidebar-search-collapsed-btn"
          title="搜索菜单 (Ctrl+K)"
          @click="handleCollapsedSearchClick"
        >
          <el-icon :size="16"><Search /></el-icon>
        </button>
      </div>

      <!-- 分组折叠菜单 (Xboard 架构 + 丝滑动效) -->
      <el-menu
        :default-active="activeMenu"
        :collapse="!isEffectiveExpanded"
        :collapse-transition="false"
        :unique-opened="false"
        router
        class="admin-menu"
      >
        <template v-for="group in filteredMenuGroups" :key="group.title">
          <!-- 1. 单项直接跳转（如仪表盘） -->
          <el-menu-item v-if="!group.children || !group.children.length" :index="group.path || ''">
            <el-icon><component :is="group.icon" /></el-icon>
            <template #title>
              <span class="menu-item-text">{{ group.title }}</span>
            </template>
          </el-menu-item>

          <!-- 2. 分组子菜单（如节点管理、订阅财务、用户运营、系统管理） -->
          <el-sub-menu v-else :index="group.title" :popper-class="'admin-menu-popper'">
            <template #title>
              <el-icon><component :is="group.icon" /></el-icon>
              <span class="menu-item-text">{{ group.title }}</span>
            </template>
            <el-menu-item
              v-for="sub in group.children"
              :key="sub.path"
              :index="sub.path"
            >
              <el-icon v-if="sub.icon"><component :is="sub.icon" /></el-icon>
              <template #title>
                <span class="menu-item-text">{{ sub.title }}</span>
              </template>
            </el-menu-item>
          </el-sub-menu>
        </template>
      </el-menu>

      <!-- 底部版本信息 -->
      <div class="admin-aside-foot">
        <div class="status-dot-wrap" :title="`${auth.username} · 在线`">
          <span class="status-pulse-dot" />
        </div>
        <span class="foot-text">{{ auth.username }} · v1.0.0</span>
      </div>
    </aside>

    <!-- 移动端抽屉 (全量展开) -->
    <el-drawer v-model="drawerOpen" direction="ltr" size="236px" :with-header="false" class="admin-drawer">
      <div class="admin-drawer-header">
        <div class="admin-logo" @click="drawerOpen = false; router.push('/admin/dashboard')">
          <div class="admin-logo-icon-wrap">
            <img v-if="site.logo" :src="site.logo" class="admin-logo-img" alt="logo" />
            <span v-else class="admin-logo-mark">X</span>
          </div>
          <span class="admin-logo-text">{{ site.appName || 'Xray 管理面板' }}</span>
        </div>
      </div>
      <el-menu :default-active="activeMenu" router class="admin-menu mobile-menu">
        <template v-for="group in adminMenuGroups" :key="group.title">
          <el-menu-item v-if="!group.children || !group.children.length" :index="group.path || ''" @click="drawerOpen = false">
            <el-icon><component :is="group.icon" /></el-icon>
            <template #title>
              <span>{{ group.title }}</span>
            </template>
          </el-menu-item>
          <el-sub-menu v-else :index="group.title">
            <template #title>
              <el-icon><component :is="group.icon" /></el-icon>
              <span>{{ group.title }}</span>
            </template>
            <el-menu-item
              v-for="sub in group.children"
              :key="sub.path"
              :index="sub.path"
              @click="drawerOpen = false"
            >
              <el-icon v-if="sub.icon"><component :is="sub.icon" /></el-icon>
              <template #title>
                <span>{{ sub.title }}</span>
              </template>
            </el-menu-item>
          </el-sub-menu>
        </template>
      </el-menu>
      <div class="admin-aside-foot">
        <div class="status-dot-wrap">
          <span class="status-pulse-dot" />
        </div>
        <span class="foot-text">{{ auth.username }} · v1.0.0</span>
      </div>
    </el-drawer>

    <!-- 主工作区 -->
    <div class="admin-main" :class="{ collapsed: isCollapsed && !isMobile }">
      <header class="admin-topbar">
        <div class="admin-topbar-left">
          <el-button v-if="isMobile" text class="admin-hamburger" @click="drawerOpen = true">
            <el-icon :size="20"><svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="18" x2="21" y2="18"/></svg></el-icon>
          </el-button>
          <div class="admin-topbar-title">{{ pageTitle }}</div>
        </div>
        <div class="admin-topbar-right">
          <!-- 亮色/深色主题切换按钮 -->
          <el-button
            circle
            size="small"
            class="theme-toggle-btn"
            :title="theme.isDark ? '切换至浅色模式' : '切换至深色模式'"
            @click="theme.toggle()"
          >
            <el-icon :size="15">
              <Sunny v-if="theme.isDark" />
              <Moon v-else />
            </el-icon>
          </el-button>

          <!-- 切换至用户端前台视图快捷入口 -->
          <el-button
            size="small"
            plain
            class="switch-view-btn"
            title="切换至用户端视图"
            @click="router.push('/dashboard')"
          >
            <el-icon :size="14"><User /></el-icon><span class="switch-view-text">&nbsp;用户端视图</span>
          </el-button>

          <el-dropdown trigger="click" class="admin-user">
            <div class="admin-avatar">{{ auth.avatarText }}</div>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item disabled>{{ auth.username }}（{{ auth.role === 'admin' ? '管理员' : '用户' }}）</el-dropdown-item>
                <el-dropdown-item divided @click="router.push('/dashboard')">
                  <el-icon><User /></el-icon>切换至用户端（我的订阅）
                </el-dropdown-item>
                <el-dropdown-item divided @click="handleLogout">
                  <el-icon><SwitchButton /></el-icon>退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </header>
      <main class="admin-content">
        <router-view />
      </main>
    </div>
  </div>
</template>

<style scoped lang="scss">
.admin-layout {
  display: flex;
  min-height: 100vh;
  background: var(--x-bg);
}

/* 侧边栏主体 (Cloudflare 丝滑贝塞尔过渡与悬停浮层模式) */
.admin-aside {
  width: 236px;
  background: var(--x-card);
  border-right: 1px solid var(--x-border);
  position: fixed;
  top: 0;
  bottom: 0;
  left: 0;
  height: 100vh;
  display: flex;
  flex-direction: column;
  z-index: 100;
  transition: width 0.24s cubic-bezier(0.2, 0, 0, 1), box-shadow 0.24s ease;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.02);

  /* 折叠模式（默认 58px Dock，悬停时浮出展开） */
  &.collapsed {
    width: 58px;

    /* 悬停靠近自动浮层展开 */
    &.is-hovered {
      width: 236px;
      z-index: 200;
      box-shadow: 8px 0 32px rgba(0, 0, 0, 0.12);

      .admin-aside-header {
        padding: 0 12px 0 14px;
        justify-content: space-between;

        .admin-logo {
          display: flex;
        }

        .sidebar-toggle-btn {
          width: 30px;
          height: 30px;
          margin: 0;
        }
      }

      .admin-aside-foot {
        padding: 0 14px;
        justify-content: flex-start;

        .status-dot-wrap {
          margin-right: 8px;
        }

        .foot-text {
          opacity: 1;
          max-width: 150px;
          transform: translateX(0);
        }
      }
    }

    /* 远离收起状态（严格隐藏所有文字、指示箭头，居中展示图标） */
    &:not(.is-hovered) {
      .admin-aside-header {
        padding: 0;
        justify-content: center;

        .admin-logo {
          display: none;
        }

        .sidebar-toggle-btn {
          width: 38px;
          height: 38px;
          border-radius: 8px;
          margin: 0 auto;
        }
      }

      .admin-aside-foot {
        padding: 0;
        justify-content: center;

        .status-dot-wrap {
          margin-right: 0;
        }

        .foot-text {
          display: none !important;
          opacity: 0 !important;
          max-width: 0 !important;
          transform: translateX(-4px);
          overflow: hidden;
        }
      }

      /* 彻底隐藏任何露出来的折叠箭头与文本 */
      :deep(.el-sub-menu__icon-arrow),
      :deep(.el-icon.el-sub-menu__icon-arrow) {
        display: none !important;
        opacity: 0 !important;
        visibility: hidden !important;
        width: 0 !important;
        height: 0 !important;
        margin: 0 !important;
        padding: 0 !important;
        pointer-events: none !important;
      }

      :deep(.menu-item-text) {
        display: none !important;
        opacity: 0 !important;
      }
    }
  }
}

/* 顶部品牌与折叠按钮栏 */
.admin-aside-header {
  height: 56px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 12px 0 14px;
  border-bottom: 1px solid var(--x-border);
  flex: none;
  overflow: hidden;
  transition: padding 0.24s cubic-bezier(0.2, 0, 0, 1);
}

.admin-drawer-header {
  height: 56px;
  display: flex;
  align-items: center;
  padding: 0 14px;
  border-bottom: 1px solid var(--x-border);
  flex: none;
}

.admin-logo {
  display: flex;
  align-items: center;
  cursor: pointer;
  overflow: hidden;
  user-select: none;
  min-width: 0;
  flex: 1;
}

.admin-logo-icon-wrap {
  width: 30px;
  height: 30px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex: none;
  margin-right: 10px;
  transition: margin 0.24s cubic-bezier(0.2, 0, 0, 1);
}

.admin-logo-mark {
  width: 28px;
  height: 28px;
  border-radius: 7px;
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
  width: 28px;
  height: 28px;
  border-radius: 7px;
  object-fit: contain;
  background: rgba(0, 0, 0, 0.03);
}

.admin-logo-text {
  font-weight: 700;
  font-size: 14.5px;
  color: var(--x-text);
  white-space: nowrap;
  letter-spacing: -0.2px;
  opacity: 1;
  max-width: 140px;
  transform: translateX(0);
  transition: opacity 0.2s cubic-bezier(0.2, 0, 0, 1) 0.04s, transform 0.2s cubic-bezier(0.2, 0, 0, 1) 0.04s, max-width 0.24s cubic-bezier(0.2, 0, 0, 1);
}

/* 顶部折叠/展开图标按钮 (Cloudflare 风格) */
.sidebar-toggle-btn {
  width: 30px;
  height: 30px;
  border-radius: 6px;
  border: none;
  background: transparent;
  color: var(--x-text-3);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  outline: none;
  flex: none;
  transition: all 0.18s cubic-bezier(0.2, 0, 0, 1);

  .toggle-svg {
    transition: transform 0.2s ease;
  }

  &:hover {
    background: var(--x-fill-2);
    color: var(--x-primary);

    .toggle-svg {
      transform: scale(1.08);
    }
  }

  &:active {
    background: var(--x-fill-1);
    transform: scale(0.95);
  }
}

/* 侧边快捷搜索栏 (Cloudflare 风格) */
.sidebar-search-area {
  padding: 8px 10px 4px 10px;
  flex: none;
}

.sidebar-search-box {
  position: relative;
  display: flex;
  align-items: center;
  background: var(--x-card-soft);
  border: 1px solid var(--x-border);
  border-radius: 6px;
  height: 32px;
  padding: 0 8px;
  transition: all 0.18s cubic-bezier(0.2, 0, 0, 1);

  &:hover {
    background: var(--x-card-muted);
    border-color: var(--x-border-soft);
  }

  &:focus-within {
    background: var(--x-card);
    border-color: var(--x-primary);
    box-shadow: 0 0 0 2px rgba(99, 102, 241, 0.12);
  }

  .search-icon {
    color: var(--x-text-3);
    font-size: 13.5px;
    margin-right: 6px;
    flex: none;
  }

  .search-input {
    flex: 1;
    min-width: 0;
    border: none;
    background: transparent;
    font-size: 12.5px;
    color: var(--x-text);
    outline: none;
    padding: 0;

    &::placeholder {
      color: var(--x-text-3);
    }
  }

  .shortcut-badge {
    font-size: 10px;
    font-family: var(--x-font-mono, monospace);
    color: var(--x-text-3);
    background: var(--x-card);
    border: 1px solid var(--x-border);
    border-radius: 4px;
    padding: 1px 4px;
    line-height: 1.2;
    user-select: none;
    flex: none;
    box-shadow: var(--x-shadow);
  }

  .clear-search-btn {
    font-size: 11px;
    color: var(--x-text-3);
    cursor: pointer;
    padding: 2px 4px;
    border-radius: 50%;
    &:hover {
      color: #ef4444;
      background: var(--x-danger-soft);
    }
  }
}

.sidebar-search-collapsed-btn {
  width: 38px;
  height: 34px;
  border-radius: 8px;
  border: 1px solid transparent;
  background: transparent;
  color: var(--x-text-3);
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 0 auto;
  cursor: pointer;
  transition: all 0.18s cubic-bezier(0.2, 0, 0, 1);

  &:hover {
    background: var(--x-fill-2);
    color: var(--x-primary);
    border-color: var(--x-border);
  }
}

/* 侧边菜单区 */
.admin-menu {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  border-right: none !important;
  padding: 6px 8px;
  background: transparent !important;

  scrollbar-width: thin;
  scrollbar-color: var(--x-border) transparent;

  &::-webkit-scrollbar {
    width: 4px;
  }
  &::-webkit-scrollbar-thumb {
    background: var(--x-border);
    border-radius: 4px;
  }

  /* 展开状态下的菜单项 */
  &:not(.el-menu--collapse) {
    :deep(.el-menu-item),
    :deep(.el-sub-menu__title) {
      height: 38px;
      line-height: 38px;
      border-radius: 6px;
      margin-bottom: 2px;
      font-size: 13.5px;
      font-weight: 500;
      color: var(--x-text-2);
      padding: 0 10px !important;
      transition: background-color 0.15s ease, color 0.15s ease;
      display: flex;
      align-items: center;

      &:hover {
        background: var(--x-fill-2);
        color: var(--x-text);
      }

      .el-icon {
        width: 20px;
        height: 20px;
        min-width: 20px;
        font-size: 16px;
        margin-right: 10px;
        color: var(--x-text-3);
        display: flex;
        align-items: center;
        justify-content: center;
        transition: color 0.15s ease;
      }

      .menu-item-text {
        white-space: nowrap;
        overflow: hidden;
        text-overflow: ellipsis;
        transition: opacity 0.2s cubic-bezier(0.2, 0, 0, 1) 0.04s;
      }

      .el-sub-menu__icon-arrow {
        position: static !important;
        margin-top: 0 !important;
        margin-left: auto !important;
        margin-right: 0 !important;
        right: auto !important;
        font-size: 12px;
        color: var(--x-text-3);
        transition: transform 0.22s cubic-bezier(0.2, 0, 0, 1);
      }
    }

    /* 选中菜单项高亮 */
    :deep(.el-menu-item.is-active) {
      background: var(--x-primary-soft) !important;
      color: var(--x-primary) !important;
      font-weight: 600;

      .el-icon {
        color: var(--x-primary) !important;
      }
    }

    /* 展开状态下的二级子菜单导轨 (Cloudflare 垂直指示线) */
    :deep(.el-sub-menu .el-menu) {
      background: transparent;
      padding: 2px 0 2px 14px;
      position: relative;

      &::before {
        content: '';
        position: absolute;
        left: 19px;
        top: 4px;
        bottom: 4px;
        width: 1.5px;
        background: var(--x-border);
        border-radius: 1px;
      }

      .el-menu-item {
        height: 34px;
        line-height: 34px;
        font-size: 13px;
        padding-left: 16px !important;
        margin-bottom: 1px;
        color: var(--x-text-2);

        &:hover {
          background: var(--x-fill-2);
          color: var(--x-text);
        }

        &.is-active {
          background: var(--x-primary-soft) !important;
          color: var(--x-primary) !important;
        }
      }
    }
  }

  /* 折叠状态下的像素级居中与无缝收缩 */
  &.el-menu--collapse {
    width: 58px !important;
    padding: 6px 0 !important;

    :deep(.el-sub-menu__icon-arrow) {
      display: none !important;
      opacity: 0 !important;
      visibility: hidden !important;
      width: 0 !important;
      height: 0 !important;
    }

    :deep(.el-menu-item),
    :deep(.el-sub-menu__title) {
      width: 40px !important;
      height: 38px !important;
      line-height: 38px !important;
      margin: 3px auto !important;
      padding: 0 !important;
      border-radius: 8px;
      display: flex !important;
      align-items: center !important;
      justify-content: center !important;
      text-align: center !important;
      color: var(--x-text-2);
      transition: all 0.15s ease;

      &:hover {
        background: var(--x-fill-2);
        color: var(--x-text);
      }

      &.is-active {
        background: var(--x-primary-soft) !important;
        color: var(--x-primary) !important;

        .el-icon {
          color: var(--x-primary) !important;
        }
      }

      .el-icon {
        margin: 0 !important;
        width: 20px !important;
        height: 20px !important;
        min-width: 20px !important;
        font-size: 17px !important;
        display: flex !important;
        align-items: center !important;
        justify-content: center !important;
        color: var(--x-text-3);
      }

      .el-tooltip__trigger {
        width: 100% !important;
        height: 100% !important;
        padding: 0 !important;
        display: flex !important;
        align-items: center !important;
        justify-content: center !important;
      }
    }
  }
}

/* 底部状态栏 */
.admin-aside-foot {
  height: 42px;
  padding: 0 14px;
  border-top: 1px solid var(--x-border);
  color: var(--x-text-3);
  font-size: 12px;
  display: flex;
  align-items: center;
  flex: none;
  background: var(--x-card-soft);
  user-select: none;
  overflow: hidden;
  transition: padding 0.24s cubic-bezier(0.2, 0, 0, 1);
}

.status-dot-wrap {
  width: 24px;
  height: 24px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex: none;
  margin-right: 8px;
  transition: margin 0.24s cubic-bezier(0.2, 0, 0, 1);
}

.status-pulse-dot {
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: #10b981;
  box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.2);
  flex: none;
}

.foot-text {
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  opacity: 1;
  max-width: 150px;
  transform: translateX(0);
  transition: opacity 0.2s cubic-bezier(0.2, 0, 0, 1) 0.04s, transform 0.2s cubic-bezier(0.2, 0, 0, 1) 0.04s, max-width 0.24s cubic-bezier(0.2, 0, 0, 1);
}

/* 主内容区自适应过渡（折叠模式下固定为 58px margin，不因悬停抖动页面布局） */
.admin-main {
  flex: 1;
  margin-left: 236px;
  min-width: 0;
  display: flex;
  flex-direction: column;
  transition: margin-left 0.24s cubic-bezier(0.2, 0, 0, 1);

  &.collapsed {
    margin-left: 58px;
  }
}

.admin-topbar {
  height: 56px;
  background: var(--x-card);
  border-bottom: 1px solid var(--x-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  position: sticky;
  top: 0;
  z-index: 10;
  transition: background-color 0.24s ease, border-color 0.24s ease;
}
.admin-topbar-left {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  flex: 1;
}
.admin-topbar-title {
  font-size: 15.5px;
  font-weight: 600;
  color: var(--x-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.admin-topbar-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}
.admin-hamburger { color: var(--x-text); flex-shrink: 0; padding: 0 4px; }

.theme-toggle-btn {
  background: var(--x-card-soft, #f8fafc);
  border: 1px solid var(--x-border);
  color: var(--x-text-2);
  transition: all 0.2s cubic-bezier(0.2, 0, 0, 1);

  &:hover {
    color: var(--x-primary);
    border-color: var(--x-primary);
    background: var(--x-primary-soft);
    transform: rotate(18deg) scale(1.05);
  }
}

.switch-view-btn {
  background: var(--x-card-soft, #f8fafc);
  border: 1px solid var(--x-border);
  color: var(--x-text);
  &:hover {
    color: var(--x-primary);
    border-color: var(--x-primary);
  }
}

.admin-avatar {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: linear-gradient(135deg, #6366f1, #a855f7);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 600;
  cursor: pointer;
}
.admin-content { flex: 1; padding: 20px 24px; }

:deep(.admin-drawer) {
  width: 236px !important;
  max-width: 75vw !important;
  box-shadow: 4px 0 24px rgba(0, 0, 0, 0.15) !important;

  .el-drawer__body {
    padding: 0;
    background: var(--x-card);
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
  }
}

@media (max-width: 900px) {
  .admin-main {
    margin-left: 0 !important;
  }
  .admin-topbar {
    padding: 0 12px;
    height: 52px;
  }
  .admin-content {
    padding: 14px;
  }
}

@media (max-width: 768px) {
  .switch-view-btn {
    padding: 0 8px;
    .switch-view-text {
      display: none !important;
    }
  }
}
</style>

<!-- 全局浮动 Popper 样式（Mini 折叠模式下的悬停子菜单） -->
<style lang="scss">
.el-popper.admin-menu-popper {
  border-radius: 8px !important;
  border: 1px solid var(--x-border) !important;
  box-shadow: var(--x-shadow-lg) !important;
  padding: 5px !important;
  background: var(--x-card) !important;

  .el-menu--popup {
    min-width: 148px;
    padding: 0;
    background: transparent !important;

    .el-menu-item {
      height: 34px;
      line-height: 34px;
      border-radius: 6px;
      font-size: 13px;
      color: var(--x-text-2);
      margin-bottom: 2px;
      padding: 0 10px;

      &:hover {
        background: var(--x-fill-2);
        color: var(--x-text);
      }

      &.is-active {
        background: var(--x-primary-soft) !important;
        color: var(--x-primary) !important;
        font-weight: 600;
      }
    }
  }
}
</style>
