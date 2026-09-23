<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  Plus,
  Refresh,
  Delete,
  Edit,
  CopyDocument,
  Connection,
  Search,
  Key,
  MagicStick,
  Switch,
  Timer,
  Coin,
} from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import BaseCard from '@/components/base/BaseCard.vue'
import TipIcon from '@/components/base/TipIcon.vue'
import {
  getAdminChannels,
  createAdminChannel,
  updateAdminChannel,
  deleteAdminChannel,
  toggleAdminChannel,
  resetAdminChannelTraffic,
  getServers,
  type ProxyChannelItem,
  type ProxyChannelPayload,
  type ServerItem,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import { formatBytes } from '@/utils/format'
import { formatDateTime } from '@/utils/timezone'

const loading = ref(false)
const channels = ref<ProxyChannelItem[]>([])
const servers = ref<ServerItem[]>([])

// 筛选状态
const filterServer = ref<number | ''>('')
const filterProto = ref<string>('')
const filterStatus = ref<string>('')
const filterKeyword = ref<string>('')

async function loadData() {
  loading.value = true
  try {
    const [chRes, srvRes] = await Promise.all([
      getAdminChannels({
        server_id: filterServer.value !== '' ? filterServer.value : undefined,
        protocol: filterProto.value || undefined,
        status: filterStatus.value || undefined,
      }),
      getServers(),
    ])
    if (chRes.data.code === 0) {
      channels.value = chRes.data.data.channels
    } else {
      ElMessage.error(chRes.data.message)
    }
    if (srvRes.data.code === 0) {
      servers.value = srvRes.data.data.items
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载独立通道失败'))
  } finally {
    loading.value = false
  }
}

onMounted(loadData)

// 客户端二次关键字过滤
const filteredChannels = computed(() => {
  const kw = filterKeyword.value.trim().toLowerCase()
  if (!kw) return channels.value
  return channels.value.filter((ch) => {
    return (
      ch.name.toLowerCase().includes(kw) ||
      String(ch.port).includes(kw) ||
      (ch.server_name && ch.server_name.toLowerCase().includes(kw)) ||
      (ch.server_host && ch.server_host.toLowerCase().includes(kw)) ||
      (ch.target_address && ch.target_address.toLowerCase().includes(kw)) ||
      (ch.username && ch.username.toLowerCase().includes(kw))
    )
  })
})

// 顶部概览统计
const totalCount = computed(() => channels.value.length)
const activeCount = computed(() => channels.value.filter((c) => c.status === 'active' && c.enabled).length)
const totalUsedTraffic = computed(() => channels.value.reduce((acc, c) => acc + (c.traffic_used_bytes || 0), 0))
const totalQuotaGB = computed(() => channels.value.reduce((acc, c) => acc + (c.traffic_limit_gb || 0), 0))

// 格式辅助函数
function protocolLabel(proto: string) {
  switch (proto) {
    case 'tunnel':
      return '四层直通'
    case 'socks5':
      return 'SOCKS5'
    case 'http':
      return 'HTTP 代理'
    default:
      return proto
  }
}

function protocolTagType(proto: string) {
  switch (proto) {
    case 'tunnel':
      return 'primary'
    case 'socks5':
      return 'success'
    case 'http':
      return 'warning'
    default:
      return 'info'
  }
}

function statusLabel(status: string, enabled: boolean) {
  if (!enabled) return '已停用'
  switch (status) {
    case 'active':
      return '正常运行'
    case 'quota_exceeded':
      return '超额熔断'
    case 'expired':
      return '已到期'
    case 'disabled':
      return '已禁用'
    default:
      return status
  }
}

function statusTagType(status: string, enabled: boolean) {
  if (!enabled) return 'info'
  switch (status) {
    case 'active':
      return 'success'
    case 'quota_exceeded':
      return 'danger'
    case 'expired':
      return 'warning'
    case 'disabled':
      return 'info'
    default:
      return 'info'
  }
}

function resetLabel(reset: string) {
  switch (reset) {
    case 'daily':
      return '每日重置'
    case 'weekly':
      return '每周重置'
    case 'monthly':
      return '每月 1 日重置'
    case 'never':
    default:
      return '从不重置'
  }
}

function progressPercent(ch: ProxyChannelItem) {
  if (!ch.traffic_limit_gb || ch.traffic_limit_gb <= 0) return 0
  const limitBytes = ch.traffic_limit_gb * 1024 * 1024 * 1024
  const pct = (ch.traffic_used_bytes / limitBytes) * 100
  return Math.min(100, Math.round(pct * 10) / 10)
}

function progressColor(pct: number) {
  if (pct >= 100) return '#f56c6c'
  if (pct >= 80) return '#e6a23c'
  return '#409eff'
}

// 快速复制
async function copyText(text: string, title = '连接信息已复制到剪贴板') {
  if (!text) return
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(title)
  } catch {
    ElMessage.error('复制失败，请手动选择复制')
  }
}

// 启停控制
async function handleToggle(ch: ProxyChannelItem) {
  try {
    const { data } = await toggleAdminChannel(ch.id)
    if (data.code === 0) {
      ch.enabled = data.data.enabled
      ch.status = data.data.status as any
      ElMessage.success(ch.enabled ? '通道已开启' : '通道已停用')
    } else {
      ElMessage.error(data.message)
      ch.enabled = !ch.enabled
    }
  } catch (e) {
    ch.enabled = !ch.enabled
    ElMessage.error(errMsg(e, '切换状态失败'))
  }
}

// 重置流量
async function handleResetTraffic(ch: ProxyChannelItem) {
  try {
    await ElMessageBox.confirm(`确定要清零独立通道「${ch.name}」的当前周期已用流量吗？`, '重置流量', {
      confirmButtonText: '确定重置',
      cancelButtonText: '取消',
      type: 'warning',
    })
    const { data } = await resetAdminChannelTraffic(ch.id)
    if (data.code === 0) {
      ElMessage.success('已用流量已成功归零')
      ch.traffic_used_bytes = 0
      ch.up_bytes = 0
      ch.down_bytes = 0
      ch.status = data.data.status as any
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(errMsg(e, '重置流量失败'))
  }
}

// 删除通道
async function handleDelete(ch: ProxyChannelItem) {
  try {
    await ElMessageBox.confirm(
      `确定要删除独立通道「${ch.name}」吗？此操作将立即从宿主服务器中注销并关闭端口 ${ch.port}。`,
      '删除通道',
      {
        confirmButtonText: '确定删除',
        cancelButtonText: '取消',
        type: 'error',
      },
    )
    const { data } = await deleteAdminChannel(ch.id)
    if (data.code === 0) {
      ElMessage.success('通道已删除')
      loadData()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    if (e !== 'cancel') ElMessage.error(errMsg(e, '删除通道失败'))
  }
}

// ---- 表单弹窗（新建 / 编辑） ----
const dialogVisible = ref(false)
const isEditing = ref(false)
const saving = ref(false)
const editId = ref(0)

const form = reactive({
  name: '',
  server_id: undefined as number | undefined,
  port: 1080,
  listen: '',
  protocol: 'tunnel',
  target_address: '',
  target_port: 443,
  proxy_protocol: false,
  username: '',
  password: '',
  allow_udp: true,
  traffic_limit_gb: 0,
  traffic_reset: 'monthly',
  expires_at: '',
  auto_disable: true,
})

function openCreate() {
  isEditing.value = false
  editId.value = 0
  form.name = ''
  form.server_id = servers.value.length > 0 ? servers.value[0].id : undefined
  form.port = 10001
  form.listen = ''
  form.protocol = 'tunnel'
  form.target_address = ''
  form.target_port = 443
  form.proxy_protocol = false
  form.username = ''
  form.password = ''
  form.allow_udp = true
  form.traffic_limit_gb = 0
  form.traffic_reset = 'monthly'
  form.expires_at = ''
  form.auto_disable = true
  dialogVisible.value = true
}

function openEdit(ch: ProxyChannelItem) {
  isEditing.value = true
  editId.value = ch.id
  form.name = ch.name
  form.server_id = ch.server_id
  form.port = ch.port
  form.listen = ch.listen || ''
  form.protocol = ch.protocol
  form.target_address = ch.target_address || ''
  form.target_port = ch.target_port || 443
  form.proxy_protocol = ch.proxy_protocol
  form.username = ch.username || ''
  form.password = ch.password || ''
  form.allow_udp = ch.allow_udp
  form.traffic_limit_gb = ch.traffic_limit_gb
  form.traffic_reset = ch.traffic_reset || 'never'
  form.expires_at = ch.expires_at || ''
  form.auto_disable = ch.auto_disable
  dialogVisible.value = true
}

function generateRandomPassword() {
  const chars = 'ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789!@#$%^&*'
  let pwd = ''
  for (let i = 0; i < 16; i++) {
    pwd += chars.charAt(Math.floor(Math.random() * chars.length))
  }
  form.password = pwd
}

async function handleSave() {
  if (!form.name.trim()) {
    ElMessage.warning('请填写通道名称')
    return
  }
  if (!form.server_id) {
    ElMessage.warning('请选择所属服务器')
    return
  }
  if (!form.port || form.port < 1 || form.port > 65535) {
    ElMessage.warning('请填写有效的端口范围 (1-65535)')
    return
  }
  if (form.protocol === 'tunnel') {
    if (!form.target_address.trim() || !form.target_port) {
      ElMessage.warning('四层直通必须填写目标地址与端口')
      return
    }
  }

  saving.value = true
  try {
    const payload: ProxyChannelPayload = {
      name: form.name.trim(),
      server_id: form.server_id,
      port: form.port,
      listen: form.listen.trim() || undefined,
      protocol: form.protocol,
      target_address: form.protocol === 'tunnel' ? form.target_address.trim() : undefined,
      target_port: form.protocol === 'tunnel' ? form.target_port : undefined,
      proxy_protocol: form.protocol === 'tunnel' ? form.proxy_protocol : undefined,
      username: form.protocol !== 'tunnel' ? form.username.trim() : undefined,
      password: form.protocol !== 'tunnel' ? form.password.trim() : undefined,
      allow_udp: form.protocol === 'socks5' ? form.allow_udp : undefined,
      traffic_limit_gb: Number(form.traffic_limit_gb) || 0,
      traffic_reset: form.traffic_reset,
      expires_at: form.expires_at || undefined,
      auto_disable: form.auto_disable,
    }

    if (isEditing.value) {
      const { data } = await updateAdminChannel(editId.value, payload)
      if (data.code === 0) {
        ElMessage.success('独立通道已更新')
        dialogVisible.value = false
        loadData()
      } else {
        ElMessage.error(data.message)
      }
    } else {
      const { data } = await createAdminChannel(payload)
      if (data.code === 0) {
        ElMessage.success('独立通道创建成功')
        dialogVisible.value = false
        loadData()
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
</script>

<template>
  <div class="channels-page x-page">
    <!-- 顶部概览指标舱 -->
    <div class="kpi-grid">
      <BaseCard class="kpi-card">
        <div class="kpi-label">
          <el-icon><Connection /></el-icon>
          <span>总独立通道</span>
        </div>
        <div class="kpi-val cell-mono">{{ totalCount }}</div>
        <div class="kpi-sub">当前系统中托管的所有通道</div>
      </BaseCard>

      <BaseCard class="kpi-card">
        <div class="kpi-label">
          <el-icon><Switch /></el-icon>
          <span>活跃运行中</span>
        </div>
        <div class="kpi-val cell-mono is-active">{{ activeCount }}</div>
        <div class="kpi-sub">状态正常且处于开启状态</div>
      </BaseCard>

      <BaseCard class="kpi-card">
        <div class="kpi-label">
          <el-icon><Coin /></el-icon>
          <span>当前总流量用量</span>
        </div>
        <div class="kpi-val cell-mono">{{ formatBytes(totalUsedTraffic) }}</div>
        <div class="kpi-sub">各通道本周期物理流量总和</div>
      </BaseCard>

      <BaseCard class="kpi-card">
        <div class="kpi-label">
          <el-icon><Timer /></el-icon>
          <span>独立总配额</span>
        </div>
        <div class="kpi-val cell-mono">{{ totalQuotaGB > 0 ? `${totalQuotaGB} GB` : '无限制' }}</div>
        <div class="kpi-sub">超额通道将自动触发熔断断流</div>
      </BaseCard>
    </div>

    <!-- 顶部操作栏与多维筛选 -->
    <BaseCard class="filter-card">
      <div class="filter-bar">
        <div class="filter-left">
          <el-input
            v-model="filterKeyword"
            placeholder="搜索名称、端口、目标或账号…"
            clearable
            style="width: 240px"
            :prefix-icon="Search"
          />

          <el-select v-model="filterProto" placeholder="协议类型" clearable style="width: 140px" @change="loadData">
            <el-option label="全部协议" value="" />
            <el-option label="四层端口转发" value="tunnel" />
            <el-option label="SOCKS5 代理" value="socks5" />
            <el-option label="HTTP 代理" value="http" />
          </el-select>

          <el-select v-model="filterServer" placeholder="所属服务器" clearable style="width: 160px" @change="loadData">
            <el-option label="全部服务器" value="" />
            <el-option v-for="srv in servers" :key="srv.id" :label="srv.name" :value="srv.id" />
          </el-select>

          <el-select v-model="filterStatus" placeholder="通道状态" clearable style="width: 130px" @change="loadData">
            <el-option label="全部状态" value="" />
            <el-option label="正常运行" value="active" />
            <el-option label="超额关停" value="quota_exceeded" />
            <el-option label="已到期" value="expired" />
            <el-option label="已禁用" value="disabled" />
          </el-select>

          <el-button :icon="Refresh" circle @click="loadData" title="刷新列表" />
        </div>

        <div class="filter-right">
          <el-button type="primary" :icon="Plus" @click="openCreate">新建独立通道</el-button>
        </div>
      </div>
    </BaseCard>

    <!-- 通道卡片列表 -->
    <div v-loading="loading" class="channel-list">
      <div v-if="filteredChannels.length === 0" class="empty-wrap">
        <el-empty description="暂无符合条件的独立通道">
          <el-button type="primary" :icon="Plus" @click="openCreate">立即新建通道</el-button>
        </el-empty>
      </div>

      <div v-else class="cards-grid">
        <BaseCard v-for="ch in filteredChannels" :key="ch.id" class="channel-card">
          <!-- 卡片顶栏 -->
          <div class="card-header">
            <div class="ch-title-wrap">
              <el-tag size="small" :type="protocolTagType(ch.protocol)" effect="dark" class="proto-tag">
                {{ protocolLabel(ch.protocol) }}
              </el-tag>
              <span class="ch-name" :title="ch.name">{{ ch.name }}</span>
            </div>
            <div class="ch-status-wrap">
              <el-tag size="small" :type="statusTagType(ch.status, ch.enabled)">
                {{ statusLabel(ch.status, ch.enabled) }}
              </el-tag>
              <el-switch
                v-model="ch.enabled"
                class="ch-switch"
                inline-prompt
                @change="handleToggle(ch)"
              />
            </div>
          </div>

          <!-- 卡片主要参数 -->
          <div class="card-body">
            <div class="info-row">
              <span class="info-label">宿主服务器</span>
              <span class="info-val host-port">
                {{ ch.server_name || '未知服务器' }}
                <span class="port-chip">:{{ ch.port }}</span>
              </span>
            </div>

            <!-- 目标或认证详情 -->
            <div v-if="ch.protocol === 'tunnel'" class="info-row">
              <span class="info-label">转发目标</span>
              <span class="info-val target-val" :title="`${ch.target_address}:${ch.target_port}`">
                ➔ {{ ch.target_address }}:{{ ch.target_port }}
                <el-tag v-if="ch.proxy_protocol" size="small" type="success" effect="plain" class="pp-badge">
                  PPv2
                </el-tag>
              </span>
            </div>

            <div v-else class="info-row">
              <span class="info-label">认证账号</span>
              <span class="info-val auth-val">
                <template v-if="ch.username">
                  <span class="user-txt">{{ ch.username }}</span>
                  <span class="pass-txt">••••••</span>
                  <el-button
                    type="primary"
                    link
                    size="small"
                    :icon="Key"
                    title="点击复制密码"
                    @click="copyText(ch.password || '', '密码已复制')"
                  />
                </template>
                <template v-else>
                  <span class="text-muted">(免密无认证)</span>
                </template>
              </span>
            </div>

            <!-- 独立计费配额进度条 -->
            <div class="quota-box">
              <div class="quota-meta">
                <span class="quota-txt">
                  已用 <strong>{{ formatBytes(ch.traffic_used_bytes) }}</strong>
                  <span class="quota-limit"> / {{ ch.traffic_limit_gb ? `${ch.traffic_limit_gb} GB` : '不限' }}</span>
                </span>
                <span class="reset-badge">{{ resetLabel(ch.traffic_reset) }}</span>
              </div>
              <el-progress
                v-if="ch.traffic_limit_gb > 0"
                :percentage="progressPercent(ch)"
                :color="progressColor(progressPercent(ch))"
                :stroke-width="6"
                :show-text="false"
              />
              <div v-else class="unlimited-bar" />
              <div class="expire-txt">
                到期时间：{{ formatDateTime(ch.expires_at, '永久有效') }}
              </div>
            </div>
          </div>

          <!-- 卡片底栏动作 -->
          <div class="card-footer">
            <el-button
              size="small"
              :icon="CopyDocument"
              @click="copyText(ch.proxy_url || `${ch.server_host}:${ch.port}`, '连接信息已复制')"
            >
              复制连接
            </el-button>
            <el-button size="small" @click="handleResetTraffic(ch)">清零用量</el-button>
            <div class="footer-right">
              <el-button size="small" :icon="Edit" circle @click="openEdit(ch)" title="编辑通道" />
              <el-button size="small" type="danger" :icon="Delete" circle @click="handleDelete(ch)" title="删除通道" />
            </div>
          </div>
        </BaseCard>
      </div>
    </div>

    <!-- 创建 / 编辑通道弹窗 -->
    <el-dialog
      v-model="dialogVisible"
      :title="isEditing ? '编辑独立通道' : '新建独立通道'"
      width="580px"
      destroy-on-close
    >
      <el-form :model="form" label-width="110px">
        <el-form-item label="所属服务器" required>
          <el-select v-model="form.server_id" :disabled="isEditing" placeholder="选择宿主服务器" style="width: 100%">
            <el-option
              v-for="srv in servers"
              :key="srv.id"
              :label="`${srv.name} (${srv.host})`"
              :value="srv.id"
            />
          </el-select>
        </el-form-item>

        <el-form-item label="通道名称" required>
          <el-input v-model="form.name" placeholder="如：海外爬虫专线、自建跳板等" maxlength="64" />
        </el-form-item>

        <el-form-item label="通道协议" required>
          <el-radio-group v-model="form.protocol" :disabled="isEditing">
            <el-radio-button value="tunnel">四层端口转发</el-radio-button>
            <el-radio-button value="socks5">SOCKS5 代理</el-radio-button>
            <el-radio-button value="http">HTTP 代理</el-radio-button>
          </el-radio-group>
        </el-form-item>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="监听端口" required>
              <el-input-number v-model="form.port" :min="1" :max="65535" controls-position="right" style="width: 100%" />
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="监听地址">
              <el-input v-model="form.listen" placeholder="留空为 0.0.0.0" />
            </el-form-item>
          </el-col>
        </el-row>

        <!-- 四层直通特有参数 -->
        <template v-if="form.protocol === 'tunnel'">
          <el-divider content-position="left">转发目标配置</el-divider>
          <el-row :gutter="16">
            <el-col :span="15">
              <el-form-item label="目标地址" required>
                <el-input v-model="form.target_address" placeholder="如：1.2.3.4 或 域名" />
              </el-form-item>
            </el-col>
            <el-col :span="9">
              <el-form-item label="目标端口" required label-width="80px">
                <el-input-number v-model="form.target_port" :min="1" :max="65535" controls-position="right" style="width: 100%" />
              </el-form-item>
            </el-col>
          </el-row>

          <el-form-item label="PROXY Proto">
            <div class="switch-with-tip">
              <el-switch v-model="form.proxy_protocol" />
              <TipIcon content="开启后，向目标服务器发送 PROXY Protocol v2 二进制头，透传访问者真实客户端公网 IP（目标端入站必须开启 acceptProxyProtocol 接收）" />
            </div>
          </el-form-item>
        </template>

        <!-- SOCKS5 / HTTP 认证配置 -->
        <template v-else>
          <el-divider content-position="left">账号认证配置</el-divider>
          <el-form-item label="认证账号">
            <el-input v-model="form.username" placeholder="留空表示免密无认证开放" maxlength="64" />
          </el-form-item>

          <el-form-item label="认证密码">
            <div class="pwd-input-wrap">
              <el-input v-model="form.password" placeholder="填写连接密码" maxlength="64" show-password />
              <el-button :icon="MagicStick" @click="generateRandomPassword">随机生成</el-button>
            </div>
          </el-form-item>

          <el-form-item v-if="form.protocol === 'socks5'" label="支持 UDP">
            <el-switch v-model="form.allow_udp" />
          </el-form-item>
        </template>

        <!-- 独立计费账户与限额体系 -->
        <el-divider content-position="left">独立计费与额度管控</el-divider>

        <el-row :gutter="16">
          <el-col :span="12">
            <el-form-item label="流量配额">
              <el-input-number v-model="form.traffic_limit_gb" :min="0" :step="10" controls-position="right" style="width: 100%" />
              <div class="field-hint">单位：GB（填 0 为不限制）</div>
            </el-form-item>
          </el-col>
          <el-col :span="12">
            <el-form-item label="重置策略">
              <el-select v-model="form.traffic_reset" style="width: 100%">
                <el-option label="从不重置" value="never" />
                <el-option label="每日重置" value="daily" />
                <el-option label="每周一重置" value="weekly" />
                <el-option label="每月 1 日重置" value="monthly" />
              </el-select>
            </el-form-item>
          </el-col>
        </el-row>

        <el-form-item label="到期时间">
          <el-date-picker
            v-model="form.expires_at"
            type="datetime"
            placeholder="留空表示永久有效"
            value-format="YYYY-MM-DDTHH:mm:ssZ"
            style="width: 100%"
          />
        </el-form-item>

        <el-form-item label="超额自动关停">
          <div class="switch-with-tip">
            <el-switch v-model="form.auto_disable" />
            <TipIcon content="当本周期流量达到设定上限或超过到期时间时，自动下发禁用指令关闭该端口，阻止超额偷跑" />
          </div>
        </el-form-item>
      </el-form>

      <template #footer>
        <el-button @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="handleSave">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped>
.channels-page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

/* 顶部概览 */
.kpi-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: 14px;
}

.kpi-card {
  padding: 16px 20px;
}

.kpi-label {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--x-text-secondary);
}

.kpi-val {
  font-size: 26px;
  font-weight: 700;
  margin: 8px 0 4px;
  color: var(--x-text-primary);
}

.kpi-val.is-active {
  color: #67c23a;
}

.kpi-sub {
  font-size: 12px;
  color: var(--x-text-placeholder);
}

/* 过滤栏 */
.filter-card {
  padding: 12px 18px;
}

.filter-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
}

.filter-left {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

/* 卡片列表 Grid */
.cards-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  gap: 16px;
}

.channel-card {
  display: flex;
  flex-direction: column;
  padding: 16px 18px;
  border-radius: 8px;
  transition: transform 0.2s, box-shadow 0.2s;
}

.channel-card:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 14px rgba(0, 0, 0, 0.08);
}

.card-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
  padding-bottom: 10px;
  border-bottom: 1px solid var(--x-border-light);
}

.ch-title-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
  max-width: 60%;
}

.proto-tag {
  font-weight: 600;
  border-radius: 4px;
}

.ch-name {
  font-size: 15px;
  font-weight: 600;
  color: var(--x-text-primary);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.ch-status-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
}

.card-body {
  display: flex;
  flex-direction: column;
  gap: 10px;
  font-size: 13px;
  flex: 1;
}

.info-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.info-label {
  color: var(--x-text-secondary);
}

.info-val {
  color: var(--x-text-primary);
  font-weight: 500;
}

.host-port {
  display: flex;
  align-items: center;
  gap: 4px;
}

.port-chip {
  background: var(--x-bg-card-hover, rgba(64, 158, 255, 0.1));
  color: var(--x-brand);
  padding: 1px 6px;
  border-radius: 4px;
  font-family: monospace;
  font-weight: 600;
}

.target-val {
  max-width: 200px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: monospace;
}

.pp-badge {
  margin-left: 4px;
  font-size: 10px;
}

.auth-val {
  display: flex;
  align-items: center;
  gap: 6px;
  font-family: monospace;
}

.user-txt {
  color: var(--x-text-primary);
}

.pass-txt {
  color: var(--x-text-placeholder);
  letter-spacing: 2px;
}

/* 配额条 */
.quota-box {
  margin-top: 6px;
  padding: 10px 12px;
  background: var(--x-bg-alt, rgba(0, 0, 0, 0.02));
  border-radius: 6px;
  border: 1px solid var(--x-border-light);
}

.quota-meta {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 6px;
  font-size: 12px;
}

.quota-txt strong {
  color: var(--x-text-primary);
}

.quota-limit {
  color: var(--x-text-secondary);
}

.reset-badge {
  font-size: 11px;
  color: var(--x-brand);
}

.unlimited-bar {
  height: 6px;
  background: #67c23a;
  border-radius: 3px;
}

.expire-txt {
  margin-top: 6px;
  font-size: 11px;
  color: var(--x-text-placeholder);
}

/* 卡片底栏 */
.card-footer {
  margin-top: 14px;
  padding-top: 10px;
  border-top: 1px solid var(--x-border-light);
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.footer-right {
  display: flex;
  align-items: center;
  gap: 8px;
}

/* 弹窗元素 */
.pwd-input-wrap {
  display: flex;
  align-items: center;
  gap: 8px;
  width: 100%;
}

.switch-with-tip {
  display: flex;
  align-items: center;
  gap: 8px;
}

.field-hint {
  font-size: 12px;
  color: var(--x-text-placeholder);
  margin-top: 4px;
}

.empty-wrap {
  padding: 40px 0;
}
</style>
