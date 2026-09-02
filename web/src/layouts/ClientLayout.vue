<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Setting, Sunny, Moon, Monitor, Fold, Expand } from '@element-plus/icons-vue'
import { clientMenus } from '@/config/menu'
import { useAuthStore } from '@/stores/auth'
import { useSiteStore } from '@/stores/site'
import { useThemeStore } from '@/stores/theme'
import UserAvatarDropdown from '@/components/UserAvatarDropdown.vue'

const auth = useAuthStore()
const site = useSiteStore()
const theme = useThemeStore()
const router = useRouter()
const route = useRoute()

const pageTitle = computed(() => (route.meta.title as string) || '控制中心')

// 折叠侧边栏状态（true = 折叠/靠近展开远离收起模式；false = 固定展开模式）
// Pad 平板屏幕（< 1024px）默认折叠为 mini dock；宽屏可读取用户缓存
const defaultCollapsed = typeof window !== 'undefined' ? window.innerWidth < 1024 : false
const storedCollapsed = typeof localStorage !== 'undefined' ? localStorage.getItem('client_sidebar_collapsed') : null
const isCollapsed = ref(storedCollapsed !== null ? storedCollapsed === '1' : defaultCollapsed)

// 悬停展开状态（靠近展开，远离收起）
const isHovered = ref(false)
let hoverLeaveTimer: ReturnType<typeof setTimeout> | null = null

// 是否当前处于视觉展开状态（固定展开 OR 悬停展开）
const isEffectiveExpanded = computed(() => !isCollapsed.value || isHovered.value)

function toggleCollapse() {
  isCollapsed.value = !isCollapsed.value
  isHovered.value = false
  localStorage.setItem('client_sidebar_collapsed', isCollapsed.value ? '1' : '0')
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

// 全局快捷键监听 (Ctrl/Cmd + B 切换固定/折叠)
function handleGlobalKeydown(e: KeyboardEvent) {
  const isMac = navigator.platform.toUpperCase().indexOf('MAC') >= 0
  const isCtrlOrCmd = isMac ? e.metaKey : e.ctrlKey

  if (isCtrlOrCmd && e.key.toLowerCase() === 'b') {
    e.preventDefault()
    toggleCollapse()
  }
}

onMounted(() => {
  window.addEventListener('keydown', handleGlobalKeydown)
})

onUnmounted(() => {
  if (hoverLeaveTimer) clearTimeout(hoverLeaveTimer)
  window.removeEventListener('keydown', handleGlobalKeydown)
})
</script>

<template>
  <div class="client-layout" :class="{ 'is-collapsed': isCollapsed }">
    <!-- 桌面端与平板端侧边栏 (>= 768px 显示) -->
    <aside
      class="client-aside"
      :class="{
        collapsed: isCollapsed,
        'is-hovered': isHovered,
      }"
      @mouseenter="handleMouseEnter"
      @mouseleave="handleMouseLeave"
    >
      <!-- 侧边栏顶部品牌 -->
      <div class="client-brand">
        <div class="client-brand-main">
          <span v-if="site.logo" class="client-logo-img-wrap">
            <img :src="site.logo" class="client-logo-img" alt="logo" />
          </span>
          <span v-else class="client-logo-mark">X</span>
          <div v-if="isEffectiveExpanded" class="client-brand-info">
            <div class="client-brand-title">{{ site.appName || 'XrayPanel' }}</div>
            <div class="client-brand-sub">Client Portal</div>
          </div>
        </div>

        <!-- 固定/折叠切换按钮（仅在展开或 hover 浮层态时展示） -->
        <button
          v-if="isEffectiveExpanded"
          type="button"
          class="sidebar-pin-btn"
          :title="isCollapsed ? '固定展开侧栏 (Ctrl+B)' : '折叠收起侧栏 (Ctrl+B)'"
          @click.stop="toggleCollapse"
        >
          <el-icon :size="15"><Fold v-if="!isCollapsed" /><Expand v-else /></el-icon>
        </button>
      </div>

      <!-- 侧边栏主导航 -->
      <nav class="client-aside-nav">
        <router-link
          v-for="item in clientMenus"
          :key="item.path"
          :to="item.path"
          class="client-nav-pill"
          :class="{
            active: route.path === item.path,
            'is-mini': !isEffectiveExpanded,
          }"
          :title="!isEffectiveExpanded ? item.title : undefined"
        >
          <el-icon class="nav-icon" :size="18"><component :is="item.icon" /></el-icon>
          <span v-if="isEffectiveExpanded" class="nav-title">{{ item.title }}</span>
        </router-link>
      </nav>

      <!-- 侧边栏底部简明状态 -->
      <div class="client-aside-foot" :class="{ 'is-mini': !isEffectiveExpanded }">
        <div class="status-dot-wrap" :title="`${auth.username} · 在线`">
          <span class="status-pulse-dot" />
        </div>
        <span v-if="isEffectiveExpanded" class="foot-text">{{ site.appName || 'XrayPanel' }}</span>
      </div>
    </aside>

    <!-- 主工作区 -->
    <div class="client-main">
      <!-- 移动端顶部 Header (< 768px 出现) -->
      <header class="client-mobile-header">
        <div class="client-mobile-logo">
          <span v-if="site.logo" class="client-logo-img-wrap">
            <img :src="site.logo" class="client-logo-img" alt="logo" />
          </span>
          <span v-else class="client-logo-mark">X</span>
          <span class="client-title">{{ site.appName || 'XrayPanel' }}</span>
        </div>

        <div class="client-mobile-actions">
          <el-button
            circle
            size="small"
            class="theme-toggle-btn"
            :title="theme.modeTooltip"
            @click="theme.toggle()"
          >
            <el-icon :size="15">
              <Sunny v-if="theme.mode === 'light'" />
              <Moon v-else-if="theme.mode === 'dark'" />
              <Monitor v-else />
            </el-icon>
          </el-button>

          <el-button
            v-if="auth.role === 'admin'"
            size="small"
            plain
            class="switch-view-btn"
            title="进入管理后台"
            @click="router.push('/admin/dashboard')"
          >
            <el-icon :size="14"><Setting /></el-icon><span class="switch-view-text">&nbsp;管理后台</span>
          </el-button>

          <!-- 用户头像下拉菜单（统一组件） -->
          <UserAvatarDropdown current-portal="client" />
        </div>
      </header>

      <!-- 桌面端与平板端 Topbar (>= 768px 出现) -->
      <header class="client-desktop-topbar">
        <div class="topbar-left">
          <button
            v-if="isCollapsed"
            type="button"
            class="topbar-expand-btn"
            title="展开侧栏 (Ctrl+B)"
            @click="toggleCollapse"
          >
            <el-icon :size="16"><Expand /></el-icon>
          </button>
          <h1 class="page-title">{{ pageTitle }}</h1>
        </div>

        <div class="topbar-right">
          <!-- 亮色/深色/跟随系统 主题切换按钮 -->
          <el-button
            circle
            size="small"
            class="theme-toggle-btn"
            :title="theme.modeTooltip"
            @click="theme.toggle()"
          >
            <el-icon :size="15">
              <Sunny v-if="theme.mode === 'light'" />
              <Moon v-else-if="theme.mode === 'dark'" />
              <Monitor v-else />
            </el-icon>
          </el-button>

          <!-- 管理员快速切换至管理后台（组件样式与管理端一致） -->
          <el-button
            v-if="auth.role === 'admin'"
            size="small"
            plain
            class="switch-view-btn"
            title="进入管理后台"
            @click="router.push('/admin/dashboard')"
          >
            <el-icon :size="14"><Setting /></el-icon><span class="switch-view-text">&nbsp;管理后台</span>
          </el-button>

          <!-- 用户头像下拉菜单（桌面端与移动端完全对齐置于右上角，使用统一组件） -->
          <UserAvatarDropdown current-portal="client" />
        </div>
      </header>

      <!-- 页面主内容区域 (Fluid 自适应流式宽度) -->
      <main class="client-content">
        <div class="client-content-inner">
          <router-view />
        </div>
      </main>

      <!-- 移动端底部固定 Tabbar (< 768px 出现) -->
      <nav class="client-tabbar">
        <router-link
          v-for="item in clientMenus"
          :key="item.path"
          :to="item.path"
          class="client-tabbar-item"
          :class="{ active: route.path === item.path }"
        >
          <el-icon :size="20"><component :is="item.icon" /></el-icon>
          <span>{{ item.title }}</span>
        </router-link>
      </nav>
    </div>
  </div>
</template>

<style scoped lang="scss">
.client-layout {
  display: flex;
  min-height: 100vh;
  background: var(--x-bg);
  width: 100%;
}

/* ===== 左侧固定侧边栏 (>= 768px 平板与桌面端) ===== */
.client-aside {
  width: 236px;
  height: 100vh;
  position: fixed;
  top: 0;
  left: 0;
  background: var(--x-card);
  border-right: 1px solid var(--x-border);
  display: flex;
  flex-direction: column;
  z-index: 90;
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.02);
  transition: width 0.22s cubic-bezier(0.2, 0, 0, 1), background-color 0.24s ease, border-color 0.24s ease, box-shadow 0.24s ease;

  @media (max-width: 767.98px) {
    display: none;
  }

  /* 折叠收起模式 (Mini Dock 58px) */
  &.collapsed {
    width: 58px;

    /* 悬停浮层模式（靠近展开，远离收起） */
    &.is-hovered {
      width: 236px;
      z-index: 1000;
      box-shadow: var(--x-shadow-xl, 0 20px 25px -5px rgba(0, 0, 0, 0.1));
    }
  }
}

.client-brand {
  height: 60px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 14px;
  border-bottom: 1px solid var(--x-border);
  flex: none;
}

.client-brand-main {
  display: flex;
  align-items: center;
  gap: 10px;
  min-width: 0;
  flex: 1;
}

.client-logo-img-wrap {
  width: 30px;
  height: 30px;
  min-width: 30px;
  min-height: 30px;
  border-radius: 6px;
  overflow: hidden;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  background: rgba(0, 0, 0, 0.03);
  flex: none;
}

.client-logo-img {
  width: 100%;
  height: 100%;
  object-fit: cover;
  display: block;
}

.client-logo-mark {
  width: 30px;
  height: 30px;
  min-width: 30px;
  border-radius: 6px;
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
  color: #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  font-weight: 800;
  flex: none;
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.35);
}

.client-brand-info {
  min-width: 0;
  flex: 1;
}

.client-brand-title {
  font-size: 14.5px;
  font-weight: 700;
  color: var(--x-text);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  letter-spacing: -0.2px;
}

.client-brand-sub {
  font-size: 10.5px;
  color: var(--x-text-3);
  font-weight: 500;
  margin-top: 1px;
}

.sidebar-pin-btn {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  border: 1px solid var(--x-border);
  background: var(--x-card-soft);
  color: var(--x-text-2);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  outline: none;
  transition: all 0.15s ease;
  flex: none;

  &:hover {
    color: var(--x-primary);
    border-color: var(--x-primary);
    background: var(--x-primary-soft);
  }
}

/* 侧边主导航列表 */
.client-aside-nav {
  flex: 1;
  padding: 14px 8px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  overflow-y: auto;
}

.client-nav-pill {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 8px;
  color: var(--x-text-2);
  font-size: 13.5px;
  font-weight: 500;
  transition: all 0.15s cubic-bezier(0.2, 0, 0, 1);
  text-decoration: none;

  .nav-icon {
    color: var(--x-text-3);
    transition: color 0.15s ease;
    flex: none;
  }

  &:hover {
    background: var(--x-fill-2);
    color: var(--x-text);

    .nav-icon {
      color: var(--x-text);
    }
  }

  &.active {
    background: var(--x-primary-soft);
    color: var(--x-primary);
    font-weight: 600;

    .nav-icon {
      color: var(--x-primary);
    }
  }

  &.is-mini {
    justify-content: center;
    padding: 10px 0;
  }
}

/* 侧边栏底部简明状态 */
.client-aside-foot {
  height: 48px;
  padding: 0 14px;
  border-top: 1px solid var(--x-border);
  background: var(--x-card-soft);
  display: flex;
  align-items: center;
  gap: 8px;
  flex: none;
  user-select: none;

  &.is-mini {
    padding: 0;
    justify-content: center;
  }

  .status-dot-wrap {
    display: flex;
    align-items: center;
    justify-content: center;
  }

  .status-pulse-dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    background: #10b981;
    box-shadow: 0 0 0 2px rgba(16, 185, 129, 0.2);
  }

  .foot-text {
    font-size: 11.5px;
    color: var(--x-text-3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
}

/* ===== 右侧主工作区 ===== */
.client-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  min-height: 100vh;
  transition: margin-left 0.22s cubic-bezier(0.2, 0, 0, 1);

  @media (min-width: 768px) {
    margin-left: 236px;
  }

  @media (max-width: 767.98px) {
    margin-left: 0;
    padding-bottom: calc(60px + env(safe-area-inset-bottom, 0px));
  }
}

.client-layout.is-collapsed .client-main {
  @media (min-width: 768px) {
    margin-left: 58px;
  }
}

/* 桌面端与平板端 Topbar (>= 768px) */
.client-desktop-topbar {
  height: 56px;
  background: var(--x-card);
  border-bottom: 1px solid var(--x-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  position: sticky;
  top: 0;
  z-index: 80;
  transition: background-color 0.24s ease, border-color 0.24s ease;

  @media (max-width: 767.98px) {
    display: none;
  }

  .topbar-left {
    display: flex;
    align-items: center;
    gap: 10px;

    .topbar-expand-btn {
      width: 30px;
      height: 30px;
      border-radius: 6px;
      border: 1px solid var(--x-border);
      background: var(--x-card-soft);
      color: var(--x-text-2);
      display: flex;
      align-items: center;
      justify-content: center;
      cursor: pointer;
      outline: none;
      transition: all 0.15s ease;

      &:hover {
        color: var(--x-primary);
        border-color: var(--x-primary);
        background: var(--x-primary-soft);
      }
    }

    .page-title {
      font-size: 15px;
      font-weight: 700;
      color: var(--x-text);
      margin: 0;
    }
  }

  .topbar-right {
    display: flex;
    align-items: center;
    gap: 10px;
  }
}

.switch-view-btn {
  background: var(--x-card-soft, #f8fafc);
  border: 1px solid var(--x-border);
  color: var(--x-text);
  font-size: 12px;
  transition: all 0.15s ease;

  &:hover {
    color: var(--x-primary);
    border-color: var(--x-primary);
    background: var(--x-primary-soft);
  }
}

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

/* 移动端 Header (< 768px) */
.client-mobile-header {
  height: 52px;
  background: var(--x-card);
  border-bottom: 1px solid var(--x-border);
  padding: 0 14px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  position: sticky;
  top: 0;
  z-index: 80;

  @media (min-width: 768px) {
    display: none;
  }

  .client-mobile-logo {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
    max-width: calc(100vw - 150px);
    overflow: hidden;

    .client-title {
      font-size: 14.5px;
      font-weight: 800;
      color: var(--x-text);
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
  }

  .client-mobile-actions {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: none;
  }

  .client-avatar {
    width: 30px;
    height: 30px;
    border-radius: 50%;
    background: linear-gradient(135deg, #6366f1, #8b5cf6);
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: 12px;
    font-weight: 700;
    cursor: pointer;
    box-shadow: 0 1px 4px rgba(99, 102, 241, 0.25);
  }
}

/* 主内容容器 (Fluid 自适应流式容器) */
.client-content {
  flex: 1;
  padding: 20px 24px;

  @media (max-width: 767.98px) {
    padding: 12px 10px 20px;
  }
}

.client-content-inner {
  width: 100%;
  max-width: 1440px;
  margin: 0 auto;
}

/* 移动端固定底部 Tabbar (< 768px) */
.client-tabbar {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  width: 100%;
  height: 54px;
  background: var(--x-card);
  border-top: 1px solid var(--x-border);
  display: flex;
  z-index: 100;
  box-shadow: 0 -4px 16px rgba(0, 0, 0, 0.04);
  padding-bottom: env(safe-area-inset-bottom, 0px);
  transition: background-color 0.24s ease, border-color 0.24s ease;

  @media (min-width: 768px) {
    display: none;
  }
}

.client-tabbar-item {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  padding: 5px 0;
  font-size: 11px;
  color: var(--x-text-3);
  font-weight: 500;
  text-decoration: none;
  transition: color 0.15s ease;

  &.active {
    color: var(--x-primary);
    font-weight: 700;
  }
}
</style>

