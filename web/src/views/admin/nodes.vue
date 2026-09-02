<script setup lang="ts">
import { computed, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { Plus, Refresh, Edit, Delete } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import InboundConfigEditor, { type InboundEditorChangePayload } from './servers/InboundConfigEditor.vue'
import {
  createInbound,
  deleteInbound,
  getInbounds,
  getPermissionGroups,
  getServers,
  toggleInbound,
  updateInbound,
  createAccessPoint,
  updateAccessPoint,
  deleteAccessPoint,
  getAccessPoints,
  type InboundItem,
  type InboundPayload,
  type PermissionGroup,
  type ServerItem,
  type UserAccessPoint,
} from '@/api/admin'
import { errMsg } from '@/api/http'

const route = useRoute()
const router = useRouter()

// 入站 / 用户接入点 双分页
const activeTab = ref<'inbounds' | 'access_points'>('inbounds')
const list = ref<InboundItem[]>([])
const servers = ref<ServerItem[]>([])
const loading = ref(false)
const serverFilter = ref<number | undefined>(undefined)

function serverName(id: number) {
  return servers.value.find((s) => s.id === id)?.name ?? `#${id}`
}

async function loadServers() {
  try {
    const { data } = await getServers()
    if (data.code === 0) servers.value = data.data.items
  } catch {
    /* 忽略 */
  }
}

async function load() {
  loading.value = true
  try {
    const { data } = await getInbounds(serverFilter.value)
    if (data.code === 0) list.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载入站失败'))
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadServers()])
  // 从服务器页跳转进入：?server_id=X 预选过滤
  const q = Number(route.query.server_id)
  if (q > 0) serverFilter.value = q
  load()
  await Promise.all([loadAccessPoints(), loadPermissionGroups()])
  if (route.query.tab === 'access_points') activeTab.value = 'access_points'
})

watch(serverFilter, (v) => {
  router.replace({ query: v ? { server_id: v } : {} })
  load()
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
  if (ap.target_type === 'inbound' && ap.target_inbound_tag) return `直连 ➜ ${ap.target_inbound_tag}`
  return '待连线'
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
const apEditingId = ref<number | null>(null)
const apSaving = ref(false)
const apForm = reactive({
  name: '',
  enabled: true,
  permission_group_ids: [] as number[],
  remark: '',
  custom_host: '',
  custom_port: 0,
  target_type: '' as '' | 'inbound',
  target_inbound_id: undefined as number | undefined,
})
const apTargetServerId = ref(0)

const apAvailableInbounds = computed(() =>
  allInbounds.value.filter((i) => i.server_id === apTargetServerId.value && i.enabled && i.type === 'user'),
)

function openCreateAccessPoint() {
  apEditingId.value = null
  apForm.name = ''
  apForm.enabled = true
  apForm.permission_group_ids = []
  apForm.remark = ''
  apForm.custom_host = ''
  apForm.custom_port = 0
  apForm.target_type = ''
  apForm.target_inbound_id = undefined
  apTargetServerId.value = servers.value[0]?.id || 0
  apDialogOpen.value = true
}

function openEditAccessPoint(ap: any) {
  apEditingId.value = ap.id
  apForm.name = ap.name
  apForm.enabled = ap.enabled
  apForm.permission_group_ids = [...(ap.permission_group_ids || [])]
  apForm.remark = ap.remark || ''
  apForm.custom_host = ap.custom_host || ''
  apForm.custom_port = ap.custom_port || 0
  apForm.target_type = ap.target_type
  apForm.target_inbound_id = ap.target_inbound_id
  const inb = allInbounds.value.find((i) => i.id === ap.target_inbound_id)
  apTargetServerId.value = inb?.server_id || servers.value[0]?.id || 0
  apDialogOpen.value = true
}

async function saveAccessPoint() {
  const name = apForm.name.trim()
  if (!name) {
    ElMessage.warning('请输入接入点名称')
    return
  }
  if (apForm.target_type === 'inbound' && !apForm.target_inbound_id) {
    ElMessage.warning('请选择直连落地入站')
    return
  }
  apSaving.value = true
  try {
    const payload = {
      name,
      enabled: apForm.enabled,
      permission_group_ids: apForm.permission_group_ids,
      remark: apForm.remark,
      custom_host: apForm.custom_host || undefined,
      custom_port: apForm.custom_port || undefined,
      target_type: apForm.target_type,
      target_inbound_id: apForm.target_type === 'inbound' ? apForm.target_inbound_id : undefined,
    }
    const { data } = apEditingId.value
      ? await updateAccessPoint(apEditingId.value, payload)
      : await createAccessPoint(payload)
    if (data.code === 0) {
      ElMessage.success(apEditingId.value ? '接入点已更新' : '接入点已创建')
      apDialogOpen.value = false
      loadAccessPoints()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存接入点失败'))
  } finally {
    apSaving.value = false
  }
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
  // 回填：InboundConfigEditor 只解析 settings_json + 顶层 network/tls_type/port/tag
  let network = 'tcp'
  let tlsType = 'reality'
  try {
    const ss = JSON.parse(editing.value.stream_settings || '{}')
    network = ss.network || network
    tlsType = ss.security || tlsType
  } catch {
    /* 保持默认 */
  }
  return JSON.stringify({
    tag: editing.value.tag,
    port: editing.value.port,
    listen: editing.value.listen,
    network,
    tls_type: tlsType,
    flow: editing.value.flow || '',
    settings_json: editing.value.settings_json,
    stream_settings: editing.value.stream_settings,
    sniffing: editing.value.sniffing,
    ratio: editing.value.ratio,
    total_gb: editing.value.total_gb ?? 0,
    expiry_time: editing.value.expiry_time ?? null,
    type: editing.value.type || 'user',
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
  })
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

  saving.value = true
  try {
    const payload: InboundPayload = {
      server_id: formServerId.value,
      tag: c.tag,
      protocol: c.protocol,
      port: c.port,
      listen: c.listen,
      settings_json: c.settingsJson,
      stream_settings: c.streamSettings,
      sniffing: c.sniffing || undefined,
      ratio: c.ratio,
      total_gb: c.total_gb,
      expiry_time: c.expiry_time ?? null,
      type: formType.value,
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
        ElMessage.success('已保存')
        editorOpen.value = false
        load()
      } else {
        ElMessage.error(data.message)
      }
    } else {
      const { data } = await createInbound(payload)
      if (data.code === 0) {
        ElMessage.success('已创建')
        editorOpen.value = false
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

// 流量/到期摘要：如 "100 GB · 09-30 到期" / "不限 · 永久" / "100 GB · 永久"
function quotaOf(row: any): string {
  const gb = typeof row.total_gb === 'number' && row.total_gb > 0 ? `${row.total_gb} GB` : '不限'
  const exp = row.expiry_time ? new Date(row.expiry_time).toLocaleDateString('zh-CN') + ' 到期' : '永久'
  return `${gb} · ${exp}`
}
</script>

<template>
  <div class="x-page">
    <el-tabs v-model="activeTab" class="page-tabs">
      <el-tab-pane label="入站 (Inbounds)" name="inbounds">
        <div class="x-toolbar">
          <div class="x-toolbar-left">
            <el-select v-model="serverFilter" placeholder="全部 Xray 服务器" clearable style="width: 200px">
              <el-option v-for="s in servers" :key="s.id" :label="s.name" :value="s.id" />
            </el-select>
            <el-button @click="load"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
          </div>
          <div style="display: flex; gap: 10px">
            <el-button type="primary" @click="openCreate"><el-icon><Plus /></el-icon>&nbsp;新增入站</el-button>
          </div>
        </div>

    <el-alert
      type="info"
      :closable="false"
      show-icon
      title="在此为 Xray 服务器配置物理入站：用户入站承载用户真实流量，转发入站作为内部链式落地节点。新增/编辑/停用后自动生成配置推送到节点（离线保存，上线补推）。"
      style="margin-bottom: 14px"
    />

    <BaseCard>
      <!-- 桌面端表格视图 -->
      <div class="desktop-table-view">
        <el-table v-loading="loading" :data="list">
          <el-table-column prop="id" label="ID" width="64">
            <template #default="{ row }"><code class="cell-mono">#{{ row.id }}</code></template>
          </el-table-column>
          <el-table-column label="服务器" min-width="110">
            <template #default="{ row }">{{ serverName(row.server_id) }}</template>
          </el-table-column>
          <el-table-column label="类型" width="90">
            <template #default="{ row }">
              <span v-if="row.type === 'relay'" class="x-chip orange">转发</span>
              <span v-else class="x-chip purple">用户</span>
            </template>
          </el-table-column>
          <el-table-column prop="tag" label="标签" min-width="130">
            <template #default="{ row }"><span style="font-weight: 600">{{ row.tag }}</span></template>
          </el-table-column>
          <el-table-column prop="protocol" label="协议" width="80">
            <template #default="{ row }">
              <span class="x-chip blue" style="text-transform: uppercase">{{ row.protocol }}</span>
            </template>
          </el-table-column>
          <el-table-column label="端口" width="80">
            <template #default="{ row }"><code class="cell-mono">{{ row.port }}</code></template>
          </el-table-column>
          <el-table-column label="传输/TLS" width="130">
            <template #default="{ row }"><code class="cell-mono" style="font-size: 11px">{{ transportOf(row) }}</code></template>
          </el-table-column>
          <el-table-column label="流量/到期" min-width="120">
            <template #default="{ row }">
              <span class="muted" style="font-size: 12px">{{ quotaOf(row) }}</span>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="80">
            <template #default="{ row }">
              <el-switch :model-value="row.enabled" @change="toggle(row)" />
            </template>
          </el-table-column>
          <el-table-column label="操作" width="120" fixed="right">
            <template #default="{ row }">
              <el-button size="small" text @click="openEdit(row)"><el-icon><Edit /></el-icon></el-button>
              <el-button size="small" text type="danger" @click="remove(row)"><el-icon><Delete /></el-icon></el-button>
            </template>
          </el-table-column>
          <template #empty>
            <div style="padding: 30px 0; color: var(--x-text-3)">
              {{ serverFilter ? '该服务器尚未配置入站，点击右上角「新增入站」' : '尚未配置任何入站。先到「服务器」页添加服务器，再点击右上角「新增入站」' }}
            </div>
          </template>
        </el-table>
      </div>

      <!-- 移动端卡片流视图 -->
      <div class="mobile-cards-view">
        <div v-if="list.length === 0" style="text-align: center; padding: 36px 0; color: var(--x-text-3); font-size: 13.5px">
          {{ serverFilter ? '该服务器尚未配置入站，点击右上角「新增入站」' : '尚未配置任何入站，点击右上角「新增入站」' }}
        </div>
        <div v-else class="mobile-data-card-list">
          <div v-for="row in list" :key="row.id" class="mobile-data-card">
            <div class="card-head">
              <div class="head-title">
                <span class="cell-mono muted" style="font-size: 11px">#{{ row.id }}</span>
                <span style="font-weight: 700">{{ row.tag }}</span>
                <span v-if="row.type === 'relay'" class="x-chip orange">转发</span>
                <span v-else class="x-chip purple">用户</span>
              </div>
              <el-switch :model-value="row.enabled" size="small" @change="toggle(row)" />
            </div>

            <div class="card-grid">
              <div class="grid-item">
                <span class="item-label">所属服务器</span>
                <div class="item-value" style="font-weight: 600">{{ serverName(row.server_id) }}</div>
              </div>
              <div class="grid-item">
                <span class="item-label">协议 / 端口</span>
                <div class="item-value">
                  <span style="text-transform: uppercase; font-weight: 600">{{ row.protocol }}</span>
                  <code class="cell-mono" style="margin-left: 4px">:{{ row.port }}</code>
                </div>
              </div>
              <div class="grid-item">
                <span class="item-label">传输与 TLS</span>
                <div class="item-value cell-mono" style="font-size: 11.5px">{{ transportOf(row) }}</div>
              </div>
            </div>

            <div class="card-foot-actions">
              <el-button size="small" type="primary" plain @click="openEdit(row)">
                <el-icon><Edit /></el-icon>&nbsp;编辑入站
              </el-button>
              <el-button size="small" type="danger" plain @click="remove(row)">
                <el-icon><Delete /></el-icon>&nbsp;删除
              </el-button>
            </div>
          </div>
        </div>
      </div>
    </BaseCard>
      </el-tab-pane>

      <!-- 用户接入点（Access Points）统一管理 -->
      <el-tab-pane label="用户接入点 (Access Points)" name="access_points">
        <div class="x-toolbar">
          <div class="x-toolbar-left">
            <el-button @click="loadAccessPoints"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
          </div>
          <el-button type="primary" @click="openCreateAccessPoint"><el-icon><Plus /></el-icon>&nbsp;新建接入点</el-button>
        </div>

        <el-alert
          type="info"
          :closable="false"
          show-icon
          title="用户接入点是订阅的唯一生成来源：定义别名与开放权限组（白名单）。订阅地址沿管道继承：直连继承入站分享地址（节点 IP/端口）或接入层端点，接入点可再用 Host/Port 覆写（如自定义中转端点）。"
          style="margin-bottom: 14px"
        />

        <BaseCard>
          <el-table v-loading="apLoading" :data="apList">
            <el-table-column prop="id" label="ID" width="64">
              <template #default="{ row }"><code class="cell-mono">#{{ row.id }}</code></template>
            </el-table-column>
            <el-table-column label="名称" min-width="170">
              <template #default="{ row }"><span style="font-weight: 600">{{ row.name }}</span></template>
            </el-table-column>
            <el-table-column label="开放权限组" min-width="160">
              <template #default="{ row }">
                <template v-if="row.permission_group_ids && row.permission_group_ids.length > 0">
                  <span
                    v-for="gid in row.permission_group_ids.slice(0, 3)"
                    :key="gid"
                    class="x-chip blue"
                    style="margin-right: 4px"
                  >{{ groupName(gid) }}</span>
                  <span v-if="row.permission_group_ids.length > 3" class="x-chip gray">+{{ row.permission_group_ids.length - 3 }}</span>
                </template>
                <span v-else class="x-chip orange">未授权（全员不可见）</span>
              </template>
            </el-table-column>
            <el-table-column label="目标" min-width="160">
              <template #default="{ row }">
                <span v-if="apTargetDesc(row) === '待连线'" class="x-chip orange">{{ apTargetDesc(row) }}</span>
                <span v-else class="x-chip purple">{{ apTargetDesc(row) }}</span>
              </template>
            </el-table-column>
            <el-table-column label="连接覆写" width="140">
              <template #default="{ row }">
                <code class="cell-mono" style="font-size: 11px">
                  {{ row.custom_host ? `${row.custom_host}:${row.custom_port || '自动'}` : '自动继承' }}
                </code>
              </template>
            </el-table-column>
            <el-table-column label="状态" width="80">
              <template #default="{ row }">
                <el-switch :model-value="row.enabled" @change="toggleAccessPoint(row)" />
              </template>
            </el-table-column>
            <el-table-column label="操作" width="120" fixed="right">
              <template #default="{ row }">
                <el-button size="small" text @click="openEditAccessPoint(row)"><el-icon><Edit /></el-icon></el-button>
                <el-button size="small" text type="danger" @click="removeAccessPoint(row)"><el-icon><Delete /></el-icon></el-button>
              </template>
            </el-table-column>
            <template #empty>
              <div style="padding: 30px 0; color: var(--x-text-3)">
                尚未创建任何用户接入点。新建后即可作为用户订阅的入口（订阅仅从接入点生成）。
              </div>
            </template>
          </el-table>
        </BaseCard>

        <!-- 新建/编辑用户接入点 -->
        <el-dialog
          v-model="apDialogOpen"
          :title="apEditingId ? '编辑用户接入点 (Access Point)' : '新建用户接入点 (Access Point)'"
          width="580px"
          append-to-body
        >
          <el-form label-position="top">
            <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0 16px; align-items: start">
              <el-form-item label="接入点 Tag 名称" required>
                <el-input v-model="apForm.name" placeholder="如 香港直连 01, 广州移动 BGP" />
              </el-form-item>
              <el-form-item label="启用状态">
                <el-switch v-model="apForm.enabled" active-text="启用" inactive-text="禁用" style="margin-top: 4px" />
              </el-form-item>
            </div>

            <el-form-item label="开放权限组（显式白名单，勾选可见的用户组）">
              <el-select
                v-model="apForm.permission_group_ids"
                multiple
                collapse-tags
                collapse-tags-tooltip
                placeholder="请勾选可见的权限组"
                style="width: 100%"
              >
                <el-option v-for="g in apGroups" :key="g.id" :label="g.name" :value="g.id" />
              </el-select>
            </el-form-item>

            <el-form-item label="目标绑定方式（亦可在拓扑画布上拖拽连线）">
              <el-radio-group v-model="apForm.target_type" style="width: 100%">
                <el-radio-button value="">待连线 / 未绑定</el-radio-button>
                <el-radio-button value="inbound">直连落地入站</el-radio-button>
              </el-radio-group>
            </el-form-item>

            <div
              v-if="apForm.target_type === 'inbound'"
              style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px; background: var(--x-card-soft); padding: 12px; border-radius: 8px; margin-bottom: 16px; border: 1px dashed var(--x-border)"
            >
              <el-form-item label="目标落地服务器" style="margin-bottom: 0">
                <el-select v-model="apTargetServerId" placeholder="选择 Xray 服务器" style="width: 100%" @change="apForm.target_inbound_id = undefined">
                  <el-option v-for="s in servers" :key="s.id" :label="`${s.name} (${s.host})`" :value="s.id" />
                </el-select>
              </el-form-item>
              <el-form-item label="目标用户入站 (Target Inbound)" style="margin-bottom: 0">
                <el-select v-model="apForm.target_inbound_id" placeholder="选择用户入站" style="width: 100%">
                  <el-option v-for="inb in apAvailableInbounds" :key="inb.id" :label="`${inb.tag} (:${inb.port})`" :value="inb.id" />
                </el-select>
              </el-form-item>
            </div>

            <div style="background: var(--x-card-soft); border: 1px solid var(--x-border); border-radius: 8px; padding: 12px; margin-bottom: 16px">
              <div style="font-size: 12px; font-weight: 600; color: var(--x-text-3); margin-bottom: 8px">
                订阅地址覆写（选填；留空沿管道继承：直连→入站分享地址 / 接入层端点）
              </div>
              <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
                <el-form-item label="自定义连接 Host" style="margin-bottom: 0">
                  <el-input v-model="apForm.custom_host" placeholder="留空自动继承" />
                </el-form-item>
                <el-form-item label="自定义连接 Port" style="margin-bottom: 0">
                  <el-input-number v-model="apForm.custom_port" :min="0" :max="65535" placeholder="0 自动继承" style="width: 100%" />
                </el-form-item>
              </div>
            </div>

            <el-form-item label="备注说明" style="margin-bottom: 0">
              <el-input v-model="apForm.remark" placeholder="选填，如 VIP 专享中转" />
            </el-form-item>
          </el-form>
          <template #footer>
            <div style="display: flex; justify-content: space-between; align-items: center">
              <div>
                <el-button v-if="apEditingId" type="danger" plain @click="removeAccessPoint(apList.find((a) => a.id === apEditingId)!)">
                  删除接入点
                </el-button>
              </div>
              <div>
                <el-button @click="apDialogOpen = false">取消</el-button>
                <el-button type="primary" :loading="apSaving" @click="saveAccessPoint">保存接入点</el-button>
              </div>
            </div>
          </template>
        </el-dialog>
      </el-tab-pane>
    </el-tabs>

    <!-- 新增/编辑入站 -->
    <el-dialog
      v-model="editorOpen"
      :title="editing ? `编辑入站 · ${editing.tag}` : '新增节点入站'"
      width="640px"
      :append-to-body="true"
      @closed="editing = null"
    >
      <div v-if="!editing" style="margin-bottom: 14px">
        <el-form-item label="所属目标服务器" style="margin-bottom: 0">
          <el-select v-model="formServerId" style="width: 100%" placeholder="请选择要绑定入站的服务器">
            <el-option v-for="s in servers" :key="s.id" :label="`${s.name} (${s.host})`" :value="s.id" />
          </el-select>
        </el-form-item>
      </div>

      <InboundConfigEditor
        :key="editing ? `edit-${editing.id}` : 'create'"
        :model-value="editorModelValue()"
        :inbound-type="formType"
        :listen="editing?.listen || '0.0.0.0'"
        :internal-uuid="editing?.internal_uuid || ''"
        :inbound-id="editing?.id || 0"
        :cert-id="formCertId"
        :server-id="formServerId"
        :layer-id="editing?.layer_id || 0"
        @change="onInboundEditorChange"
        @update:inbound-type="(v: string) => (formType = v)"
        @update:cert-id="(v: number) => (formCertId = v || 0)"
        @internal-uuid-changed="load"
      />
      <template #footer>
        <el-button @click="editorOpen = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存入站</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.cell-mono {
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 12.5px;
  color: var(--x-text-2);
}
.muted {
  color: var(--x-text-3);
}
.cfg-view {
  background: #171b2e;
  color: #c7d2fe;
  border-radius: 8px;
  padding: 14px;
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.6;
  max-height: 480px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
