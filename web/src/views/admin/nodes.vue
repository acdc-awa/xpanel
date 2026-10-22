<script setup lang="ts">
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
  CsForm,
  CsFormField,
  CsModal,
  CsPage,
  CsSelect,
  CsSpinner,
  CsTabs,
  CsToggle,
  CsToolbar,
  CsTooltip,
  ElMessage,
  ElMessageBox,
  type TabItem,
} from '@/components/cloudscape'
import TipIcon from '@/components/base/TipIcon.vue'
import InboundConfigEditor, { type InboundEditorChangePayload } from './servers/InboundConfigEditor.vue'
import AccessPointDialog from './servers/AccessPointDialog.vue'
import {
  createInbound,
  deleteInbound,
  getInbounds,
  getPermissionGroups,
  toggleInbound,
  updateInbound,
  updateAccessPoint,
  deleteAccessPoint,
  getAccessPoints,
  type InboundItem,
  type InboundPayload,
  type PermissionGroup,
  type UserAccessPoint,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import { formatDateOnly } from '@/utils/timezone'
import { useServerScope } from '@/composables/useServerScope'

const route = useRoute()
const router = useRouter()

// 入站页保留「全部服务器」语义：未选服务器时展示所有服务器的入站。
const { servers, serverFilter, serverLoading, loadServers, syncServerQuery } = useServerScope({
  autoSelectFirst: false,
})

const tabs: TabItem[] = [
  { id: 'inbounds', label: '入站 (Inbounds)' },
  { id: 'access_points', label: '接入点' },
]
const activeTab = ref('inbounds')

const serverFilterOptions = computed(() => servers.value.map((s) => ({ label: s.name, value: s.id })))
const serverFormOptions = computed(() =>
  servers.value.map((s) => ({ label: `${s.name} (${s.host})`, value: s.id })),
)
const typeFilterOptions = [
  { label: '用户入站', value: 'user' },
  { label: '直通通道', value: 'forward' },
  { label: '四层直通', value: 'tunnel' },
]

const list = ref<InboundItem[]>([])
const loading = ref(false)
const typeFilter = ref('')

// 本地类型过滤（列表数据已全量在本地；历史行 type 为空按用户入站口径处理）
const filteredInbounds = computed(() =>
  typeFilter.value ? list.value.filter((r) => (r.type || 'user') === typeFilter.value) : list.value,
)

type BadgeColor =
  | 'grey'
  | 'green'
  | 'blue'
  | 'red'
  | 'severity-medium'
  | 'severity-neutral'

function inboundChipText(row: InboundItem): string {
  if (row.type === 'tunnel' || row.type === 'forward' || row.type === 'channel' || row.protocol === 'dokodemo-door') return '四层直通'
  if (row.protocol === 'hysteria') return 'Hysteria 2'
  return 'VLESS'
}

function inboundChipColor(row: InboundItem): BadgeColor {
  if (row.type === 'tunnel' || row.type === 'forward' || row.type === 'channel' || row.protocol === 'dokodemo-door') return 'grey'
  if (row.protocol === 'hysteria') return 'green'
  return 'blue'
}

function serverName(id: number) {
  return servers.value.find((s) => s.id === id)?.name ?? `#${id}`
}

async function load() {
  loading.value = true
  try {
    // CsSelect 清空时写回 ''，这里统一归一化为「全部服务器」
    const sid = typeof serverFilter.value === 'number' ? serverFilter.value : undefined
    const { data } = await getInbounds(sid)
    if (data.code === 0) list.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载入站失败'))
  } finally {
    loading.value = false
  }
}

function syncTabQuery() {
  const query: LocationQueryRaw = { ...route.query }
  if (activeTab.value === 'access_points') query.tab = 'access_points'
  else delete query.tab
  router.replace({ query })
}

let initialized = false

onMounted(async () => {
  // 从服务器页跳转进入：?server_id=X 预选过滤
  const q = Number(route.query.server_id)
  if (q > 0) serverFilter.value = q
  await loadServers()
  await Promise.all([load(), loadAccessPoints(), loadPermissionGroups()])
  if (route.query.tab === 'access_points') activeTab.value = 'access_points'
  initialized = true
  syncServerQuery()
})

watch(serverFilter, () => {
  if (!initialized) return
  syncServerQuery()
  load()
})

watch(activeTab, () => {
  if (!initialized) return
  syncTabQuery()
})

// ---- 用户接入点（Access Points）统一管理 ----
const apList = ref<UserAccessPoint[]>([])
const apLoading = ref(false)
const apGroups = ref<PermissionGroup[]>([])
const allInbounds = ref<InboundItem[]>([])

async function loadPermissionGroups() {
  try {
    const { data } = await getPermissionGroups()
    if (data.code === 0) apGroups.value = data.data.items
  } catch {
    /* 忽略 */
  }
}

function groupName(id: number) {
  return apGroups.value.find((g) => g.id === id)?.name ?? `#${id}`
}

function apTargetDesc(ap: any): string {
  if (ap.target_type === 'inbound' && ap.target_inbound_tag) {
    if (ap.is_tunnel) {
      const landing = ap.tunnel_landing_desc ? ` ➔ ${ap.tunnel_landing_desc}` : ''
      return `直通 ➜ ${ap.target_inbound_tag}${landing}`
    }
    return `直连 ➜ ${ap.target_inbound_tag}`
  }
  return '待连线'
}

function apTargetColor(ap: any): BadgeColor {
  if (apTargetDesc(ap) === '待连线') return 'severity-medium'
  if (ap.is_tunnel) return 'grey'
  return 'blue'
}

async function loadAccessPoints() {
  apLoading.value = true
  try {
    const [res, inbRes] = await Promise.all([getAccessPoints(), getInbounds(undefined)])
    if (res.data.code === 0) apList.value = res.data.data.items
    else ElMessage.error(res.data.message)
    if (inbRes.data.code === 0) allInbounds.value = inbRes.data.data.items
  } catch (e) {
    ElMessage.error(errMsg(e, '加载接入点失败'))
  } finally {
    apLoading.value = false
  }
}

const apDialogOpen = ref(false)
const apEditingItem = ref<UserAccessPoint | null>(null)

function openCreateAccessPoint() {
  apEditingItem.value = null
  apDialogOpen.value = true
}

function openEditAccessPoint(ap: UserAccessPoint) {
  apEditingItem.value = ap
  apDialogOpen.value = true
}

async function removeAccessPoint(ap: any) {
  try {
    await ElMessageBox.confirm(`确认删除接入点「${ap.name}」？订阅将立即移除该入口。`, '删除接入点', { type: 'warning' })
  } catch {
    return
  }
  try {
    const { data } = await deleteAccessPoint(ap.id)
    if (data.code === 0) {
      ElMessage.success('接入点已删除')
      loadAccessPoints()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

async function toggleAccessPoint(ap: any) {
  try {
    const { data } = await updateAccessPoint(ap.id, {
      name: ap.name,
      enabled: !ap.enabled,
      custom_host: ap.custom_host,
      custom_port: ap.custom_port || 0,
      remark: ap.remark || '',
      target_type: ap.target_type,
      target_inbound_id: ap.target_inbound_id,
    })
    if (data.code === 0) {
      ElMessage.success(ap.enabled ? '接入点已停用' : '接入点已启用')
      loadAccessPoints()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '操作失败'))
  }
}

// ---- 新增 / 编辑（复用 InboundConfigEditor） ----
const editorOpen = ref(false)
const editing = ref<InboundItem | null>(null)
const inboundChange = ref<InboundEditorChangePayload | null>(null)
const formServerId = ref(0)
const formType = ref('user')
const formCertId = ref(0)
const saving = ref(false)

function editorModelValue(): string {
  if (!editing.value) return '{}'
  let settingsObj: Record<string, any> = {}
  let streamObj: Record<string, any> = {}
  let sniffObj: Record<string, any> | undefined
  try {
    settingsObj = JSON.parse(editing.value.settings_json || '{}')
    streamObj = JSON.parse(editing.value.stream_settings || '{}')
    sniffObj = editing.value.sniffing ? JSON.parse(editing.value.sniffing) : undefined
  } catch {
    /* 保持空对象，交给编辑器按默认值处理 */
  }
  return JSON.stringify({
    tag: editing.value.tag,
    port: editing.value.port,
    listen: editing.value.listen,
    protocol: editing.value.protocol,
    settings: settingsObj,
    streamSettings: streamObj,
    ...(sniffObj ? { sniffing: sniffObj } : {}),
    _panel: {
      remark: editing.value.remark || '',
      traffic_reset: editing.value.traffic_reset || 'never',
      flow: editing.value.flow || '',
      ratio: editing.value.ratio,
      total_gb: editing.value.total_gb ?? 0,
      expiry_time: editing.value.expiry_time ?? null,
      type: editing.value.type || 'user',
      target_inbound_id: editing.value.target_inbound_id,
      target_address: editing.value.target_address,
      target_port: editing.value.target_port,
      cert_id: editing.value.cert_id || 0,
      share_addr_strategy: editing.value.share_addr_strategy,
      share_addr: editing.value.share_addr,
      share_port: editing.value.share_port,
      share_security: editing.value.share_security,
      share_sni: editing.value.share_sni,
      share_host: editing.value.share_host,
      share_path: editing.value.share_path,
      share_allow_insecure: editing.value.share_allow_insecure,
      layer_id: editing.value.layer_id || 0,
    },
  })
}

function closeEditor() {
  editorOpen.value = false
  editing.value = null
  inboundChange.value = null
}

function openCreate() {
  editing.value = null
  inboundChange.value = null
  formServerId.value = serverFilter.value || servers.value[0]?.id || 0
  formType.value = 'user'
  formCertId.value = 0
  editorOpen.value = true
}

function openEdit(row: any) {
  editing.value = row as InboundItem
  inboundChange.value = null
  formServerId.value = row.server_id
  formType.value = row.type || 'user'
  formCertId.value = row.cert_id || 0
  editorOpen.value = true
}

function onInboundEditorChange(payload: InboundEditorChangePayload) {
  inboundChange.value = payload
}

async function save() {
  const c = inboundChange.value
  if (!c) {
    ElMessage.warning('请先在表单中编辑入站配置')
    return
  }
  if (!formServerId.value) {
    ElMessage.warning('请选择所属服务器')
    return
  }
  if (!c.tag.trim() || !c.port) {
    ElMessage.warning('请填写标签与端口')
    return
  }
  if (c.validationError) {
    ElMessage.warning(`请先修正配置：${c.validationError}`)
    return
  }

  saving.value = true
  try {
    const payload: InboundPayload = {
      server_id: formServerId.value,
      tag: c.tag,
      remark: c.remark || undefined,
      traffic_reset: c.traffic_reset || undefined,
      protocol: c.protocol,
      port: c.port,
      listen: c.listen,
      settings_json: c.settingsJson,
      stream_settings: c.streamSettings,
      sniffing: c.sniffing || undefined,
      ratio: c.ratio,
      total_gb: c.total_gb,
      expiry_time: (c.expiry_time && c.expiry_time.trim()) ? c.expiry_time : null,
      type: c.type || formType.value,
      target_inbound_id: c.targetInboundId || 0,
      target_address: c.targetAddress || '',
      target_port: c.targetPort || 0,
      cert_id: formCertId.value || undefined,
      flow: c.flow || undefined,
      share_addr_strategy: c.shareAddrStrategy || undefined,
      share_addr: c.shareAddr || undefined,
      share_port: c.sharePort || undefined,
      share_security: c.shareSecurity || undefined,
      share_sni: c.shareSni || undefined,
      share_host: c.shareHost || undefined,
      share_path: c.sharePath || undefined,
      share_allow_insecure: c.shareAllowInsecure,
      layer_id: c.layerId || 0,
    }

    if (editing.value) {
      const { data } = await updateInbound(editing.value.id, payload)
      if (data.code === 0) {
        ElMessage.success('入站已保存')
        closeEditor()
        load()
      } else {
        ElMessage.error(data.message)
      }
    } else {
      const { data } = await createInbound(payload)
      if (data.code === 0) {
        ElMessage.success('入站已创建')
        closeEditor()
        load()
      } else {
        ElMessage.error(data.message)
      }
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

// ---- 启用 / 删除 ----
async function toggle(row: any) {
  try {
    const { data } = await toggleInbound(row.id)
    if (data.code === 0) {
      ElMessage.success(row.enabled ? '已停用' : '已启用')
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '操作失败'))
  }
}

async function remove(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除入站「${row.tag}」？`, '删除入站', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deleteInbound(row.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      load()
    } else {
      ElMessage.error(errMsg(data.message, '删除失败'))
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

function transportOf(row: any): string {
  try {
    const ss = JSON.parse(row.stream_settings || '{}')
    return `${ss.network || '—'}/${ss.security || '—'}`
  } catch {
    return '—'
  }
}

// 流量/到期摘要：如 "100 GB · 09-30 到期" / "不限 · 永久" / "100 GB · 永久 · 每天重置"
function quotaOf(row: any): string {
  const gb = typeof row.total_gb === 'number' && row.total_gb > 0 ? `${row.total_gb} GB` : '不限'
  const exp = row.expiry_time ? formatDateOnly(row.expiry_time) + ' 到期' : '永久'
  const resetMap: Record<string, string> = { daily: '每天重置', weekly: '每周重置', monthly: '每月重置' }
  const rst = row.traffic_reset && resetMap[row.traffic_reset] ? ` · ${resetMap[row.traffic_reset]}` : ''
  return `${gb} · ${exp}${rst}`
}
</script>

<template>
  <CsPage>
    <CsTabs v-model="activeTab" :tabs="tabs" aria-label="入站与接入点">
      <!-- ==================== Tab 1: 入站 (Inbounds) ==================== -->
      <div v-if="activeTab === 'inbounds'" class="nodes-tab-body">
        <CsToolbar>
          <CsSelect
            v-model="serverFilter"
            :options="serverFilterOptions"
            :loading="serverLoading"
            clearable
            filterable
            placeholder="全部 Xray 服务器"
            aria-label="按服务器筛选入站"
            style="width: 220px"
          />
          <CsSelect
            v-model="typeFilter"
            :options="typeFilterOptions"
            clearable
            placeholder="全部类型"
            aria-label="按类型筛选入站"
            style="width: 150px"
          />
          <CsButton icon-name="refresh" @click="load">刷新</CsButton>
          <TipIcon
            content="在此为 Xray 服务器配置物理入站：VLESS / Hysteria 2 代理入站支持多源复用（同时承载用户与链式中继），四层直通管道实现多级穿透。新增/编辑/停用后自动生成配置推送到服务器（离线保存，上线补推）。"
          />
          <template #actions>
            <CsButton variant="primary" icon-name="add-plus" @click="openCreate">新增入站</CsButton>
          </template>
        </CsToolbar>

        <CsContainer header="入站列表">
          <div v-if="loading" class="nodes-loading">
            <CsSpinner size="large" />
          </div>

          <CsEmpty
            v-else-if="filteredInbounds.length === 0"
            icon-name="globe"
            :description="
              list.length === 0
                ? '尚未为任何服务器添加入站。点击右上角「新增入站」开始配置。'
                : '未找到匹配当前筛选的入站'
            "
          />

          <CsCardGrid v-else>
            <CsCard
              v-for="row in filteredInbounds"
              :key="row.id"
              :class="{ 'node-card--disabled': !row.enabled }"
            >
              <template #header>
                <div class="node-card-head">
                  <span class="cs-text-mono cs-text-secondary">#{{ row.id }}</span>
                  <CsButton variant="link" class="node-card-name" @click="openEdit(row)">
                    {{ row.tag }}
                  </CsButton>
                  <span v-if="row.remark" class="cs-text-secondary node-card-remark">({{ row.remark }})</span>
                  <CsBadge :color="inboundChipColor(row)">{{ inboundChipText(row) }}</CsBadge>
                </div>
              </template>

              <template #actions>
                <CsTooltip :content="row.enabled ? '已启用，点击禁用' : '已禁用，点击启用'">
                  <CsToggle
                    :model-value="row.enabled"
                    :aria-label="`启用入站 ${row.tag}`"
                    @change="toggle(row)"
                  />
                </CsTooltip>
              </template>

              <CsCardFacts :columns="2">
                <CsCardFact label="所属服务器" :value="serverName(row.server_id)" />
                <CsCardFact label="协议与端口">
                  <span class="node-card-proto">
                    <CsBadge color="blue">{{ (row.protocol || '').toUpperCase() }}</CsBadge>
                    <span class="cs-text-mono">:{{ row.port }}</span>
                  </span>
                </CsCardFact>
                <CsCardFact label="传输与 TLS" :value="transportOf(row)" mono />
                <CsCardFact label="流量与到期" :value="quotaOf(row)" mono />
              </CsCardFacts>

              <template #footer>
                <CsButton icon-name="edit" @click="openEdit(row)">编辑入站</CsButton>
                <CsButton icon-name="remove" @click="remove(row)">删除</CsButton>
              </template>
            </CsCard>
          </CsCardGrid>
        </CsContainer>
      </div>

      <!-- ==================== Tab 2: 用户接入点 (Access Points) ==================== -->
      <div v-else class="nodes-tab-body">
        <CsToolbar>
          <CsButton icon-name="refresh" @click="loadAccessPoints">刷新</CsButton>
          <TipIcon
            content="接入点是用户订阅中实际看到的「节点」，通过白名单权限组控制哪些权限组可见。接入点可直接绑定入站，亦可在拓扑中参与链式转发中转。"
          />
          <template #actions>
            <CsButton variant="primary" icon-name="add-plus" @click="openCreateAccessPoint">新建接入点</CsButton>
          </template>
        </CsToolbar>

        <CsContainer header="接入点分发列表">
          <div v-if="apLoading" class="nodes-loading">
            <CsSpinner size="large" />
          </div>

          <CsEmpty
            v-else-if="apList.length === 0"
            icon-name="share"
            description="尚未创建任何接入点。新建后即可作为用户订阅的入口（订阅仅从接入点生成）。"
          />

          <CsCardGrid v-else>
            <CsCard v-for="row in apList" :key="row.id" :class="{ 'node-card--disabled': !row.enabled }">
              <template #header>
                <div class="node-card-head">
                  <span class="cs-text-mono cs-text-secondary">#{{ row.id }}</span>
                  <CsButton variant="link" class="node-card-name" @click="openEditAccessPoint(row)">
                    {{ row.name }}
                  </CsButton>
                </div>
              </template>

              <template #actions>
                <CsTooltip :content="row.enabled ? '已启用，点击禁用' : '已禁用，点击启用'">
                  <CsToggle
                    :model-value="row.enabled"
                    :aria-label="`启用接入点 ${row.name}`"
                    @change="toggleAccessPoint(row)"
                  />
                </CsTooltip>
              </template>

              <CsCardFacts :columns="2">
                <CsCardFact label="目标绑定">
                  <CsBadge :color="apTargetColor(row)">{{ apTargetDesc(row) }}</CsBadge>
                </CsCardFact>
                <CsCardFact
                  label="连接地址覆写"
                  :value="row.custom_host ? `${row.custom_host}:${row.custom_port || '自动'}` : '自动继承入站地址'"
                  mono
                />
                <CsCardFact label="开放权限组 (白名单)" full-width>
                  <div class="ap-group-badges">
                    <template v-if="row.permission_group_ids && row.permission_group_ids.length > 0">
                      <CsBadge
                        v-for="gid in row.permission_group_ids.slice(0, 3)"
                        :key="gid"
                        color="blue"
                      >
                        {{ groupName(gid) }}
                      </CsBadge>
                      <CsBadge v-if="row.permission_group_ids.length > 3" color="grey">
                        +{{ row.permission_group_ids.length - 3 }}
                      </CsBadge>
                    </template>
                    <CsBadge v-else color="severity-medium">全员不可见</CsBadge>
                  </div>
                </CsCardFact>
              </CsCardFacts>

              <template #footer>
                <CsButton icon-name="edit" @click="openEditAccessPoint(row)">编辑接入点</CsButton>
                <CsButton icon-name="remove" @click="removeAccessPoint(row)">删除</CsButton>
              </template>
            </CsCard>
          </CsCardGrid>
        </CsContainer>

        <!-- 新建/编辑用户接入点（弹窗本体仍在后续批次迁移） -->
        <AccessPointDialog
          v-model="apDialogOpen"
          :access-point="apEditingItem"
          :servers="servers"
          :inbounds="allInbounds"
          :permission-groups="apGroups"
          @saved="loadAccessPoints"
          @deleted="loadAccessPoints"
        />
      </div>
    </CsTabs>

    <!-- 新增/编辑入站（编辑器本体仍在后续批次迁移） -->
    <CsModal
      v-model:visible="editorOpen"
      :header="editing ? `编辑入站 · ${editing.tag}` : '新建入站'"
      size="large"
      scrollable
      :close-on-click-overlay="false"
      @close="closeEditor"
    >
      <CsForm v-if="!editing" label-position="top">
        <CsFormField label="所属目标服务器" required>
          <CsSelect
            v-model="formServerId"
            :options="serverFormOptions"
            placeholder="请选择要绑定入站的服务器"
          />
        </CsFormField>
      </CsForm>

      <InboundConfigEditor
        :key="editing ? `edit-${editing.id}` : 'create'"
        :model-value="editorModelValue()"
        :protocol="editing?.protocol || 'vless'"
        :inbound-type="formType"
        :saved-inbound-type="editing?.type || ''"
        :listen="editing?.listen || '0.0.0.0'"
        :internal-uuid="editing?.internal_uuid || ''"
        :inbound-id="editing?.id || 0"
        :cert-id="formCertId"
        :server-id="formServerId"
        :layer-id="editing?.layer_id || 0"
        :target-inbound-id="editing?.target_inbound_id"
        :target-address="editing?.target_address || ''"
        :target-port="editing?.target_port || 0"
        @change="onInboundEditorChange"
        @update:inbound-type="(v: string) => (formType = v)"
        @update:cert-id="(v: number) => (formCertId = v || 0)"
        @internal-uuid-changed="load"
      />

      <template #footer>
        <CsButton @click="closeEditor">取消</CsButton>
        <CsButton variant="primary" :loading="saving" @click="save">保存入站</CsButton>
      </template>
    </CsModal>
  </CsPage>
</template>

<style scoped lang="scss">
.nodes-loading {
  padding: 48px 0;
  text-align: center;
}

/* CsTabs 内容区默认只有上 16px 内边距（左右为 0），这里补工具栏与列表之间的 20px */
.nodes-tab-body {
  display: flex;
  flex-direction: column;
  gap: 20px;
  min-inline-size: 0;
}

.node-card-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  min-inline-size: 0;
}

.node-card-name {
  font-size: 14px;
  font-weight: 600;
  overflow-wrap: anywhere;
}

.node-card-remark {
  font-size: 12px;
}

.node-card--disabled {
  opacity: 0.7;
}

.node-card-proto {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.ap-group-badges {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
}
</style>
