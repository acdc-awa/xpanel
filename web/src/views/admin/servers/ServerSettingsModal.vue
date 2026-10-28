<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { CsButton, CsModal, CsTabs, type TabItem } from '@/components/cloudscape'
import type { ServerItem } from '@/api/admin'
import { formatExpireDate, getExpiryStatus } from '@/utils/vps'
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

// 只在「重新打开」或「换了一台服务器」时回到概览。
// 监听器返回字符串而不是数组：数组是每次求值新建的引用，保存后父组件按 id 回填 server 快照
// 时会被 hasChanged 判成变化，把正在编辑的用户弹回概览页。
watch(
  () => `${props.modelValue}:${props.server?.id ?? 0}`,
  () => {
    if (props.modelValue) activeTab.value = 'overview'
  },
)

// 服务器身份摘要放在标题下：任何页签都能确认「我在看哪台机器」，
// 又不会变成第二个编辑入口（概览里的同名字段已移除）。
const summary = computed(() => {
  const s = props.server
  if (!s) return ''
  const parts: string[] = []
  if (s.host) parts.push(s.host)
  if (s.location) parts.push(s.location)
  const expiry = getExpiryStatus(s.expire_at)
  if (expiry) parts.push(`到期 ${formatExpireDate(s.expire_at)}（${expiry.text}）`)
  return parts.join(' · ')
})

// 保存动作统一在 footer：表单跨「基础信息 / VPS 资产」两张卡，
// 按钮挂在任何一张卡上都会让人误判保存范围。
const generalTab = ref<InstanceType<typeof ServerGeneralTab> | null>(null)
const generalDirty = computed(() => Boolean(generalTab.value?.dirty))
const generalSaving = computed(() => Boolean(generalTab.value?.saving))

function saveGeneral() {
  void generalTab.value?.save()
}

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
    :description="summary || undefined"
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
        ref="generalTab"
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
      <OnlineUsersPanel v-if="activeTab === 'online'" :server-id="server.id" boxed />
    </CsTabs>
    <template #footer>
      <span v-if="activeTab === 'general' && generalDirty" class="server-modal-dirty">有未保存的修改</span>
      <CsButton @click="visible = false">关闭</CsButton>
      <CsButton
        v-if="activeTab === 'general'"
        variant="primary"
        :loading="generalSaving"
        :disabled="!generalDirty"
        @click="saveGeneral"
      >保存修改</CsButton>
    </template>
  </CsModal>
</template>

<style scoped lang="scss">
.server-modal-dirty {
  margin-inline-end: auto;
  font-size: 12.5px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}
</style>
