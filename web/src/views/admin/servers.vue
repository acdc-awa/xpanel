<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { Plus, Search, Refresh, View, Document, Delete, Key, CopyDocument, Edit, Setting, RefreshRight, TrendCharts, Upload, MoreFilled } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import ServerNodeDrawer from './servers/ServerNodeDrawer.vue'
import ServerMetricsDrawer from './servers/ServerMetricsDrawer.vue'
import {
  createServer,
  deleteServer,
  getInbounds,
  getServers,
  resetServerSecret,
  serverCommand,
  upgradeAgent,
  getAgentLatestVersion,
  type CommandResult,
  type ServerItem,
  updateServer,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import { compareVersion } from '@/utils/version'

const router = useRouter()

const list = ref<ServerItem[]>([])
const loading = ref(false)
const keyword = ref('')

const metricsOpen = ref(false)
const metricsServer = ref<ServerItem | null>(null)

function openMetrics(row: any) {
  metricsServer.value = row
  metricsOpen.value = true
}

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

// ---- 官方 Agent 最新版本感知与比对 ----
const latestAgentVersion = ref('')
const fetchingLatestAgent = ref(false)

async function loadLatestAgentVersion(refresh = false) {
  fetchingLatestAgent.value = true
  try {
    const { data } = await getAgentLatestVersion(refresh)
    if (data.code === 0 && data.data?.latest_version) {
      latestAgentVersion.value = data.data.latest_version
    }
  } catch {
    // 静默降级
  } finally {
    fetchingLatestAgent.value = false
  }
}

function getAgentVersionStatus(currentVer: string) {
  if (!currentVer) return { type: 'unknown', label: '未知' }
  if (!latestAgentVersion.value) return { type: 'normal', label: '' }
  const cmp = compareVersion(currentVer, latestAgentVersion.value)
  if (cmp < 0) {
    return { type: 'outdated', label: `有新版 ${latestAgentVersion.value}` }
  }
  return { type: 'latest', label: '最新' }
}

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
  loadLatestAgentVersion()
}
onMounted(load)

function fmtTime(t: string | null) {
  if (!t) return '—'
  const d = new Date(t)
  return d.toLocaleString('zh-CN', { hour12: false })
}

// ---- 新增服务器 ----
const createOpen = ref(false)
const createForm = reactive({ server_type: 'xray' as 'xray', name: '', host: '', location: '', remark: '' })
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

// ---- Agent 升级（面板触发节点自升级，节点从 GitHub Releases 拉取） ----
const upgradingId = ref(0)

async function upgradeNodeAgent(row: any) {
  const currentVer = row.agent_version || ''
  const latest = latestAgentVersion.value
  let msg = ''
  let isAlreadyLatest = false

  if (latest && currentVer) {
    const cmp = compareVersion(currentVer, latest)
    if (cmp >= 0) {
      isAlreadyLatest = true
      msg = `节点「${row.name}」当前 Agent 版本（${currentVer}）已是官方最新版本（${latest}）。\n\n是否仍要重新拉取并覆盖安装？`
    } else {
      msg = `检测到官方最新版本 Agent：\n- 当前节点版本：${currentVer}\n- 官方最新版本：${latest}\n\n确认将节点「${row.name}」升级至最新版？（sha256 校验，完成后节点自动重启，期间短暂离线）`
    }
  } else {
    msg = `将从 GitHub Releases 下载最新版 Agent 并在节点「${row.name}」上升级（sha256 校验），完成后节点自动重启，期间短暂离线。当前版本：${currentVer || '未知'}`
  }

  try {
    await ElMessageBox.confirm(
      msg,
      isAlreadyLatest ? '重新安装 Agent' : '升级 Agent',
      {
        type: isAlreadyLatest ? 'info' : 'warning',
        confirmButtonText: isAlreadyLatest ? '重新安装' : '立即升级',
        cancelButtonText: '取消',
      },
    )
  } catch {
    return
  }

  upgradingId.value = row.id
  try {
    const { data } = await upgradeAgent(row.id, {
      target: latest || undefined,
      force: isAlreadyLatest,
    })
    if (data.code === 0 && data.data.ok) {
      ElMessage.success((data.data.data as string) || '已触发升级')
      load()
      // 新版本号要等节点重启后的首次心跳（约 30s）才回填，延迟再刷一次
      setTimeout(load, 35000)
    } else {
      ElMessage.error(data.data?.error || data.message || '升级失败')
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '升级失败（节点可能仍在下载，可稍后刷新查看版本；若节点 Agent 过旧不支持远程升级，请在节点重跑安装命令）'))
  } finally {
    upgradingId.value = 0
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

// ---- 编辑服务器 ----
const editOpen = ref(false)
const editForm = reactive({ id: 0, server_type: 'xray' as 'xray', name: '', host: '', location: '', remark: '' })
const editSaving = ref(false)

function openEdit(row: any) {
  Object.assign(editForm, {
    id: row.id,
    server_type: 'xray',
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
      server_type: editForm.server_type,
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

// ---- 更多操作（编辑/重置密钥/删除/状态/日志/升级/重启） ----
function onMore(cmd: string, row: any) {
  if (cmd === 'edit') openEdit(row)
  else if (cmd === 'reset') resetSecret(row)
  else if (cmd === 'delete') removeServer(row)
  else if (cmd === 'status') openStatus(row)
  else if (cmd === 'restart') restartXray(row)
  else if (cmd === 'logs') openLogs(row)
  else if (cmd === 'upgrade') upgradeNodeAgent(row)
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
        <el-tag
          v-if="latestAgentVersion"
          size="default"
          type="info"
          class="cell-mono"
          style="cursor: pointer; height: 32px; display: inline-flex; align-items: center"
          title="官方最新 Agent 版本，点击刷新最新版本检测"
          @click="loadLatestAgentVersion(true)"
        >
          官方最新: {{ latestAgentVersion }}
        </el-tag>
      </div>
      <el-button type="primary" @click="createOpen = true"><el-icon><Plus /></el-icon>&nbsp;新增服务器</el-button>
    </div>

    <BaseCard title="服务器节点列表">
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <el-icon class="is-loading" style="font-size: 26px; color: var(--x-primary)"><Loading /></el-icon>
      </div>

      <div v-else-if="filtered.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
        <el-icon style="font-size: 32px; color: var(--x-text-3)"><Platform /></el-icon>
        <p style="margin-top: 8px">{{ keyword ? '未找到匹配服务器' : '尚未添加服务器，点击右上角「新增服务器」' }}</p>
      </div>

      <!-- 全局统一服务器卡片网格流 (自适应 1~4 列) -->
      <div v-else class="server-card-grid">
        <div v-for="row in filtered" :key="row.id" class="server-card">
          <!-- 头部 -->
          <div class="card-head">
            <div class="head-title">
              <span class="x-status-dot" :class="row.status === 1 ? 'online' : 'offline'" />
              <span class="server-name" title="点击查看详情" @click="openDrawer(row)">{{ row.name }}</span>
              <span class="x-chip" :class="row.status === 1 ? 'green' : 'gray'" style="font-size: 10px; padding: 1px 5px">
                {{ row.status === 1 ? '在线' : '离线' }}
              </span>
              <span v-if="row.location" class="x-chip blue" style="font-size: 10px; padding: 1px 5px">{{ row.location }}</span>
            </div>
            <el-tooltip
              v-if="row.config_status === 'pending' && row.push_error"
              :content="`最后失败：${row.push_error}（已尝试 ${row.push_attempts || 0} 次）`"
              placement="top"
            >
              <span class="x-chip orange" style="cursor: help; font-size: 10.5px">待推送</span>
            </el-tooltip>
            <span v-else-if="row.config_status === 'pushed'" class="x-chip green" style="font-size: 10.5px">已同步</span>
            <span v-else-if="row.config_status === 'pending'" class="x-chip orange" style="font-size: 10.5px">待推送</span>
            <span v-else class="x-chip gray" style="font-size: 10.5px">未生成</span>
          </div>

          <!-- 属性网格 -->
          <div class="card-grid">
            <div class="grid-item full-width">
              <span class="item-label">节点地址</span>
              <div class="item-value">
                <code class="cell-mono font-12" style="cursor: pointer; color: var(--x-primary); font-weight: 600" title="点击复制" @click="copyText(row.host, '节点地址')">
                  {{ row.host }}
                </code>
              </div>
            </div>
            <div class="grid-item">
              <span class="item-label">接入点</span>
              <div class="item-value">
                <el-link type="primary" :underline="false" style="font-size: 12.5px; font-weight: 600" @click="goInbounds(row)">
                  {{ inboundCount(row.id) }} 个入站
                </el-link>
              </div>
            </div>
            <div class="grid-item">
              <span class="item-label">Agent 版本</span>
              <div class="item-value" style="display: flex; align-items: center; gap: 4px; flex-wrap: wrap">
                <span class="cell-mono muted font-12">{{ row.agent_version || 'v—' }}</span>
                <span
                  v-if="getAgentVersionStatus(row.agent_version).type === 'latest'"
                  class="x-chip green"
                  style="font-size: 9.5px; padding: 0 4px"
                >最新</span>
                <span
                  v-else-if="getAgentVersionStatus(row.agent_version).type === 'outdated'"
                  class="x-chip orange"
                  style="font-size: 9.5px; padding: 0 4px"
                  :title="`官方最新版本 ${latestAgentVersion}`"
                >有新版</span>
                <el-link
                  v-if="row.status === 1"
                  :type="getAgentVersionStatus(row.agent_version).type === 'outdated' ? 'warning' : 'primary'"
                  :underline="false"
                  :disabled="upgradingId === row.id"
                  style="font-size: 11px; font-weight: 600"
                  @click="upgradeNodeAgent(row)"
                >
                  {{ getAgentVersionStatus(row.agent_version).type === 'outdated' ? '升级' : (getAgentVersionStatus(row.agent_version).type === 'latest' ? '重装' : '升级') }}
                </el-link>
              </div>
            </div>
            <div class="grid-item full-width">
              <span class="item-label">最后心跳</span>
              <div class="item-value cell-mono muted" style="font-size: 11.5px">
                {{ row.last_seen_at ? fmtTime(row.last_seen_at) : '未有心跳记录' }}
              </div>
            </div>
          </div>

          <!-- 操作按钮栏 -->
          <div class="card-foot-actions">
            <el-button size="small" type="primary" plain @click="openDrawer(row)">
              <el-icon><Setting /></el-icon>&nbsp;详情
            </el-button>
            <el-button size="small" type="success" plain @click="openMetrics(row)">
              <el-icon><TrendCharts /></el-icon>&nbsp;监控
            </el-button>
            <el-button size="small" @click="restartXray(row)">
              <el-icon><RefreshRight /></el-icon>&nbsp;重启
            </el-button>
            <el-dropdown trigger="click" @command="(cmd: string) => onMore(cmd, row)">
              <el-button size="small" style="flex: none; padding: 0 8px">
                <el-icon><MoreFilled /></el-icon>
              </el-button>
              <template #dropdown>
                <el-dropdown-menu>
                  <el-dropdown-item command="status"><el-icon><View /></el-icon>运行状态</el-dropdown-item>
                  <el-dropdown-item command="logs"><el-icon><Document /></el-icon>节点日志</el-dropdown-item>
                  <el-dropdown-item command="upgrade">
                    <el-icon><Upload /></el-icon>
                    {{ getAgentVersionStatus(row.agent_version).type === 'outdated' ? '升级 Agent' : (getAgentVersionStatus(row.agent_version).type === 'latest' ? '重新安装 Agent' : '升级 Agent') }}
                  </el-dropdown-item>
                  <el-dropdown-item divided command="edit"><el-icon><Edit /></el-icon>编辑服务器</el-dropdown-item>
                  <el-dropdown-item command="reset"><el-icon><Key /></el-icon>重置密钥</el-dropdown-item>
                  <el-dropdown-item command="delete" divided style="color: var(--el-color-danger)">
                    <el-icon><Delete /></el-icon>删除服务器
                  </el-dropdown-item>
                </el-dropdown-menu>
              </template>
            </el-dropdown>
          </div>
        </div>
      </div>
    </BaseCard>

    <!-- 新增服务器 -->
    <el-dialog v-model="createOpen" title="新增服务器" width="680px" @close="closeCreate">
      <template v-if="!createdResult">
        <el-form label-position="top">
          <el-form-item label="名称"><el-input v-model="createForm.name" placeholder="如 Tokyo-01 / 广州移动 BGP" /></el-form-item>
          <el-form-item label="地址"><el-input v-model="createForm.host" placeholder="如 tokyo01.example.com / 120.232.x.x" /></el-form-item>
          <el-form-item label="地区"><el-input v-model="createForm.location" placeholder="如 日本 / 广州（选填）" /></el-form-item>
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

    <!-- 状态详情 -->
    <el-dialog v-model="statusOpen" title="节点状态" width="420px">
      <div v-loading="statusLoading" class="status-rows">
        <template v-if="statusData">
          <div class="row"><span class="k">Xray 运行</span><span class="v">{{ statusData.data?.xray_running ? '运行中' : '已停止' }}</span></div>
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

    <!-- 时序性能监控抽屉 -->
    <ServerMetricsDrawer
      v-model="metricsOpen"
      :server-id="metricsServer?.id || 0"
      :server-name="metricsServer?.name || ''"
    />
  </div>
</template>

<style scoped lang="scss">
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-text-2); }
.agent-cell { display: flex; align-items: center; gap: 8px; }
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

/* ================= 全局统一服务器卡片网格流 ================= */
.server-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}

.server-card {
  background: var(--x-card, #ffffff);
  border: 1px solid var(--x-border, #e5e7eb);
  border-radius: var(--x-radius, 10px);
  padding: 14px;
  transition: all 0.2s cubic-bezier(0.2, 0, 0, 1);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  display: flex;
  flex-direction: column;
  justify-content: space-between;

  &:hover {
    border-color: var(--x-border-hover, #cbd5e1);
    box-shadow: var(--x-shadow-md);
    transform: translateY(-1px);
  }

  .card-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding-bottom: 10px;
    border-bottom: 1px dashed var(--x-border, #e5e7eb);

    .head-title {
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
    }

    .server-name {
      font-weight: 600;
      font-size: 14px;
      color: var(--x-text, #111827);
      cursor: pointer;
      &:hover {
        color: var(--x-primary);
      }
    }
  }

  .card-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px 12px;
    padding: 10px 0;

    .grid-item {
      display: flex;
      flex-direction: column;
      gap: 2px;

      &.full-width {
        grid-column: 1 / -1;
      }

      .item-label {
        font-size: 11px;
        color: var(--x-text-3, #9ca3af);
      }

      .item-value {
        font-size: 12.5px;
        color: var(--x-text, #1f2937);
        font-weight: 500;
      }
    }
  }

  .card-foot-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    padding-top: 10px;
    border-top: 1px solid var(--x-border-light, #f1f5f9);
    margin-top: 6px;

    .el-button {
      flex: 1;
      margin: 0;
      font-size: 12px;
      padding: 6px 8px;
      height: 30px;
    }
  }
}

@media (max-width: 768px) {
  .secret-row {
    flex-direction: column;
    align-items: flex-start;
    gap: 6px;

    .k {
      width: auto;
      font-weight: 600;
    }
  }

  .status-rows .row {
    flex-direction: column;
    align-items: flex-start;
    gap: 4px;
    padding: 8px 0;
  }
}

@media (max-width: 640px) {
  .server-card-grid {
    grid-template-columns: 1fr;
  }
}
</style>