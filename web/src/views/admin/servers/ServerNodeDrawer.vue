<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { Delete, Key, CopyDocument, VideoPlay, Connection } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import {
  deleteServer,
  generateAndPushConfig,
  getInbounds,
  resetServerSecret,
  type InboundItem,
  type ServerItem,
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

// ---- 概览：接入点摘要（增删改统一在「节点（接入点）」页） ----
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
      `确认删除节点「${props.server.name}」？关联的入站、出站、路由规则、待推送配置与节点上报将一并删除，该节点 Agent 将无法再连接。`,
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

// ---- 配置预览 Tab（uuid 脱敏展示，复制时保留原文） ----
const cfgLoading = ref(false)
const cfgText = ref('')
const cfgMessage = ref('')

async function generatePreview() {
  if (!props.server) return
  cfgLoading.value = true
  cfgText.value = ''
  cfgMessage.value = ''
  try {
    const { data } = await generateAndPushConfig(props.server.id)
    if (data.code === 0) {
      cfgText.value = data.data.config || ''
      cfgMessage.value = data.data.message || ''
      if (data.data.ok) ElMessage.success(data.data.message || '配置已生成')
      else ElMessage.warning(data.data.message || '配置已保存但未推送')
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    cfgMessage.value = `生成失败：${errMsg(e)}`
    ElMessage.error(errMsg(e, '生成失败'))
  } finally {
    cfgLoading.value = false
  }
}

watch(
  () => [props.modelValue, props.server],
  () => {
    if (props.modelValue && props.server) {
      activeTab.value = 'overview'
      secretInfo.value = null
      cfgText.value = ''
      cfgMessage.value = ''
      loadInbounds()
    }
  },
  { immediate: true },
)
</script>

<template>
  <el-drawer
    :model-value="visible"
    :title="`节点管理 · ${server?.name ?? ''}`"
    size="78%"
    class="node-drawer"
    @update:model-value="(v: boolean) => (visible = v)"
  >
    <el-tabs v-model="activeTab" class="drawer-tabs">
      <!-- 概览 -->
      <el-tab-pane label="概览" name="overview">
        <template v-if="server">
          <div class="desc-grid">
            <div class="desc-row"><span class="k">名称</span><span class="v">{{ server.name }}</span></div>
            <div class="desc-row"><span class="k">地址</span><span class="v"><code class="cell-mono">{{ server.host }}</code></span></div>
            <div class="desc-row"><span class="k">Node ID</span><span class="v"><code class="cell-mono">{{ server.node_id }}</code>
              <el-button size="small" text @click="copyText(server.node_id, 'node_id')"><el-icon><CopyDocument /></el-icon></el-button>
            </span></div>
            <div class="desc-row"><span class="k">地区</span><span class="v">{{ server.location || '—' }}</span></div>
            <div class="desc-row"><span class="k">备注</span><span class="v">{{ server.remark || '—' }}</span></div>
            <div class="desc-row"><span class="k">状态</span><span class="v">
              <el-tag :type="server.status === 1 ? 'success' : 'info'" size="small">
                <span class="x-status-dot" :class="server.status === 1 ? 'online' : 'offline'" />{{ server.status === 1 ? '在线' : '离线' }}
              </el-tag>
            </span></div>
            <div class="desc-row"><span class="k">配置同步</span><span class="v">
              <el-tag v-if="server.config_status === 'pushed'" type="success" size="small">已同步</el-tag>
              <el-tag v-else-if="server.config_status === 'pending'" type="warning" size="small">待推送</el-tag>
              <el-tag v-else type="info" size="small" effect="plain">未生成</el-tag>
            </span></div>
            <div class="desc-row"><span class="k">最后心跳</span><span class="v">{{ fmtTime(server.last_seen_at) }}</span></div>
            <div class="desc-row">
              <span class="k">接入点</span>
              <span class="v">
                <template v-if="inboundsLoading">…</template>
                <template v-else>{{ inbounds.length }} 个</template>
                <el-button size="small" text type="primary" @click="goInbounds">
                  <el-icon><Connection /></el-icon>&nbsp;管理
                </el-button>
              </span>
            </div>
          </div>

          <el-divider />
          <div class="sec-title">节点密钥</div>
          <p class="muted tip">
            密钥仅在创建/重置时显示一次；重置后需更新节点 /etc/xray-agent/config.yml 中的 secret（或重新执行安装命令）。
          </p>
          <div v-if="secretInfo" class="secret-box">
            <div class="secret-row"><span class="k">node_id</span><code>{{ secretInfo.node_id }}</code></div>
            <div class="secret-row">
              <span class="k">secret</span><code>{{ secretInfo.secret }}</code>
              <el-button size="small" text @click="copyText(secretInfo.secret, 'secret')"><el-icon><CopyDocument /></el-icon></el-button>
            </div>
            <div v-if="secretInfo.install_cmd" class="secret-row">
              <span class="k">安装</span>
              <code class="install-cmd">{{ secretInfo.install_cmd }}</code>
              <el-button size="small" text @click="copyText(secretInfo.install_cmd!, '安装命令')"><el-icon><CopyDocument /></el-icon></el-button>
            </div>
          </div>
          <div class="action-row">
            <el-button :loading="resetting" @click="resetSecret"><el-icon><Key /></el-icon>&nbsp;重置密钥</el-button>
            <el-button type="danger" plain :loading="deleting" @click="removeServer"><el-icon><Delete /></el-icon>&nbsp;删除节点</el-button>
          </div>
        </template>
        <el-empty v-else description="未选择节点" />
      </el-tab-pane>

      <!-- 配置预览 -->
      <el-tab-pane label="配置预览" name="config">
        <div class="tab-toolbar">
          <el-button size="small" type="primary" :loading="cfgLoading" @click="generatePreview">
            <el-icon><VideoPlay /></el-icon>&nbsp;生成并预览
          </el-button>
          <el-button
            size="small"
            :disabled="!cfgText"
            @click="copyText(cfgText, '配置 JSON')"
          >
            <el-icon><CopyDocument /></el-icon>&nbsp;复制配置
          </el-button>
        </div>
        <p v-if="cfgMessage" class="cfg-message">{{ cfgMessage }}</p>
        <p class="muted tip" style="margin: 0 0 8px">
          按该节点启用入站 + 出站 + 路由规则 + 全部启用用户生成完整 Xray 配置；生成即保存待推送（节点在线自动下发）。
          展示内容已对用户 UUID 打码，复制配置得到原始 JSON。
        </p>
        <pre v-loading="cfgLoading" class="cfg-view">{{ maskUUIDs(cfgText) || '点击「生成并预览」生成配置…' }}</pre>
      </el-tab-pane>
    </el-tabs>
  </el-drawer>
</template>

<style scoped lang="scss">
.drawer-tabs {
  height: 100%;
}
.tab-toolbar {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  margin-bottom: 12px;
}
.sec-title {
  font-weight: 600;
  font-size: 13px;
  margin: 4px 0 10px;
  color: var(--x-primary);
}
.muted {
  color: var(--x-text-3);
}
.tip {
  font-size: 12px;
}
.desc-grid {
  display: grid;
  gap: 0;
}
.desc-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 10px 0;
  border-bottom: 1px solid var(--x-border);
  font-size: 13.5px;
  .k {
    color: var(--x-text-2);
    flex: none;
  }
  .v {
    font-weight: 500;
    display: flex;
    align-items: center;
    gap: 4px;
  }
}
.cell-mono {
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 12.5px;
  color: var(--x-text-2);
}
.secret-box {
  display: grid;
  gap: 10px;
  margin-bottom: 12px;
}
.secret-row {
  display: flex;
  align-items: center;
  gap: 10px;
  background: var(--x-primary-soft);
  border-radius: 8px;
  padding: 10px 12px;
  .k {
    color: var(--x-text-2);
    font-size: 12.5px;
    flex: none;
    width: 56px;
  }
  code {
    font-family: ui-monospace, Menlo, Consolas, monospace;
    font-size: 12.5px;
    color: var(--x-primary);
    word-break: break-all;
    flex: 1;
  }
  .install-cmd {
    font-size: 11.5px;
  }
}
.action-row {
  display: flex;
  gap: 10px;
}
.cfg-message {
  color: var(--x-text-2);
  font-size: 13px;
  margin: 0 0 8px;
}
.cfg-view {
  background: #171b2e;
  color: #c7d2fe;
  border-radius: 8px;
  padding: 14px;
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.6;
  max-height: 560px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>
