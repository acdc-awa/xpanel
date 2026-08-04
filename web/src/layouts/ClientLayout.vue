<script setup lang="ts">
import { useRoute, useRouter } from 'vue-router'
import { ElMessageBox } from 'element-plus'
import { clientMenus } from '@/config/menu'
import { useAuthStore } from '@/stores/auth'

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()

async function handleLogout() {
  await ElMessageBox.confirm('确认退出登录？', '提示', { type: 'warning' })
  auth.logout()
  router.replace('/login')
}
const activeMenu = () => route.path
</script>

<template>
  <div class="client-stage">
    <div class="client-app">
      <!-- 顶栏：移动端品牌栏，桌面端含顶部导航 -->
      <header class="client-header">
        <div class="client-logo"><span class="client-logo-mark">X</span><span>我的机场</span></div>

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
          <el-button text circle>
            <el-icon :size="17"><svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/></svg></el-icon>
          </el-button>
          <el-dropdown trigger="click">
  <div class="client-avatar">{{ auth.avatarText }}</div>
  <template #dropdown>
    <el-dropdown-menu>
      <el-dropdown-item disabled>{{ auth.username }}（{{ auth.role === 'admin' ? '管理员' : '用户' }}）</el-dropdown-item>
      <el-dropdown-item divided @click="handleLogout">退出登录</el-dropdown-item>
    </el-dropdown-menu>
  </template>
</el-dropdown>
        </div>
      </header>

      <main class="client-body">
        <router-view />
      </main>

      <!-- 底部 Tab（仅移动端） -->
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
.client-stage { background: #e9ebf5; min-height: 100vh; padding: 24px 0; }

.client-app {
  max-width: 520px;
  margin: 0 auto;
  background: var(--x-bg);
  min-height: calc(100vh - 48px);
  border-radius: 16px;
  box-shadow: var(--x-shadow-lg);
  position: relative;
  padding-bottom: 78px;
  overflow: hidden;
  display: flex;
  flex-direction: column;
}

.client-header {
  position: sticky;
  top: 0;
  z-index: 10;
  background: var(--x-card);
  border-bottom: 1px solid var(--x-border);
  padding: 14px 18px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.client-logo { display: flex; align-items: center; gap: 8px; font-weight: 700; font-size: 15px; }
.client-logo-mark {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: linear-gradient(135deg, #6366f1, #a855f7);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  flex: none;
}
.client-nav { display: none; align-items: center; gap: 4px; }
.client-nav-item {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 7px 14px;
  border-radius: 8px;
  font-size: 13.5px;
  color: var(--x-text-2);
  font-weight: 500;
  &:hover, &.active { color: var(--x-primary); background: var(--x-primary-soft); }
}
.client-header-right { display: flex; align-items: center; gap: 12px; }
.client-avatar {
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
}
.client-body { flex: 1; }

.client-tabbar {
  position: absolute;
  bottom: 0;
  left: 0;
  right: 0;
  background: var(--x-card);
  border-top: 1px solid var(--x-border);
  display: flex;
  z-index: 10;
}
.client-tabbar-item {
  flex: 1;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 3px;
  padding: 9px 0 10px;
  font-size: 11px;
  color: var(--x-text-3);
  &.active { color: var(--x-primary); }
}

@media (min-width: 768px) {
  .client-stage { padding: 0; }
  .client-app {
    max-width: 1200px;
    min-height: 100vh;
    border-radius: 0;
    box-shadow: none;
    padding-bottom: 0;
  }
  .client-nav { display: flex; }
  .client-tabbar { display: none; }
}
</style>
