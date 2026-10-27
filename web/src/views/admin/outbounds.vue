<script setup lang="ts">
// 出站管理：从路由页拆出的独立页面（2026-10-22）。
// 只负责出站对象本身：列表、新建、编辑、启停、删除。
// 默认兜底出口与路由 DomainStrategy 属于全局路由策略，仍留在路由页。
// 四层直通的派生出口由通道/入站生命周期自动维护，本页只读展示。
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter, type LocationQueryRaw } from 'vue-router'
import {
  CsBadge,
  CsButton,
  CsCard,
  CsCardFact,
  CsCardFacts,
  CsCardGrid,
  CsContainer,
  CsEmpty,
  CsPage,
  CsSelect,
  CsSpinner,
  CsToggle,
  CsToolbar,
  ElMessage,
  ElMessageBox,
} from '@/components/cloudscape'
import OutboundConfigEditor from './servers/OutboundConfigEditor.vue'
import { useServerInbounds, useServerScope } from '@/composables/useServerScope'
import {
  deleteServerOutbound,
  getServerOutbounds,
  updateServerOutbound,
  type ServerOutbound,
} from '@/api/admin'
import { errMsg } from '@/api/http'

const route = useRoute()
const router = useRouter()

const { servers, serverFilter, currentServer, serverLoading, loadServers, syncServerQuery } =
  useServerScope()
const { ownerTagOf, loadServerInbounds } = useServerInbounds(serverFilter)

const serverOptions = computed(() =>
  servers.value.map((s) => ({ label: `${s.name} (${s.host})`, value: s.id }))
)

const outbounds = ref<ServerOutbound[]>([])
const loading = ref(false)
const editorOpen = ref(false)
const editing = ref<ServerOutbound | null>(null)
const defaultOutboundTag = computed(() => currentServer.value?.default_outbound_tag || 'direct')

type EffectiveOutbound = ServerOutbound & { ownerInboundTag?: string }

const displayOutbounds = computed<EffectiveOutbound[]>(() =>
  outbounds.value.map((o) => ({
    ...o,
    ownerInboundTag: o.owner_inbound_id ? ownerTagOf.value[o.owner_inbound_id] || '' : '',
  }))
)

async function loadOutbounds() {
  if (!serverFilter.value) {
    outbounds.value = []
    return
  }
  loading.value = true
  try {
    const { data } = await getServerOutbounds(serverFilter.value)
    if (data.code === 0) outbounds.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载出站失败'))
  } finally {
    loading.value = false
  }
}

async function reloadList() {
  await Promise.all([loadOutbounds(), loadServerInbounds()])
}

async function refreshAll() {
  await loadServers()
  await reloadList()
}

function openCreate() {
  editing.value = null
  editorOpen.value = true
}

function openEdit(row: EffectiveOutbound) {
  editing.value = row
  editorOpen.value = true
}

async function toggleOutbound(row: EffectiveOutbound) {
  if (!serverFilter.value) return
  try {
    const { data } = await updateServerOutbound(serverFilter.value, row.id, { enabled: !row.enabled })
    if (data.code === 0) {
      ElMessage.success(data.data.outbound.enabled ? '已启用' : '已停用')
      loadOutbounds()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '操作失败'))
  }
}

async function removeOutbound(row: EffectiveOutbound) {
  if (!serverFilter.value) return
  try {
    await ElMessageBox.confirm(`确认删除出站「${row.tag}」？`, '删除出站', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deleteServerOutbound(serverFilter.value, row.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      // 删除当前默认出口时后端会回退到 direct，顺带刷新服务器以更新「默认出口」徽标
      await Promise.all([loadOutbounds(), loadServers()])
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

function isBlockCN(row: EffectiveOutbound): boolean {
  if (row.protocol !== 'freedom') return false
  try {
    const s = JSON.parse(row.settings_json || '{}')
    return !!s.block_cn
  } catch {
    return false
  }
}

// 出站域名解析策略（freedom settings.domainStrategy）：唯一入口在出站编辑器，卡片正面只读展示
function freedomDS(row: EffectiveOutbound): string {
  if (row.protocol !== 'freedom') return ''
  try {
    const s = JSON.parse(row.settings_json || '{}')
    return s.domainStrategy || 'AsIs'
  } catch {
    return 'AsIs'
  }
}

type BadgeColor = 'grey' | 'green' | 'blue' | 'red' | 'severity-medium' | 'severity-neutral'

// 引用草稿：非基础协议、没有引用目标、也没配载荷，等待连线。
function isDraftOutbound(row: EffectiveOutbound): boolean {
  if (row.owner_inbound_id || row.inbound_ref || row.outbound_json) return false
  if (row.protocol === 'freedom' || row.protocol === 'blackhole') return false
  return !row.settings_json && !row.stream_settings_json
}

function protocolBadge(row: EffectiveOutbound): { text: string; color: BadgeColor } {
  if (isDraftOutbound(row)) return { text: '待连线', color: 'severity-medium' }
  switch (row.protocol) {
    case 'freedom':
      return { text: '直连 (freedom)', color: 'green' }
    case 'vless':
      return { text: 'VLESS 链', color: 'blue' }
    case 'hysteria':
      return { text: 'Hysteria 2', color: 'blue' }
    case 'socks':
      return { text: 'SOCKS5', color: 'severity-medium' }
    case 'http':
      return { text: 'HTTP(S)', color: 'severity-neutral' }
    case 'blackhole':
      return { text: '黑洞 (blackhole)', color: 'red' }
    case 'custom':
      return { text: '原生 JSON', color: 'severity-medium' }
    default:
      return { text: row.protocol, color: 'grey' }
  }
}

function connectionLabel(row: EffectiveOutbound): string {
  if (row.owner_inbound_id) {
    return `直通「${row.ownerInboundTag || '#' + row.owner_inbound_id}」`
  }
  if (row.inbound_ref) return `引用入站 #${row.inbound_ref}`
  if (row.outbound_json) return '自定义透传'
  if (row.tag === 'direct') return '服务器直连互联网'
  if (row.tag === 'blocked') return '黑洞丢弃阻断'
  if (row.protocol === 'freedom') return '自主直连'
  if (row.protocol === 'blackhole') return '黑洞阻断'
  if (row.settings_json || row.stream_settings_json) return '旧版代理（待转换）'
  return '待连接入站'
}

let initialized = false
watch(serverFilter, (v) => {
  if (!initialized) return
  syncServerQuery()
  if (!v) {
    outbounds.value = []
    return
  }
  loadOutbounds()
  loadServerInbounds()
})

onMounted(async () => {
  const q = Number(route.query.server_id)
  if (q > 0) serverFilter.value = q
  await loadServers()
  await reloadList()
  initialized = true
  syncServerQuery()

  if (route.query.create === '1') {
    openCreate()
    const query: LocationQueryRaw = { ...route.query }
    delete query.create
    router.replace({ query })
  }
})
</script>

<template>
  <CsPage>
    <CsToolbar>
      <CsSelect
        v-model="serverFilter"
        :options="serverOptions"
        :loading="serverLoading"
        filterable
        placeholder="选择目标服务器"
        aria-label="选择目标服务器"
        style="width: 260px"
      />
      <CsButton icon-name="refresh" @click="refreshAll">刷新</CsButton>
      <template #actions>
        <CsButton variant="primary" icon-name="add-plus" :disabled="!serverFilter" @click="openCreate">
          新增出站
        </CsButton>
      </template>
    </CsToolbar>

    <CsContainer header="出站通道">
      <div v-if="loading" class="outbounds-loading">
        <CsSpinner size="large" />
      </div>

      <CsEmpty
        v-else-if="displayOutbounds.length === 0"
        icon-name="globe"
        :description="serverFilter ? '该服务器暂无出站通道，点击右上角「新增出站」' : '请先选择目标服务器'"
      />

      <CsCardGrid v-else>
        <CsCard v-for="row in displayOutbounds" :key="row.id">
          <template #header>
            <div class="outbound-head">
              <span class="outbound-tag">{{ row.tag }}</span>
              <CsBadge v-if="row.owner_inbound_id" color="blue">系统内置·直通</CsBadge>
              <CsBadge v-else-if="row.tag === defaultOutboundTag" color="green">默认出口</CsBadge>
              <CsBadge v-if="row.tag === 'direct' || row.tag === 'blocked'" color="grey">系统内置</CsBadge>
              <CsBadge v-if="row.send_proxy_protocol" color="blue">PPv2</CsBadge>
              <CsBadge v-if="isBlockCN(row)" color="red">阻断回国</CsBadge>
            </div>
          </template>

          <template #actions>
            <CsToggle
              :model-value="row.enabled"
              :disabled="row.tag === 'direct' || !!row.owner_inbound_id"
              :aria-label="`启用出站 ${row.tag}`"
              @change="toggleOutbound(row)"
            />
          </template>

          <CsCardFacts>
            <CsCardFact label="出站协议">
              <CsBadge :color="protocolBadge(row).color">
                {{ protocolBadge(row).text }}
              </CsBadge>
            </CsCardFact>
            <CsCardFact label="连接与引用" :value="connectionLabel(row)" />
            <CsCardFact v-if="row.send_through" label="发送出口 IP" :value="row.send_through" mono />
            <CsCardFact v-if="row.protocol === 'freedom'" label="域名解析策略" :value="freedomDS(row)" mono />
            <CsCardFact label="优先级" :value="row.priority ?? 0" mono />
            <CsCardFact v-if="row.remark" label="备注说明" :value="row.remark" full-width />
          </CsCardFacts>

          <template #footer>
            <template v-if="row.owner_inbound_id">
              <span class="outbound-derived-note">
                随四层直通「{{ row.ownerInboundTag || '#' + row.owner_inbound_id }}」自动维护，目标在通道页/入站页修改
              </span>
            </template>
            <template v-else>
              <CsButton icon-name="edit" @click="openEdit(row)">编辑出站</CsButton>
              <CsButton
                icon-name="remove"
                :disabled="row.tag === 'direct' || row.tag === 'blocked'"
                :title="row.tag === 'direct' || row.tag === 'blocked' ? '系统内置出站不可删除' : undefined"
                @click="removeOutbound(row)"
              >
                删除
              </CsButton>
            </template>
          </template>
        </CsCard>
      </CsCardGrid>
    </CsContainer>

    <OutboundConfigEditor
      v-if="editorOpen"
      :server-id="serverFilter ?? 0"
      :outbound="editing"
      @saved="loadOutbounds"
      @close="editorOpen = false"
    />
  </CsPage>
</template>

<style scoped lang="scss">
.outbounds-loading {
  padding: 48px 0;
  text-align: center;
}

.outbound-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  min-inline-size: 0;
}

.outbound-tag {
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 14px;
  font-weight: 700;
  color: var(--color-text-body-default-b2us04, #0f141a);
  overflow-wrap: anywhere;
}

.outbound-derived-note {
  flex: 1 1 100%;
  text-align: center;
  font-size: 12px;
  line-height: 18px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}
</style>
