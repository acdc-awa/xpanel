<script setup lang="ts">
import { ref, watch } from 'vue'
import { CsBadge, CsButton, CsContainer, CsEmpty, CsSpinner } from '@/components/cloudscape'
import { getServerOnlineIPs, type OnlineUserIPItem } from '@/api/admin'
import TipIcon from '@/components/base/TipIcon.vue'

// 节点在线用户面板（agent 心跳的连接级实时快照）：
// 服务器设置弹窗（boxed）与仪表盘在线设备弹窗共用；serverId 变化即自动加载。
const props = withDefaults(
  defineProps<{
    serverId: number
    /** 外面已经是裸内容区（如仪表盘弹窗）时用默认值；设置弹窗里套一层卡片，刷新/说明进卡片头。 */
    boxed?: boolean
  }>(),
  { boxed: false },
)

const ONLINE_TIP =
  '当前持有活跃连接的用户与其连接来源 IP，随连接建立/断开实时变化（空闲保持的连接也计为在线）。列表为空表示当前无人在线，或服务器 Agent 版本过旧未上报。'

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

function kindText(kind: OnlineUserIPItem['kind']) {
  if (kind === 'user') return '用户'
  if (kind === 'relay') return '中转'
  return '其他'
}

function kindColor(kind: OnlineUserIPItem['kind']) {
  if (kind === 'user') return 'green' as const
  if (kind === 'relay') return 'blue' as const
  return 'grey' as const
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
  <!-- 同一份内容按宿主切换外壳：设置弹窗里套卡片（刷新/说明在卡片头），仪表盘弹窗里保持裸内容区。 -->
  <component :is="boxed ? CsContainer : 'div'" :header="boxed ? '在线用户' : undefined">
    <template v-if="boxed" #actions>
      <CsButton icon-name="refresh" :loading="onlineLoading" @click="loadOnlineUsers">刷新</CsButton>
    </template>
    <template v-if="boxed" #info>
      <TipIcon :content="ONLINE_TIP" />
    </template>

    <div v-if="!boxed" class="online-toolbar">
      <CsButton icon-name="refresh" :loading="onlineLoading" @click="loadOnlineUsers">刷新</CsButton>
      <TipIcon :content="ONLINE_TIP" />
    </div>

    <div v-if="onlineLoading && onlineUsers.length === 0" class="online-loading">
      <CsSpinner size="large" />
    </div>
    <CsEmpty v-else-if="onlineUsers.length === 0" icon-name="user-profile" description="当前没有在线用户" />
    <div v-else class="online-list">
      <div v-for="u in onlineUsers" :key="u.email" class="online-row">
        <div class="online-user">
          <CsBadge :color="kindColor(u.kind)">{{ kindText(u.kind) }}</CsBadge>
          <span class="online-name">{{ u.kind === 'user' && u.name ? u.name : u.email }}</span>
          <span v-if="u.kind === 'user' && u.name" class="online-email">{{ u.email }}</span>
          <span v-if="u.ips.length > 1" class="online-count">× {{ u.ips.length }}</span>
        </div>
        <div class="online-ips">
          <code v-for="ip in u.ips" :key="ip" class="online-ip">{{ ip }}</code>
        </div>
      </div>
    </div>
  </component>
</template>

<style scoped lang="scss">
.online-toolbar {
  display: flex;
  justify-content: flex-end;
  align-items: center;
  gap: 8px;
  margin-bottom: 10px;
}

.online-loading {
  padding: 24px 0;
  text-align: center;
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
  border: 1px solid var(--color-border-divider-default-diwd4w, #dedee3);
  background: var(--color-background-container-content-mjfil9, #ffffff);
}

.online-user {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  font-size: 13px;
  font-weight: 500;
}

.online-email,
.online-count {
  font-size: 12px;
  font-weight: 400;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.online-ips {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.online-ip {
  padding: 2px 8px;
  border-radius: 6px;
  border: 1px solid var(--color-border-divider-default-diwd4w, #dedee3);
  background: var(--color-background-container-content-mjfil9, #ffffff);
  font-family: ui-monospace, SFMono-Regular, Menlo, Consolas, monospace;
  font-size: 12px;
  overflow-wrap: anywhere;
}
</style>
