<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Delete, Key, CopyDocument, Refresh, Plus, Edit } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  deleteServer,
  getServerConfigPreview,
  getInbounds,
  resetServerSecret,
  getL4Rules,
  createL4Rule,
  updateL4Rule,
  deleteL4Rule,
  getPermissionGroups,
  getServers,
  type InboundItem,
  type ServerItem,
  type L4PortRule,
  type PermissionGroup,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import { maskUUIDs } from '@/utils/mask'

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

const activeTab = ref('overview')

function fmtTime(t: string | null) {
  if (!t) return '—'
  return new Date(t).toLocaleString('zh-CN', { hour12: false })
}

// ---- 概览：接入点摘要 ----
const inbounds = ref<InboundItem[]>([])
const inboundsLoading = ref(false)

async function loadInbounds() {
  if (!props.server || props.server.server_type === 'l4_relay') return
  inboundsLoading.value = true
  try {
    const { data } = await getInbounds(props.server.id)
    if (data.code === 0) inbounds.value = data.data.items
  } catch {
    /* 摘要加载失败不打扰 */
  } finally {
    inboundsLoading.value = false
  }
}

function goInbounds() {
  if (!props.server) return
  visible.value = false
  router.push({ path: '/admin/nodes', query: { server_id: props.server.id } })
}

// ---- L4 转发规则管理 ----
const l4Rules = ref<L4PortRule[]>([])
const l4RulesLoading = ref(false)
const l4RuleOpen = ref(false)
const l4RuleSaving = ref(false)
const l4RuleEditing = ref<L4PortRule | null>(null)
const allServers = ref<ServerItem[]>([])
const allInbounds = ref<InboundItem[]>([])
const permissionGroups = ref<PermissionGroup[]>([])

const l4Form = reactive({
  listen_port: 30001,
  target_server_id: 0,
  target_inbound_id: 0,
  remark: '',
  enabled: true,
  permission_group_ids: [] as number[],
})

const availableXrayServers = computed(() => {
  return allServers.value.filter((s) => s.server_type !== 'l4_relay')
})

const availableTargetInbounds = computed(() => {
  if (!l4Form.target_server_id) return []
  return allInbounds.value.filter(
    (i) => i.server_id === l4Form.target_server_id && i.enabled && i.type === 'user',
  )
})

async function loadL4Rules() {
  if (!props.server || props.server.server_type !== 'l4_relay') return
  l4RulesLoading.value = true
  try {
    const { data } = await getL4Rules(props.server.id)
    if (data.code === 0) l4Rules.value = data.data
  } catch {
    /* ignore */
  } finally {
    l4RulesLoading.value = false
  }
}

async function prepareL4FormData() {
  try {
    const [s, i, g] = await Promise.all([getServers(), getInbounds(), getPermissionGroups()])
    if (s.data.code === 0) allServers.value = s.data.data.items
    if (i.data.code === 0) allInbounds.value = i.data.data.items
    if (g.data.code === 0) permissionGroups.value = g.data.data.items
  } catch {}
}

async function openCreateL4Rule() {
  if (!props.server) return
  await prepareL4FormData()
  l4RuleEditing.value = null
  l4Form.listen_port = 30001
  l4Form.target_server_id = availableXrayServers.value[0]?.id || 0
  l4Form.target_inbound_id = 0
  l4Form.remark = ''
  l4Form.enabled = true
  l4Form.permission_group_ids = []
  l4RuleOpen.value = true
}

async function openEditL4Rule(rule: L4PortRule) {
  if (!props.server) return
  await prepareL4FormData()
  l4RuleEditing.value = rule
  l4Form.listen_port = rule.listen_port
  l4Form.target_server_id = rule.target_server_id
  l4Form.target_inbound_id = rule.target_inbound_id
  l4Form.remark = rule.remark || ''
  l4Form.enabled = rule.enabled
  l4Form.permission_group_ids = rule.permission_group_ids || []
  l4RuleOpen.value = true
}

async function submitL4Rule() {
  if (!props.server) return
  if (!l4Form.listen_port || l4Form.listen_port <= 0 || l4Form.listen_port > 65535) {
    ElMessage.warning('请填写有效的监听端口 (1-65535)')
    return
  }
  if (!l4Form.target_inbound_id) {
    ElMessage.warning('请选择目标落地入站')
    return
  }
  l4RuleSaving.value = true
  try {
    const payload = {
      listen_port: l4Form.listen_port,
      target_server_id: l4Form.target_server_id,
      target_inbound_id: l4Form.target_inbound_id,
      remark: l4Form.remark,
      enabled: l4Form.enabled,
      permission_group_ids: l4Form.permission_group_ids,
    }
    const { data } = l4RuleEditing.value
      ? await updateL4Rule(props.server.id, l4RuleEditing.value.id, payload)
      : await createL4Rule(props.server.id, payload)
    if (data.code === 0) {
      ElMessage.success(l4RuleEditing.value ? '转发规则已更新' : '转发规则已创建')
      l4RuleOpen.value = false
      loadL4Rules()
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存规则失败'))
  } finally {
    l4RuleSaving.value = false
  }
}

async function removeL4Rule(rule: L4PortRule) {
  if (!props.server) return
  try {
    await ElMessageBox.confirm(`确认删除端口 :${rule.listen_port} 的转发规则？`, '删除规则', { type: 'warning' })
  } catch {
    return
  }
  try {
    const { data } = await deleteL4Rule(props.server.id, rule.id)
    if (data.code === 0) {
      ElMessage.success('已删除转发规则')
      loadL4Rules()
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

// ---- 概览：重置密钥 / 删除 ----
const secretInfo = ref<{ node_id: string; secret: string; install_cmd?: string } | null>(null)
const resetting = ref(false)

async function resetSecret() {
  if (!props.server) return
  try {
    await ElMessageBox.confirm(
      `确认重置节点「${props.server.name}」的密钥？旧密钥立即失效，需在节点 Agent 配置（/etc/xray-agent/config.yml）中更新。`,
      '重置密钥',
      { type: 'warning' },
    )
  } catch {
    return
  }
  resetting.value = true
  try {
    const { data } = await resetServerSecret(props.server.id)
    if (data.code === 0) {
      secretInfo.value = { node_id: data.data.node_id, secret: data.data.secret, install_cmd: data.data.install_cmd }
      ElMessage.success('密钥已重置')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '重置失败'))
  } finally {
    resetting.value = false
  }
}

const deleting = ref(false)
async function removeServer() {
  if (!props.server) return
  try {
    await ElMessageBox.confirm(
      `确认删除服务器「${props.server.name}」？关联的所有规则将一并删除。`,
      '删除服务器',
      { type: 'error' },
    )
  } catch {
    return
  }
  deleting.value = true
  try {
    const { data } = await deleteServer(props.server.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      emit('removed')
      visible.value = false
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  } finally {
    deleting.value = false
  }
}

async function copyText(text: string, label: string) {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`${label}已复制`)
  } catch {
    ElMessage.warning('复制失败，请手动复制')
  }
}

// ---- 配置预览 Tab（声明式只读预览） ----
const cfgLoading = ref(false)
const cfgText = ref('')

async function loadConfigPreview() {
  if (!props.server || props.server.server_type === 'l4_relay') return
  cfgLoading.value = true
  cfgText.value = ''
  try {
    const { data } = await getServerConfigPreview(props.server.id)
    if (data.code === 0) {
      cfgText.value = data.data.config || ''
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载配置预览失败'))
  } finally {
    cfgLoading.value = false
  }
}

watch(
  () => [props.modelValue, props.server, activeTab.value],
  () => {
    if (props.modelValue && props.server) {
      if (props.server.server_type === 'l4_relay') {
        if (l4Rules.value.length === 0) loadL4Rules()
      } else {
        if (activeTab.value === 'overview' && inbounds.value.length === 0) {
          loadInbounds()
        } else if (activeTab.value === 'config' && !cfgText.value) {
          loadConfigPreview()
        }
      }
    }
  },
  { immediate: true },
)

watch(
  () => [props.modelValue, props.server],
  () => {
    if (props.modelValue && props.server) {
      activeTab.value = props.server.server_type === 'l4_relay' ? 'l4_rules' : 'overview'
      secretInfo.value = null
      cfgText.value = ''
      if (props.server.server_type === 'l4_relay') {
        loadL4Rules()
      } else {
        loadInbounds()
      }
    }
  },
)
</script>

<template>
  <el-dialog
    v-model="visible"
    :title="server?.name ? `服务器详情 · ${server.name}` : '服务器详情'"
    width="680px"
    :append-to-body="true"
  >
    <el-tabs v-model="activeTab" class="dialog-tabs">
      <!-- 概览 -->
      <el-tab-pane label="基本信息" name="overview">
        <template v-if="server">
          <div class="desc-grid">
            <div class="desc-row"><span class="k">节点名称</span><span class="v">{{ server.name }}</span></div>
            <div class="desc-row">
              <span class="k">服务器类型</span>
              <span class="v">
                <span v-if="server.server_type === 'l4_relay'" class="x-chip purple">L4 纯端口转发服务器</span>
                <span v-else class="x-chip blue">Xray 托管计算节点</span>
              </span>
            </div>
            <div class="desc-row"><span class="k">主机地址</span><span class="v"><code class="cell-mono">{{ server.host }}</code></span></div>
            <div v-if="server.server_type !== 'l4_relay'" class="desc-row">
              <span class="k">Node ID</span>
              <span class="v">
                <code class="cell-mono">{{ server.node_id }}</code>
                <el-button size="small" text @click="copyText(server.node_id, 'Node ID')"><el-icon><CopyDocument /></el-icon></el-button>
              </span>
            </div>
            <div class="desc-row"><span class="k">所在地区</span><span class="v">{{ server.location || '—' }}</span></div>
            <div class="desc-row"><span class="k">备注说明</span><span class="v">{{ server.remark || '—' }}</span></div>
            <div class="desc-row">
              <span class="k">运行状态</span>
              <span class="v">
                <span v-if="server.server_type === 'l4_relay'" class="x-chip purple">
                  <span class="x-status-dot online" style="background: #c084fc; box-shadow: 0 0 8px #c084fc" /> 就绪
                </span>
                <span v-else class="x-chip" :class="server.status === 1 ? 'green' : 'gray'">
                  <span class="x-status-dot" :class="server.status === 1 ? 'online' : 'offline'" />{{ server.status === 1 ? '在线' : '离线' }}
                </span>
              </span>
            </div>
            <div v-if="server.server_type !== 'l4_relay'" class="desc-row">
              <span class="k">配置同步</span>
              <span class="v">
                <span v-if="server.config_status === 'pushed'" class="x-chip green">已同步</span>
                <span v-else-if="server.config_status === 'pending'" class="x-chip orange">待推送</span>
                <span v-else class="x-chip gray">未生成</span>
              </span>
            </div>
            <div v-if="server.server_type !== 'l4_relay'" class="desc-row"><span class="k">最后心跳</span><span class="v">{{ fmtTime(server.last_seen_at) }}</span></div>
            <div v-if="server.server_type === 'l4_relay'" class="desc-row">
              <span class="k">转发规则</span>
              <span class="v">
                <template v-if="l4RulesLoading">…</template>
                <template v-else>{{ l4Rules.length }} 条端口规则</template>
              </span>
            </div>
            <div v-else class="desc-row">
              <span class="k">已配置接入点</span>
              <span class="v">
                <template v-if="inboundsLoading">…</template>
                <template v-else>{{ inbounds.length }} 个入站</template>
                <el-button size="small" text type="primary" @click="goInbounds">去管理</el-button>
              </span>
            </div>
          </div>

          <div v-if="server.server_type !== 'l4_relay' && secretInfo" class="secret-box" style="margin-top: 14px">
            <div class="sec-title">重置凭据</div>
            <div class="secret-row"><span class="k">node_id</span><code>{{ secretInfo.node_id }}</code></div>
            <div class="secret-row">
              <span class="k">secret</span><code>{{ secretInfo.secret }}</code>
              <el-button size="small" text @click="copyText(secretInfo.secret, 'secret')"><el-icon><CopyDocument /></el-icon></el-button>
            </div>
            <div v-if="secretInfo.install_cmd" class="secret-row">
              <span class="k">一键安装</span>
              <code class="install-cmd">{{ secretInfo.install_cmd }}</code>
              <el-button size="small" text @click="copyText(secretInfo.install_cmd!, '安装命令')"><el-icon><CopyDocument /></el-icon></el-button>
            </div>
          </div>

          <div class="action-row" style="margin-top: 18px">
            <el-button v-if="server.server_type !== 'l4_relay'" :loading="resetting" @click="resetSecret">
              <el-icon><Key /></el-icon>&nbsp;重置密钥
            </el-button>
            <el-button type="danger" plain :loading="deleting" @click="removeServer">
              <el-icon><Delete /></el-icon>&nbsp;删除服务器
            </el-button>
          </div>
        </template>
        <el-empty v-else description="未选择节点" />
      </el-tab-pane>

      <!-- L4 端口转发规则 Tab -->
      <el-tab-pane v-if="server?.server_type === 'l4_relay'" label="端口转发规则" name="l4_rules">
        <div class="tab-toolbar">
          <el-button size="small" :loading="l4RulesLoading" @click="loadL4Rules">
            <el-icon><Refresh /></el-icon>&nbsp;刷新
          </el-button>
          <el-button size="small" type="primary" @click="openCreateL4Rule">
            <el-icon><Plus /></el-icon>&nbsp;新增转发端口
          </el-button>
        </div>

        <el-table v-loading="l4RulesLoading" :data="l4Rules" size="small" style="width: 100%">
          <el-table-column prop="listen_port" label="监听端口" width="110">
            <template #default="{ row }">
              <span class="cell-mono" style="font-weight: 700; color: #c084fc">:{{ row.listen_port }}</span>
            </template>
          </el-table-column>
          <el-table-column label="目标用户入站" min-width="160">
            <template #default="{ row }">
              <span v-if="row.target_inbound_tag" class="tag out-tag ref">
                {{ row.target_server_name || '#' + row.target_server_id }} / {{ row.target_inbound_tag }}
              </span>
              <span v-else class="tag out-tag draft">待连线映射</span>
            </template>
          </el-table-column>
          <el-table-column prop="remark" label="备注" min-width="120">
            <template #default="{ row }">{{ row.remark || '—' }}</template>
          </el-table-column>
          <el-table-column label="状态" width="80">
            <template #default="{ row }">
              <el-tag size="small" :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '启用' : '禁用' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="120" fixed="right">
            <template #default="{ row }">
              <el-button size="small" text type="primary" @click="openEditL4Rule(row as any)">编辑</el-button>
              <el-button size="small" text type="danger" @click="removeL4Rule(row as any)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 配置预览（仅 Xray 节点） -->
      <el-tab-pane v-if="server?.server_type !== 'l4_relay'" label="配置预览" name="config">
        <div class="tab-toolbar">
          <el-button size="small" :loading="cfgLoading" @click="loadConfigPreview">
            <el-icon><Refresh /></el-icon>&nbsp;刷新
          </el-button>
          <el-button
            size="small"
            type="primary"
            :disabled="!cfgText"
            @click="copyText(cfgText, '配置 JSON')"
          >
            <el-icon><CopyDocument /></el-icon>&nbsp;复制配置
          </el-button>
        </div>
        <p class="muted tip" style="margin: 0 0 10px; font-size: 12.5px">
          此配置为主控根据当前入站、出站、路由规则与有效用户实时渲染的目标 Xray 配置（业务变更系统会自动同步推送节点）。展示内容已对用户 UUID 脱敏，复制时保留原文。
        </p>
        <pre v-loading="cfgLoading" class="cfg-view">{{ maskUUIDs(cfgText) || '正在计算并加载配置预览…' }}</pre>
      </el-tab-pane>
    </el-tabs>

    <template #footer>
      <el-button type="primary" @click="visible = false">关闭</el-button>
    </template>
  </el-dialog>

  <!-- L4 端口转发规则编辑弹窗 -->
  <el-dialog v-model="l4RuleOpen" :title="l4RuleEditing ? '编辑 L4 转发规则' : '新增 L4 转发规则'" width="520px" append-to-body>
    <el-form label-position="top">
      <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
        <el-form-item label="中转监听端口 ListenPort" required>
          <el-input-number v-model="l4Form.listen_port" :min="1" :max="65535" style="width: 100%" />
        </el-form-item>
        <el-form-item label="目标落地服务器" required>
          <el-select v-model="l4Form.target_server_id" style="width: 100%" placeholder="选择目标节点" @change="l4Form.target_inbound_id = 0">
            <el-option v-for="s in availableXrayServers" :key="s.id" :label="`${s.name} (${s.host})`" :value="s.id" />
          </el-select>
        </el-form-item>
      </div>
      <el-form-item label="目标用户入站 (Target Inbound)" required>
        <el-select v-model="l4Form.target_inbound_id" style="width: 100%" placeholder="请选择目标用户入站">
          <el-option v-for="inb in availableTargetInbounds" :key="inb.id" :label="`${inb.tag} (:${inb.port})`" :value="inb.id" />
        </el-select>
      </el-form-item>
      <el-form-item label="开放权限组（显式白名单，留空对全员不可见）">
        <el-select v-model="l4Form.permission_group_ids" multiple collapse-tags collapse-tags-tooltip placeholder="请勾选可见权限组" style="width: 100%">
          <el-option v-for="g in permissionGroups" :key="g.id" :label="g.name" :value="g.id" />
        </el-select>
      </el-form-item>
      <el-form-item label="备注说明">
        <el-input v-model="l4Form.remark" placeholder="如 广州移动 10G BGP 优化" />
      </el-form-item>
    </el-form>
    <template #footer>
      <el-button @click="l4RuleOpen = false">取消</el-button>
      <el-button type="primary" :loading="l4RuleSaving" @click="submitL4Rule">保存规则</el-button>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.dialog-tabs {
  margin-top: -6px;
}
.tab-toolbar {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-bottom: 10px;
}
.sec-title {
  font-weight: 600;
  font-size: 13px;
  margin-bottom: 8px;
}
.desc-grid {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.desc-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 12px;
  border-radius: 8px;
  background: var(--x-bg-card);
  border: 1px solid var(--x-border);
  font-size: 13px;
  .k {
    color: var(--x-text-3);
  }
  .v {
    font-weight: 500;
    display: flex;
    align-items: center;
    gap: 6px;
  }
}
.action-row {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
}
.cfg-view {
  background: #0f172a;
  color: #e2e8f0;
  padding: 14px;
  border-radius: 8px;
  font-family: monospace;
  font-size: 12px;
  max-height: 400px;
  overflow: auto;
  line-height: 1.5;
}
</style>
