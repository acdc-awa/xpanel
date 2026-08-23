<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { Plus, Delete, Edit, Refresh, Key, Connection, Check } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  getInboundEndpoints,
  createInboundEndpoint,
  updateInboundEndpoint,
  deleteInboundEndpoint,
  getPermissionGroups,
  type InboundEndpoint,
  type InboundItem,
  type PermissionGroup,
} from '@/api/admin'
import { errMsg } from '@/api/http'

const props = defineProps<{
  modelValue: boolean
  inbound: InboundItem | null
  serverName?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'changed'): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const list = ref<InboundEndpoint[]>([])
const loading = ref(false)
const groups = ref<PermissionGroup[]>([])

// 编辑弹窗
const editOpen = ref(false)
const isCreate = ref(true)
const editingId = ref(0)
const form = ref({
  name: '',
  host: '',
  port: 443,
  permission_group_ids: [] as number[],
  enabled: true,
  priority: 0,
  remark: '',
})
const saving = ref(false)

async function loadGroups() {
  try {
    const { data } = await getPermissionGroups()
    if (data.code === 0) groups.value = data.data.items
  } catch {
    /* ignore */
  }
}

async function loadEndpoints() {
  if (!props.inbound) return
  loading.value = true
  try {
    const { data } = await getInboundEndpoints(props.inbound.id)
    if (data.code === 0) {
      list.value = data.data.items
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载附加接入点失败'))
  } finally {
    loading.value = false
  }
}

watch(
  () => props.modelValue,
  (v) => {
    if (v && props.inbound) {
      loadGroups()
      loadEndpoints()
    }
  },
)

function openCreate(prefill?: { host?: string; port?: number; name?: string }) {
  isCreate.value = true
  editingId.value = 0
  form.value = {
    name: prefill?.name || '',
    host: prefill?.host || '',
    port: prefill?.port || 443,
    permission_group_ids: [],
    enabled: true,
    priority: list.value.length * 10,
    remark: '',
  }
  editOpen.value = true
}

function openEdit(ep: InboundEndpoint) {
  isCreate.value = false
  editingId.value = ep.id
  form.value = {
    name: ep.name,
    host: ep.host,
    port: ep.port,
    permission_group_ids: ep.permission_group_ids ? [...ep.permission_group_ids] : [],
    enabled: ep.enabled,
    priority: ep.priority,
    remark: ep.remark || '',
  }
  editOpen.value = true
}

async function saveEndpoint() {
  if (!props.inbound) return
  if (!form.value.name.trim()) {
    ElMessage.warning('请输入接入点名称后缀')
    return
  }
  if (!form.value.host.trim()) {
    ElMessage.warning('请输入接入主机地址 (IP或域名)')
    return
  }
  if (form.value.port < 1 || form.value.port > 65535) {
    ElMessage.warning('端口范围须在 1-65535 之间')
    return
  }

  saving.value = true
  try {
    if (isCreate.value) {
      const { data } = await createInboundEndpoint(props.inbound.id, {
        name: form.value.name.trim(),
        host: form.value.host.trim(),
        port: form.value.port,
        permission_group_ids: form.value.permission_group_ids,
        enabled: form.value.enabled,
        priority: form.value.priority,
        remark: form.value.remark.trim(),
      })
      if (data.code === 0) {
        ElMessage.success('已创建附加接入点')
        editOpen.value = false
        loadEndpoints()
        emit('changed')
      } else {
        ElMessage.error(data.message)
      }
    } else {
      const { data } = await updateInboundEndpoint(props.inbound.id, editingId.value, {
        name: form.value.name.trim(),
        host: form.value.host.trim(),
        port: form.value.port,
        permission_group_ids: form.value.permission_group_ids,
        enabled: form.value.enabled,
        priority: form.value.priority,
        remark: form.value.remark.trim(),
      })
      if (data.code === 0) {
        ElMessage.success('已更新附加接入点')
        editOpen.value = false
        loadEndpoints()
        emit('changed')
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

async function removeEndpoint(ep: InboundEndpoint) {
  if (!props.inbound) return
  try {
    await ElMessageBox.confirm(`确认删除接入点「${ep.name}」(${ep.host}:${ep.port})？`, '删除接入点', {
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    const { data } = await deleteInboundEndpoint(props.inbound.id, ep.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      loadEndpoints()
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

function getGroupNames(ids?: number[]): string {
  if (!ids || ids.length === 0) return '未配置（对所有用户不可见）'
  const names: string[] = []
  for (const id of ids) {
    const g = groups.value.find((item) => item.id === id)
    names.push(g ? g.name : `组#${id}`)
  }
  return names.join(', ')
}

defineExpose({
  openCreate,
})
</script>

<template>
  <el-drawer
    v-model="visible"
    :title="`附加接入点管理 - ${inbound?.tag || ''} (${serverName || ''})`"
    size="680px"
    destroy-on-close
  >
    <div class="drawer-content">
      <!-- 提示条 -->
      <div class="info-banner">
        <div class="info-title">💡 单入站多接入点体系 (Inbound Endpoints)</div>
        <div class="info-desc">
          Xray 核心仅需监听 1 个物理入站（节省端口与内存），在此处可配置多个外部公网连接点（如 BGP 中转、IPv6 直连、备用 CDN 域名）。
          <strong>仅指定了开放权限组的用户才会在订阅中拉取到该线路。</strong>
        </div>
      </div>

      <!-- 操作栏 -->
      <div class="actions-bar">
        <div class="bar-left">
          <span style="font-size: 13px; font-weight: 600; color: var(--x-text-2)">
            已配置附加接入点 ({{ list.length }})
          </span>
        </div>
        <div class="bar-right">
          <el-button size="small" @click="loadEndpoints">
            <el-icon><Refresh /></el-icon>&nbsp;刷新
          </el-button>
          <el-button size="small" type="primary" @click="openCreate()">
            <el-icon><Plus /></el-icon>&nbsp;新增接入点
          </el-button>
        </div>
      </div>

      <!-- 接入点列表 -->
      <div v-loading="loading" class="endpoints-list">
        <div v-if="list.length === 0" class="empty-box">
          <el-empty description="暂无附加接入点，点击上方「新增接入点」添加 BGP 中转或 IPv6 入口" />
        </div>
        <div v-else class="endpoint-card-list">
          <div v-for="ep in list" :key="ep.id" class="endpoint-card" :class="{ disabled: !ep.enabled }">
            <div class="card-head">
              <div class="head-left">
                <span class="ep-badge">{{ ep.name }}</span>
                <span class="ep-addr cell-mono">{{ ep.host }}:{{ ep.port }}</span>
                <el-tag v-if="!ep.enabled" size="small" type="info">已禁用</el-tag>
              </div>
              <div class="head-actions">
                <el-button size="small" text type="primary" @click="openEdit(ep)">
                  <el-icon><Edit /></el-icon>&nbsp;编辑
                </el-button>
                <el-button size="small" text type="danger" @click="removeEndpoint(ep)">
                  <el-icon><Delete /></el-icon>&nbsp;删除
                </el-button>
              </div>
            </div>

            <div class="card-body">
              <div class="prop-item">
                <span class="prop-label">开放权限组:</span>
                <span v-if="ep.permission_group_ids && ep.permission_group_ids.length > 0" class="prop-val">
                  <el-tag
                    v-for="gid in ep.permission_group_ids"
                    :key="gid"
                    size="small"
                    type="success"
                    effect="plain"
                    style="margin-right: 4px"
                  >
                    {{ groups.find((g) => g.id === gid)?.name || `组#${gid}` }}
                  </el-tag>
                </span>
                <span v-else class="prop-val text-warning" style="font-size: 12px">
                  未配置（对所有用户不可见）
                </span>
              </div>
              <div v-if="ep.remark" class="prop-item">
                <span class="prop-label">备注说明:</span>
                <span class="prop-val text-muted">{{ ep.remark }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- 创建/编辑接入点弹窗 -->
    <el-dialog
      v-model="editOpen"
      :title="isCreate ? '新增附加接入点' : '编辑附加接入点'"
      width="540px"
      append-to-body
    >
      <el-form label-position="top">
        <el-form-item label="接入点名称后缀" required>
          <el-input v-model="form.name" placeholder="如 广州BGP中转、IPv6直连、备用CDN" />
          <div class="form-tip">订阅中展示为「服务器名 | 入站名 | 接入点名称」</div>
        </el-form-item>

        <div style="display: grid; grid-template-columns: 2fr 1fr; gap: 0 16px">
          <el-form-item label="接入地址 (Host / IP / 域名)" required>
            <el-input v-model="form.host" placeholder="如 120.24.1.1 或 gz.bgp.com" />
          </el-form-item>
          <el-form-item label="接入端口 (Port)" required>
            <el-input-number v-model="form.port" :min="1" :max="65535" style="width: 100%" />
          </el-form-item>
        </div>

        <el-form-item label="开放权限组（显式白名单，留空=对所有人隐藏）">
          <el-select
            v-model="form.permission_group_ids"
            multiple
            collapse-tags
            collapse-tags-tooltip
            placeholder="请选择允许拉取该接入点的权限组"
            style="width: 100%"
          >
            <el-option v-for="g in groups" :key="g.id" :label="g.name" :value="g.id" />
          </el-select>
          <div class="form-tip">
            <span class="text-warning">⚠️ 显式安全模型：</span>未选权限组的接入点对所有用户隐藏，防止未就绪的专线意外泄露。
          </div>
        </el-form-item>

        <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
          <el-form-item label="启用状态">
            <el-switch v-model="form.enabled" active-text="启用" inactive-text="停用" />
          </el-form-item>
          <el-form-item label="排序优先级">
            <el-input-number v-model="form.priority" :min="0" style="width: 100%" />
          </el-form-item>
        </div>

        <el-form-item label="备注说明">
          <el-input v-model="form.remark" placeholder="如 仅限 VIP 用户、移动网络专享" />
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="editOpen = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="saveEndpoint">保存接入点</el-button>
      </template>
    </el-dialog>
  </el-drawer>
</template>

<style scoped lang="scss">
.drawer-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.info-banner {
  background: rgba(56, 189, 248, 0.08);
  border: 1px solid rgba(56, 189, 248, 0.2);
  border-radius: 8px;
  padding: 12px 14px;

  .info-title {
    font-size: 13.5px;
    font-weight: 600;
    color: #38bdf8;
    margin-bottom: 4px;
  }
  .info-desc {
    font-size: 12.5px;
    color: var(--x-text-2);
    line-height: 1.5;
  }
}

.actions-bar {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.endpoint-card-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.endpoint-card {
  background: rgba(15, 23, 42, 0.6);
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 12px 14px;
  transition: all 0.2s;

  &:hover {
    border-color: rgba(56, 189, 248, 0.3);
  }

  &.disabled {
    opacity: 0.6;
  }

  .card-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 8px;

    .head-left {
      display: flex;
      align-items: center;
      gap: 8px;
    }

    .ep-badge {
      font-size: 13.5px;
      font-weight: 600;
      color: #38bdf8;
    }

    .ep-addr {
      font-size: 13px;
      color: var(--x-text-1);
    }
  }

  .card-body {
    display: flex;
    flex-direction: column;
    gap: 6px;
    font-size: 12.5px;

    .prop-item {
      display: flex;
      align-items: center;
      gap: 6px;

      .prop-label {
        color: var(--x-text-3);
      }
    }
  }
}

.form-tip {
  font-size: 12px;
  color: var(--x-text-3);
  margin-top: 4px;
}

.text-warning {
  color: #fbbf24;
}
.text-muted {
  color: var(--x-text-3);
}
.cell-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}
</style>
