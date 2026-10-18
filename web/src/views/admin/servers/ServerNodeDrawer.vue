<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Delete, Key, CopyDocument, Refresh, Link } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from '@/components/cloudscape'
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
    width="700px"
    :append-to-body="true"
  >
    <el-tabs v-model="activeTab" class="dialog-tabs">
      <!-- 概览 -->
      <el-tab-pane label="基本信息" name="overview">
        <template v-if="server">
          <div class="drawer-cards-stack">
            <!-- 1. 运行时与通信状态微卡片 -->
            <div class="x-micro-card">
              <div class="x-micro-card-head">
                <div class="x-micro-card-title">
                  <span class="x-dot" :class="server.status === 1 ? 'emerald' : 'rose'"></span>
                  <span>运行时与通信状态</span>
                </div>
                <div class="card-head-chips">
                  <!-- 配置同步状态 -->
                  <span v-if="pushState === 'drift'" class="x-chip red">磁盘偏离</span>
                  <span v-else-if="pushState === 'rejected'" class="x-chip red">推送被拒</span>
                  <span v-else-if="pushState === 'synced'" class="x-chip emerald font-mono-num">SYNCED</span>
                  <span v-else-if="pushState === 'pending'" class="x-chip amber">待推送</span>
                  <span v-else class="x-chip gray">未投递</span>
                </div>
              </div>

              <div class="meta-data-grid">
                <div class="meta-cell">
                  <span class="cell-k">通信状态</span>
                  <span class="cell-v">
                    <span class="x-chip" :class="server.status === 1 ? 'emerald' : 'gray'">
                      <span class="x-status-dot" :class="server.status === 1 ? 'online' : 'offline'" />
                      {{ server.status === 1 ? '在线' : '离线' }}
                    </span>
                  </span>
                </div>

                <div class="meta-cell">
                  <span class="cell-k">Xray 进程</span>
                  <span class="cell-v">
                    <span class="x-chip" :class="xrayChip.cls">
                      <span class="x-status-dot" :class="server.xray_running ? 'online' : 'offline'" />
                      {{ xrayChip.text }}
                    </span>
                  </span>
                </div>

                <div class="meta-cell">
                  <span class="cell-k">Agent 版本</span>
                  <span class="cell-v font-mono-num font-medium">
                    {{ server.agent_version || '—' }}
                  </span>
                </div>

                <div class="meta-cell">
                  <span class="cell-k">最后心跳</span>
                  <span class="cell-v font-mono-num" style="color: var(--x-text-2)">
                    {{ fmtTime(server.last_seen_at) }}
                  </span>
                </div>

                <div class="meta-cell col-span-2">
                  <span class="cell-k">托管接入点</span>
                  <span class="cell-v" style="justify-content: space-between; flex: 1">
                    <span class="font-mono-num">
                      <template v-if="inboundsLoading">加载中…</template>
                      <template v-else>{{ inbounds.length }} 个入站监听器</template>
                    </span>
                    <el-button size="small" text type="primary" @click="goInbounds">
                      前往管理入站 ↗
                    </el-button>
                  </span>
                </div>
              </div>

              <!-- 异常诊断条 -->
              <div v-if="server.xray_last_error" class="notice-bar red" style="margin-top: 10px">
                <b>{{ xrayFailed ? '启动失败：' : '上次启动警告：' }}</b> {{ server.xray_last_error }}
                <template v-if="server.xray_failures">（连续失败 {{ server.xray_failures }} 次，最后尝试 {{ fmtTime(server.xray_error_at ?? null) }}）</template>
                <template v-if="server.xray_state === 'failed'">（已停止重试，可尝试「重启 Xray」）</template>
              </div>

              <div v-if="server.config_drift" class="notice-bar red" style="margin-top: 10px">
                <b>磁盘偏离：</b>节点磁盘配置与主控不一致。磁盘哈希 {{ (server.xray_disk_hash || '').slice(0, 12) || '未上报' }}…，重新生成并推送一次即可对齐。
              </div>

              <div v-if="server.config_status === 'pending' && server.push_error" class="notice-bar amber" style="margin-top: 10px">
                <b>推送失败：</b>{{ server.push_error }}（已重试 {{ server.push_attempts || 0 }} 次，最后尝试 {{ fmtTime(server.push_last_try_at ?? null) }}）
              </div>
            </div>

            <!-- 2. 网络接入与 VPS 资产微卡片 -->
            <div class="x-micro-card">
              <div class="x-micro-card-head">
                <div class="x-micro-card-title">
                  <span class="x-dot blue"></span>
                  <span>网络接入与 VPS 资产</span>
                </div>
              </div>

              <div class="meta-data-grid">
                <div class="meta-cell">
                  <span class="cell-k">主机地址 Host</span>
                  <span class="cell-v">
                    <code class="font-mono-num font-semibold" style="color: var(--x-primary)">{{ server.host }}</code>
                    <el-button size="small" text @click="copyText(server.host, '主机地址')"><el-icon><CopyDocument /></el-icon></el-button>
                  </span>
                </div>

                <div class="meta-cell">
                  <span class="cell-k">所在地区</span>
                  <span class="cell-v">
                    <span v-if="server.location" class="x-chip blue">{{ server.location }}</span>
                    <span v-else style="color: var(--x-text-3)">—</span>
                  </span>
                </div>

                <div class="meta-cell">
                  <span class="cell-k">到期日</span>
                  <span class="cell-v">
                    <template v-if="server.expire_at">
                      <span class="font-mono-num font-semibold">{{ formatExpireDate(server.expire_at) }}</span>
                      <span
                        v-if="getExpiryStatus(server.expire_at)"
                        class="x-chip"
                        :class="getExpiryStatus(server.expire_at)!.cls"
                        :title="getExpiryStatus(server.expire_at)!.tip"
                        style="font-size: 10px; padding: 1px 5px"
                      >
                        {{ getExpiryStatus(server.expire_at)!.text }}
                      </span>
                    </template>
                    <span v-else style="color: var(--x-text-3)">—</span>
                  </span>
                </div>

                <div class="meta-cell">
                  <span class="cell-k">计费与价格</span>
                  <span class="cell-v font-mono-num">
                    <span v-if="server.price" style="font-weight: 600; color: var(--x-primary)">{{ server.price }}</span>
                    <span v-if="server.billing_cycle" class="x-chip gray" style="margin-left: 6px">{{ server.billing_cycle }}</span>
                    <span v-if="!server.price && !server.billing_cycle" style="color: var(--x-text-3)">—</span>
                  </span>
                </div>

                <div class="meta-cell col-span-2">
                  <span class="cell-k">IDC 控制台</span>
                  <span class="cell-v">
                    <template v-if="server.idc_address">
                      <el-link
                        v-if="isUrl(server.idc_address)"
                        type="primary"
                        :underline="false"
                        :href="normalizeUrl(server.idc_address)"
                        target="_blank"
                        style="font-size: 12px"
                      >
                        <el-icon style="margin-right: 2px"><Link /></el-icon>{{ server.idc_address }}
                      </el-link>
                      <span v-else class="font-mono-num">{{ server.idc_address }}</span>
                      <el-button size="small" text @click="copyText(server.idc_address, 'IDC 控制台')"><el-icon><CopyDocument /></el-icon></el-button>
                    </template>
                    <span v-else style="color: var(--x-text-3)">—</span>
                  </span>
                </div>

                <div v-if="server.remark" class="meta-cell col-span-2">
                  <span class="cell-k">备注说明</span>
                  <span class="cell-v" style="color: var(--x-text-2)">{{ server.remark }}</span>
                </div>
              </div>
            </div>

            <!-- 3. 通信凭据微卡片 -->
            <div class="x-micro-card">
              <div class="x-micro-card-head">
                <div class="x-micro-card-title">
                  <span class="x-dot purple"></span>
                  <span>节点通信凭据 (Credentials)</span>
                </div>
              </div>

              <div class="meta-data-grid">
                <div class="meta-cell col-span-2">
                  <span class="cell-k">Node ID</span>
                  <span class="cell-v">
                    <code class="font-mono-num font-semibold" style="color: var(--x-primary)">{{ server.node_id }}</code>
                    <el-button size="small" text @click="copyText(server.node_id, 'Node ID')"><el-icon><CopyDocument /></el-icon></el-button>
                  </span>
                </div>
              </div>

              <!-- 若刚刚重置了密钥 -->
              <div v-if="secretInfo" class="secret-box" style="margin-top: 12px">
                <div class="sec-title">最新重置凭据</div>
                <div class="secret-row">
                  <span class="k">Node Secret</span>
                  <code class="font-mono-num text-amber-400">{{ secretInfo.secret }}</code>
                  <el-button size="small" text @click="copyText(secretInfo.secret, 'Secret 密钥')"><el-icon><CopyDocument /></el-icon></el-button>
                </div>
                <div v-if="secretInfo.install_cmd" class="secret-row">
                  <span class="k">一键安装指令</span>
                  <code class="install-cmd font-mono-num">{{ secretInfo.install_cmd }}</code>
                  <el-button size="small" text @click="copyText(secretInfo.install_cmd!, '安装命令')"><el-icon><CopyDocument /></el-icon></el-button>
                </div>
              </div>

              <!-- 底部操作按钮 -->
              <div class="action-footer" style="margin-top: 12px">
                <el-button :loading="resetting" @click="resetSecret">
                  <el-icon><Key /></el-icon>&nbsp;重置通信密钥
                </el-button>
                <el-button type="danger" plain :loading="deleting" @click="removeServer">
                  <el-icon><Delete /></el-icon>&nbsp;删除服务器
                </el-button>
              </div>
            </div>
          </div>
        </template>
        <el-empty v-else description="未选择服务器" />
      </el-tab-pane>

      <!-- 在线用户（连接级实时快照） -->
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
        <pre v-loading="cfgLoading" class="cfg-view font-mono-num">{{ maskUUIDs(cfgText) || '正在计算并加载配置预览…' }}</pre>
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

.drawer-cards-stack {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

/* 规范业务分组微卡片 */
.x-micro-card {
  background: var(--x-card-bg, #fff);
  border: 1px solid var(--x-border, #e2e8f0);
  border-radius: 8px;
  padding: 12px 14px;
  transition: border-color 0.15s ease;
  &:hover {
    border-color: var(--x-border-hover, #cbd5e1);
  }
}
.x-micro-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
  padding-bottom: 6px;
  border-bottom: 1px dashed var(--x-border, #e2e8f0);
}
.x-micro-card-title {
  display: inline-flex;
  align-items: center;
  gap: 7px;
  font-size: 12.5px;
  font-weight: 600;
  color: var(--x-text-1, #0f172a);
}

.x-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  flex-shrink: 0;
  &.emerald { background: #10b981; box-shadow: 0 0 6px rgba(16, 185, 129, 0.5); }
  &.blue { background: #3b82f6; box-shadow: 0 0 6px rgba(59, 130, 246, 0.5); }
  &.purple { background: #8b5cf6; box-shadow: 0 0 6px rgba(139, 92, 246, 0.5); }
  &.rose { background: #f43f5e; box-shadow: 0 0 6px rgba(244, 63, 94, 0.5); }
}

.meta-data-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 8px 14px;
}
@media (max-width: 600px) {
  .meta-data-grid {
    grid-template-columns: 1fr;
  }
}
.col-span-2 {
  grid-column: 1 / -1;
}

.meta-cell {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 10px;
  background: var(--x-bg, #f8fafc);
  border: 1px solid var(--x-border, #e2e8f0);
  border-radius: 6px;
  font-size: 12px;

  .cell-k {
    color: var(--x-text-3, #94a3b8);
    font-size: 11.5px;
  }
  .cell-v {
    font-weight: 500;
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
}

.notice-bar {
  padding: 8px 12px;
  border-radius: 6px;
  font-size: 11.5px;
  line-height: 1.4;
  &.red {
    background: var(--x-danger-soft, rgba(239, 68, 68, 0.08));
    color: var(--el-color-danger, #ef4444);
    border-left: 3px solid var(--el-color-danger, #ef4444);
  }
  &.amber {
    background: var(--x-warning-soft, rgba(245, 158, 11, 0.08));
    color: var(--el-color-warning, #f59e0b);
    border-left: 3px solid var(--el-color-warning, #f59e0b);
  }
}

.secret-box {
  background: var(--x-bg, #f8fafc);
  border: 1px solid var(--x-border, #e2e8f0);
  border-radius: 6px;
  padding: 10px 12px;
  .sec-title {
    font-weight: 600;
    font-size: 12px;
    margin-bottom: 6px;
    color: var(--x-text-1, #0f172a);
  }
  .secret-row {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 11.5px;
    margin-bottom: 4px;
    .k {
      color: var(--x-text-3, #94a3b8);
      width: 80px;
    }
    .install-cmd {
      flex: 1;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      font-size: 11px;
    }
  }
}

.action-footer {
  display: flex;
  justify-content: flex-end;
  gap: 10px;
  padding-top: 8px;
  border-top: 1px dashed var(--x-border, #e2e8f0);
}

.cfg-view {
  background: #0b0f19;
  color: #34d399;
  padding: 14px;
  border-radius: 8px;
  font-family: var(--x-font-mono, monospace);
  font-size: 11.5px;
  max-height: 420px;
  overflow: auto;
  line-height: 1.5;
  border: 1px solid var(--x-border, #1e293b);
}
</style>
