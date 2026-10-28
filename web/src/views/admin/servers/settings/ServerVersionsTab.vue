<script setup lang="ts">
import { computed, onUnmounted, ref, watch } from 'vue'
import {
  CsAlert,
  CsBadge,
  CsButton,
  CsContainer,
  CsForm,
  CsFormItem,
  CsSegmentedControl,
  CsSelect,
  CsSteps,
  ElMessage,
  ElMessageBox,
  type SegmentedOption,
  type StepItem,
  type StepsStatus,
} from '@/components/cloudscape'
import {
  getAgentVersions,
  getServerUpgradeStatus,
  getXrayVersions,
  runServerCommand,
  type AgentUpgradeStatus,
  type CommandDispatchResult,
  type ServerItem,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import { compareVersion } from '@/utils/version'
import { formatDateTime } from '@/utils/timezone'
import TipIcon from '@/components/base/TipIcon.vue'

type VersionSubject = 'agent' | 'xray'

const props = defineProps<{
  server: ServerItem
  active: boolean
}>()

const emit = defineEmits<{
  (e: 'changed'): void
}>()

const subjectOptions: SegmentedOption[] = [
  { id: 'agent', text: 'Agent' },
  { id: 'xray', text: 'Xray 内核' },
]

const subject = ref<VersionSubject>('agent')
const options = ref<string[]>([])
const optionsLoading = ref(false)
const hint = ref('')
const target = ref('')
const switching = ref(false)
const loaded = ref(false)

const upgradeStatus = ref<AgentUpgradeStatus | null>(null)
const upgradeKind = ref<VersionSubject>('agent')
const upgradeIsRollback = ref(false)
const rollbackTarget = ref('')
let pollTimer: ReturnType<typeof setInterval> | null = null

function normVersionTag(v: string) {
  const s = (v || '').trim()
  if (!s) return ''
  return s.startsWith('v') ? s : `v${s}`
}

const subjectLabel = computed(() => (subject.value === 'agent' ? 'Agent' : 'Xray 内核'))

const current = computed(() =>
  normVersionTag(
    subject.value === 'agent' ? props.server.agent_version || '' : props.server.xray_version || '',
  ),
)

const versionCompare = computed(() => {
  if (!current.value || !target.value) return null
  return compareVersion(current.value, target.value)
})

const actionLabel = computed(() => {
  if (!target.value) return '切换'
  if (!current.value) return '安装'
  if (versionCompare.value === 0) return '重新安装'
  return (versionCompare.value ?? 0) < 0 ? '升级' : '降级'
})

const selectOptions = computed(() => options.value.map((v) => ({ label: v, value: v })))

const upgradeFrom = computed(() =>
  upgradeKind.value === 'xray' ? props.server.xray_version || '未知' : props.server.agent_version || '未知',
)

const upgradeTo = computed(() => rollbackTarget.value || target.value || '目标版本')

const upgradeActiveStep = computed(() => {
  if (!upgradeStatus.value) return 0
  switch (upgradeStatus.value.phase) {
    case 'starting':
    case 'checking':
      return 0
    case 'downloading':
      return 1
    case 'verifying':
      return 2
    case 'replacing':
    case 'restarting':
      return 3
    case 'success':
      return 4
    case 'failed':
      return 1
    default:
      return 0
  }
})

const steps = computed<StepItem[]>(() => {
  const defs = [
    {
      header: '版本解析',
      details:
        upgradeKind.value === 'xray'
          ? '确认目标内核'
          : upgradeIsRollback.value
            ? '检查指定版本'
            : '检查远端版本',
    },
    { header: '下载资源', details: upgradeKind.value === 'xray' ? 'XTLS Releases' : 'GitHub Releases' },
    { header: '完整性校验', details: 'SHA256 校验' },
    { header: '重启生效', details: upgradeKind.value === 'xray' ? '重启 xray' : '服务重载就绪' },
  ]
  const s = upgradeStatus.value
  const active = upgradeActiveStep.value
  return defs.map((def, i) => {
    let status: StepsStatus = 'not-started'
    if (s) {
      if (s.phase === 'success') status = 'success'
      else if (s.phase === 'failed') {
        status = i < active ? 'success' : i === active ? 'error' : 'not-started'
      } else {
        status = i < active ? 'success' : i === active ? 'in-progress' : 'not-started'
      }
    }
    return { status, header: def.header, details: def.details }
  })
})

const progressAlert = computed(() => {
  const s = upgradeStatus.value
  if (!s) return null
  if (s.phase === 'success') {
    return { type: 'success' as const, header: '切换完成', body: s.message }
  }
  if (s.phase === 'failed') {
    return { type: 'error' as const, header: '切换失败', body: s.error || s.message }
  }
  return { type: 'info' as const, header: '正在切换', body: s.message }
})

const upgradeRunning = computed(
  () => Boolean(upgradeStatus.value) && upgradeStatus.value!.phase !== 'success' && upgradeStatus.value!.phase !== 'failed',
)

const risk = computed(() => versionCompare.value !== null && versionCompare.value > 0)

function defaultTarget() {
  const latest = options.value[0]
  if (latest && (!current.value || compareVersion(current.value, latest) < 0)) return normVersionTag(latest)
  return current.value
}

async function loadOptions(refresh = false) {
  optionsLoading.value = true
  try {
    const { data } =
      subject.value === 'agent' ? await getAgentVersions(refresh) : await getXrayVersions(refresh)
    if (data.code === 0) {
      options.value = data.data.versions || []
      hint.value = data.data.checked_at
        ? `版本列表更新于 ${formatDateTime(data.data.checked_at)}（只列最近版本，更老的版本可直接输入）`
        : '可直接输入版本号'
    }
  } catch (e) {
    hint.value = `版本列表获取失败，可直接手动输入（${errMsg(e, '网络异常')}）`
  } finally {
    optionsLoading.value = false
  }
}

async function loadForSubject() {
  await loadOptions(false)
  target.value = defaultTarget()
}

function onSubjectChange(value: string | number) {
  const next = value as VersionSubject
  if (next === subject.value) return
  subject.value = next
  target.value = ''
  void loadForSubject()
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

async function pollStatus() {
  try {
    const { data } = await getServerUpgradeStatus(props.server.id)
    if (data?.data?.status) {
      upgradeStatus.value = data.data.status
      if (data.data.status.phase === 'success') {
        stopPolling()
        emit('changed')
      } else if (data.data.status.phase === 'failed') {
        stopPolling()
      }
    }
  } catch {
    /* 忽略轮询抖动 */
  }
}

function startPolling() {
  stopPolling()
  pollTimer = setInterval(() => {
    void pollStatus()
  }, 1500)
}

async function apply() {
  const t = normVersionTag(target.value)
  if (!t || !/^v?\d+(\.\d+)+(-[0-9A-Za-z.-]+)?$/.test(t)) {
    ElMessage.error('版本号格式不正确（示例：v0.1.19 / v26.10.21）')
    return
  }
  if (props.server.status !== 1) {
    ElMessage.warning('服务器离线，无法切换版本')
    return
  }

  const cmp = versionCompare.value
  const label = actionLabel.value
  const kind = subject.value
  const lines = [
    `服务器：${props.server.name}`,
    `当前 ${subjectLabel.value}：${current.value || '未知（旧 Agent 未上报）'}`,
    `目标 ${subjectLabel.value}：${t}`,
    '',
  ]
  if (kind === 'agent') {
    if (cmp !== null && cmp > 0) {
      lines.push('⚠️ 降级风险：更老的 Agent 可能不认识主控新下发的协议消息（未知消息会被静默丢弃），请确认主控与之兼容。')
    }
    lines.push('将从 XPanel-Node Releases 下载 Agent 二进制并做 sha256 校验，完成后服务自动重启（短暂离线）。')
  } else if (cmp !== null && cmp > 0) {
    lines.push('⚠️ 降级风险：Xray ≥26.9.8 起 REALITY 强制要求 MLKEM keyshare，更老的内核也可能无法加载当前配置。')
    lines.push('安装失败会自动回滚到原内核；期间 xray 会短暂中断。')
  } else {
    lines.push('节点将从 XTLS/Xray-core Releases 下载并做 SHA-256 校验，完成后重启 xray（短暂中断）。')
    lines.push('geoip.dat / geosite.dat 会一并同步为该发布包内的数据。')
  }

  try {
    await ElMessageBox.confirm(lines.join('\n'), `确认${label}${subjectLabel.value}`, {
      type: 'warning',
      confirmButtonText: `立即${label}`,
      cancelButtonText: '取消',
    })
  } catch {
    return
  }

  upgradeKind.value = kind
  upgradeIsRollback.value = cmp !== null && cmp > 0
  rollbackTarget.value = t
  upgradeStatus.value = {
    phase: 'starting',
    target: t,
    message: `正在向服务器下发${subjectLabel.value}${label}指令...`,
    ts: Math.floor(Date.now() / 1000),
    kind,
  }
  switching.value = true
  startPolling()

  try {
    const force = cmp === null || cmp >= 0
    // 与批量共用同一个指令端点：这里只是 ids 长度为 1。派发后立即返回，
    // 真实进度由上面的 1.5s 轮询（GET upgrade-status）补齐，不再同步等 5 分钟。
    const { data } = await runServerCommand<CommandDispatchResult>({
      ids: [props.server.id],
      type: kind === 'agent' ? 'upgrade_agent' : 'upgrade_xray',
      target: t,
      force,
    })
    if (data.code !== 0) {
      upgradeStatus.value = {
        phase: 'failed',
        target: t,
        message: `${subjectLabel.value}切换请求被拒绝`,
        error: data.message || `${subjectLabel.value}切换失败`,
        ts: Math.floor(Date.now() / 1000),
        kind,
      }
      stopPolling()
      return
    }
    const skip = data.data.skipped?.[0]
    if (skip) {
      // 预检跳过（离线 / 已是最新 / 不支持在线回滚）：本次没有下发，按失败呈现原因
      upgradeStatus.value = {
        phase: 'failed',
        target: t,
        message: skip.reason,
        error: skip.reason,
        ts: Math.floor(Date.now() / 1000),
        kind,
      }
      stopPolling()
      return
    }
    // 已派发：保持 starting，由轮询写入真实阶段
  } catch (e: any) {
    upgradeStatus.value = {
      phase: 'failed',
      target: t,
      message: `${subjectLabel.value}切换请求超时或网络异常`,
      error: errMsg(e, '切换失败（服务器可能仍在后台下载，可稍后刷新查看版本）'),
      ts: Math.floor(Date.now() / 1000),
      kind,
    }
    stopPolling()
  } finally {
    switching.value = false
  }
}

watch(
  () => props.active,
  (active) => {
    if (active && !loaded.value) {
      loaded.value = true
      void loadForSubject()
    }
  },
  { immediate: true },
)

onUnmounted(stopPolling)
</script>

<template>
  <div class="server-tab-stack">
    <CsContainer header="版本切换">
      <template #info>
        <TipIcon content="Agent 与内核版本相互独立，切换其中一个不会改动另一个。" />
      </template>
      <template #actions>
        <CsSegmentedControl :model-value="subject" :options="subjectOptions" label="版本对象" @update:model-value="onSubjectChange" />
      </template>

      <div class="server-version-flow">
        <span class="server-version-label">当前</span>
        <CsBadge color="grey">{{ current || '未知（旧 Agent 未上报）' }}</CsBadge>
        <span class="server-version-arrow">→</span>
        <span class="server-version-label">目标</span>
        <CsBadge :color="risk ? 'severity-medium' : 'blue'">{{ target || '未选择' }}</CsBadge>
      </div>

      <CsForm>
        <CsFormItem label="目标版本" :description="hint">
          <CsSelect
            :model-value="target"
            :options="selectOptions"
            filterable
            allow-create
            :loading="optionsLoading"
            :placeholder="
              subject === 'agent' ? '选择版本，或直接输入（如 v0.1.19）' : '选择版本，或直接输入（如 v26.10.21）'
            "
            @update:model-value="(v: string | number | (string | number)[] | '') => (target = String(v ?? ''))"
          />
        </CsFormItem>
      </CsForm>

      <CsAlert
        v-if="risk"
        type="warning"
        :header="`降级${subjectLabel}有风险`"
        class="server-version-alert"
      >
        {{
          subject === 'agent'
            ? '更老的 Agent 可能不认识主控新下发的协议消息（未知消息会被静默丢弃），请确认主控与之兼容。服务重启期间节点短暂离线。'
            : 'Xray ≥26.9.8 起 REALITY 强制要求 MLKEM keyshare，更老的内核也可能无法加载当前配置。安装失败会自动回滚，期间 xray 短暂中断。'
        }}
      </CsAlert>
      <CsAlert
        v-else
        type="info"
        :header="`将${actionLabel} ${subjectLabel}`"
        class="server-version-alert"
      >
        {{
          subject === 'agent'
            ? '节点从 XPanel-Node Releases 下载并做 sha256 校验，完成后服务自动重启。'
            : '节点从官方 Releases 下载并做 SHA-256 校验；内核与规则库（geoip/geosite）一起切换。'
        }}
      </CsAlert>

      <div class="server-version-actions">
        <CsButton :loading="optionsLoading" @click="loadOptions(true)">刷新版本列表</CsButton>
        <CsButton
          variant="primary"
          :loading="switching"
          :disabled="!target || server.status !== 1"
          @click="apply"
        >{{ actionLabel }}</CsButton>
      </div>
      <p v-if="server.status !== 1" class="server-version-hint">服务器离线，无法切换版本。</p>
    </CsContainer>

    <CsContainer v-if="upgradeStatus" header="切换进度">
      <div class="server-version-flow">
        <span class="server-version-label">版本流转</span>
        <CsBadge color="grey">{{ upgradeFrom }}</CsBadge>
        <span class="server-version-arrow">→</span>
        <CsBadge :color="upgradeIsRollback ? 'severity-medium' : 'green'">{{ upgradeTo }}</CsBadge>
      </div>

      <CsSteps :steps="steps" orientation="horizontal" aria-label="版本切换进度" class="server-version-steps" />

      <CsAlert
        v-if="progressAlert"
        :type="progressAlert.type"
        :header="progressAlert.header"
        class="server-version-alert"
      >{{ progressAlert.body }}</CsAlert>

      <p v-if="upgradeRunning" class="server-version-hint">
        服务器正在后台执行操作，通常耗时 10–60 秒；关闭弹窗会停止进度轮询，但不会中断升级。
      </p>
    </CsContainer>
  </div>
</template>

<style scoped lang="scss">
.server-tab-stack {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.server-version-label {
  color: var(--color-text-body-secondary-66ygbb, #656871);
  font-size: 12.5px;
  flex: none;
}

.server-version-flow {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  margin-bottom: 16px;
}

.server-version-arrow {
  color: var(--color-text-body-secondary-66ygbb, #656871);
  font-weight: 600;
}

.server-version-alert {
  margin-top: 14px;
}

.server-version-actions {
  display: flex;
  flex-wrap: wrap;
  justify-content: flex-end;
  gap: 8px;
  margin-top: 16px;
}

.server-version-hint {
  margin: 10px 0 0;
  font-size: 12.5px;
  line-height: 1.5;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.server-version-steps {
  margin-bottom: 4px;
}
</style>
