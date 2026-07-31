<script setup lang="ts">
import { ref, computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import { adminMenus } from '@/config/menu'

const route = useRoute()
const pageTitle = computed(() => (route.meta.title as string) ?? '')
const activeMenu = computed(() => route.path)

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
</script>

<template>
  <div class="admin-layout">
    <!-- 桌面端侧栏 -->
    <aside v-if="!isMobile" class="admin-aside">
      <div class="admin-logo"><span class="admin-logo-mark">X</span><span>Xray 管理面板</span></div>
      <el-menu :default-active="activeMenu" router class="admin-menu">
        <el-menu-item v-for="item in adminMenus" :key="item.path" :index="item.path">
          <el-icon><component :is="item.icon" /></el-icon>
          <span>{{ item.title }}</span>
        </el-menu-item>
      </el-menu>
      <div class="admin-aside-foot">v0.1 初版</div>
    </aside>

    <!-- 移动端抽屉 -->
    <el-drawer v-model="drawerOpen" direction="ltr" :size="240" :with-header="false" class="admin-drawer">
      <div class="admin-logo"><span class="admin-logo-mark">X</span><span>Xray 管理面板</span></div>
      <el-menu :default-active="activeMenu" router class="admin-menu">
        <el-menu-item v-for="item in adminMenus" :key="item.path" :index="item.path" @click="drawerOpen = false">
          <el-icon><component :is="item.icon" /></el-icon>
          <span>{{ item.title }}</span>
        </el-menu-item>
      </el-menu>
    </el-drawer>

    <div class="admin-main">
      <header class="admin-topbar">
        <div class="admin-topbar-left">
          <el-button v-if="isMobile" text class="admin-hamburger" @click="drawerOpen = true">
            <el-icon :size="20"><svg viewBox="0 0 24 24" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><line x1="3" y1="6" x2="21" y2="6"/><line x1="3" y1="12" x2="21" y2="12"/><line x1="3" y1="18" x2="21" y2="18"/></svg></el-icon>
          </el-button>
          <div class="admin-topbar-title">{{ pageTitle }}</div>
        </div>
        <div class="admin-topbar-right">
          <el-button text circle><el-icon :size="17"><svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="23 4 23 10 17 10"/><polyline points="1 20 1 14 7 14"/><path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"/></svg></el-icon></el-button>
          <el-button text circle><el-icon :size="17"><svg viewBox="0 0 24 24" width="17" height="17" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/></svg></el-icon></el-button>
          <div class="admin-avatar">管</div>
        </div>
      </header>
      <main class="admin-content">
        <router-view />
      </main>
    </div>
  </div>
</template>

<style scoped lang="scss">
.admin-layout { display: flex; min-height: 100vh; }

.admin-aside {
  width: 224px;
  background: var(--x-sidebar-bg);
  color: #fff;
  position: fixed;
  top: 0;
  bottom: 0;
  left: 0;
  display: flex;
  flex-direction: column;
  z-index: 20;
}
.admin-logo {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 18px 20px;
  font-weight: 700;
  font-size: 16px;
  border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  color: #fff;
}
.admin-logo-mark {
  width: 30px;
  height: 30px;
  border-radius: 8px;
  background: linear-gradient(135deg, #6366f1, #a855f7);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 15px;
  flex: none;
}
.admin-menu {
  flex: 1;
  border-right: none;
  padding: 10px 12px;
  --el-menu-bg-color: transparent;
  --el-menu-text-color: #a0a5bd;
  --el-menu-active-color: #fff;
  --el-menu-hover-bg-color: rgba(255, 255, 255, 0.06);
  .el-menu-item {
    border-radius: 8px;
    margin-bottom: 2px;
    &.is-active { background: var(--x-primary); }
  }
}
.admin-aside-foot {
  padding: 14px 16px;
  border-top: 1px solid rgba(255, 255, 255, 0.06);
  color: #a0a5bd;
  font-size: 12px;
}

.admin-main { flex: 1; margin-left: 224px; min-width: 0; display: flex; flex-direction: column; }
.admin-topbar {
  height: 58px;
  background: var(--x-card);
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
.admin-topbar-title { font-size: 16px; font-weight: 600; }
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
}
.admin-content { flex: 1; }

:global(.admin-drawer .el-drawer__body) {
  padding: 0;
  background: var(--x-sidebar-bg);
}

@media (max-width: 900px) {
  .admin-main { margin-left: 0; }
  .admin-topbar { padding: 0 14px; height: 54px; }
}
</style>
