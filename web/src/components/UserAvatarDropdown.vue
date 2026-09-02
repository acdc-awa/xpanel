<script setup lang="ts">
import { useRouter } from 'vue-router'
import { User, Setting, SwitchButton } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{
  currentPortal?: 'client' | 'admin'
}>()

const auth = useAuthStore()
const router = useRouter()

async function handleLogout() {
  try {
    await ElMessageBox.confirm('确定退出当前登录账号？', '提示', {
      confirmButtonText: '退出登录',
      cancelButtonText: '取消',
      type: 'warning',
    })
  } catch {
    return
  }
  await auth.logout()
  await router.replace('/login')
}
</script>

<template>
  <el-dropdown trigger="click" class="user-avatar-dropdown-wrap">
    <div class="user-avatar-badge" :title="`${auth.username}（${auth.role === 'admin' ? '管理员' : '普通用户'}）`">
      {{ auth.avatarText }}
    </div>
    <template #dropdown>
      <el-dropdown-menu class="user-avatar-menu">
        <div class="user-menu-header">
          <div class="u-name">{{ auth.username }}</div>
          <div class="u-role">
            <span class="x-chip" :class="auth.role === 'admin' ? 'purple' : 'gray'">
              {{ auth.role === 'admin' ? '管理员' : '普通用户' }}
            </span>
          </div>
        </div>

        <el-dropdown-item divided @click="router.push('/account')">
          <el-icon><User /></el-icon>账户与安全中心
        </el-dropdown-item>

        <el-dropdown-item
          v-if="auth.role === 'admin' && props.currentPortal !== 'admin'"
          @click="router.push('/admin/dashboard')"
        >
          <el-icon><Setting /></el-icon>进入管理后台
        </el-dropdown-item>

        <el-dropdown-item
          v-if="props.currentPortal === 'admin'"
          @click="router.push('/dashboard')"
        >
          <el-icon><User /></el-icon>返回用户端
        </el-dropdown-item>

        <el-dropdown-item divided @click="handleLogout">
          <el-icon><SwitchButton /></el-icon>退出登录
        </el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>

<style scoped lang="scss">
.user-avatar-dropdown-wrap {
  display: inline-flex;
  align-items: center;
  outline: none;
}

.user-avatar-badge {
  width: 32px;
  height: 32px;
  border-radius: 50%;
  background: linear-gradient(135deg, #6366f1 0%, #8b5cf6 100%);
  color: #ffffff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 700;
  cursor: pointer;
  box-shadow: 0 2px 6px rgba(99, 102, 241, 0.28);
  border: 2px solid transparent;
  transition: all 0.2s cubic-bezier(0.2, 0, 0, 1);
  user-select: none;

  &:hover {
    transform: scale(1.06);
    box-shadow: 0 4px 12px rgba(99, 102, 241, 0.4);
    border-color: rgba(255, 255, 255, 0.8);
  }
}

.user-menu-header {
  padding: 8px 16px 6px;
  display: flex;
  flex-direction: column;
  gap: 4px;
  border-bottom: 1px solid var(--x-border);
  min-width: 160px;

  .u-name {
    font-size: 13.5px;
    font-weight: 700;
    color: var(--x-text);
  }
  .u-role {
    display: flex;
  }
}
</style>
