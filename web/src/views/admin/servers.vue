<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Plus, Search, Refresh, View, Document, VideoPlay, Delete, Key, CopyDocument, ArrowDown, Edit, Setting } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import ServerNodeDrawer from './servers/ServerNodeDrawer.vue'
import {
  createServer,
  deleteServer,
  generateAndPushConfig,
  getInbounds,
  getServers,
  resetServerSecret,
  serverCommand,
  type CommandResult,
  type ServerItem,
  updateServer,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import { maskUUIDs } from '@/utils/mask'

const router = useRouter()

const list = ref<ServerItem[]>([])
const loading = ref(false)
const keyword = ref('')

// 接入点计数：server_id → 入站数（服务器页摘要，跳转节点页按服务器过滤）
const inboundCountMap = ref<Record<number, number>>({})

async function loadInboundCounts() {
  try {
    const { data } = await getInbounds()
    if (data.code === 0) {
      const m: Record<number, number> = {}
      for (const ib of data.data.items) m[ib.server_id] = (m[ib.server_id] ?? 0) + 1
      inboundCountMap.value = m
    }
  } catch {
    /* 计数失败不阻塞列表 */
  }
}

function inboundCount(id: number) {
  return inboundCountMap.value[id] ?? 0
}

function goInbounds(row: any) {
  router.push({ path: '/admin/nodes', query: { server_id: row.id } })
}

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return list.value
  return list.value.filter(
    (s) =>
      s.name.toLowerCase().includes(kw) ||
      s.host.toLowerCase().includes(kw) ||
      (s.location ?? '').toLowerCase().includes(kw),
  )
})

async function load() {
  loading.value = true
  try {
    const { data } = await getServers()
    if (data.code === 0) list.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载服务器失败'))
  } finally {
    loading.value = false
  }
  loadInboundCounts()
}
onMounted(load)

function fmtTime(t: string | null) {
  if (!t) return '—'
  const d = new Date(t)
  return d.toLocaleString('zh-CN', { hour12: false })
}

// ---- 新增服务器 ----
const createOpen = ref(false)
const createForm = reactive({ name: '', host: '', location: '', remark: '' })
const creating = ref(false)
const createdResult = ref<{ node_id: string; secret: string; install_cmd: string } | null>(null)

async function submitCreate() {
  if (!createForm.name || !createForm.host) {
    ElMessage.warning('请填写名称与地址')
    return
  }
  creating.value = true
  try {
    const { data } = await createServer({ ...createForm })
    if (data.code === 0) {
      createdResult.value = { node_id: data.data.node_id, secret: data.data.secret, install_cmd: data.data.install_cmd }
      createForm.name = ''
      createForm.host = ''
      createForm.location = ''
      createForm.remark = ''
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '创建失败'))
  } finally {
    creating.value = false
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

function closeCreate() {
  createOpen.value = false
  createdResult.value = null
}

// ---- 节点管理抽屉 ----
const drawerOpen = ref(false)
const drawerServer = ref<ServerItem | null>(null)

function openDrawer(row: any) {
  drawerServer.value = row
  drawerOpen.value = true
}

// ---- 下发配置（走 generate-config：主控生成完整配置并保存待推送，在线自动下发） ----
const pushOpen = ref(false)
const pushTarget = ref<ServerItem | null>(null)
const pushLoading = ref(false)
const pushResult = ref('')
const pushConfigText = ref('')

function openPush(row: any) {
  pushTarget.value = row
  pushResult.value = ''
  pushConfigText.value = ''
  pushOpen.value = true
  runPush()
}

async function runPush() {
  if (!pushTarget.value) return
  pushLoading.value = true
  pushResult.value = ''
  try {
    const { data } = await generateAndPushConfig(pushTarget.value.id)
    if (data.code === 0) {
      pushConfigText.value = data.data.config || ''
      pushResult.value = data.data.message
      if (data.data.ok) ElMessage.success(data.data.message || '配置已下发')
      else ElMessage.warning(data.data.message || '配置已保存，节点上线后自动补推')
      load()
    } else {
      pushResult.value = `失败：${data.message}`
    }
  } catch (e) {
    pushResult.value = `失败：${errMsg(e)}`
    ElMessage.error(errMsg(e, '生成失败'))
  } finally {
    pushLoading.value = false
  }
}

// ---- 状态详情 ----
const statusOpen = ref(false)
const statusData = ref<CommandResult<any> | null>(null)
const statusLoading = ref(false)

async function openStatus(row: any) {
  statusOpen.value = true
  statusData.value = null
  statusLoading.value = true
  try {
    const { data } = await serverCommand(row.id, 'get_status')
    statusData.value = data.data
  } catch (e) {
    ElMessage.error(errMsg(e, '查询状态失败'))
  } finally {
    statusLoading.value = false
  }
}

// ---- 重启 ----
async function restartXray(row: any) {
  try {
    await ElMessageBox.confirm(`确认重启节点「${row.name}」的 Xray？约 1-2 秒断线。`, '重启 Xray', {
      type: 'warning',
    })
  } catch {
    return
  }
  try {
    const { data } = await serverCommand(row.id, 'restart_xray')
    if (data.code === 0 && data.data.ok) ElMessage.success('已重启')
    else ElMessage.error(data.data.error || '重启失败')
  } catch (e) {
    ElMessage.error(errMsg(e, '重启失败'))
  }
}

// ---- 日志 ----
const logOpen = ref(false)
const logContent = ref('')
const logLoading = ref(false)
const logTarget = ref<ServerItem | null>(null)

async function openLogs(row: any) {
  logTarget.value = row
  logOpen.value = true
  logContent.value = ''
  logLoading.value = true
  try {
    const { data } = await serverCommand(row.id, 'get_logs', { lines: 200 })
    logContent.value = (data.data.data as string) || '（空）'
  } catch (e) {
    logContent.value = `读取失败：${errMsg(e)}`
  } finally {
    logLoading.value = false
  }
}

// ---- 生成并下发配置 ----
const genOpen = ref(false)
const genLoading = ref(false)
const genResult = ref('')

async function genPush(row: any) {
  try {
    await ElMessageBox.confirm(
      `将按「${row.name}」的启用入站 + 全部启用用户生成 Xray 配置并自动推送（节点离线时保存，上线自动补推），确认？`,
      '生成并下发配置',
      { type: 'warning' },
    )
  } catch {
    return
  }
  genOpen.value = true
  genLoading.value = true
  genResult.value = ''
  try {
    const { data } = await generateAndPushConfig(row.id)
    if (data.code === 0 && data.data.ok) {
      ElMessage.success(data.data.message || '配置已生成')
      genResult.value = data.data.config
    } else {
      ElMessage.error(data.data?.message || data.message)
      genResult.value = data.data?.config ?? ''
    }
  } catch (e) {
    genResult.value = `失败：${errMsg(e)}`
    ElMessage.error(errMsg(e, '生成失败'))
  } finally {
    genLoading.value = false
  }
}
// ---- 编辑服务器 ----
const editOpen = ref(false)
const editForm = reactive({ id: 0, name: '', host: '', location: '', remark: '' })
const editSaving = ref(false)

function openEdit(row: any) {
  Object.assign(editForm, {
    id: row.id,
    name: row.name,
    host: row.host,
    location: row.location ?? '',
    remark: row.remark ?? '',
  })
  editOpen.value = true
}

async function submitEdit() {
  if (!editForm.name || !editForm.host) {
    ElMessage.warning('请填写名称与地址')
    return
  }
  editSaving.value = true
  try {
    const { data } = await updateServer(editForm.id, {
      name: editForm.name,
      host: editForm.host,
      location: editForm.location,
      remark: editForm.remark,
    })
    if (data.code === 0) {
      ElMessage.success('已保存')
      editOpen.value = false
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    editSaving.value = false
  }
}

// ---- 重置密钥 ----
const secretOpen = ref(false)
const secretInfo = ref<{ node_id: string; secret: string; install_cmd?: string } | null>(null)

async function resetSecret(row: any) {
  try {
    await ElMessageBox.confirm(
      `确认重置节点「${row.name}」的密钥？旧密钥立即失效，需在节点 Agent 配置（/etc/xray-agent/config.yml）中更新。`,
      '重置密钥',
      { type: 'warning' },
    )
  } catch {
    return
  }
  try {
    const { data } = await resetServerSecret(row.id)
    if (data.code === 0) {
      secretInfo.value = { node_id: data.data.node_id, secret: data.data.secret, install_cmd: data.data.install_cmd }
      secretOpen.value = true
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '重置失败'))
  }
}

// ---- 更多操作（编辑/重置密钥/删除） ----
function onMore(cmd: string, row: any) {
  if (cmd === 'edit') openEdit(row)
  else if (cmd === 'reset') resetSecret(row)
  else if (cmd === 'delete') removeServer(row)
}

// ---- 删除 ----
async function removeServer(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除节点「${row.name}」？关联的入站、待推送配置与节点上报将一并删除，该节点 Agent 将无法再连接。`, '删除服务器', {
      type: 'error',
    })
  } catch {
    return
  }
  try {
    const { data } = await deleteServer(row.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-input v-model="keyword" placeholder="搜索名称 / 地址 / 地区" :prefix-icon="Search" clearable style="width: 240px" />
        <el-button @click="load"><el-icon><Refresh /></el-icon>&nbsp;刷新</el-button>
      </div>
      <el-button type="primary" @click="createOpen = true"><el-icon><Plus /></el-icon>&nbsp;新增服务器</el-button>
    </div>

    <BaseCard>
      <el-table v-loading="loading" :data="filtered">
        <el-table-column prop="name" label="名称" min-width="120">
          <template #default="{ row }"><span style="font-weight: 600">{{ row.name }}</span></template>
        </el-table-column>
        <el-table-column prop="host" label="地址" min-width="160">
          <template #default="{ row }"><code class="cell-mono">{{ row.host }}</code></template>
        </el-table-column>
        <el-table-column prop="location" label="地区" width="110">
          <template #default="{ row }">{{ row.location || '—' }}</template>
        </el-table-column>
        <el-table-column label="状态" width="100">
          <template #default="{ row }">
            <el-tag :type="row.status === 1 ? 'success' : 'info'" size="small">
              <span class="x-status-dot" :class="row.status === 1 ? 'online' : 'offline'" />{{ row.status === 1 ? '在线' : '离线' }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column label="接入点" width="90">
          <template #default="{ row }">
            <el-link type="primary" :underline="false" @click="goInbounds(row)">
              {{ inboundCount(row.id) }} 个
            </el-link>
          </template>
        </el-table-column>
        <el-table-column label="配置同步" width="100">
          <template #default="{ row }">
            <el-tag v-if="row.config_status === 'pushed'" type="success" size="small">已同步</el-tag>
            <el-tag v-else-if="row.config_status === 'pending'" type="warning" size="small">待推送</el-tag>
            <el-tag v-else type="info" size="small" effect="plain">未生成</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="最后心跳" width="170">
          <template #default="{ row }"><span class="muted">{{ fmtTime(row.last_seen_at) }}</span></template>
        </el-table-column>
        <el-table-column label="操作" width="360" fixed="right">
          <template #default="{ row }">
            <el-button size="small" text type="primary" @click="openDrawer(row)"><el-icon><Setting /></el-icon>&nbsp;管理</el-button>
            <el-button size="small" text @click="openStatus(row)"><el-icon><View /></el-icon>&nbsp;状态</el-button>
            <el-button size="small" text type="success" @click="genPush(row)"><el-icon><VideoPlay /></el-icon>&nbsp;生成</el-button>
            <el-button size="small" text @click="openPush(row)"><el-icon><Document /></el-icon>&nbsp;下发</el-button>
            <el-button size="small" text @click="restartXray(row)"><el-icon><VideoPlay /></el-icon>&nbsp;重启</el-button>
            <el-button size="small" text @click="openLogs(row)"><el-icon><Key /></el-icon>&nbsp;日志</el-button>
            <el-dropdown trigger="click" @command="(cmd: string) => onMore(cmd, row)">
              <el-button size="small" text>更多<el-icon style="margin-left: 2px"><ArrowDown /></el-icon></el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="edit"><el-icon><Edit /></el-icon>编辑</el-dropdown-item>
                  <el-dropdown-item command="reset"><el-icon><Key /></el-icon>重置密钥</el-dropdown-item>
                  <el-dropdown-item command="delete" divided><el-icon><Delete /></el-icon>删除</el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </template>
        </el-table-column>
        <template #empty>
          <div style="padding: 30px 0; color: var(--x-text-3)">
            尚未添加服务器。点击右上角「新增服务器」，按提示配置节点 Agent。
          </div>
        </template>
      </el-table>
    </BaseCard>

    <!-- 新增服务器 -->
    <el-dialog v-model="createOpen" title="新增服务器" width="680px" @close="closeCreate">
      <template v-if="!createdResult">
        <el-form label-position="top">
          <el-form-item label="名称"><el-input v-model="createForm.name" placeholder="如 Tokyo-01" /></el-form-item>
          <el-form-item label="地址"><el-input v-model="createForm.host" placeholder="如 tokyo01.example.com" /></el-form-item>
          <el-form-item label="地区"><el-input v-model="createForm.location" placeholder="如 日本（选填）" /></el-form-item>
          <el-form-item label="备注"><el-input v-model="createForm.remark" placeholder="选填" /></el-form-item>
        </el-form>
      </template>
      <template v-else>
        <el-alert type="success" :closable="false" show-icon title="服务器已创建" description="在节点上以 root 执行以下一键安装命令：" style="margin-bottom: 12px" />
        <div class="secret-box">
          <div class="secret-row install-row">
            <span class="k">安装</span>
            <code class="install-cmd">{{ createdResult.install_cmd }}</code>
            <el-button size="small" text @click="copyText(createdResult.install_cmd, '安装命令')"><el-icon><CopyDocument /></el-icon></el-button>
          </div>
          <div class="secret-row">
            <span class="k">node_id</span>
            <code>{{ createdResult.node_id }}</code>
            <el-button size="small" text @click="copyText(createdResult.node_id, 'node_id')"><el-icon><CopyDocument /></el-icon></el-button>
          </div>
          <div class="secret-row">
            <span class="k">secret</span>
            <code>{{ createdResult.secret }}</code>
            <el-button size="small" text @click="copyText(createdResult.secret, 'secret')"><el-icon><CopyDocument /></el-icon></el-button>
          </div>
          <p class="muted tip" style="margin: 0">secret 仅显示这一次；安装后回到「服务器」页可查看节点在线状态并「生成」下发配置。</p>
        </div>
      </template>
      <template #footer>
        <template v-if="!createdResult">
          <el-button @click="createOpen = false">取消</el-button>
          <el-button type="primary" :loading="creating" @click="submitCreate">创建</el-button>
        </template>
        <template v-else>
          <el-button type="primary" @click="closeCreate">完成</el-button>
        </template>
      </template>
    </el-dialog>

    <!-- 下发配置（主控生成完整配置并推送） -->
    <el-dialog v-model="pushOpen" :title="`生成并下发配置 · ${pushTarget?.name ?? ''}`" width="680px">
      <p class="muted" style="margin: 0 0 10px; font-size: 12.5px">
        按该节点启用入站 + 出站 + 路由规则 + 全部启用用户生成完整 Xray 配置：保存待推送，节点在线立即下发（离线保留，上线自动补推）。
      </p>
      <pre v-loading="pushLoading" class="log-view">{{ pushResult ? `结果：${pushResult}\n\n` : '' }}{{ maskUUIDs(pushConfigText) || (pushLoading ? '正在生成配置…' : '') }}</pre>
      <div v-if="pushConfigText" style="display: flex; gap: 8px; margin-top: 10px">
        <el-button size="small" @click="copyText(pushConfigText, '配置 JSON')"><el-icon><CopyDocument /></el-icon>&nbsp;复制配置</el-button>
      </div>
      <template #footer>
        <el-button type="primary" @click="pushOpen = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 生成结果 -->
    <el-dialog v-model="genOpen" title="生成并下发配置" width="680px">
      <p class="muted" style="margin: 0 0 10px; font-size: 12.5px">
        生成的完整 Xray 配置如下（已保存待推送；节点在线则立即下发，离线则上线自动补推）：
      </p>
      <pre v-loading="genLoading" class="log-view">{{ maskUUIDs(genResult) || '正在生成…' }}</pre>
      <div v-if="genResult && !genLoading" style="display: flex; gap: 8px; margin-top: 10px">
        <el-button size="small" @click="copyText(genResult, '配置 JSON')"><el-icon><CopyDocument /></el-icon>&nbsp;复制配置</el-button>
      </div>
      <template #footer>
        <el-button type="primary" @click="genOpen = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 状态详情 -->
    <el-dialog v-model="statusOpen" title="节点状态" width="420px">
      <div v-loading="statusLoading" class="status-rows">
        <template v-if="statusData">
          <div class="row"><span class="k">Xray 运行</span><span class="v">{{ statusData.data?.xray_running ? '✅ 运行中' : '⛔ 已停止' }}</span></div>
          <div class="row"><span class="k">进程 PID</span><span class="v">{{ statusData.data?.pid ?? '—' }}</span></div>
          <div class="row"><span class="k">启动时间</span><span class="v">{{ statusData.data?.started_at ? fmtTime(statusData.data.started_at) : '—' }}</span></div>
          <div class="row"><span class="k">运行时长</span><span class="v">{{ statusData.data?.uptime_sec ?? 0 }} 秒</span></div>
          <div class="row"><span class="k">配置路径</span><span class="v"><code class="cell-mono">{{ statusData.data?.config_path ?? '—' }}</code></span></div>
        </template>
        <el-empty v-else-if="!statusLoading" description="无数据" />
      </div>
    </el-dialog>

    <!-- 日志 -->
    <el-dialog v-model="logOpen" :title="`最近日志 · ${logTarget?.name ?? ''}`" width="620px">
      <pre v-loading="logLoading" class="log-view">{{ logContent }}</pre>
    </el-dialog>

    <!-- 编辑服务器 -->
    <el-dialog v-model="editOpen" title="编辑服务器" width="460px">
      <el-form label-position="top">
        <el-form-item label="名称"><el-input v-model="editForm.name" /></el-form-item>
        <el-form-item label="地址"><el-input v-model="editForm.host" placeholder="如 tokyo01.example.com" /></el-form-item>
        <el-form-item label="地区"><el-input v-model="editForm.location" placeholder="选填" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="editForm.remark" placeholder="选填" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editOpen = false">取消</el-button>
        <el-button type="primary" :loading="editSaving" @click="submitEdit">保存</el-button>
      </template>
    </el-dialog>

    <!-- 重置密钥 -->
    <el-dialog v-model="secretOpen" title="重置密钥" width="600px">
      <el-alert type="warning" :closable="false" show-icon title="新密钥已生成（仅显示这一次）" description="请更新节点 /etc/xray-agent/config.yml 中的 secret 后重启 xray-agent 服务；或重新执行下方安装命令" style="margin-bottom: 12px" />
      <div v-if="secretInfo" class="secret-box">
        <div class="secret-row">
          <span class="k">node_id</span>
          <code>{{ secretInfo.node_id }}</code>
        </div>
        <div class="secret-row">
          <span class="k">secret</span>
          <code>{{ secretInfo.secret }}</code>
          <el-button size="small" text @click="copyText(secretInfo.secret, 'secret')"><el-icon><CopyDocument /></el-icon></el-button>
        </div>
        <div v-if="secretInfo.install_cmd" class="secret-row">
          <span class="k">安装</span>
          <code class="install-cmd">{{ secretInfo.install_cmd }}</code>
          <el-button size="small" text @click="copyText(secretInfo.install_cmd!, '安装命令')"><el-icon><CopyDocument /></el-icon></el-button>
        </div>
      </div>
      <template #footer>
        <el-button type="primary" @click="secretOpen = false">关闭</el-button>
      </template>
    </el-dialog>

    <!-- 节点管理抽屉 -->
    <ServerNodeDrawer
      v-model="drawerOpen"
      :server="drawerServer"
      @removed="load"
      @changed="load"
    />
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-text-2); }
.muted { color: var(--x-text-3); }
.secret-box { display: grid; gap: 10px; }
.tip { font-size: 12px; color: var(--x-text-3); }
.install-cmd { font-size: 11.5px; }
.secret-row {
  display: flex;
  align-items: center;
  gap: 10px;
  background: var(--x-primary-soft);
  border-radius: 8px;
  padding: 10px 12px;
  .k { color: var(--x-text-2); font-size: 12.5px; flex: none; width: 56px; }
  code { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-primary); word-break: break-all; flex: 1; }
}
.ok-text { color: var(--x-success); font-size: 13px; }
.err-text { color: var(--x-danger); font-size: 13px; }
.status-rows .row { display: flex; justify-content: space-between; padding: 11px 0; border-bottom: 1px solid var(--x-border); font-size: 13.5px; }
.status-rows .k { color: var(--x-text-2); }
.status-rows .v { font-weight: 500; }
.log-view {
  background: #171b2e;
  color: #c7d2fe;
  border-radius: 8px;
  padding: 14px;
  font-family: ui-monospace, Menlo, Consolas, monospace;
  font-size: 12px;
  line-height: 1.6;
  max-height: 420px;
  overflow: auto;
  white-space: pre-wrap;
  word-break: break-all;
}
</style>