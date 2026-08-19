<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import { Setting, SwitchButton } from '@element-plus/icons-vue'
import { clientMenus } from '@/config/menu'
import { useAuthStore } from '@/stores/auth'
import { useSiteStore } from '@/stores/site'

const auth = useAuthStore()
const site = useSiteStore()
const router = useRouter()
const route = useRoute()

async function handleLogout() {
  try {
    await ElMessageBox.confirm('确认退出登录？', '提示', { type: 'warning' })
  } catch {
    return
  }
  await auth.logout()
  await router.replace('/login')
}
const activeMenu = () => route.path
</script>

<template>
  <div class="client-stage">
    <div class="client-app">
      <!-- 顶栏：移动端品牌栏，桌面端含顶部导航 -->
      <header class="client-header">
        <div class="client-logo">
          <img v-if="site.logo" :src="site.logo" class="client-logo-img" alt="logo" />
          <span v-else class="client-logo-mark">X</span>
          <span class="client-title">{{ site.appName || 'XrayPanel' }}</span>
        </div>

        <nav class="client-nav">
          <router-link
            v-for="item in clientMenus"
            :key="item.path"
            :to="item.path"
            class="client-nav-item"
            :class="{ active: activeMenu() === item.path }"
          >
            <el-icon :size="16"><component :is="item.icon" /></el-icon>
            <span>{{ item.title }}</span>
          </router-link>
        </nav>

        <div class="client-header-right">
          <!-- 管理员专属快捷按钮 -->
          <el-button
            v-if="auth.role === 'admin'"
            size="small"
            type="primary"
            plain
            class="admin-portal-btn"
            @click="router.push('/admin/dashboard')"
          >
            <el-icon :size="14"><Setting /></el-icon>&nbsp;管理后台
          </el-button>

          <el-dropdown trigger="click">
            <div class="client-avatar" title="用户菜单">{{ auth.avatarText }}</div>
            <template #dropdown>
              <el-dropdown-menu>
                <el-dropdown-item disabled>{{ auth.username }}（{{ auth.role === 'admin' ? '管理员' : '用户' }}）</el-dropdown-item>
                <el-dropdown-item v-if="auth.role === 'admin'" divided @click="router.push('/admin/dashboard')">
                  <el-icon><Setting /></el-icon>进入管理后台
                </el-dropdown-item>
                <el-dropdown-item divided @click="handleLogout">
                  <el-icon><SwitchButton /></el-icon>退出登录
                </el-dropdown-item>
              </el-dropdown-menu>
            </template>
          </el-dropdown>
        </div>
      </header>

      <main class="client-body">
        <router-view />
      </main>

      <!-- 底部 Tab（移动端固定在屏幕底部） -->
      <nav class="client-tabbar">
        <router-link
          v-for="item in clientMenus"
          :key="item.path"
          :to="item.path"
          class="client-tabbar-item"
          :class="{ active: activeMenu() === item.path }"
        >
          <el-icon :size="20"><component :is="item.icon" /></el-icon>
          <span>{{ item.title }}</span>
        </router-link>
      </nav>
    </div>
  </div>
</template>

<style scoped lang="scss">
.client-stage {
  background: var(--x-bg);
  min-height: 100vh;
  padding: 0;
}

.client-app {
  width: 100%;
  max-width: 100%;
  margin: 0 auto;
  background: var(--x-bg);
  min-height: 100vh;
  padding-bottom: calc(64px + env(safe-area-inset-bottom, 0px));
  position: relative;
  display: flex;
  flex-direction: column;
}

.client-header {
  position: sticky;
  top: 0;
  z-index: 100;
  background: rgba(255, 255, 255, 0.95);
  backdrop-filter: blur(12px);
  -webkit-backdrop-filter: blur(12px);
  border-bottom: 1px solid var(--x-border);
  padding: 12px 20px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.client-logo {
  display: flex;
  align-items: center;
  gap: 10px;
  font-weight: 800;
  font-size: 16px;
  color: var(--x-text);
}

.client-logo-mark {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  font-weight: 800;
  flex: none;
  box-shadow: 0 2px 8px rgba(99, 102, 241, 0.35);
}

.client-logo-img {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  object-fit: contain;
  background: rgba(0, 0, 0, 0.03);
  flex: none;
}

.client-nav {
  display: none;
  align-items: center;
  gap: 6px;
}

.client-nav-item {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 7px 16px;
  border-radius: var(--x-radius-sm);
  font-size: 13.5px;
  color: var(--x-text-2);
  font-weight: 500;
  transition: all 0.15s ease;
  &:hover,
  &.active {
    color: var(--x-primary);
    background: var(--x-primary-soft);
    font-weight: 600;
  }
}

.client-header-right {
  display: flex;
  align-items: center;
  gap: 12px;
}

.client-avatar {
  width: 34px;
  height: 34px;
  border-radius: 50%;
  background: linear-gradient(135deg, #6366f1, #8b5cf6);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
  box-shadow: 0 2px 6px rgba(99, 102, 241, 0.25);
  transition: transform 0.15s ease;
  &:hover {
    transform: scale(1.05);
  }
}

.client-body {
  flex: 1;
}

/* 移动端固定底部 Tabbar */
.client-tabbar {
  position: fixed;
  bottom: 0;
  left: 0;
  right: 0;
  width: 100%;
  height: 56px;
  background: rgba(255, 255, 255, 0.96);
  backdrop-filter: blur(16px);
  -webkit-backdrop-filter: blur(16px);
  border-top: 1px solid var(--x-border);
  display: flex;
  z-index: 1000;
  box-shadow: 0 -4px 16px rgba(0, 0, 0, 0.04);
  padding-bottom: env(safe-area-inset-bottom, 0px);
}

.client-tabbar-item {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 2px;
  padding: 6px 0;
  font-size: 11px;
  color: var(--x-text-3);
  font-weight: 500;
  transition: color 0.15s ease;
  &.active {
    color: var(--x-primary);
    font-weight: 700;
  }
}

/* 桌面端大屏适配 */
@media (min-width: 768px) {
  .client-stage {
    padding: 0;
    background: var(--x-bg);
  }
  .client-app {
    max-width: 1200px;
    min-height: 100vh;
    border-radius: 0;
    box-shadow: none;
    padding-bottom: 36px;
  }
  .client-nav {
    display: flex;
  }
  .client-tabbar {
    display: none;
  }
}
</style>
