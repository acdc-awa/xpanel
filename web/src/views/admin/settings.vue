<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref, watch } from 'vue'
import TipIcon from '@/components/base/TipIcon.vue'
import ImageCropperDialog from '@/components/ImageCropperDialog.vue'
import {
  CsAlert,
  CsBadge,
  CsBox,
  CsButton,
  CsCard,
  CsCardGrid,
  CsConfirmDialog,
  CsContainer,
  CsEmpty,
  CsForm,
  CsFormItem,
  CsIcon,
  CsInput,
  CsInputNumber,
  CsKeyValuePairs,
  CsLoading,
  CsModal,
  CsOption,
  CsPage,
  CsSelect,
  CsSpinner,
  CsStatusIndicator,
  CsSteps,
  CsTable,
  CsTabs,
  CsToggle,
  CsToolbar,
  CsTooltip,
  ElMessage,
} from '@/components/cloudscape'
import type { KeyValuePair, StepItem, StepsStatus, TabItem, TableColumn } from '@/components/cloudscape'
import {
  getSettings,
  updateSettings,
  getBackups,
  createBackup,
  deleteBackup,
  uploadBackup,
  restoreBackup,
  getSystemStatus,
  checkUpdate,
  applyUpdate,
  listReleases,
  getUpdateStatus,
  type SiteGroup,
  type CaptchaGroup,
  type AgentGroup,
  type TimezoneGroup,
  type BackupItem,
  type SystemStatus,
  type UpdateCheckResult,
  type UpdateProgress,
  type ReleaseItem,
} from '@/api/admin'
import { apiBase } from '@/config/site'
import { errMsg } from '@/api/http'
import { setDisplayTimezone, effectiveTimezoneLabel, formatDateTime } from '@/utils/timezone'
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
const isConfigTab = computed(() => ['site', 'subscribe', 'captcha', 'agent', 'timezone'].includes(activeTab.value))
const loading = ref(false)
const saving = ref(false)
const logoInputRef = ref<HTMLInputElement | null>(null)
const faviconInputRef = ref<HTMLInputElement | null>(null)
const backupInputRef = ref<HTMLInputElement | null>(null)

const settingsTabs: TabItem[] = [
  { id: 'site', label: '站点品牌' },
  { id: 'subscribe', label: '订阅与清洗网关' },
  { id: 'captcha', label: '安全设置' },
  { id: 'agent', label: '服务器上报' },
  { id: 'timezone', label: '时间与时区' },
  { id: 'backup', label: '数据备份' },
  { id: 'system', label: '系统状态' },
]

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
  sub_profile_title: '',
  sub_update_interval: '24',
  tos_url: '',
  stop_register: '0',
  currency: 'CNY',
  currency_symbol: '¥',
})
const emptyCaptcha = (): CaptchaGroup => ({
  captcha_enable: '0',
  captcha_type: 'turnstile',
  turnstile_site_key: '',
  // secret 只写不读：表单里只存本次新输入的明文，后端从不回显
  turnstile_secret_key: '',
  turnstile_secret_key_configured: '0',
  turnstile_secret_key_clear: '',
})
const emptyAgent = (): AgentGroup => ({
  agent_report_interval: '60',
  agent_heartbeat_interval: '5',
})
const emptyTimezone = (): TimezoneGroup => ({
  business_timezone: 'Asia/Shanghai',
  display_timezone: 'browser',
})

const form = reactive({
  site: emptySite(),
  captcha: emptyCaptcha(),
  agent: emptyAgent(),
  timezone: emptyTimezone(),
})

// 图片类字段：上传后存的是几十 KB 的 data URI，直接绑到输入框会把输入框变成一条乱码。
// 输入框只承担「粘贴图片 URL」这一种编辑；内联 Base64 用缩略图 + 提示表达，
// 要换图走右侧按钮，要清空走「恢复默认 / 清空」。
function imageField(key: 'logo' | 'favicon') {
  const isInline = () => form.site[key].startsWith('data:')
  const url = computed({
    get: () => (isInline() ? '' : form.site[key]),
    set: (v: string) => {
      form.site[key] = v.trim()
    },
  })
  const placeholder = computed(() =>
    isInline()
      ? key === 'logo'
        ? '已上传本地图片，粘贴 URL 可替换'
        : '已上传本地图标，粘贴 URL 可替换'
      : key === 'logo'
        ? '可粘贴图片 URL 或点击右侧上传本地图片'
        : '可粘贴 Favicon URL 或点击右侧上传本地图标'
  )
  return { inline: computed(isInline), url, placeholder }
}

const { url: logoUrl, placeholder: logoPlaceholder } = imageField('logo')
const { url: faviconUrl, placeholder: faviconPlaceholder } = imageField('favicon')

// secret 只写不读：GET 只回显「是否已配置」，保存时留空 = 保持原值，清除需显式指令
const secretConfigured = computed(() => form.captcha.turnstile_secret_key_configured === '1')
const secretClearing = computed(() => form.captcha.turnstile_secret_key_clear === '1')
function markSecretClear() {
  form.captcha.turnstile_secret_key = ''
  form.captcha.turnstile_secret_key_clear = '1'
}
function undoSecretClear() {
  form.captcha.turnstile_secret_key_clear = ''
}
// 只提交需要变更的 secret 字段：未输入新值时不发，避免把已配置的密钥覆盖成空
function captchaPayload(): Record<string, string> {
  const payload: Record<string, string> = {
    captcha_enable: form.captcha.captcha_enable,
    captcha_type: form.captcha.captcha_type,
    turnstile_site_key: form.captcha.turnstile_site_key,
  }
  if (form.captcha.turnstile_secret_key_clear === '1') {
    payload.turnstile_secret_key_clear = '1'
  } else if (form.captcha.turnstile_secret_key) {
    payload.turnstile_secret_key = form.captcha.turnstile_secret_key
  }
  return payload
}

// 时区候选：优先浏览器支持的完整 IANA 列表，缺失时回退常用清单（select 支持过滤/自定义输入）。
const timezoneOptions = computed<string[]>(() => {
  try {
    const all = (Intl as any).supportedValuesOf?.('timeZone')
    if (Array.isArray(all) && all.length) return all as string[]
  } catch {
    /* 旧浏览器忽略 */
  }
  return [
    'Asia/Shanghai', 'Asia/Hong_Kong', 'Asia/Taipei', 'Asia/Tokyo', 'Asia/Seoul',
    'Asia/Singapore', 'Asia/Bangkok', 'Asia/Dubai', 'Asia/Kolkata',
    'Europe/London', 'Europe/Paris', 'Europe/Moscow',
    'America/New_York', 'America/Chicago', 'America/Los_Angeles', 'UTC',
  ]
})

// 节点上报周期：表单存字符串（与 settings 表一致），输入控件要数字——computed 桥接
const agentReportSec = computed<number>({
  get: () => Number(form.agent.agent_report_interval) || 60,
  set: (v) => {
    form.agent.agent_report_interval = String(v ?? 60)
  },
})
// 订阅更新间隔（小时）：同上桥接，缺省 24
const subUpdateHours = computed<number>({
  get: () => Number(form.site.sub_update_interval) || 24,
  set: (v) => {
    form.site.sub_update_interval = String(v ?? 24)
  },
})
const agentHeartbeatSec = computed<number>({
  get: () => Number(form.agent.agent_heartbeat_interval) || 5,
  set: (v) => {
    form.agent.agent_heartbeat_interval = String(v ?? 5)
  },
})

/**
 * 设置表把开关存成 '1' / '0' 字符串，而 CsToggle 只认 boolean，
 * 这里逐个桥接，写入时仍落回字符串，保持接口与库内口径不变。
 */
function boolSetting(get: () => string, set: (v: string) => void) {
  return computed<boolean>({
    get: () => get() === '1',
    set: (v) => set(v ? '1' : '0'),
  })
}
// 注册准入：库内是 stop_register（1=停止注册），界面按「开放注册」正向表达，避免双重否定
const openRegister = computed<boolean>({
  get: () => form.site.stop_register !== '1',
  set: (v) => (form.site.stop_register = v ? '0' : '1'),
})
const subCleanUa = boolSetting(
  () => form.site.sub_clean_ua,
  (v) => (form.site.sub_clean_ua = v),
)
const subStrictUa = boolSetting(
  () => form.site.sub_strict_ua,
  (v) => (form.site.sub_strict_ua = v),
)
const captchaEnable = boolSetting(
  () => form.captcha.captcha_enable,
  (v) => (form.captcha.captcha_enable = v),
)

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
      if (data.data.timezone) {
        Object.assign(form.timezone, emptyTimezone(), data.data.timezone)
        // 载入即应用展示时区，使本页与其它管理页时间渲染口径一致。
        setDisplayTimezone(form.timezone.display_timezone)
      }
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
const backupTotalSize = computed(() => backups.value.reduce((sum, b) => sum + (b.size || 0), 0))

async function loadBackups() {
  backupLoading.value = true
  try {
    const { data } = await getBackups()
    // 后端空目录返回 []（已修）；这里再兜一层，避免任何 null 把整页渲染打断
    if (data.code === 0) backups.value = data.data.items || []
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

async function removeBackup(row: any) {
  const file = String(row.file)
  askConfirm({
    header: '删除这个备份？',
    warning: '此操作不可恢复。',
    message: '删除只移除该备份文件，不影响当前数据库与自动轮转策略。',
    confirmText: '删除',
    action: async () => {
      try {
        const { data } = await deleteBackup(file)
        if (data.code === 0) {
          ElMessage.success('备份已删除')
          loadBackups()
        } else {
          ElMessage.error(data.message)
        }
      } catch (e) {
        ElMessage.error(errMsg(e, '删除备份失败'))
      }
    },
  })
}

// ---- 备份上传与恢复 ----
const backupUploading = ref(false)
const backupRestoring = ref(false)

/**
 * 破坏性操作确认走 CsConfirmDialog（AWS 控制台口径：红色「Warning:」+ 红色按钮）。
 * pending 里存本次要执行的动作，确认后调用。
 */
const confirmState = reactive({
  visible: false,
  header: '',
  warning: '',
  message: '',
  confirmText: '确认',
  action: null as null | (() => void | Promise<void>),
})

function askConfirm(opts: {
  header: string
  warning: string
  message?: string
  confirmText?: string
  action: () => void | Promise<void>
}) {
  confirmState.header = opts.header
  confirmState.warning = opts.warning
  confirmState.message = opts.message ?? ''
  confirmState.confirmText = opts.confirmText ?? '确认'
  confirmState.action = opts.action
  confirmState.visible = true
}

async function runConfirmed() {
  const action = confirmState.action
  confirmState.visible = false
  confirmState.action = null
  if (action) await action()
}

const backupColumns: TableColumn[] = [
  { id: 'file', header: '备份文件', minWidth: 220 },
  { id: 'size', header: '大小', width: 110 },
  { id: 'created', header: '创建时间', width: 170 },
  { id: 'actions', header: '操作', width: 264, align: 'end' },
]

/** 文件选择器承载上传：隐藏 input + 按钮，替换掉 el-upload（本页只用到 file 一项）。 */
function pickBackupFile() {
  backupInputRef.value?.click()
}

function onBackupFileSelected(e: Event) {
  const target = e.target as HTMLInputElement
  const file = target.files?.[0]
  target.value = ''
  if (file) onUploadBackup({ file })
}

/** el-upload 自定义上传：走后端 multipart 接口，由后端校验后落入备份目录。 */
async function onUploadBackup(options: any) {
  const file = options?.file as File | undefined
  if (!file) return
  backupUploading.value = true
  try {
    const { data } = await uploadBackup(file)
    if (data.code === 0) {
      ElMessage.success(`已上传并校验通过：${data.data.file}`)
      loadBackups()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '上传备份失败'))
  } finally {
    backupUploading.value = false
  }
}

/** 面板恢复后重启，轮询 /healthz 待其重新上线再刷新（会话可能随库失效，刷新后回登录页）。 */
async function waitRestartThenReload() {
  for (let i = 0; i < 90; i++) {
    await new Promise((r) => setTimeout(r, 2000))
    try {
      const res = await fetch('/healthz', { cache: 'no-store' })
      if (res.ok) {
        window.location.href = '/'
        return
      }
    } catch {
      /* 面板重启中，继续等待 */
    }
  }
  window.location.reload()
}

async function restoreFromBackup(row: any) {
  const file = String(row.file)
  askConfirm({
    header: '用这个备份覆盖当前数据库？',
    warning: '面板将自动重启并短暂离线，管理员登录凭据以该备份库为准。',
    message: '恢复前系统会自动为当前库生成一份安全快照；恢复后会话可能失效，需要重新登录。',
    confirmText: '确认恢复',
    action: async () => {
      backupRestoring.value = true
      try {
        const { data } = await restoreBackup(file)
        if (data.code !== 0) {
          ElMessage.error(data.message)
          backupRestoring.value = false
          return
        }
        ElMessage.warning(`恢复已安排（安全快照：${data.data.safety_backup.file}），面板即将重启...`)
        waitRestartThenReload()
      } catch (e) {
        ElMessage.error(errMsg(e, '恢复失败'))
        backupRestoring.value = false
      }
    },
  })
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

/**
 * 系统状态信息对。value 里需要嵌徽标/等宽代码的两项由模板的 #value 插槽接管
 * （key === 'db' / 'version' / 'panel'），其余直接用字符串。
 */
const systemPairs = computed<KeyValuePair[]>(() => {
  const s = system.value
  if (!s) return []
  const uptimeH = Math.floor(s.uptime_seconds / 3600)
  const uptimeM = Math.floor((s.uptime_seconds % 3600) / 60)
  return [
    { key: 'app', label: '应用', value: `${s.app_name}（${s.app_env}）` },
    { key: 'panel', label: '面板版本', value: s.panel_version },
    { key: 'go', label: 'Go 版本', value: s.go_version },
    { key: 'uptime', label: '运行时长', value: `${uptimeH}h ${uptimeM}m` },
    { key: 'goroutines', label: 'Goroutines', value: s.goroutines },
    { key: 'db', label: '数据库', value: '' },
    { key: 'backup', label: '备份调度', value: s.backup_enabled ? '已启用' : '未启用' },
    { key: 'version', label: '数据库版本', value: '' },
    { key: 'mem', label: '内存占用', value: `${s.mem_alloc_mb} MB` },
    { key: 'time', label: '服务时间', value: formatDateTime(s.server_time) },
  ]
})

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

// 版本列表（安装历史版本 / 回滚）
const releases = ref<ReleaseItem[]>([])
const releasesLoading = ref(false)
const selectedRelease = ref('')
const showHistoryReleases = ref(false)

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

/**
 * 把 phase 映射成官方的逐步状态。失败的落点是「最后经历的有效阶段」：
 * 该步标记 error，后续步骤保持 not-started，和原 el-steps 的 process-status 口径一致。
 */
const updateSteps = computed<StepItem[]>(() => {
  const phase = updateStatus.value?.phase
  const failed = phase === 'failed'
  const active = updateActiveStep.value
  const done = phase === 'success'
  const labels = [
    { header: '下载更新包', details: 'GitHub Release' },
    { header: '校验与自检', details: 'SHA-256 / self-test' },
    { header: '替换文件', details: '备份并原子替换' },
    { header: '重启容器', details: 'unless-stopped 拉起' },
  ]
  return labels.map((label, index) => {
    let status: StepsStatus
    if (done) status = 'success'
    else if (index < active) status = 'success'
    else if (index === active) status = failed ? 'error' : 'in-progress'
    else status = 'not-started'
    return { ...label, status }
  })
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

// 检查更新 / 版本列表的失败在这两处只显示一次：手动点「重新检查」弹 toast，
// 进入页签自动跑的那一路静默失败，落到卡片内的提示行（GitHub 不通时不再每次进页签都弹窗）
const updateError = ref('')
const releasesError = ref('')

async function onCheckUpdate(silent = false) {
  updateChecking.value = true
  updateError.value = ''
  try {
    const { data } = await checkUpdate()
    if (data.code === 0) {
      updateInfo.value = data.data
    } else {
      updateError.value = data.message || '检查更新失败'
      if (!silent) ElMessage.error(updateError.value)
    }
  } catch (e) {
    updateError.value = errMsg(e, '检查更新失败')
    if (!silent) ElMessage.error(updateError.value)
  } finally {
    updateChecking.value = false
  }
}

// 进入「系统状态」页签即自动完成原先的「检查更新 + 加载版本列表」两次点击。
// 有新版时卡片里会出现「应用更新」按钮（见模板 v-if），无需先手动检查。
let lastUpdateRefreshAt = 0

async function refreshUpdate(silent = false) {
  // 更新流程进行中（监控弹窗在轮询）不打扰；已有请求在跑时不重复发起
  if (updateStatus.value?.running) return
  if (updateChecking.value || releasesLoading.value) return
  // 自动那一路加一分钟节流：update/check 与 list/releases 都是未认证直连 GitHub API
  // （60 次/小时/IP），来回切页签不该把配额打光；手点「重新检查」不受此限
  if (silent && Date.now() - lastUpdateRefreshAt < 60_000) return
  lastUpdateRefreshAt = Date.now()
  await onCheckUpdate(silent)
  if (updateInfo.value?.enabled !== false) await onLoadReleases(silent)
}

watch(activeTab, (tab) => {
  if (tab === 'system') void refreshUpdate(true)
})

async function confirmApply() {
  if (!updateInfo.value?.available) return
  const target = updateInfo.value.latest_version
  const current = updateInfo.value.current_version
  askConfirm({
    header: `更新到 ${target}？`,
    warning: '更新期间面板会短暂离线，新版本启动失败将自动回滚。',
    message: `将下载 ${target} 并替换当前版本（${current}）。流程：下载 → SHA-256 校验 → 自检 → 替换 → 容器自动重启（unless-stopped）。`,
    confirmText: '应用更新',
    action: async () => {
      updateApplying.value = true
      try {
        // 显式传目标版本：审计日志记录请求体，空串会导致日志只见 {"version":""}
        const { data } = await applyUpdate(target || '')
        if (data.code !== 0) {
          ElMessage.error(data.message)
          return
        }
        if (data.data?.started === false && !data.data.progress?.running) {
          // 「已是最新版本」等无需进入监控弹窗的情形
          ElMessage.success(data.data.message || data.message)
          return
        }
        updateTargetVer.value = data.data?.version || target || ''
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
    },
  })
}

// 拉取版本列表（安装历史版本 / 回滚）：来自 GitHub Release API，安装复用更新链路。
async function onLoadReleases(silent = false) {
  releasesLoading.value = true
  releasesError.value = ''
  try {
    const { data } = await listReleases()
    if (data.code === 0) {
      releases.value = data.data.releases || []
      // 不再自动预选第一个可安装版本：预选会让「安装」按钮一进页面就是可点状态，
      // 与「有新版才出现更新按钮」的按需风格冲突；降级/回滚也该由人明确挑一个版本。
      // 已经选过的版本若仍在列表里则保留，避免自动刷新把人的选择清掉
      if (selectedRelease.value && !releases.value.some((r) => r.version === selectedRelease.value)) {
        selectedRelease.value = ''
      }
    } else {
      releasesError.value = data.message || '查询版本列表失败'
      if (!silent) ElMessage.error(releasesError.value)
    }
  } catch (e) {
    releasesError.value = errMsg(e, '查询版本列表失败')
    if (!silent) ElMessage.error(releasesError.value)
  } finally {
    releasesLoading.value = false
  }
}

// 安装所选历史版本（回滚）：与「应用更新」同一条 下载 → SHA-256 → 自检 → 原子替换 → 重启 链路。
async function confirmInstallRelease() {
  const v = selectedRelease.value
  if (!v) {
    ElMessage.warning('请选择要安装的版本')
    return
  }
  const rel = releases.value.find((r) => r.version === v)
  if (rel && !rel.installable) {
    ElMessage.warning('该版本没有当前架构的安装包')
    return
  }
  askConfirm({
    header: `安装 ${v}？`,
    warning: '降级/回滚同样走更新链路，若跨越数据库结构变更，降级前请先备份数据库。',
    message: `将下载并安装 ${v}（当前 ${updateInfo.value?.current_version || '—'}）。流程同应用更新：下载 → SHA-256 校验 → 自检 → 原子替换 → 容器重启，期间面板短暂不可用。`,
    confirmText: '下载并安装',
    action: async () => {
      updateApplying.value = true
      try {
        const { data } = await applyUpdate(v)
        if (data.code !== 0) {
          ElMessage.error(data.message)
          return
        }
        if (data.data?.started === false && !data.data.progress?.running) {
          ElMessage.success(data.data.message || data.message)
          return
        }
        updateTargetVer.value = data.data?.version || v
        updateDowntimeAt = 0
        resumeUpdateWatch(
          data.data?.progress?.running
            ? data.data.progress
            : { running: true, phase: 'checking', target_version: updateTargetVer.value, message: '正在解析更新源...' },
        )
      } catch (e: any) {
        ElMessage.error(e?.response?.data?.message || errMsg(e, '触发安装失败'))
      } finally {
        updateApplying.value = false
      }
    },
  })
}

async function save() {
  saving.value = true
  try {
    const { data } = await updateSettings({
      site: { ...form.site },
      captcha: captchaPayload(),
      agent: { ...form.agent },
      timezone: { ...form.timezone },
    })
    if (data.code === 0) {
      ElMessage.success('设置已保存并立即全站生效')
      setDisplayTimezone(form.timezone.display_timezone)
      // 用服务端回写重置验证分组：清掉本次输入的 secret，并刷新「已配置」状态
      Object.assign(form.captcha, emptyCaptcha(), data.data.captcha)
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
  <CsPage>
    <CsToolbar>
      <CsBox variant="span" font-size="heading-s" font-weight="bold">站点设置</CsBox>
      <CsBox variant="span" font-size="body-s" color="text-body-secondary">
        站点品牌、人机验证与订阅入口的统一配置，保存后立即生效。
      </CsBox>
      <template #actions>
        <CsButton v-if="isConfigTab" variant="primary" :loading="saving" :icon-name="'check'" @click="save">保存配置</CsButton>
      </template>
    </CsToolbar>

    <CsContainer class="settings-card" header="站点配置">
      <CsLoading :loading="loading">
        <CsTabs v-model="activeTab" :tabs="settingsTabs">
        <!-- ==================== TAB 1: 站点 ==================== -->
        <div v-show="activeTab === 'site'">
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

          <CsForm label-position="top" style="max-width: 680px">
            <div class="form-grid">
              <CsFormItem>
                <template #label>
                  站点名称
                  <TipIcon content="用于浏览器页面标题、系统邮件及订阅默认文件名前缀" />
                </template>
                <CsInput v-model="form.site.app_name" placeholder="例如：星云机场" :maxlength="64" />
              </CsFormItem>
              <CsFormItem>
                <template #label>
                  站点描述
                  <TipIcon content="站点的副标题或一句话描述，展示在登录认证页与页脚" />
                </template>
                <CsInput v-model="form.site.app_description" placeholder="一句话描述站点" :maxlength="200" />
              </CsFormItem>
            </div>

            <!-- LOGO 上传与自定义 -->
            <CsFormItem>
              <template #label>
                站点 LOGO
                <TipIcon content="用于管理端侧栏、用户端顶栏及登录页。支持 png/jpg/ico/svg/webp 等格式，内置 1:1 智能裁剪与平滑缩放，并转为轻量 Base64 存储。" />
              </template>
              <div class="upload-row">
                <span v-if="form.site.logo" class="upload-thumb">
                  <img :src="form.site.logo" alt="站点 LOGO 预览" />
                </span>
                <CsInput
                  v-model="logoUrl"
                  :placeholder="logoPlaceholder"
                  clearable
                />
                <CsButton :icon-name="'upload'" @click="logoInputRef?.click()">
                  上传并裁剪
                </CsButton>
                <CsButton
                  v-if="form.site.logo"
                  :icon-name="'edit'"
                  @click="openCropperForCurrent('logo')"
                >
                  重新裁剪
                </CsButton>
                <CsButton v-if="form.site.logo" :icon-name="'remove'" @click="form.site.logo = ''">
                  恢复默认
                </CsButton>
              </div>
            </CsFormItem>

            <!-- Favicon 上传与自定义 -->
            <CsFormItem>
              <template #label>
                Favicon
                <TipIcon content="浏览器标签页左侧展示的小图标，建议尺寸 32x32 或 48x48。" />
              </template>
              <div class="upload-row">
                <span v-if="form.site.favicon" class="upload-thumb is-small">
                  <img :src="form.site.favicon" alt="Favicon 预览" />
                </span>
                <CsInput
                  v-model="faviconUrl"
                  :placeholder="faviconPlaceholder"
                  clearable
                />
                <CsButton :icon-name="'upload'" @click="faviconInputRef?.click()">
                  上传并裁剪
                </CsButton>
                <CsButton
                  v-if="form.site.favicon"
                  :icon-name="'edit'"
                  @click="openCropperForCurrent('favicon')"
                >
                  重新裁剪
                </CsButton>
                <CsButton v-if="form.site.favicon" :icon-name="'remove'" @click="form.site.favicon = ''">
                  清空
                </CsButton>
              </div>
            </CsFormItem>

            <!-- 品牌全景实时预览卡片 -->
            <div class="brand-preview-section">
              <div class="preview-header">
                <CsIcon name="grid-view" />&nbsp;全场景实时效果预览（当前编辑状态）
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
                      <div class="p-auth-sub">{{ form.site.app_description || '订阅与节点管理平台' }}</div>
                    </div>
                  </div>
                </div>
              </div>
            </div>

            <div class="form-grid" style="margin-top: 14px">
              <CsFormItem label="服务条款 URL">
                <CsInput v-model="form.site.tos_url" placeholder="https://example.com/tos" />
              </CsFormItem>
              <CsFormItem label="货币代码">
                <CsInput v-model="form.site.currency" placeholder="CNY" :maxlength="8" />
              </CsFormItem>
              <CsFormItem label="货币符号">
                <CsInput v-model="form.site.currency_symbol" placeholder="¥" :maxlength="8" />
              </CsFormItem>
            </div>

            <CsFormItem>
              <template #label>
                用户注册准入
                <TipIcon content="开启后，新用户可通过前端注册页开户（仍受邀请码机制限制）；关闭后禁止新用户注册。" />
              </template>
              <CsToggle v-model="openRegister">
  <template #default>
    {{ openRegister ? '开放注册（需邀请码）' : '已关闭（禁止新用户注册）' }}
  </template>
</CsToggle>
            </CsFormItem>
          </CsForm>
        </div>

        <!-- ==================== TAB 2: 订阅与清洗网关 ==================== -->
        <div v-show="activeTab === 'subscribe'">
          <CsForm label-position="top" style="max-width: 820px">
            <CsAlert
              header="物理端口隔离与反代解耦"
              type="info"
              style="margin-bottom: 20px"
            >
              <div style="font-size: 13px; line-height: 1.6">
                将<b>用户拉取订阅服务</b>与<b>面板 Web 界面 / 管理 API / 服务器长连接</b>物理分离。
                独立端口仅运行纯净、轻量的订阅生成与清洗引擎，<b>绝不暴露任何后台管理接口或服务器网关</b>，彻底杜绝订阅域名被扫描导致的敏感信息泄露！
              </div>
            </CsAlert>

            <CsBox variant="div" font-size="heading-s" font-weight="bold">订阅入口与行为参数</CsBox>
            <div class="form-grid">
              <CsFormItem>
                <template #label>
                  对外订阅根地址
                  <TipIcon content="用户端获取的订阅链接根域名。订阅域名整域反代到独立订阅端口（APP_SUB_PORT），不设置则默认使用当前面板域名 + 订阅路径。" />
                </template>
                <CsInput v-model="form.site.subscribe_url" placeholder="如 https://sub.example.com" clearable />
              </CsFormItem>

              <CsFormItem>
                <template #label>
                  订阅入口路径
                  <TipIcon content="订阅端口唯一入口路径（支持 /path/:token 与 ?token=xxx），其余路径一律按下方拒绝码返回。保存后订阅服务自动重载。" />
                </template>
                <CsInput v-model="form.site.subscribe_path" placeholder="如 /sub 或 /mysub（默认 /sub）" clearable />
              </CsFormItem>

              <CsFormItem>
                <template #label>
                  非订阅路径拒绝码
                  <TipIcon content="订阅端口上访问非订阅路径的请求或无效订阅 Token 统一返回该 HTTP 状态码。404 可有效迷惑自动化扫描器。" />
                </template>
                <CsSelect v-model="form.site.sub_deny_code" style="width: 100%">
                  <CsOption label="404 Not Found（推荐，防探测）" value="404" />
                  <CsOption label="401 Unauthorized（客户端感知鉴权失败）" value="401" />
                </CsSelect>
              </CsFormItem>

              <CsFormItem>
                <template #label>
                  订阅配置标题
                  <TipIcon content="Clash/Sing-box 等客户端显示的订阅配置文件名（Content-Disposition filename 响应头）。留空回退站点名称，仍为空时兜底 xray；引号/反斜杠/换行会被自动剥离。" />
                </template>
                <CsInput v-model="form.site.sub_profile_title" placeholder="留空则使用站点名称（app_name）" clearable :maxlength="64" />
              </CsFormItem>

              <CsFormItem>
                <template #label>
                  客户端更新间隔（小时）
                  <TipIcon content="Profile-Update-Interval 响应头：客户端按该周期自动重新拉取订阅，默认 24 小时（范围 1–168 小时）。" />
                </template>
                <CsInputNumber v-model="subUpdateHours" :min="1" :max="168" style="width: 100%" />
              </CsFormItem>

              <CsFormItem>
                <template #label>
                  多域名分发
                  <CsBadge color="grey">暂未开放</CsBadge>
                  <TipIcon content="暂未开放。用于轮询或主备容灾的多订阅分发域名（英文逗号分隔）。" />
                </template>
                <CsInput v-model="form.site.subscribe_domain" disabled placeholder="sub1.com,sub2.com（暂未开放）" />
              </CsFormItem>
            </div>

            <CsBox variant="div" font-size="heading-s" font-weight="bold" :margin="{ top: 'l' }">
              原生订阅清洗防探测网关 (Subscribe Sieve)
              <TipIcon content="在独立订阅端口前置针对 HTTP User-Agent 与请求特征的深度过滤，有效阻断扫描探测与未授权爬虫。" />
            </CsBox>
            <div class="form-grid">
              <CsFormItem>
                <template #label>
                  智能 UA 过滤与爬虫拦截
                  <TipIcon content="自动识别并阻断 curl、python-requests、空 User-Agent 及已知自动化扫描脚本。" />
                </template>
                <CsToggle v-model="subCleanUa">
  <template #default>
    {{ subCleanUa ? '开启（阻断空 UA / 常见脚本）' : '放行所有' }}
  </template>
</CsToggle>
              </CsFormItem>

              <CsFormItem>
                <template #label>
                  严格客户端白名单模式
                  <TipIcon content="仅放行知名代理客户端（如 Clash、Sing-box、Shadowrocket、Surge、V2rayN 等），其余均直接阻断。" />
                </template>
                <CsToggle v-model="subStrictUa">
  <template #default>
    {{ subStrictUa ? '严格白名单' : '宽松模式' }}
  </template>
</CsToggle>
              </CsFormItem>
            </div>

            <CsFormItem>
              <template #label>
                自定义封禁 UA 关键词
                <TipIcon content="匹配 User-Agent 包含指定关键词时直接拦截并返回拒绝码，英文逗号分隔，如 scan,exploit,badbot。" />
              </template>
              <CsInput
                v-model="form.site.sub_blocked_ua"
                placeholder="如 scan,exploit,badbot（英文逗号分隔，命中即拦截）"
                clearable
              />
            </CsFormItem>

            <!-- 反代配置代码片段展示 -->
            <CsBox variant="div" font-size="heading-s" font-weight="bold" :margin="{ top: 'l' }">生产反向代理配置参考 (含 Cloudflare 真实 IP 透传)</CsBox>
            <div class="proxy-snippet-box">
              <div class="snippet-header">
                <span>Caddyfile 配置示例（推荐）</span>
                <CsButton :icon-name="'copy'" @click="copySnippet(caddySnippet, 'Caddyfile 配置')">
                  复制 Caddy 配置
                </CsButton>
              </div>
              <pre class="snippet-code"><code>{{ caddySnippet }}</code></pre>
            </div>

            <div class="proxy-snippet-box" style="margin-top: 14px">
              <div class="snippet-header">
                <span>Nginx 配置示例</span>
                <CsButton :icon-name="'copy'" @click="copySnippet(nginxSnippet, 'Nginx 配置')">
                  复制 Nginx 配置
                </CsButton>
              </div>
              <pre class="snippet-code"><code>{{ nginxSnippet }}</code></pre>
            </div>
          </CsForm>
        </div>

        <!-- ==================== TAB 3: 安全（人机验证） ==================== -->
        <div v-show="activeTab === 'captcha'">
          <CsForm label-position="top" style="max-width: 640px">
            <CsFormItem>
              <template #label>
                人机验证保护
                <TipIcon content="开启后，用户在登录和注册页面必须通过人机挑战验证，有效防御恶意撞库与脚本批量注册。" />
              </template>
              <CsToggle v-model="captchaEnable">
  <template #default>
    {{ captchaEnable ? '已开启（登录与注册保护）' : '已关闭（开发/内网环境）' }}
  </template>
</CsToggle>
            </CsFormItem>
            <CsFormItem>
              <template #label>
                验证提供商
                <TipIcon content="当前支持 Cloudflare Turnstile 智能无感验证。" />
              </template>
              <CsSelect v-model="form.captcha.captcha_type" style="width: 100%">
                <CsOption label="Cloudflare Turnstile（智能无感验证）" value="turnstile" />
              </CsSelect>
            </CsFormItem>
            <CsFormItem>
              <template #label>
                Turnstile Site Key
                <TipIcon content="从 Cloudflare 控制台 Turnstile 站点获取的公开前端密钥。" />
              </template>
              <CsInput v-model="form.captcha.turnstile_site_key" placeholder="0x4A...（Cloudflare 控制台获取）" />
            </CsFormItem>
            <CsFormItem>
              <template #label>
                Turnstile Secret Key
                <TipIcon content="服务端与 Cloudflare 校验使用的私钥。只写不读：保存后不再回显，留空表示保持原密钥不变。" />
              </template>
              <div class="secret-row">
                <CsInput
                  v-model="form.captcha.turnstile_secret_key"
                  class="secret-input"
                  type="password"
                  show-password
                  :disabled="secretClearing"
                  :placeholder="secretConfigured ? '已配置（留空保持不变）' : '0x4B...（Cloudflare 控制台获取）'"
                />
                <CsBadge v-if="secretClearing" color="red">保存后清除</CsBadge>
                <CsBadge v-else-if="secretConfigured" color="green">已配置</CsBadge>
                <CsBadge v-else color="grey">未配置</CsBadge>
                <CsButton v-if="secretConfigured && !secretClearing" @click="markSecretClear">清除</CsButton>
                <CsButton v-else-if="secretClearing" @click="undoSecretClear">撤销</CsButton>
              </div>
            </CsFormItem>
            <CsAlert type="info">
              在 Cloudflare 控制台添加 Turnstile 小部件并配置域名，填入密钥即可。开启状态下若未配置有效密钥将默认阻断验证（Fail-Closed）。
            </CsAlert>
          </CsForm>
        </div>

        <!-- ==================== TAB 3.5: 节点上报 ==================== -->
        <div v-show="activeTab === 'agent'">
          <CsForm label-position="top" style="max-width: 640px">
            <CsFormItem>
              <template #label>
                流量增量上报周期（秒）
                <TipIcon content="服务器向面板同步 xray 用户计费流量增量的频率。较长周期（如 60-120 秒）可减少数据库写入压力；超额/到期判定将在落库后即时处置。" />
              </template>
              <CsInputNumber v-model="agentReportSec" :min="5" :max="1800" :step="10" style="width: 220px" />
            </CsFormItem>
            <CsFormItem>
              <template #label>
                状态心跳周期（秒）
                <TipIcon content="服务器向面板汇报 CPU / 内存 / 磁盘 / 实时网速 / 在线用户等实时健康状态的频率。默认 5 秒（推荐 3-5 秒）；状态仅更新在主控内存，历史快照按 1 分钟窗口聚合落库，高频心跳不会增大数据库体积或写入负担。" />
              </template>
              <CsInputNumber v-model="agentHeartbeatSec" :min="3" :max="1800" :step="1" style="width: 220px" />
            </CsFormItem>
            <CsAlert type="info">
              配置保存后将通过长连接即时下发至在线服务器生效（离线服务器重连后自动拉取同步）；agent.yaml 的本地配置作为兜底。
            </CsAlert>
          </CsForm>
        </div>

        <!-- ==================== TAB 3.6: 时间与时区 ==================== -->
        <div v-show="activeTab === 'timezone'">
          <CsForm label-position="top" style="max-width: 680px">
            <CsFormItem>
              <template #label>
                业务时区（统计切天口径）
                <TipIcon content="决定「今日/本月流量汇总」「每日流量快照」「清理边界」以及「入站重置」的切天时间点。建议设置为主要用户群所在时区。" />
              </template>
              <CsSelect v-model="form.timezone.business_timezone" filterable allow-create style="width: 300px">
                <CsOption v-for="tz in timezoneOptions" :key="tz" :label="tz" :value="tz" />
              </CsSelect>
            </CsFormItem>
            <CsFormItem>
              <template #label>
                展示时区
                <TipIcon content="控制后台管理端表格与日志中的时间戳显示格式；选择「跟随浏览器」将根据您的本地设备自动换算。" />
              </template>
              <div style="display: flex; align-items: center; gap: 10px; flex-wrap: wrap">
                <CsSelect v-model="form.timezone.display_timezone" filterable style="width: 300px">
                  <CsOption label="跟随浏览器" value="browser" />
                  <CsOption v-for="tz in timezoneOptions" :key="tz" :label="tz" :value="tz" />
                </CsSelect>
                <CsBadge color="blue">当前识别：{{ effectiveTimezoneLabel() }}</CsBadge>
              </div>
            </CsFormItem>
            <CsAlert type="info">
              底层数据统一以 UTC 存储绝对时间戳。修改业务时区将调整每日 0 点的计算基准，切换当日历史聚合数据可能会有短暂平滑窗口。
            </CsAlert>
          </CsForm>
        </div>

        <!-- ==================== TAB 4: 备份 ==================== -->
        <div v-show="activeTab === 'backup'">
          <div style="max-width: 720px">
            <CsToolbar>
              <CsButton :loading="backupLoading" :icon-name="'refresh'" @click="loadBackups">刷新列表</CsButton>
              <CsBox variant="span" font-size="body-s" color="text-body-secondary">
                共 {{ backups.length }} 份 · 占用 {{ fmtSize(backupTotalSize) }}
              </CsBox>
              <template #actions>
                <input
                  ref="backupInputRef"
                  type="file"
                  accept=".db,.sqlite,.db3"
                  style="display: none"
                  @change="onBackupFileSelected"
                />
                <CsButton :icon-name="'upload'" :loading="backupUploading" @click="pickBackupFile">上传备份</CsButton>
                <CsButton variant="primary" :loading="backupCreating" @click="createBackupNow">立即备份</CsButton>
              </template>
            </CsToolbar>
            <CsTable
              :items="backups"
              :columns="backupColumns"
              :loading="backupLoading"
              loading-text="正在加载备份列表..."
              empty-text="暂无备份，点击右上角「立即备份」"
              aria-label="备份列表"
            >
              <template #cell-file="{ row }"><code class="cs-text-mono">{{ row.file }}</code></template>
              <template #cell-size="{ row }">{{ fmtSize(row.size) }}</template>
              <template #cell-created="{ row }">{{ formatDateTime(row.created_at) }}</template>
              <template #cell-actions="{ row }">
                <div class="row-actions">
                  <CsButton :wrap-text="false" :disabled="backupRestoring" @click="restoreFromBackup(row)">恢复</CsButton>
                  <CsButton :wrap-text="false" icon-name="download" @click="downloadBackup(row.file)">下载</CsButton>
                  <CsButton :wrap-text="false" icon-name="remove" @click="removeBackup(row)">删除</CsButton>
                </div>
              </template>
            </CsTable>
            <CsAlert type="info">
              备份文件保存在主控备份目录并按配置定期轮转，建议定期下载重要备份离线保存。「上传备份」会先校验完整性与数据库版本再入库；恢复前系统会自动生成安全快照，恢复后将自动重启并需重新登录（凭据以备份库为准）。
            </CsAlert>
          </div>
        </div>

        <!-- ==================== TAB 6: 系统状态 ==================== -->
        <div v-show="activeTab === 'system'">
          <CsLoading :loading="systemLoading" min-height="200px" class="system-pane">
            <template v-if="system">
              <!-- 窄屏必须降到 1 列：2 列时每格只剩 ~100px，「数据库版本」这类标签会被
                   折成「数据库版 / 本」、「schema v2 / 期望 v2 / 最低兼容 v2」折成三行，
                   整张表看起来像被裁掉了一半，也读不出标签与值的对应关系 -->
              <CsKeyValuePairs :items="systemPairs" :columns="isMobile ? 1 : 2" aria-label="系统状态">
                <template #value="{ item }">
                  <template v-if="item.key === 'db'">
                    <CsBadge :color="system.db_ok ? 'green' : 'red'">{{ system.db_ok ? '正常' : '异常' }}</CsBadge>
                    <span style="margin-left: 6px">{{ system.db_driver }} · {{ system.db_latency_ms }}ms</span>
                  </template>
                  <template v-else-if="item.key === 'version'">
                    <CsBox variant="code">schema v{{ system.schema_version || '—' }} / 期望 v{{ system.schema_expected ?? '—' }}</CsBox>
                    <CsBox variant="span" color="text-body-secondary" :margin="{ left: 'xs' }">
                      最低兼容 v{{ system.schema_min_compatible }}
                    </CsBox>
                  </template>
                  <CsBox v-else-if="item.key === 'panel'" variant="code">{{ item.value }}</CsBox>
                  <template v-else>{{ item.value }}</template>
                </template>
              </CsKeyValuePairs>
              <!-- 计数块：与系统状态表同源，放在同一列宽里，用 CsCardGrid 的官方卡距 -->
              <CsCardGrid class="status-count-grid" :min-width="160" :gap="10">
                <CsCard v-for="(v, k) in system.counts" :key="k">
                  <CsBox variant="span" font-size="heading-l" font-weight="bold" display="block">{{ v }}</CsBox>
                  <CsBox variant="span" font-size="body-s" color="text-body-secondary">
                    {{ { users: '用户', servers: '服务器', inbounds: '入站', orders: '订单', gift_cards: '礼品卡', audit_logs: '审计日志' }[k] ?? k }}
                  </CsBox>
                </CsCard>
              </CsCardGrid>
              <CsAlert v-if="!system.db_ok" type="error" header="数据库异常">{{ system.db_error }}</CsAlert>

              <!-- 面板内更新（容器形态自更新）：进入本页签自动检查更新 + 拉取版本列表，
                   有新版才出现「应用更新」，不再常驻「检查更新 / 加载版本列表」两个按钮 -->
              <!-- 面板内更新卡片 -->
              <CsContainer header="面板更新">
                <template #info>
                  <CsStatusIndicator v-if="updateChecking" type="loading" text="检查中…" />
                  <CsStatusIndicator v-else-if="updateInfo?.available" type="success" text="发现新版本" />
                  <CsStatusIndicator v-else-if="updateInfo && !updateInfo.enabled" type="stopped" text="更新已禁用" />
                  <CsStatusIndicator v-else-if="updateInfo" type="info" text="已是最新" />
                </template>
                <template #actions>
                  <CsTooltip
                    placement="top"
                    :show-delay="200"
                    content="更新机制：下载官方 Release 并校验 SHA-256，原子替换后由容器守护自动拉起；若启动失败将自动安全回滚至上一版本。"
                  >
                    <CsButton variant="icon" icon-name="status-info" aria-label="更新机制说明" />
                  </CsTooltip>
                  <CsButton
                    :icon-name="'refresh'"
                    :loading="updateChecking || releasesLoading"
                    @click="refreshUpdate(false)"
                  >
                    检查更新
                  </CsButton>
                </template>

                <!-- 场景 1：有可用更新 -> 升级对比舱 -->
                <div v-if="updateInfo?.available" class="update-action-banner">
                  <div class="update-route-box">
                    <div class="ver-pill current">
                      <span class="ver-tag">当前版本</span>
                      <code class="ver-code">{{ updateInfo?.current_version || system.panel_version }}</code>
                    </div>
                    <div class="route-arrow">
                      <CsIcon name="angle-right" />
                    </div>
                    <div class="ver-pill target">
                      <span class="ver-tag">目标版本</span>
                      <code class="ver-code">{{ updateInfo?.latest_version }}</code>
                    </div>
                  </div>

                  <CsButton
                    class="update-apply-btn"
                    type="primary"
                    :icon-name="'download'"
                    :disabled="!!updateStatus?.running"
                    :loading="updateApplying"
                    @click="confirmApply"
                  >
                    立即更新到 {{ updateInfo?.latest_version }}
                  </CsButton>
                </div>

                <!-- 场景 2：已是最新或检查完毕无需更新 -->
                <div v-else-if="updateInfo && updateInfo.enabled" class="update-status-row">
                  <div class="uptodate-box">
                    <div class="status-check-circle">
                      <CsIcon name="check" />
                    </div>
                    <div class="uptodate-info">
                      <span class="cur-ver-text">
                        当前运行版本：<code class="cs-text-mono">{{ updateInfo?.current_version || system.panel_version }}</code>
                      </span>
                      <span class="uptodate-hint">系统已安装最新版本，各项组件运行正常。</span>
                    </div>
                  </div>
                </div>

                <!-- 场景 3：未启用自更新 -->
                <div v-else-if="updateInfo && !updateInfo.enabled" class="update-status-row">
                  <div class="disabled-box">
                    <span class="cur-ver-text">
                      当前运行版本：<code class="cs-text-mono">{{ updateInfo?.current_version || system.panel_version }}</code>
                    </span>
                    <span class="disabled-hint">当前部署形态未启用面板内在线更新（由环境设定），可在宿主机手动替换镜像或二进制版本。</span>
                  </div>
                </div>

                <!-- 错误提示（若接口报错）：两个请求各报各的。合并成一条会把「版本列表拉取失败」
                     伪装成「暂无可安装版本」，而两者直连同一个 GitHub API、往往同时失败。 -->
                <CsAlert v-if="updateError || releasesError" type="error" header="更新检查失败">
                  <!-- 后端会把上游 err.Error() 原样透出（含长 URL / 长 hash），必须允许断词，否则横向顶出卡片 -->
                  <div class="update-error-lines">
                    <div v-if="updateError">检查更新失败：{{ updateError }}</div>
                    <div v-if="releasesError">版本列表获取失败：{{ releasesError }}</div>
                  </div>
                </CsAlert>

                <!-- 历史版本与回退区（可折叠高级选项，默认收起） -->
                <div v-if="updateInfo?.enabled" class="update-hist-section">
                  <button
                    type="button"
                    class="hist-collapse-trigger"
                    :class="{ 'is-expanded': showHistoryReleases }"
                    @click="showHistoryReleases = !showHistoryReleases"
                  >
                    <div class="trigger-label">
                      <CsIcon name="angle-right" class="trigger-chevron" />
                      <span>历史版本与安全回退</span>
                    </div>
                  </button>

                  <div v-if="showHistoryReleases" class="hist-collapse-panel">
                    <div class="hist-form-row">
                      <span class="hist-label">指定版本</span>
                      <CsSelect
                        v-model="selectedRelease"
                        :loading="releasesLoading"
                        :placeholder="releases.length ? '选择要安装或回滚的历史版本' : '暂无可安装版本'"
                        :disabled="!!updateStatus?.running || !releases.length"
                        class="hist-select"
                      >
                        <CsOption
                          v-for="r in releases"
                          :key="r.version"
                          :label="r.version + (r.current ? '（当前）' : r.installable ? '' : '（无当前架构包）')"
                          :value="r.version"
                          :disabled="!r.installable || r.current"
                        />
                      </CsSelect>
                      <CsButton
                        v-if="selectedRelease"


                        :disabled="!!updateStatus?.running"
                        :loading="updateApplying"
                        @click="confirmInstallRelease"
                      >
                        安装 {{ selectedRelease }}
                      </CsButton>
                    </div>
                    <p class="hist-warning-note">
                      <CsIcon name="status-warning" />
                      <span>降级/回滚走相同更新链路。若跨越数据库结构变更，降级前请务必先备份数据库。</span>
                    </p>
                  </div>
                </div>

                <!-- 底部精炼提示条：1 句话核心提示，不再大段平铺 -->
                <CsAlert type="info">
                  更新期间面板将短暂离线重启；若新版本启动失败，系统将自动安全回滚至上一版本。
                </CsAlert>
              </CsContainer>

              <!-- 客户端 IP 来源说明 -->
              <CsAlert type="warning" header="客户端真实 IP 识别架构">
                面板按 <code>CF-Connecting-IP</code> → <code>X-Real-IP</code> → <code>X-Forwarded-For</code> →
                <code>RemoteAddr</code> 的优先级识别真实客户端 IP，用于登录/订阅限流、审计日志与人机验证。
                请保持「面板端口绑定 127.0.0.1 + 反向代理（Caddy/Nginx）前置」的部署形态：反代会覆盖/追加可信的 IP
                头，限流与审计才能按真实 IP 生效。切勿将面板端口直接暴露公网，否则攻击者可伪造 IP 头绕过按 IP 的限流。
              </CsAlert>

              <CsToolbar class="system-refresh-bar">
                <template #actions>
                  <CsButton :icon-name="'refresh'" @click="loadSystem">刷新状态</CsButton>
                </template>
              </CsToolbar>
            </template>
            <CsEmpty v-else-if="!systemLoading" description="暂无状态数据" icon-name="status-info" />
          </CsLoading>
        </div>
        </CsTabs>
      </CsLoading>
    </CsContainer>

    <!-- 图片裁剪弹窗 -->
    <ImageCropperDialog
      v-model="cropperVisible"
      :image-src="cropperImageSrc"
      :title="cropperTitle"
      :target-size="cropperTargetSize"
      @crop="onCropFinished"
    />

    <!-- 面板更新进度弹窗（交互对齐节点 Agent 升级监控：phase 驱动步骤条 + 轮询状态） -->
    <CsModal
      v-model:visible="updateModalOpen"
      header="面板更新进度"
      size="medium"
      :close-on-click-overlay="false"
      :close-on-escape="false"
    >
      <CsSteps :steps="updateSteps" orientation="vertical" style="display: flex; justify-content: center" />

      <div class="update-status-box" :class="updateBoxClass">
        <div class="status-icon">
          <CsIcon v-if="updateStatus?.phase === 'success'" name="status-positive" class="icon-success" />
          <CsIcon v-else-if="updateStatus?.phase === 'failed'" name="status-negative" class="icon-error" />
          <CsSpinner v-else size="normal" class="icon-process" />
        </div>
        <div class="status-texts">
          <div class="status-msg">{{ updateDisplayMessage }}</div>
          <div v-if="updateStatus?.error" class="status-err cell-mono">{{ updateStatus.error }}</div>
          <div v-else-if="updateStatus?.phase !== 'success' && updateStatus?.phase !== 'failed'" class="status-hint">
            操作在服务端后台执行，刷新页面后重新打开本页可继续查看进度。
          </div>
        </div>
      </div>
    </CsModal>

    <!-- 破坏性操作确认（AWS 控制台口径：红色 Warning 行 + 红描边/红实心按钮） -->
    <CsConfirmDialog
      v-model:visible="confirmState.visible"
      :header="confirmState.header"
      :warning="confirmState.warning"
      :message="confirmState.message"
      :confirm-text="confirmState.confirmText"
      :loading="updateApplying || backupRestoring"
      @confirm="runConfirmed"
    />
  </CsPage>
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
/* 系统状态页签的内容宽度上限（原 v-loading 容器上的 max-width） */
.system-pane {
  max-width: 720px;
}
/* 备份表操作列：三个按钮排成一行，不换行 */
.row-actions {
  display: flex;
  gap: 6px;
  align-items: center;
  justify-content: flex-end;
  flex-wrap: nowrap;
}

/* 面板更新卡片 */










/* 升级对比舱（有可用新版本） */
.update-action-banner {
  margin-top: 14px;
  padding: 14px 18px;
  border-radius: var(--border-radius-input-k1q34z);
  background: linear-gradient(135deg, var(--color-background-item-selected-dotkup) 0%, var(--color-background-cell-shaded-twxsbz) 100%);
  border: 1px solid rgba(99, 102, 241, 0.2);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  flex-wrap: wrap;
}

.update-route-box {
  display: flex;
  align-items: center;
  gap: 14px;
}

.ver-pill {
  display: flex;
  flex-direction: column;
  gap: 2px;

  .ver-tag {
    font-size: 11px;
    color: var(--color-text-body-secondary-66ygbb);
   -transform: uppercase;
    letter-spacing: 0.5px;
  }
  .ver-code {
    font-family: var(--font-family-monospace-842lr2);
    font-size: 14px;
    font-weight: 700;
    color: var(--color-text-body-default-b2us04);
  }
  &.target .ver-code {
    color: var(--color-text-link-default-qabnuk);
  }
}

.route-arrow {
  color: var(--color-text-body-secondary-66ygbb);
  font-size: 14px;
  display: flex;
  align-items: center;
}

.update-apply-btn {
  font-weight: 600;
}

/* 状态展示行（最新 / 禁用） */
.update-status-row {
  margin-top: 14px;
  padding: 12px 16px;
  background: var(--color-background-cell-shaded-twxsbz);
  border: 1px solid var(--color-border-divider-default-diwd4w);
  border-radius: var(--border-radius-input-k1q34z);
}

.uptodate-box {
  display: flex;
  align-items: center;
  gap: 12px;
}

.status-check-circle {
  width: 26px;
  height: 26px;
  border-radius: 50%;
  background: var(--color-background-alert-success-9dyn87);
  color: var(--color-text-status-success-1i2jf8);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 14px;
  font-weight: bold;
  flex: none;
}

.uptodate-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.cur-ver-text {
  font-size: 12.5px;
  color: var(--color-text-body-default-b2us04);
  code {
    font-family: var(--font-family-monospace-842lr2);
    color: var(--color-text-link-default-qabnuk);
    font-weight: 600;
  }
}

.uptodate-hint {
  font-size: 11.5px;
  color: var(--color-text-body-secondary-66ygbb);
}

.disabled-box {
  display: flex;
  flex-direction: column;
  gap: 3px;
  .disabled-hint {
    font-size: 11.5px;
    color: var(--color-text-body-secondary-66ygbb);
  }
}

/* 更新失败原因：后端会把上游 err.Error() 原样透出（含长 URL / 长 hash），
   不折断会横向顶出容器 */
.update-error-lines {
  min-inline-size: 0;
  overflow-wrap: anywhere;
}

/* 历史版本与安全回退 */
.update-hist-section {
  margin-top: 12px;
  border-top: 1px solid var(--color-border-divider-default-diwd4w);
  padding-top: 10px;
}

.hist-collapse-trigger {
  display: flex;
  align-items: center;
  justify-content: space-between;
  width: 100%;
  background: none;
  border: none;
  padding: 6px 0;
  cursor: pointer;
  font-size: 12.5px;
  color: var(--color-text-body-secondary-66ygbb);
  transition: color 0.15s ease;

  &:hover {
    color: var(--color-text-link-default-qabnuk);
  }

  .trigger-label {
    display: flex;
    align-items: center;
    gap: 6px;
    font-weight: 500;
  }

  .trigger-chevron {
    font-size: 11px;
    transition: transform 0.2s cubic-bezier(0.2, 0, 0, 1);
  }

  &.is-expanded .trigger-chevron {
    transform: rotate(90deg);
  }
}

.hist-collapse-panel {
  margin-top: 8px;
  padding: 12px 14px;
  background: var(--color-background-cell-shaded-twxsbz);
  border: 1px solid var(--color-border-divider-default-diwd4w);
  border-radius: var(--border-radius-input-k1q34z);
}

.hist-form-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;

  .hist-label {
    font-size: 12px;
    color: var(--color-text-body-secondary-66ygbb);
    flex: none;
  }
  .hist-select {
    flex: 1;
    min-width: 200px;
    max-width: 320px;
  }
}

.hist-warning-note {
  margin: 8px 0 0;
  font-size: 11.5px;
  color: var(--color-text-status-warning-fabq6h);
  display: flex;
  align-items: center;
  gap: 5px;
  line-height: 1.4;
}

@media (max-width: 640px) {
  .update-action-banner {
    flex-direction: column;
    align-items: stretch;
    gap: 12px;
  }
  .update-apply-btn {
    width: 100%;
  }
  .hist-form-row {
    flex-direction: column;
    align-items: stretch;
    .hist-select {
      max-width: 100%;
    }
  }
}

.upload-row {
  display: flex;
  gap: 8px;
  align-items: center;
  width: 100%;
  flex-wrap: wrap;

  :deep(.cs-input-root) {
    flex: 1 1 200px;
    min-width: 180px;
  }

  .upload-thumb {
    flex: 0 0 auto;
    width: 34px;
    height: 34px;
    display: flex;
    align-items: center;
    justify-content: center;
    overflow: hidden;
    border: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
    border-radius: 8px;
    background: var(--color-background-container-content-mjfil9, #ffffff);

    &.is-small {
      width: 26px;
      height: 26px;
      border-radius: 6px;
    }

    img {
      max-width: 100%;
      max-height: 100%;
      object-fit: contain;
    }
  }
}

.brand-preview-section {
  margin-top: 8px;
  padding: 14px 16px;
  background: var(--color-background-cell-shaded-twxsbz);
  border: 1px dashed var(--color-border-divider-default-diwd4w);
  border-radius: var(--border-radius-item-card-default-nurfq3);
}

.preview-header {
  font-size: 13px;
  font-weight: 600;
  color: var(--color-text-body-secondary-66ygbb);
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
  border: 1px solid var(--color-border-divider-default-diwd4w);
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
    background: var(--color-background-container-content-mjfil9);
    border-color: var(--color-border-divider-default-diwd4w);
    box-shadow: var(--shadow-container-quglkl);
    .preview-tag { background: var(--color-background-item-selected-dotkup); color: var(--color-text-link-default-qabnuk); }
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
  background: var(--color-background-cell-shaded-twxsbz);
  border: 1px solid var(--color-border-divider-default-diwd4w);
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
  color: var(--color-text-body-default-b2us04);
}

.p-auth-sub {
  font-size: 11px;
  color: var(--color-text-body-secondary-66ygbb);
  margin-top: 1px;
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
    font-family: var(--font-family-monospace-842lr2);
    font-size: 12px;
    line-height: 1.5;
    color: #38bdf8;
    overflow-x: auto;
    white-space: pre;
  }
}

/* 面板更新进度状态盒（样式与节点 Agent 升级监控一致） */
.update-status-box {
  display: flex;
  align-items: flex-start;
  gap: 14px;
  padding: 14px 16px;
  border-radius: 8px;
  background: var(--color-background-cell-shaded-twxsbz);
  border: 1px solid var(--color-border-divider-default-diwd4w);
  margin-top: 12px;

  // 结果面板的状态色统一走语义 token：原来写死亮色底/亮色描边，
  // 深色模式下这几块会变成刺眼的浅色块（本文件里没有任何 html.dark 覆盖）
  &.is-running {
    border-color: var(--color-text-status-info-su6k9n);
    background: var(--color-background-alert-info-lqhdg0);
    .icon-process {
      font-size: 22px;
      color: var(--color-text-status-info-su6k9n);
    }
  }
  &.is-success {
    border-color: var(--color-text-status-success-1i2jf8);
    background: var(--color-background-alert-success-9dyn87);
    .icon-success {
      font-size: 22px;
      color: var(--color-text-status-success-1i2jf8);
      font-weight: bold;
    }
  }
  &.is-failed {
    border-color: var(--color-text-status-error-ri38gt);
    background: var(--color-background-alert-error-dpj69i);
    .icon-error {
      font-size: 22px;
      color: var(--color-text-status-error-ri38gt);
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
      color: var(--color-text-body-default-b2us04);
      line-height: 1.5;
    }
    .status-err {
      margin-top: 6px;
      color: var(--color-text-status-error-ri38gt);
      font-size: 11.5px;
      line-height: 1.4;
      word-break: break-all;
      background: var(--color-background-alert-error-dpj69i);
      padding: 6px 8px;
      border-radius: 4px;
    }
    .status-hint {
      margin-top: 4px;
      color: var(--color-text-body-secondary-66ygbb);
      font-size: 11.5px;
    }
  }
}

/* Secret 只写不读：输入框与前端的「已配置/清除」状态同一行 */
.secret-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;

  .secret-input {
    flex: 1 1 260px;
    min-width: 0;
  }
}
</style>
