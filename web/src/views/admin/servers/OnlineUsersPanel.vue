<script setup lang="ts">
import { ref, watch } from 'vue'
import { Refresh } from '@element-plus/icons-vue'
import { getServerOnlineIPs, type OnlineUserIPItem } from '@/api/admin'

// 节点在线用户面板（agent 心跳的连接级实时快照）：
// 服务器详情抽屉与仪表盘在线设备弹窗共用；serverId 变化即自动加载。
const props = defineProps<{ serverId: number }>()

const onlineUsers = ref<OnlineUserIPItem[]>([])
const onlineLoading = ref(false)

async function loadOnlineUsers() {
  if (!props.serverId) return
  onlineLoading.value = true
  try {
    const { data } = await getServerOnlineIPs(props.serverId)
    if (data.code === 0) onlineUsers.value = data.data.users
  } catch {
    /* 面板内展示，失败不打扰 */
  } finally {
    onlineLoading.value = false
  }
}

watch(
  () => props.serverId,
  (id) => {
    onlineUsers.value = []
    if (id) loadOnlineUsers()
  },
  { immediate: true },
)

// 供宿主在 Tab 重新激活/弹窗重开时强制重拉（serverId 未变时 watch 不触发）
defineExpose({ reload: loadOnlineUsers })
</script>

<template>
  <div>
    <div class="tab-toolbar">
      <el-button size="small" :loading="onlineLoading" @click="loadOnlineUsers">
        <el-icon><Refresh /></el-icon>&nbsp;刷新
      </el-button>
    </div>
    <p class="muted tip" style="margin: 0 0 10px; font-size: 12.5px">
      当前持有活跃连接的用户与其连接来源 IP，随连接建立/断开实时变化（空闲保持的连接也计为在线）。空列表 = 无人在线，或节点 Agent 版本过旧未上报。
    </p>
    <div v-loading="onlineLoading">
      <el-empty v-if="!onlineLoading && onlineUsers.length === 0" description="当前没有在线用户" :image-size="72" />
      <div v-else class="online-list">
        <div v-for="u in onlineUsers" :key="u.email" class="online-row">
          <div class="online-user">
            <span class="x-chip" :class="u.kind === 'user' ? 'green' : u.kind === 'relay' ? 'blue' : 'gray'">
              {{ u.kind === 'user' ? '用户' : u.kind === 'relay' ? '中转' : '其他' }}
            </span>
            <span class="name">{{ u.kind === 'user' && u.name ? u.name : u.email }}</span>
            <span v-if="u.kind === 'user' && u.name" class="muted email">{{ u.email }}</span>
            <span v-if="u.ips.length > 1" class="muted count">× {{ u.ips.length }}</span>
          </div>
          <div class="online-ips">
            <code v-for="ip in u.ips" :key="ip" class="ip-tag">{{ ip }}</code>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.tab-toolbar {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-bottom: 10px;
}
.online-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.online-row {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 10px 12px;
  border-radius: 8px;
  background: var(--x-bg-card);
  border: 1px solid var(--x-border);
}
.online-user {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 500;
  .email {
    font-size: 12px;
    font-weight: 400;
  }
  .count {
    font-size: 12px;
  }
}
.online-ips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  .ip-tag {
    font-size: 12px;
    padding: 2px 8px;
    border-radius: 6px;
    background: var(--x-bg, #f5f7fa);
    border: 1px solid var(--x-border);
    font-family: monospace;
  }
}
</style>
