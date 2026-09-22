<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Delete, Key, CopyDocument, Refresh, Link } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import {
  deleteServer,
  getServerConfigPreview,
  getInbounds,
  resetServerSecret,
  type InboundItem,
  type ServerItem,
  } from '@/api/admin'
import { errMsg } from '@/api/http'
import { maskUUIDs } from '@/utils/mask'
import { formatDateTime } from '@/utils/timezone'
import { getExpiryStatus, formatExpireDate, normalizeUrl, isUrl } from '@/utils/vps'
import OnlineUsersPanel from './OnlineUsersPanel.vue'
import TipIcon from '@/components/base/TipIcon.vue'

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
const onlinePanelRef = ref<InstanceType<typeof OnlineUsersPanel> | null>(null)

function fmtTime(t: string | null) {
  if (!t) return '—'
  return formatDateTime(t, '—')
}

// Xray 进程状态徽标：优先用 agent 回传的 xray_state（能区分"重试中"与"已停止拉起"），
// 旧 agent 不带该字段时退回 xray_running 布尔。
const xrayFailed = computed(() => {
  const s = props.server?.xray_state
  return s === 'failed' || s === 'restarting'
})
const xrayChip = computed(() => {
  const s = props.server?.xray_state
  if (s === 'failed') return { cls: 'red', text: '启动失败（已停止重试）' }
  if (s === 'restarting') return { cls: 'orange', text: '启动失败，重试中' }
  if (props.server?.xray_running) return { cls: 'green', text: '运行中' }
  return { cls: 'red', text: '未运行' }
})

// 配置同步状态：后端 push_state 优先（四态 + 磁盘偏离），旧后端只有 config_status 时按三态退回
const pushState = computed(() => {
  const s = props.server?.push_state
  if (s) return s
  const cs = props.server?.config_status
  if (cs === 'pushed') return 'synced'
  if (cs === 'pending') return 'pending'
  return 'none'
})

// ---- 概览：接入点摘要 ----
const inbounds = ref<InboundItem[]>([])
const inboundsLoading = ref(false)

async function loadInbounds() {
  if (!props.server) return
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

// ---- 概览：重置密钥 / 删除 ----
const secretInfo = ref<{ node_id: string; secret: string; install_cmd?: string } | null>(null)
const resetting = ref(false)

async function resetSecret() {
  if (!props.server) return
  try {
    await ElMessageBox.confirm(
      `确认重置服务器「${props.server.name}」的密钥？旧密钥立即失效，需在服务器 Agent 配置（/etc/xray-agent/config.yml）中更新。`,
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
  if (!props.server) return
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
      if (activeTab.value === 'overview' && inbounds.value.length === 0) {
        loadInbounds()
      } else if (activeTab.value === 'config' && !cfgText.value) {
        loadConfigPreview()
      } else if (activeTab.value === 'online') {
        // 首次激活由面板自身 immediate 加载（lazy 挂载）；再次激活重拉保证快照新鲜
        onlinePanelRef.value?.reload()
      }
    }
  },
  { immediate: true },
)

watch(
  () => [props.modelValue, props.server],
  () => {
    if (props.modelValue && props.server) {
      activeTab.value = 'overview'
      secretInfo.value = null
      cfgText.value = ''
      loadInbounds()
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
            <div class="desc-row"><span class="k">服务器名称</span><span class="v">{{ server.name }}</span></div>
            <div class="desc-row">
              <span class="k">服务器类型</span>
              <span class="v">
                <span class="x-chip blue">Xray 服务器</span>
              </span>
            </div>
            <div class="desc-row"><span class="k">主机地址</span><span class="v"><code class="cell-mono">{{ server.host }}</code></span></div>
            <div class="desc-row">
              <span class="k">Node ID</span>
              <span class="v">
                <code class="cell-mono">{{ server.node_id }}</code>
                <el-button size="small" text @click="copyText(server.node_id, 'Node ID')"><el-icon><CopyDocument /></el-icon></el-button>
              </span>
            </div>
            <div class="desc-row"><span class="k">所在地区</span><span class="v">{{ server.location || '—' }}</span></div>
            <div class="desc-row"><span class="k">备注说明</span><span class="v">{{ server.remark || '—' }}</span></div>
            <div class="desc-row">
              <span class="k">到期日</span>
              <span class="v">
                <template v-if="server.expire_at">
                  <span class="cell-mono">{{ formatExpireDate(server.expire_at) }}</span>
                  <span
                    v-if="getExpiryStatus(server.expire_at)"
                    class="x-chip"
                    :class="getExpiryStatus(server.expire_at)!.cls"
                    :title="getExpiryStatus(server.expire_at)!.tip"
                    style="margin-left: 6px; font-size: 10px; padding: 1px 5px"
                  >
                    {{ getExpiryStatus(server.expire_at)!.text }}
                  </span>
                </template>
                <template v-else>—</template>
              </span>
            </div>
            <div class="desc-row"><span class="k">计费周期</span><span class="v">{{ server.billing_cycle || '—' }}</span></div>
            <div class="desc-row"><span class="k">续费价格</span><span class="v"><code v-if="server.price" class="cell-mono font-12" style="color: var(--x-primary); font-weight: 600">{{ server.price }}</code><template v-else>—</template></span></div>
            <div class="desc-row">
              <span class="k">IDC 地址</span>
              <span class="v">
                <template v-if="server.idc_address">
                  <el-link
                    v-if="isUrl(server.idc_address)"
                    type="primary"
                    :underline="false"
                    :href="normalizeUrl(server.idc_address)"
                    target="_blank"
                    style="margin-right: 6px; font-size: 12.5px"
                  >
                    <el-icon style="margin-right: 2px"><Link /></el-icon>{{ server.idc_address }}
                  </el-link>
                  <span v-else class="cell-mono" style="margin-right: 6px">{{ server.idc_address }}</span>
                  <el-button size="small" text @click="copyText(server.idc_address, 'IDC 地址')"><el-icon><CopyDocument /></el-icon></el-button>
                </template>
                <template v-else>—</template>
              </span>
            </div>
            <div class="desc-row">
              <span class="k">运行状态</span>
              <span class="v">
                <span class="x-chip" :class="server.status === 1 ? 'green' : 'gray'">
                  <span class="x-status-dot" :class="server.status === 1 ? 'online' : 'offline'" />{{ server.status === 1 ? '在线' : '离线' }}
                </span>
              </span>
            </div>
            <div class="desc-row">
              <span class="k">Xray 进程</span>
              <span class="v">
                <span class="x-chip" :class="xrayChip.cls">
                  <span class="x-status-dot" :class="server.xray_running ? 'online' : 'offline'" />{{ xrayChip.text }}
                </span>
              </span>
            </div>
            <div v-if="server.xray_last_error" class="desc-row">
              <span class="k">{{ xrayFailed ? '启动失败' : '上次启动失败' }}</span>
              <span class="v" :style="{ color: xrayFailed ? 'var(--el-color-danger, #ef4444)' : 'var(--el-text-color-secondary, #909399)', fontSize: '12px', wordBreak: 'break-all' }">
                {{ server.xray_last_error }}<template v-if="server.xray_failures">（连续失败 {{ server.xray_failures }} 次，最后尝试 {{ fmtTime(server.xray_error_at ?? null) }}）</template>
                <template v-if="server.xray_state === 'failed'">已停止自动拉起，修复后点「重启 Xray」或重新下发配置即可恢复。</template>
              </span>
            </div>
            <div class="desc-row">
              <span class="k">配置同步</span>
              <span class="v">
                <span v-if="pushState === 'drift'" class="x-chip red">磁盘偏离</span>
                <span v-else-if="pushState === 'rejected'" class="x-chip red">推送被拒</span>
                <span v-else-if="pushState === 'synced'" class="x-chip green">已同步</span>
                <span v-else-if="pushState === 'pending'" class="x-chip orange">待推送</span>
                <span v-else class="x-chip gray">未投递</span>
              </span>
            </div>
            <div v-if="server.config_drift" class="desc-row">
              <span class="k">磁盘偏离</span>
              <span class="v" style="color: var(--el-color-danger, #ef4444); font-size: 12px; word-break: break-all">
                节点磁盘上的配置与主控记录不一致（可能被手工改过、落盘失败或节点回退过配置）。
                节点磁盘哈希 {{ (server.xray_disk_hash || '').slice(0, 12) || '未上报' }}…，重新生成并推送一次即可对齐。
              </span>
            </div>
            <div v-if="server.config_status === 'pending' && server.push_error" class="desc-row">
              <span class="k">推送失败</span>
              <span class="v" style="color: var(--el-color-danger, #ef4444); font-size: 12px; word-break: break-all">
                {{ server.push_error }}（已尝试 {{ server.push_attempts || 0 }} 次，最后尝试 {{ fmtTime(server.push_last_try_at ?? null) }}）
              </span>
            </div>
            <div v-if="server.xray_running_hash" class="desc-row">
              <span class="k">运行中配置</span>
              <span class="v mono" style="font-size: 12px">
                {{ (server.xray_running_hash || '').slice(0, 12) }}…
                <span style="color: var(--el-text-color-secondary, #909399)">（xray 只在启动时读一次配置，热更落盘只改磁盘、不改这里）</span>
              </span>
            </div>
            <div class="desc-row"><span class="k">Agent 版本</span><span class="v mono">{{ server.agent_version || '—' }}</span></div>
            <div class="desc-row"><span class="k">最后心跳</span><span class="v">{{ fmtTime(server.last_seen_at) }}</span></div>
            <div class="desc-row">
              <span class="k">已配置接入点</span>
              <span class="v">
                <template v-if="inboundsLoading">…</template>
                <template v-else>{{ inbounds.length }} 个入站</template>
                <el-button size="small" text type="primary" @click="goInbounds">管理入站</el-button>
              </span>
            </div>
          </div>

          <div v-if="secretInfo" class="secret-box" style="margin-top: 14px">
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
            <el-button :loading="resetting" @click="resetSecret">
              <el-icon><Key /></el-icon>&nbsp;重置密钥
            </el-button>
            <el-button type="danger" plain :loading="deleting" @click="removeServer">
              <el-icon><Delete /></el-icon>&nbsp;删除服务器
            </el-button>
          </div>
        </template>
        <el-empty v-else description="未选择服务器" />
      </el-tab-pane>

      <!-- 在线用户（连接级实时快照）：lazy 避免抽屉首开即打一次 online-ips -->
      <el-tab-pane label="在线用户" name="online" lazy>
        <OnlineUsersPanel v-if="server" ref="onlinePanelRef" :server-id="server.id" />
        <el-empty v-else description="未选择服务器" />
      </el-tab-pane>

      <!-- 配置预览 -->
      <el-tab-pane label="配置预览" name="config">
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
          <TipIcon
            content="此配置为主控根据当前入站、出站、路由规则与有效用户实时渲染的目标 Xray 配置（业务变更系统会自动同步推送至服务器）。展示内容已对用户 UUID 脱敏，复制时保留原文。"
          />
        </div>
        <pre v-loading="cfgLoading" class="cfg-view">{{ maskUUIDs(cfgText) || '正在计算并加载配置预览…' }}</pre>
      </el-tab-pane>
    </el-tabs>

    <template #footer>
      <el-button type="primary" @click="visible = false">关闭</el-button>
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
  align-items: center;
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
