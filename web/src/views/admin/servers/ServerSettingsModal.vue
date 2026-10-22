<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { CsButton, CsModal, CsTabs, type TabItem } from '@/components/cloudscape'
import type { ServerItem } from '@/api/admin'
import OnlineUsersPanel from './OnlineUsersPanel.vue'
import ServerOverviewTab from './settings/ServerOverviewTab.vue'
import ServerGeneralTab from './settings/ServerGeneralTab.vue'
import ServerVersionsTab from './settings/ServerVersionsTab.vue'
import ServerDiagnosticsTab from './settings/ServerDiagnosticsTab.vue'

const props = defineProps<{
  modelValue: boolean
  server: ServerItem | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'removed'): void
  (e: 'changed'): void
}>()

const router = useRouter()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const tabs: TabItem[] = [
  { id: 'overview', label: '概览' },
  { id: 'general', label: '设置' },
  { id: 'versions', label: '版本' },
  { id: 'diagnostics', label: '诊断' },
  { id: 'online', label: '在线用户' },
]

const activeTab = ref('overview')

watch(
  () => [props.modelValue, props.server?.id],
  () => {
    if (props.modelValue) activeTab.value = 'overview'
  },
)

function goInbounds() {
  if (!props.server) return
  visible.value = false
  router.push({ path: '/admin/nodes', query: { server_id: props.server.id } })
}

function onRemoved() {
  visible.value = false
  emit('removed')
}
</script>

<template>
  <CsModal
    v-model:visible="visible"
    :header="server?.name ? `服务器设置 · ${server.name}` : '服务器设置'"
    size="large"
    scrollable
    :close-on-click-overlay="false"
  >
    <CsTabs v-if="server" v-model="activeTab" :tabs="tabs">
      <ServerOverviewTab
        v-show="activeTab === 'overview'"
        :server="server"
        :active="activeTab === 'overview'"
        @open-inbounds="goInbounds"
      />
      <ServerGeneralTab
        v-show="activeTab === 'general'"
        :server="server"
        :active="activeTab === 'general'"
        @changed="emit('changed')"
        @removed="onRemoved"
      />
      <ServerVersionsTab
        v-show="activeTab === 'versions'"
        :server="server"
        :active="activeTab === 'versions'"
        @changed="emit('changed')"
      />
      <ServerDiagnosticsTab
        v-show="activeTab === 'diagnostics'"
        :server="server"
        :active="activeTab === 'diagnostics'"
      />
      <div v-if="activeTab === 'online'">
        <OnlineUsersPanel :server-id="server.id" />
      </div>
    </CsTabs>
    <template #footer>
      <CsButton @click="visible = false">关闭</CsButton>
    </template>
  </CsModal>
</template>
