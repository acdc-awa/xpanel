<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue'
import { Check, Close, Download, Loading, Refresh, Upload, Delete, Picture, Scissor, CopyDocument } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import ImageCropperDialog from '@/components/ImageCropperDialog.vue'
import {
  getSettings,
  updateSettings,
  getBackups,
  createBackup,
  getSystemStatus,
  checkUpdate,
  applyUpdate,
  getUpdateStatus,
  type SiteGroup,
  type CaptchaGroup,
  type AgentGroup,
  type BackupItem,
  type SystemStatus,
  type UpdateCheckResult,
  type UpdateProgress,
} from '@/api/admin'
import { apiBase } from '@/config/site'
import { errMsg } from '@/api/http'
import { useSiteStore } from '@/stores/site'

const isMobile = ref(false)
let mq: MediaQueryList | null = null
const onMq = (e: MediaQueryListEvent | MediaQueryList) => {
  isMobile.value = e.matches
}
onMounted(() => {
  mq = window.matchMedia('(max-width: 768px)')
  onMq(mq)
  mq.addEventListener('change', onMq)
})
onUnmounted(() => mq?.removeEventListener('change', onMq))

const siteStore = useSiteStore()
const activeTab = ref('site')
const loading = ref(false)
const saving = ref(false)
const logoInputRef = ref<HTMLInputElement | null>(null)
const faviconInputRef = ref<HTMLInputElement | null>(null)

// 图片裁剪状态
const cropperVisible = ref(false)
const cropperImageSrc = ref('')
const cropperTitle = ref('')
const cropperTargetSize = ref(256)
const cropperType = ref<'logo' | 'favicon'>('logo')

const emptySite = (): SiteGroup => ({
  app_name: '',
  app_description: '',
  logo: '',
  favicon: '',
  subscribe_domain: '',
  subscribe_url: '',
  subscribe_path: '/sub',
  sub_deny_code: '404',
  sub_clean_ua: '1',
  sub_strict_ua: '0',
  sub_blocked_ua: '',
  tos_url: '',
  stop_register: '0',
  currency: 'CNY',
  currency_symbol: '¥',
})
const emptyCaptcha = (): CaptchaGroup => ({
  captcha_enable: '0',
  captcha_type: 'turnstile',
  turnstile_site_key: '',
  turnstile_secret_key: '',
})
const emptyAgent = (): AgentGroup => ({
  agent_report_interval: '60',
  agent_heartbeat_interval: '30',
})

const form = reactive({
  site: emptySite(),
  captcha: emptyCaptcha(),
  agent: emptyAgent(),
})

// 节点上报周期：表单存字符串（与 settings 表一致），输入控件要数字——computed 桥接
const agentReportSec = computed<number>({
  get: () => Number(form.agent.agent_report_interval) || 60,
  set: (v) => {
    form.agent.agent_report_interval = String(v ?? 60)
  },
})
const agentHeartbeatSec = computed<number>({
  get: () => Number(form.agent.agent_heartbeat_interval) || 30,
  set: (v) => {
    form.agent.agent_heartbeat_interval = String(v ?? 30)
  },
})

const subHostName = computed(() => {  if (form.site.subscribe_url) {
    try {
      const u = new URL(form.site.subscribe_url)
      return u.host || form.site.subscribe_url
    } catch {
      return form.site.subscribe_url.replace(/^https?:\/\//, '').split('/')[0] || 'sub.example.com'
    }
  }
  return 'sub.example.com'
})

const caddySnippet = computed(() => {
  return `${subHostName.value} {
    # 订阅整域反代至独立订阅端口（端口见 .env 的 APP_SUB_PORT，默认 6000）
    encode zstd gzip
    reverse_proxy localhost:6000
}`
})

const nginxSnippet = computed(() => {
  return `server {
    listen 80;
    listen 443 ssl http2;
    server_name ${subHostName.value};

    # 订阅整域反代至独立订阅端口（端口见 .env 的 APP_SUB_PORT，默认 6000）
    location / {
        proxy_pass http://127.0.0.1:6000;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header CF-Connecting-IP $http_cf_connecting_ip;
    }
}`
})

function copySnippet(text: string, label: string) {
  if (!text) return
  navigator.clipboard?.writeText(text).then(
    () => ElMessage.success(`${label}已复制到剪贴板`),
    () => ElMessage.warning('复制失败，请手动复制'),
  )
}

function onLogoFileSelected(e: Event) {
  const target = e.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    cropperImageSrc.value = reader.result as string
    cropperTitle.value = '裁剪站点 LOGO 图标'
    cropperTargetSize.value = 256
    cropperType.value = 'logo'
    cropperVisible.value = true
  }
  reader.onerror = () => {
    ElMessage.error('读取图片文件失败')
  }
  reader.readAsDataURL(file)
  target.value = ''
}

function onFaviconFileSelected(e: Event) {
  const target = e.target as HTMLInputElement
  const file = target.files?.[0]
  if (!file) return
  const reader = new FileReader()
  reader.onload = () => {
    cropperImageSrc.value = reader.result as string
    cropperTitle.value = '裁剪 Favicon 标签页图标'
    cropperTargetSize.value = 64
    cropperType.value = 'favicon'
    cropperVisible.value = true
  }
  reader.onerror = () => {
    ElMessage.error('读取图标文件失败')
  }
  reader.readAsDataURL(file)
  target.value = ''
}

function openCropperForCurrent(type: 'logo' | 'favicon') {
  const src = type === 'logo' ? form.site.logo : form.site.favicon
  if (!src) return
  cropperImageSrc.value = src
  cropperTitle.value = type === 'logo' ? '裁剪站点 LOGO 图标' : '裁剪 Favicon 标签页图标'
  cropperTargetSize.value = type === 'logo' ? 256 : 64
  cropperType.value = type
  cropperVisible.value = true
}

function onCropFinished(dataUrl: string) {
  if (cropperType.value === 'logo') {
    form.site.logo = dataUrl
    ElMessage.success('LOGO 已裁剪并生成轻量 Base64 格式')
  } else {
    form.site.favicon = dataUrl
    ElMessage.success('Favicon 已裁剪并生成轻量 Base64 格式')
  }
}

async function load() {
  loading.value = true
  try {
    const { data } = await getSettings()
    if (data.code === 0) {
      Object.assign(form.site, emptySite(), data.data.site)
      Object.assign(form.captcha, emptyCaptcha(), data.data.captcha)
      Object.assign(form.agent, emptyAgent(), data.data.agent)
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载设置失败'))
  } finally {
    loading.value = false
  }
}
onMounted(() => {
  load()
  loadBackups()
  loadSystem()
})

// ---- 备份管理（ISSUE-17） ----
const backups = ref<BackupItem[]>([])
const backupLoading = ref(false)
const backupCreating = ref(false)

async function loadBackups() {
  backupLoading.value = true
  try {
    const { data } = await getBackups()
    if (data.code === 0) backups.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载备份列表失败'))
  } finally {
    backupLoading.value = false
  }
}

async function createBackupNow() {
  backupCreating.value = true
  try {
    const { data } = await createBackup()
    if (data.code === 0) {
      ElMessage.success(`备份成功：${data.data.file}`)
      loadBackups()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '创建备份失败'))
  } finally {
    backupCreating.value = false
  }
}

function downloadBackup(file: string) {
  window.open(`${apiBase}/admin/backup/${encodeURIComponent(file)}`, '_blank')
}

function fmtSize(n: number) {
  if (!n) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

// ---- 系统状态（ISSUE-17） ----
const system = ref<SystemStatus | null>(null)
const systemLoading = ref(false)

async function loadSystem() {
  systemLoading.value = true
  try {
    const { data } = await getSystemStatus()
    if (data.code === 0) system.value = data.data
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载系统状态失败'))
  } finally {
    systemLoading.value = false
  }
}

// ---- 面板内更新（容器形态：后台执行 + phase 进度轮询，替换后容器自重启，失败自动回滚） ----
const updateInfo = ref<UpdateCheckResult | null>(null)
const updateChecking = ref(false)
const updateApplying = ref(false)

// 进度监控（对齐 agent 升级监控交互：apply 立即返回，轮询 update/status 驱动步骤条；
// 容器重启导致轮询失败属预期，进入「等待恢复」宽限，面板回来后按版本比对判定成败）
const updateModalOpen = ref(false)
const updateStatus = ref<UpdateProgress | null>(null)
const updateTargetVer = ref('')
const updateWaitingRestart = ref(false)
const updateEverRunning = ref(false)
let updateTimer: any = null
let updateDowntimeAt = 0 // 首次轮询失败时刻（0=未进入离线窗口）
let updateLastPhase = '' // 失败时步骤条错误图标落点=最后经历的有效阶段

const updateActiveStep = computed(() => {
  const p = updateStatus.value?.phase === 'failed' ? updateLastPhase : updateStatus.value?.phase
  switch (p) {
    case 'downloading':
      return 0
    case 'verifying':
      return 1
    case 'replacing':
      return 2
    case 'restarting':
    case 'success':
      return 3
    default:
      return 0
  }
})

const updateBoxClass = computed(() => {
  const phase = updateStatus.value?.phase
  if (phase === 'success') return 'is-success'
  if (phase === 'failed') return 'is-failed'
  return 'is-running'
})

function stopUpdatePolling() {
  if (updateTimer) {
    clearInterval(updateTimer)
    updateTimer = null
  }
}

onUnmounted(stopUpdatePolling)

// 挂载时恢复进行中的更新监控（覆盖用户在更新期间刷新页面的场景）
onMounted(async () => {
  try {
    const { data } = await getUpdateStatus()
    if (data.code === 0 && data.data?.progress?.running) {
      resumeUpdateWatch(data.data.progress)
    }
  } catch {
    /* 静默：无进行中更新属常态 */
  }
})

function resumeUpdateWatch(progress: UpdateProgress) {
  updateTargetVer.value = progress.target_version || updateTargetVer.value
  updateStatus.value = progress
  updateEverRunning.value = true
  updateModalOpen.value = true
  stopUpdatePolling()
  updateTimer = setInterval(pollUpdateStatus, 1500)
}

async function pollUpdateStatus() {
  let currentVersion = ''
  let progress: UpdateProgress | undefined
  try {
    const { data } = await getUpdateStatus()
    if (data.code !== 0) throw new Error(data.message)
    currentVersion = data.data.current_version
    progress = data.data.progress
    updateDowntimeAt = 0
    updateWaitingRestart.value = false
  } catch {
    // 轮询失败 = 容器重启中（status 探测不到才会失败）：宽限 5 分钟等面板恢复
    if (updateEverRunning.value) updateWaitingRestart.value = true
    if (!updateDowntimeAt) updateDowntimeAt = Date.now()
    if (Date.now() - updateDowntimeAt > 5 * 60 * 1000) {
      stopUpdatePolling()
      updateWaitingRestart.value = false
      updateStatus.value = {
        running: false,
        phase: 'failed',
        target_version: updateTargetVer.value,
        message: '等待容器恢复超时',
        error: '5 分钟内面板未恢复，请到宿主机执行 docker logs <容器> 排查（新版本启动失败会自动回滚上一版本）。',
      }
    }
    return
  }

  if (progress && progress.phase && progress.phase !== 'failed') updateLastPhase = progress.phase
  if (progress?.running) {
    updateStatus.value = progress
    if (progress.phase === 'restarting') updateWaitingRestart.value = true
    return
  }

  // 不在运行：失败 → 展示错误；否则进程已重启（内存状态清零）→ 按版本比对判定成败
  if (progress?.phase === 'failed') {
    stopUpdatePolling()
    updateStatus.value = progress
    return
  }
  if (updateTargetVer.value && currentVersion === updateTargetVer.value) {
    stopUpdatePolling()
    updateWaitingRestart.value = false
    updateStatus.value = { running: false, phase: 'success', target_version: currentVersion, message: `已更新到 ${currentVersion}，面板已恢复` }
    ElMessage.success(`面板已更新到 ${currentVersion}，即将刷新页面加载新前端`)
    setTimeout(() => window.location.reload(), 1800)
    return
  }
  if (currentVersion !== updateTargetVer.value && updateDowntimeAt) {
    // 经历过离线但版本没变 → 新版本启动失败已被 entrypoint 回滚
    stopUpdatePolling()
    updateWaitingRestart.value = false
    updateStatus.value = {
      running: false,
      phase: 'failed',
      target_version: updateTargetVer.value,
      message: `面板已恢复，但版本仍为 ${currentVersion}`,
      error: `新版本（${updateTargetVer.value}）启动失败已被自动回滚，请查看容器日志排查后重试。`,
    }
  }
  // 其余（未离线且状态为空）= 流程尚未推进到重启，继续轮询
}

const updateDisplayMessage = computed(() => {
  if (updateWaitingRestart.value) return '容器重启中，等待面板恢复（通常 10~40 秒，请勿关闭页面）...'
  return updateStatus.value?.message || '正在准备更新...'
})

async function onCheckUpdate() {
  updateChecking.value = true
  try {
    const { data } = await checkUpdate()
    if (data.code === 0) updateInfo.value = data.data
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '检查更新失败'))
  } finally {
    updateChecking.value = false
  }
}

async function confirmApply() {
  if (!updateInfo.value?.available) return
  try {
    await ElMessageBox.confirm(
      `将下载 ${updateInfo.value.latest_version} 并替换当前版本（${updateInfo.value.current_version}）。\n流程：下载 → sha256 校验 → 自检 → 替换 → 容器自动重启（restart: unless-stopped）。\n期间面板短暂不可用；新版本启动失败会自动回滚上一版本。`,
      '应用更新',
      { type: 'warning', confirmButtonText: '应用更新', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  updateApplying.value = true
  try {
    // 显式传目标版本：审计日志记录请求体，空串会导致日志只见 {"version":""}
    const { data } = await applyUpdate(updateInfo.value.latest_version || '')
    if (data.code !== 0) {
      ElMessage.error(data.message)
      return
    }
    if (data.data?.started === false && !data.data.progress?.running) {
      // 「已是最新版本」等无需进入监控弹窗的情形
      ElMessage.success(data.data.message || data.message)
      return
    }
    updateTargetVer.value = data.data?.version || updateInfo.value.latest_version || ''
    updateDowntimeAt = 0
    resumeUpdateWatch(
      data.data?.progress?.running
        ? data.data.progress
        : { running: true, phase: 'checking', target_version: updateTargetVer.value, message: '正在解析更新源...' },
    )
  } catch (e: any) {
    ElMessage.error(e?.response?.data?.message || errMsg(e, '触发更新失败'))
  } finally {
    updateApplying.value = false
  }
}

async function save() {
  saving.value = true
  try {
    const { data } = await updateSettings({
      site: { ...form.site },
      captcha: { ...form.captcha },
      agent: { ...form.agent },
    })
    if (data.code === 0) {
      ElMessage.success('设置已保存并立即全站生效')
      siteStore.applyConfig({
        ...form.site,
        stop_register: form.site.stop_register === '1',
      })
      await siteStore.fetchConfig()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <span style="font-weight: 600">站点设置</span>
        <span class="muted" style="font-size: 12px">站点品牌、人机验证与订阅入口的统一配置，保存后立即生效。</span>
      </div>
      <el-button type="primary" :loading="saving" :icon="Check" @click="save">保存全部</el-button>
    </div>

    <BaseCard v-loading="loading" style="max-width: 860px">
      <el-tabs v-model="activeTab">
        <!-- ==================== TAB 1: 站点 ==================== -->
        <el-tab-pane :label="isMobile ? '品牌' : '站点品牌'" name="site">
          <input
            ref="logoInputRef"
            type="file"
            accept="image/*,.ico"
            style="display: none"
            @change="onLogoFileSelected"
          />
          <input
            ref="faviconInputRef"
            type="file"
            accept="image/*,.ico"
            style="display: none"
            @change="onFaviconFileSelected"
          />

          <el-form label-position="top" style="max-width: 680px">
            <div class="form-grid">
              <el-form-item label="站点名称（浏览器标题 / 订阅文件名）">
                <el-input v-model="form.site.app_name" placeholder="例如：星云机场" maxlength="64" />
              </el-form-item>
              <el-form-item label="站点描述">
                <el-input v-model="form.site.app_description" placeholder="一句话描述站点" maxlength="200" />
              </el-form-item>
            </div>

            <!-- LOGO 上传与自定义 -->
            <el-form-item label="站点 LOGO（管理端侧栏 / 用户端顶栏 / 登录页品牌）">
              <div class="upload-row">
                <el-input
                  v-model="form.site.logo"
                  placeholder="可粘贴图片 URL / Base64 或点击右侧上传本地图片"
                  clearable
                />
                <el-button type="primary" plain :icon="Upload" @click="logoInputRef?.click()">
                  上传并裁剪
                </el-button>
                <el-button
                  v-if="form.site.logo"
                  plain
                  :icon="Scissor"
                  @click="openCropperForCurrent('logo')"
                >
                  重新裁剪
                </el-button>
                <el-button v-if="form.site.logo" type="danger" text :icon="Delete" @click="form.site.logo = ''">
                  恢复默认
                </el-button>
              </div>
              <div class="form-item-tip">
                支持 <code>.png</code>, <code>.jpg</code>, <code>.ico</code>, <code>.svg</code>, <code>.webp</code> 等格式，内置 1:1 智能裁剪与平滑缩放，生成轻量 Base64 格式安全存储。
              </div>
            </el-form-item>

            <!-- Favicon 上传与自定义 -->
            <el-form-item label="Favicon（浏览器标签页图标）">
              <div class="upload-row">
                <el-input
                  v-model="form.site.favicon"
                  placeholder="可粘贴 Favicon URL / Base64 或点击右侧上传本地图标"
                  clearable
                />
                <el-button plain :icon="Upload" @click="faviconInputRef?.click()">
                  上传并裁剪
                </el-button>
                <el-button
                  v-if="form.site.favicon"
                  plain
                  :icon="Scissor"
                  @click="openCropperForCurrent('favicon')"
                >
                  重新裁剪
                </el-button>
                <el-button v-if="form.site.favicon" type="danger" text :icon="Delete" @click="form.site.favicon = ''">
                  清空
                </el-button>
              </div>
            </el-form-item>

            <!-- 品牌全景实时预览卡片 -->
            <div class="brand-preview-section">
              <div class="preview-header">
                <el-icon><Picture /></el-icon>&nbsp;全场景实时效果预览（当前编辑状态）
              </div>
              <div class="preview-grid">
                <!-- 1. 管理端侧栏效果 -->
                <div class="preview-item dark-theme">
                  <div class="preview-tag">管理端侧栏</div>
                  <div class="preview-logo-box">
                    <img v-if="form.site.logo" :src="form.site.logo" class="p-logo-img" alt="logo" />
                    <span v-else class="p-logo-mark">X</span>
                    <span class="p-title">{{ form.site.app_name || 'Xray 管理面板' }}</span>
                  </div>
                </div>

                <!-- 2. 用户端顶栏效果 -->
                <div class="preview-item light-theme">
                  <div class="preview-tag">用户端顶栏</div>
                  <div class="preview-logo-box">
                    <img v-if="form.site.logo" :src="form.site.logo" class="p-logo-img" alt="logo" />
                    <span v-else class="p-logo-mark">X</span>
                    <span class="p-title">{{ form.site.app_name || 'XrayPanel' }}</span>
                  </div>
                </div>

                <!-- 3. 登录页品牌效果 -->
                <div class="preview-item auth-theme">
                  <div class="preview-tag">登录认证页</div>
                  <div class="preview-auth-box">
                    <img v-if="form.site.logo" :src="form.site.logo" class="p-auth-img" alt="logo" />
                    <span v-else class="p-auth-logo">X</span>
                    <div>
                      <div class="p-auth-title">{{ form.site.app_name || 'Xray 面板' }}</div>
                      <div class="p-auth-sub">{{ form.site.app_description || '主控 · 节点 · 用户 一体化代理分发系统' }}</div>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <div class="form-grid" style="margin-top: 14px">
              <el-form-item label="服务条款 URL">
                <el-input v-model="form.site.tos_url" placeholder="https://example.com/tos" />
              </el-form-item>
              <el-form-item label="货币代码">
                <el-input v-model="form.site.currency" placeholder="CNY" maxlength="8" />
              </el-form-item>
              <el-form-item label="货币符号">
                <el-input v-model="form.site.currency_symbol" placeholder="¥" maxlength="8" />
              </el-form-item>
            </div>

            <el-form-item label="关闭注册">
              <el-switch
                v-model="form.site.stop_register"
                active-value="1"
                inactive-value="0"
                active-text="已关闭（新用户无法注册）"
                inactive-text="开放注册（需邀请码）"
              />
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <!-- ==================== TAB 2: 订阅与清洗网关 ==================== -->
        <el-tab-pane :label="isMobile ? '网关' : '订阅与清洗网关'" name="subscribe">
          <el-form label-position="top" style="max-width: 820px">
            <el-alert
              title="物理端口隔离与反代解耦"
              type="info"
              :closable="false"
              show-icon
              style="margin-bottom: 20px"
            >
              <template #default>
                <div style="font-size: 13px; line-height: 1.6">
                  将<b>用户拉取订阅服务</b>与<b>面板 Web 界面 / 管理 API / 节点长连接</b>物理分离。
                  独立端口仅运行纯净、轻量的订阅生成与清洗引擎，<b>绝不暴露任何后台管理接口或节点网关</b>，彻底杜绝订阅域名被扫描导致的敏感信息泄露！
                </div>
              </template>
            </el-alert>

            <div class="section-subtitle">订阅入口与错误码</div>
            <div class="form-grid">
              <el-form-item label="对外订阅根地址 (subscribe_url)">
                <el-input v-model="form.site.subscribe_url" placeholder="如 https://sub.example.com" clearable />
                <span class="muted" style="font-size: 12px; margin-top: 4px; display: block">
                  用户端获取的订阅链接根域名。订阅域名整域反代到独立订阅端口（APP_SUB_PORT），
                  不设置则默认使用当前面板域名 + 订阅路径。
                </span>
              </el-form-item>

              <el-form-item label="订阅入口路径 (subscribe_path)">
                <el-input v-model="form.site.subscribe_path" placeholder="如 /sub 或 /ehisnodn（默认 /sub）" clearable />
                <span class="muted" style="font-size: 12px; margin-top: 4px; display: block">
                  订阅端口<b>唯一入口</b>：只认该路径（支持 /path/:token 与 ?token=xxx），
                  其余路径一律按下方拒绝码返回。保存后订阅服务自动重载。
                </span>
              </el-form-item>

              <el-form-item label="非订阅路径拒绝码 (sub_deny_code)">
                <el-select v-model="form.site.sub_deny_code" style="width: 100%">
                  <el-option label="404 Not Found（推荐，防探测）" value="404" />
                  <el-option label="401 Unauthorized（客户端感知鉴权失败）" value="401" />
                </el-select>
                <span class="muted" style="font-size: 12px; margin-top: 4px; display: block">
                  订阅端口上非订阅路径的请求与<b>无效订阅 token</b> 统一返回该错误码。
                </span>
              </el-form-item>

              <el-form-item label="多域名分发（预留）">
                <el-input v-model="form.site.subscribe_domain" placeholder="sub1.com,sub2.com（逗号分隔）" clearable />
                <span class="muted" style="font-size: 12px; margin-top: 4px; display: block">
                  用于轮询或备份的多订阅分发域名。
                </span>
              </el-form-item>
            </div>

            <div class="section-subtitle" style="margin-top: 24px">原生订阅清洗防探测网关 (Subscribe Sieve)</div>
            <div class="form-grid">
              <el-form-item label="智能 UA 过滤与爬虫拦截">
                <el-switch
                  v-model="form.site.sub_clean_ua"
                  active-value="1"
                  inactive-value="0"
                  active-text="已开启（阻断 curl/python/空UA/扫描器）"
                  inactive-text="已关闭（放行所有请求）"
                />
              </el-form-item>

              <el-form-item label="严格客户端白名单模式">
                <el-switch
                  v-model="form.site.sub_strict_ua"
                  active-value="1"
                  inactive-value="0"
                  active-text="已开启（仅放行知名代理客户端）"
                  inactive-text="已关闭（宽松模式）"
                />
              </el-form-item>
            </div>

            <el-form-item label="自定义封禁 UA 关键词 (sub_blocked_ua)">
              <el-input
                v-model="form.site.sub_blocked_ua"
                placeholder="如 scan,exploit,badbot（英文逗号分隔，命中即拦截）"
                clearable
              />
            </el-form-item>

            <!-- 反代配置代码片段展示 -->
            <div class="section-subtitle" style="margin-top: 24px">生产反向代理配置参考 (含 Cloudflare 真实 IP 透传)</div>
            <div class="proxy-snippet-box">
              <div class="snippet-header">
                <span>Caddyfile 配置示例（推荐）</span>
                <el-button size="small" :icon="CopyDocument" @click="copySnippet(caddySnippet, 'Caddyfile 配置')">
                  复制 Caddy 配置
                </el-button>
              </div>
              <pre class="snippet-code"><code>{{ caddySnippet }}</code></pre>
            </div>

            <div class="proxy-snippet-box" style="margin-top: 14px">
              <div class="snippet-header">
                <span>Nginx 配置示例</span>
                <el-button size="small" :icon="CopyDocument" @click="copySnippet(nginxSnippet, 'Nginx 配置')">
                  复制 Nginx 配置
                </el-button>
              </div>
              <pre class="snippet-code"><code>{{ nginxSnippet }}</code></pre>
            </div>
          </el-form>
        </el-tab-pane>

        <!-- ==================== TAB 3: 安全（人机验证） ==================== -->
        <el-tab-pane :label="isMobile ? '安全' : '安全设置'" name="captcha">
          <el-form label-position="top" style="max-width: 640px">
            <el-form-item label="Cloudflare Turnstile 人机验证（登录 / 注册）">
              <el-switch
                v-model="form.captcha.captcha_enable"
                active-value="1"
                inactive-value="0"
                active-text="已开启（登录与注册需通过验证）"
                inactive-text="已关闭（内网 / 开发环境可关）"
              />
            </el-form-item>
            <el-form-item label="验证类型">
              <el-select v-model="form.captcha.captcha_type" style="width: 100%">
                <el-option label="Turnstile（Cloudflare）" value="turnstile" />
              </el-select>
            </el-form-item>
            <el-form-item label="Turnstile Site Key（前端公开）">
              <el-input v-model="form.captcha.turnstile_site_key" placeholder="0x4A...（Cloudflare 控制台获取）" />
            </el-form-item>
            <el-form-item label="Turnstile Secret Key（后端校验，仅管理端可见）">
              <el-input v-model="form.captcha.turnstile_secret_key" type="password" show-password placeholder="0x4B...（请勿泄露）" />
            </el-form-item>
            <p class="muted tip">
              在 Cloudflare 控制台创建站点并添加 Turnstile 小部件后，将 Site Key / Secret Key 填入此处并开启开关即可。
              未配置密钥时即使开启也会拒绝所有请求（fail-closed）。
            </p>
          </el-form>
        </el-tab-pane>

        <!-- ==================== TAB 3.5: 节点上报 ==================== -->
        <el-tab-pane :label="isMobile ? '上报' : '节点上报'" name="agent">
          <el-form label-position="top" style="max-width: 640px">
            <el-form-item label="流量上报周期（秒）">
              <el-input-number v-model="agentReportSec" :min="5" :max="1800" :step="10" style="width: 220px" />
              <span class="muted tip" style="margin-left: 12px">节点每隔多久把 xray 流量增量上报给面板</span>
            </el-form-item>
            <el-form-item label="状态心跳周期（秒）">
              <el-input-number v-model="agentHeartbeatSec" :min="5" :max="1800" :step="5" style="width: 220px" />
              <span class="muted tip" style="margin-left: 12px">CPU / 内存 / 磁盘 / 在线用户等状态的上报频率</span>
            </el-form-item>
            <p class="muted tip">
              保存后立即下发到所有在线节点（离线节点重连后自动生效）；agent.yaml 的本地配置作为兜底。
              缩短流量上报周期可加快超额 / 到期用户的踢除时效（配合流量落库后的即时处置，最坏延迟 ≈ 一个上报周期）；
              周期过小会增大节点与面板的负载，建议 15–120 秒。
            </p>
          </el-form>
        </el-tab-pane>

        <!-- ==================== TAB 4: 备份 ==================== -->
        <el-tab-pane :label="isMobile ? '备份' : '数据备份'" name="backup">
          <div style="max-width: 720px">
            <div class="x-toolbar" style="margin-bottom: 12px">
              <div class="x-toolbar-left">
                <el-button :loading="backupLoading" :icon="Refresh" @click="loadBackups">刷新列表</el-button>
              </div>
              <el-button type="primary" :loading="backupCreating" @click="createBackupNow">立即备份</el-button>
            </div>
            <el-table v-loading="backupLoading" :data="backups" size="small">
              <el-table-column prop="file" label="备份文件" min-width="220">
                <template #default="{ row }"><code class="cell-mono">{{ row.file }}</code></template>
              </el-table-column>
              <el-table-column label="大小" width="110">
                <template #default="{ row }">{{ fmtSize(row.size) }}</template>
              </el-table-column>
              <el-table-column label="创建时间" width="170">
                <template #default="{ row }">{{ String(row.created_at).replace('T', ' ').slice(0, 19) }}</template>
              </el-table-column>
              <el-table-column label="操作" width="100" fixed="right">
                <template #default="{ row }">
                  <el-button link type="primary" :icon="Download" @click="downloadBackup(row.file)">下载</el-button>
                </template>
              </el-table-column>
            </el-table>
            <p class="muted tip">备份文件保存在主控备份目录，按配置定期轮转；建议每月下载一份离线保存。</p>
          </div>
        </el-tab-pane>

        <!-- ==================== TAB 6: 系统状态 ==================== -->
        <el-tab-pane :label="isMobile ? '状态' : '系统状态'" name="system">
          <div v-loading="systemLoading" style="max-width: 720px; min-height: 200px">
            <template v-if="system">
              <el-descriptions :column="2" border size="small">
                <el-descriptions-item label="应用">{{ system.app_name }}（{{ system.app_env }}）</el-descriptions-item>
                <el-descriptions-item label="面板版本"><code class="cell-mono">{{ system.panel_version }}</code></el-descriptions-item>
                <el-descriptions-item label="Go 版本">{{ system.go_version }}</el-descriptions-item>
                <el-descriptions-item label="运行时长">{{ Math.floor(system.uptime_seconds / 3600) }}h {{ Math.floor((system.uptime_seconds % 3600) / 60) }}m</el-descriptions-item>
                <el-descriptions-item label="Goroutines">{{ system.goroutines }}</el-descriptions-item>
                <el-descriptions-item label="数据库">
                  <el-tag :type="system.db_ok ? 'success' : 'danger'" size="small">{{ system.db_ok ? '正常' : '异常' }}</el-tag>
                  <span style="margin-left: 6px">{{ system.db_driver }} · {{ system.db_latency_ms }}ms</span>
                </el-descriptions-item>
                <el-descriptions-item label="备份调度">{{ system.backup_enabled ? '已启用' : '未启用' }}</el-descriptions-item>
                <el-descriptions-item label="内存占用">{{ system.mem_alloc_mb }} MB</el-descriptions-item>
                <el-descriptions-item label="服务时间">{{ system.server_time }}</el-descriptions-item>
              </el-descriptions>
              <div class="status-count-grid">
                <div v-for="(v, k) in system.counts" :key="k" class="status-count">
                  <strong>{{ v }}</strong>
                  <span>{{ { users: '用户', servers: '服务器', inbounds: '入站', orders: '订单', gift_cards: '礼品卡', audit_logs: '审计日志' }[k] ?? k }}</span>
                </div>
              </div>
              <p v-if="!system.db_ok" class="muted tip">数据库异常：{{ system.db_error }}</p>

              <!-- 面板内更新（容器形态自更新） -->
              <div class="update-card">
                <p class="ip-hdr-title">面板更新</p>
                <div class="update-row">
                  <span class="muted">当前</span>
                  <code class="cell-mono">{{ system.panel_version }}</code>
                  <span class="muted">最新</span>
                  <code class="cell-mono">{{ updateInfo?.latest_version || '—' }}</code>
                  <el-tag v-if="updateInfo?.available" type="success" size="small" style="margin-left: 8px">有可用更新</el-tag>
                  <el-tag v-else-if="updateInfo && !updateInfo.enabled" type="info" size="small" style="margin-left: 8px">更新已禁用</el-tag>
                </div>
                <div class="x-toolbar" style="margin-top: 10px">
                  <el-button :icon="Refresh" :loading="updateChecking" @click="onCheckUpdate">检查更新</el-button>
                  <el-button
                    type="primary"
                    :icon="Download"
                    :disabled="!updateInfo?.available || !!updateStatus?.running"
                    :loading="updateApplying"
                    @click="confirmApply"
                  >
                    应用更新
                  </el-button>
                </div>
                <p class="muted tip">
                  应用更新将下载最新 release 包并强制校验 sha256，替换后进程主动退出，由容器
                  <code>restart: unless-stopped</code> 自动拉起新版本；新版本启动失败时自动回滚上一版本。应用更新前请先手动备份。
                </p>
              </div>

              <!-- 客户端 IP 来源说明（与 util.GetRealIP 语义对应） -->
              <div class="ip-hdr-note">
                <p class="ip-hdr-title">客户端 IP 识别</p>
                <p class="muted tip">
                  面板按 <code>CF-Connecting-IP</code> → <code>X-Real-IP</code> → <code>X-Forwarded-For</code> →
                  <code>RemoteAddr</code> 的优先级识别真实客户端 IP，用于登录/订阅限流、审计日志与人机验证。
                  请保持「面板端口仅绑定 127.0.0.1 + 由反向代理（Caddy/Nginx）前置」的部署形态：
                  反代会覆盖/追加可信的 IP 头，限流与审计才能按真实 IP 生效。
                  切勿将面板端口直接暴露公网（否则攻击者可伪造 IP 头绕过按 IP 的限流）。
                </p>
              </div>

              <div class="x-toolbar" style="margin-top: 12px">
                <el-button :icon="Refresh" @click="loadSystem">刷新状态</el-button>
              </div>
            </template>
            <el-empty v-else-if="!systemLoading" description="暂无状态数据" />
          </div>
        </el-tab-pane>
      </el-tabs>
    </BaseCard>

    <!-- 图片裁剪弹窗 -->
    <ImageCropperDialog
      v-model="cropperVisible"
      :image-src="cropperImageSrc"
      :title="cropperTitle"
      :target-size="cropperTargetSize"
      @crop="onCropFinished"
    />

    <!-- 面板更新进度弹窗（交互对齐节点 Agent 升级监控：phase 驱动步骤条 + 轮询状态） -->
    <el-dialog v-model="updateModalOpen" title="面板更新进度" width="560px" :close-on-click-modal="false">
      <el-steps
        :active="updateActiveStep"
        finish-status="success"
        :process-status="updateStatus?.phase === 'failed' ? 'error' : 'process'"
        align-center
        style="margin: 20px 0 8px"
      >
        <el-step title="下载更新包" description="GitHub Release" />
        <el-step title="校验与自检" description="sha256 / self-test" />
        <el-step title="替换文件" description="备份并原子替换" />
        <el-step title="重启容器" description="unless-stopped 拉起" />
      </el-steps>

      <div class="update-status-box" :class="updateBoxClass">
        <div class="status-icon">
          <el-icon v-if="updateStatus?.phase === 'success'" class="icon-success"><Check /></el-icon>
          <el-icon v-else-if="updateStatus?.phase === 'failed'" class="icon-error"><Close /></el-icon>
          <el-icon v-else class="is-loading icon-process"><Loading /></el-icon>
        </div>
        <div class="status-texts">
          <div class="status-msg">{{ updateDisplayMessage }}</div>
          <div v-if="updateStatus?.error" class="status-err cell-mono">{{ updateStatus.error }}</div>
          <div v-else-if="updateStatus?.phase !== 'success' && updateStatus?.phase !== 'failed'" class="status-hint">
            更新在服务端后台执行，刷新页面后重新打开本页可继续查看进度。
          </div>
        </div>
      </div>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 16px;
  @media (max-width: 640px) {
    grid-template-columns: 1fr;
  }
}
.tip { font-size: 12.5px; margin: 4px 0; line-height: 1.7; color: var(--x-text-3); }
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-text-2); }
.form-item-tip { font-size: 11.5px; color: var(--x-text-3); margin-top: 4px; line-height: 1.5; }
.ip-hdr-note {
  margin-top: 14px;
  padding: 10px 12px;
  border: 1px dashed var(--x-border);
  border-radius: 8px;
  background: var(--x-bg);
  code {
    font-family: ui-monospace, Menlo, Consolas, monospace;
    font-size: 11.5px;
    color: var(--x-primary);
  }
}
.ip-hdr-title { font-size: 12.5px; font-weight: 600; color: var(--x-text-2); margin-bottom: 4px; }
.update-card {
  margin-top: 14px;
  padding: 10px 12px;
  border: 1px dashed var(--x-border);
  border-radius: 8px;
  background: var(--x-bg);
}
.update-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-size: 13px;
}

.upload-row {
  display: flex;
  gap: 8px;
  align-items: center;
  width: 100%;
  flex-wrap: wrap;

  .el-input {
    flex: 1 1 200px;
    min-width: 180px;
  }
}

.brand-preview-section {
  margin-top: 8px;
  padding: 14px 16px;
  background: var(--x-bg, #f8fafc);
  border: 1px dashed var(--x-border, #e2e8f0);
  border-radius: var(--x-radius, 12px);
}

.preview-header {
  font-size: 13px;
  font-weight: 600;
  color: var(--x-text-2, #475569);
  margin-bottom: 12px;
  display: flex;
  align-items: center;
}

.preview-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
  gap: 12px;
}

.preview-item {
  border-radius: 10px;
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  border: 1px solid var(--x-border, #e2e8f0);
  position: relative;

  .preview-tag {
    font-size: 10px;
    font-weight: 600;
    padding: 2px 6px;
    border-radius: 4px;
    align-self: flex-start;
    opacity: 0.8;
  }

  &.dark-theme {
    background: #111422;
    color: #ffffff;
    border-color: #1e2338;
    .preview-tag { background: rgba(255, 255, 255, 0.1); color: #c7d2fe; }
    .p-title { color: #ffffff; font-weight: 700; font-size: 14px; }
  }

  &.light-theme {
    background: #ffffff;
    color: #0f172a;
    box-shadow: 0 1px 3px rgba(0, 0, 0, 0.05);
    .preview-tag { background: #f1f5f9; color: #475569; }
    .p-title { color: #0f172a; font-weight: 800; font-size: 14px; }
  }

  &.auth-theme {
    background: var(--x-card);
    border-color: var(--x-border);
    box-shadow: var(--x-shadow-md);
    .preview-tag { background: var(--x-primary-soft); color: var(--x-primary); }
  }
}

.preview-logo-box {
  display: flex;
  align-items: center;
  gap: 8px;
}

.p-logo-img {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  object-fit: contain;
  background: rgba(255, 255, 255, 0.08);
  flex: none;
}

.p-logo-mark {
  width: 28px;
  height: 28px;
  border-radius: 6px;
  background: linear-gradient(135deg, #6366f1, #a855f7);
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 13px;
  font-weight: 800;
  flex: none;
}

.preview-auth-box {
  display: flex;
  align-items: center;
  gap: 10px;
}

.p-auth-img {
  width: 38px;
  height: 38px;
  border-radius: 10px;
  object-fit: contain;
  background: var(--x-card-soft);
  border: 1px solid var(--x-border);
  flex: none;
}

.p-auth-logo {
  width: 38px;
  height: 38px;
  border-radius: 10px;
  background: linear-gradient(135deg, #6366f1, #a855f7);
  color: #fff;
  font-size: 18px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex: none;
  box-shadow: 0 4px 12px rgba(99, 102, 241, 0.3);
}

.p-auth-title {
  font-size: 15px;
  font-weight: 700;
  color: var(--x-text);
}

.p-auth-sub {
  font-size: 11px;
  color: var(--x-text-2);
  margin-top: 1px;
}

.status-count-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
  margin-top: 14px;
}
.status-count {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 12px;
  border: 1px solid var(--x-border);
  border-radius: 10px;
  background: var(--x-card-soft);
}
.status-count strong { font-size: 20px; font-variant-numeric: tabular-nums; color: var(--x-text); }
.status-count span { font-size: 12px; color: var(--x-text-3); }

.section-subtitle {
  font-size: 14px;
  font-weight: 700;
  color: var(--x-text);
  margin-bottom: 12px;
  display: flex;
  align-items: center;
  gap: 6px;
}

.proxy-snippet-box {
  background: #0f172a;
  border-radius: 10px;
  border: 1px solid #1e293b;
  overflow: hidden;

  .snippet-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 8px 14px;
    background: #1e293b;
    color: #94a3b8;
    font-size: 12px;
    font-weight: 600;
  }

  .snippet-code {
    margin: 0;
    padding: 12px 14px;
    font-family: var(--x-font-mono, 'JetBrains Mono', Consolas, monospace);
    font-size: 12px;
    line-height: 1.5;
    color: #38bdf8;
    overflow-x: auto;
    white-space: pre;
  }
}

@media (max-width: 768px) {
  :deep(.el-tabs__header) {
    margin-bottom: 14px;
  }
  :deep(.el-tabs__nav-wrap) {
    padding: 0 !important;
  }
  :deep(.el-tabs__nav-prev),
  :deep(.el-tabs__nav-next) {
    display: none !important;
  }
  :deep(.el-tabs__nav-scroll) {
    overflow-x: auto;
    scrollbar-width: none;
    &::-webkit-scrollbar {
      display: none;
    }
  }
  :deep(.el-tabs__item) {
    padding: 0 10px !important;
    font-size: 13px !important;
  }
}

/* 面板更新进度状态盒（样式与节点 Agent 升级监控一致） */
.update-status-box {
  display: flex;
  align-items: flex-start;
  gap: 14px;
  padding: 14px 16px;
  border-radius: 8px;
  background: var(--x-bg, #f9fafb);
  border: 1px solid var(--x-border, #e5e7eb);
  margin-top: 12px;

  &.is-running {
    border-color: #93c5fd;
    background: #eff6ff;
    .icon-process {
      font-size: 22px;
      color: #2563eb;
    }
  }
  &.is-success {
    border-color: #86efac;
    background: #f0fdf4;
    .icon-success {
      font-size: 22px;
      color: #16a34a;
      font-weight: bold;
    }
  }
  &.is-failed {
    border-color: #fca5a5;
    background: #fef2f2;
    .icon-error {
      font-size: 22px;
      color: #dc2626;
      font-weight: bold;
    }
  }

  .status-icon {
    line-height: 1.2;
  }
  .status-texts {
    flex: 1;
    .status-msg {
      font-weight: 600;
      font-size: 13.5px;
      color: var(--x-text);
      line-height: 1.5;
    }
    .status-err {
      margin-top: 6px;
      color: #b91c1c;
      font-size: 11.5px;
      line-height: 1.4;
      word-break: break-all;
      background: rgba(254, 226, 226, 0.7);
      padding: 6px 8px;
      border-radius: 4px;
    }
    .status-hint {
      margin-top: 4px;
      color: var(--x-text-3);
      font-size: 11.5px;
    }
  }
}
</style>
