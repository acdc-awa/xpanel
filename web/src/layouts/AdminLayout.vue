<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import { User, SwitchButton, DArrowLeft, DArrowRight } from '@element-plus/icons-vue'
import { adminMenuGroups } from '@/config/menu'
import { useAuthStore } from '@/stores/auth'
import { useSiteStore } from '@/stores/site'

const auth = useAuthStore()
const site = useSiteStore()
const router = useRouter()
const route = useRoute()
const pageTitle = computed(() => (route.meta.title as string) ?? '')
const activeMenu = computed(() => route.path)

// 折叠侧边栏状态（从 localStorage 读取持久化配置）
const isCollapsed = ref(localStorage.getItem('admin_sidebar_collapsed') === '1')

function toggleCollapse() {
  isCollapsed.value = !isCollapsed.value
  localStorage.setItem('admin_sidebar_collapsed', isCollapsed.value ? '1' : '0')
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
})
onUnmounted(() => mq?.removeEventListener('change', onMq))

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
    <!-- 桌面端侧栏 -->
    <aside v-if="!isMobile" class="admin-aside" :class="{ collapsed: isCollapsed }">
      <!-- 顶部 Logo 与站点品牌 -->
      <div class="admin-logo" @click="router.push('/admin/dashboard')">
        <div class="admin-logo-icon-wrap">
          <img v-if="site.logo" :src="site.logo" class="admin-logo-img" alt="logo" />
          <span v-else class="admin-logo-mark">X</span>
        </div>
        <span class="admin-logo-text">{{ site.appName || 'Xray 管理面板' }}</span>
      </div>

      <!-- 分组折叠菜单 (Xboard 架构) -->
      <el-menu
        :default-active="activeMenu"
        :collapse="isCollapsed"
        :collapse-transition="false"
        :unique-opened="false"
        router
        class="admin-menu"
      >
        <template v-for="group in adminMenuGroups" :key="group.title">
          <!-- 1. 单项直接跳转（如仪表盘） -->
          <el-menu-item v-if="!group.children || !group.children.length" :index="group.path || ''">
            <el-icon><component :is="group.icon" /></el-icon>
            <template #title>
              <span>{{ group.title }}</span>
            </template>
          </el-menu-item>

          <!-- 2. 分组子菜单（如节点管理、订阅财务、用户运营、系统管理） -->
          <el-sub-menu v-else :index="group.title" :popper-class="'admin-menu-popper'">
            <template #title>
              <el-icon><component :is="group.icon" /></el-icon>
              <span>{{ group.title }}</span>
            </template>
            <el-menu-item
              v-for="sub in group.children"
              :key="sub.path"
              :index="sub.path"
            >
              <el-icon v-if="sub.icon"><component :is="sub.icon" /></el-icon>
              <template #title>
                <span>{{ sub.title }}</span>
              </template>
            </el-menu-item>
          </el-sub-menu>
        </template>
      </el-menu>

      <!-- 悬浮折叠/展开圆形触发器 (Xboard 核心交互) -->
      <button
        type="button"
        class="sidebar-floating-toggle"
        :title="isCollapsed ? '展开侧边栏' : '折叠侧边栏'"
        @click="toggleCollapse"
      >
        <el-icon :size="12">
          <DArrowRight v-if="isCollapsed" />
          <DArrowLeft v-else />
        </el-icon>
      </button>

      <!-- 底部版本信息 -->
      <div class="admin-aside-foot">
        <div class="status-dot-wrap">
          <span class="status-pulse-dot" />
        </div>
        <span class="foot-text">{{ auth.username }} · v1.0.0</span>
      </div>
    </aside>

    <!-- 移动端抽屉 (全量展开) -->
    <el-drawer v-model="drawerOpen" direction="ltr" :size="250" :with-header="false" class="admin-drawer">
      <div class="admin-logo" @click="drawerOpen = false; router.push('/admin/dashboard')">
        <div class="admin-logo-icon-wrap">
          <img v-if="site.logo" :src="site.logo" class="admin-logo-img" alt="logo" />
          <span v-else class="admin-logo-mark">X</span>
        </div>
        <span class="admin-logo-text">{{ site.appName || 'Xray 管理面板' }}</span>
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
          <!-- 切换至用户端前台视图快捷入口 -->
          <el-button
            size="small"
            plain
            class="switch-view-btn"
            @click="router.push('/dashboard')"
          >
            <el-icon :size="14"><User /></el-icon>&nbsp;用户端视图
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

/* 侧边栏主体 */
.admin-aside {
  width: 228px;
  background: #ffffff;
  border-right: 1px solid var(--x-border);
  position: fixed;
  top: 0;
  bottom: 0;
  left: 0;
  height: 100vh;
  display: flex;
  flex-direction: column;
  z-index: 100;
  transition: width 0.22s cubic-bezier(0.25, 0.1, 0.25, 1);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.02);

  &.collapsed {
    width: 64px;

    .admin-logo {
      padding: 0 16px;
      .admin-logo-icon-wrap {
        margin-right: 0;
      }
      .admin-logo-text {
        opacity: 0;
        max-width: 0;
        overflow: hidden;
      }
    }

    .admin-aside-foot {
      padding: 0 16px;
      .status-dot-wrap {
        margin-right: 0;
      }
      .foot-text {
        opacity: 0;
        max-width: 0;
        overflow: hidden;
      }
    }
  }
}

/* 顶部 Logo */
.admin-logo {
  height: 58px;
  display: flex;
  align-items: center;
  padding: 0 16px;
  border-bottom: 1px solid var(--x-border);
  flex: none;
  cursor: pointer;
  overflow: hidden;
  user-select: none;
  transition: padding 0.22s cubic-bezier(0.25, 0.1, 0.25, 1);
}
.admin-logo-icon-wrap {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex: none;
  margin-right: 10px;
  transition: margin 0.22s cubic-bezier(0.25, 0.1, 0.25, 1);
}
.admin-logo-mark {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  background: linear-gradient(135deg, #6366f1, #a855f7);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 15px;
  font-weight: 700;
}
.admin-logo-img {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  object-fit: contain;
  background: rgba(0, 0, 0, 0.03);
}
.admin-logo-text {
  font-weight: 700;
  font-size: 15px;
  color: var(--x-text);
  white-space: nowrap;
  letter-spacing: -0.2px;
  opacity: 1;
  max-width: 150px;
  transition: opacity 0.18s ease, max-width 0.22s cubic-bezier(0.25, 0.1, 0.25, 1);
}

/* 侧边菜单区 */
.admin-menu {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
  border-right: none !important;
  padding: 10px 6px;
  background: transparent !important;

  scrollbar-width: thin;
  scrollbar-color: rgba(0, 0, 0, 0.15) transparent;

  &::-webkit-scrollbar {
    width: 4px;
  }
  &::-webkit-scrollbar-thumb {
    background: rgba(0, 0, 0, 0.12);
    border-radius: 4px;
  }

  /* 展开状态下的基础菜单项 */
  &:not(.el-menu--collapse) {
    :deep(.el-menu-item),
    :deep(.el-sub-menu__title) {
      height: 40px;
      line-height: 40px;
      border-radius: 8px;
      margin-bottom: 3px;
      font-size: 13.5px;
      font-weight: 500;
      color: #475569;
      padding: 0 10px !important;
      transition: background-color 0.15s ease, color 0.15s ease;
      display: flex;
      align-items: center;

      &:hover {
        background: #f1f5f9;
        color: #0f172a;
      }

      .el-icon {
        width: 24px;
        height: 24px;
        min-width: 24px;
        font-size: 17px;
        margin-right: 10px;
        color: #64748b;
        display: flex;
        align-items: center;
        justify-content: center;
      }

      .el-sub-menu__icon-arrow {
        position: static !important;
        margin-top: 0 !important;
        margin-left: auto !important;
        margin-right: 0 !important;
        right: auto !important;
      }
    }

    /* 选中项高亮 */
    :deep(.el-menu-item.is-active) {
      background: var(--x-primary-soft, #eef2ff) !important;
      color: var(--x-primary, #6366f1) !important;
      font-weight: 600;

      .el-icon {
        color: var(--x-primary, #6366f1) !important;
      }
    }

    /* 展开状态下的二级子菜单导轨 (Xboard 垂直指示线) */
    :deep(.el-sub-menu .el-menu) {
      background: transparent;
      padding: 2px 0 4px 18px;
      position: relative;

      &::before {
        content: '';
        position: absolute;
        left: 21px;
        top: 6px;
        bottom: 6px;
        width: 1.5px;
        background: #e2e8f0;
        border-radius: 1px;
      }

      .el-menu-item {
        height: 36px;
        line-height: 36px;
        font-size: 13px;
        padding-left: 18px !important;
        margin-bottom: 2px;
      }
    }
  }

  /* 折叠状态下的像素级绝对居中（解决对齐与瞬移） */
  &.el-menu--collapse {
    width: 64px !important;
    padding: 10px 0 !important;

    :deep(.el-sub-menu__icon-arrow) {
      display: none !important;
    }

    :deep(.el-menu-item),
    :deep(.el-sub-menu__title) {
      width: 44px !important;
      height: 40px !important;
      line-height: 40px !important;
      margin: 4px auto !important;
      padding: 0 !important;
      border-radius: 8px;
      display: flex !important;
      align-items: center !important;
      justify-content: center !important;
      text-align: center !important;

      &:hover {
        background: #f1f5f9;
      }

      &.is-active {
        background: var(--x-primary-soft, #eef2ff) !important;
        color: var(--x-primary, #6366f1) !important;

        .el-icon {
          color: var(--x-primary, #6366f1) !important;
        }
      }

      .el-icon {
        margin: 0 !important;
        width: 24px !important;
        height: 24px !important;
        min-width: 24px !important;
        font-size: 18px !important;
        display: flex !important;
        align-items: center !important;
        justify-content: center !important;
      }

      .el-tooltip__trigger {
        width: 100% !important;
        height: 100% !important;
        padding: 0 !important;
        display: flex !important;
        align-items: center !important;
        justify-content: center !important;
      }

      .el-sub-menu__icon-arrow {
        display: none !important;
      }
    }
  }
}

/* 悬浮折叠/展开圆形触发器 (Xboard 核心交互) */
.sidebar-floating-toggle {
  position: absolute;
  top: 50%;
  right: -13px;
  transform: translateY(-50%);
  width: 26px;
  height: 26px;
  border-radius: 50%;
  background: #ffffff;
  border: 1px solid #e2e8f0;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
  color: #64748b;
  z-index: 110;
  outline: none;
  transition: all 0.2s cubic-bezier(0.25, 0.1, 0.25, 1);

  &:hover {
    color: var(--x-primary);
    border-color: var(--x-primary);
    box-shadow: 0 4px 12px rgba(99, 102, 241, 0.25);
    transform: translateY(-50%) scale(1.1);
  }
}

/* 底部状态栏 */
.admin-aside-foot {
  height: 42px;
  padding: 0 16px;
  border-top: 1px solid var(--x-border);
  color: #94a3b8;
  font-size: 12px;
  display: flex;
  align-items: center;
  flex: none;
  background: #fafbfc;
  user-select: none;
  overflow: hidden;
  transition: padding 0.22s cubic-bezier(0.25, 0.1, 0.25, 1);
}
.status-dot-wrap {
  width: 32px;
  height: 32px;
  display: flex;
  align-items: center;
  justify-content: center;
  flex: none;
  margin-right: 10px;
  transition: margin 0.22s cubic-bezier(0.25, 0.1, 0.25, 1);
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
  transition: opacity 0.18s ease, max-width 0.22s cubic-bezier(0.25, 0.1, 0.25, 1);
}

/* 主内容区自适应 */
.admin-main {
  flex: 1;
  margin-left: 228px;
  min-width: 0;
  display: flex;
  flex-direction: column;
  transition: margin-left 0.22s cubic-bezier(0.25, 0.1, 0.25, 1);

  &.collapsed {
    margin-left: 64px;
  }
}

.admin-topbar {
  height: 58px;
  background: #ffffff;
  border-bottom: 1px solid var(--x-border);
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 24px;
  position: sticky;
  top: 0;
  z-index: 10;
}
.admin-topbar-left { display: flex; align-items: center; gap: 12px; }
.admin-topbar-title { font-size: 16px; font-weight: 600; color: var(--x-text); }
.admin-topbar-right { display: flex; align-items: center; gap: 8px; }
.admin-hamburger { color: var(--x-text); }
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
  .el-drawer__body {
    padding: 0;
    background: #ffffff;
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
    padding: 0 14px;
    height: 54px;
  }
  .admin-content {
    padding: 14px;
  }
}
</style>

<!-- 全局浮动 Popper 样式（Mini 折叠模式下的悬停子菜单） -->
<style lang="scss">
.el-popper.admin-menu-popper {
  border-radius: 10px !important;
  border: 1px solid #e2e8f0 !important;
  box-shadow: 0 10px 25px -5px rgba(0, 0, 0, 0.1), 0 8px 10px -6px rgba(0, 0, 0, 0.08) !important;
  padding: 6px !important;
  background: #ffffff !important;

  .el-menu--popup {
    min-width: 150px;
    padding: 0;
    background: transparent !important;

    .el-menu-item {
      height: 36px;
      line-height: 36px;
      border-radius: 6px;
      font-size: 13px;
      color: #475569;
      margin-bottom: 2px;
      padding: 0 12px;

      &:hover {
        background: #f1f5f9;
        color: #0f172a;
      }

      &.is-active {
        background: #eef2ff !important;
        color: #6366f1 !important;
        font-weight: 600;
      }
    }
  }
}
</style>
