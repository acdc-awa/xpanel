<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import {
  CsAlert,
  CsButton,
  CsContainer,
  CsDatePicker,
  CsExpandableSection,
  CsForm,
  CsFormItem,
  CsInput,
  CsInputNumber,
  CsSelect,
  toast,
  messageBox,
} from '@/components/cloudscape'
import TipIcon from '@/components/base/TipIcon.vue'
import { deleteServer, resetServerSecret, updateServer, type NicQuotaDirection, type ServerItem } from '@/api/admin'
import { errMsg } from '@/api/http'

// 设置页只做编辑：名称/地址/地区/备注、计费与流量周期、节点通信凭据、危险操作。
// 保存动作由弹窗 footer 承载（ServerSettingsModal），所以这里不放大按钮 ——
// 否则按钮挂在某一张卡上，会让人误以为只保存那张卡的字段。

const props = defineProps<{
  server: ServerItem
  active: boolean
}>()

const emit = defineEmits<{
  (e: 'changed'): void
  (e: 'removed'): void
}>()

// 计费周期是流量周期的唯一来源：只允许固定档位，保证「计费周期 + 到期日」的推导可预测。
const baseBillingOptions = [
  { label: '月付', value: '月付' },
  { label: '季付', value: '季付' },
  { label: '半年付', value: '半年付' },
  { label: '年付', value: '年付' },
  { label: '两年付', value: '两年付' },
  { label: '三年付', value: '三年付' },
  { label: '一次性 / 永久', value: '一次性' },
]

const quotaDirectionOptions = [
  { label: '双向合计', value: 'total' },
  { label: '仅下行（Rx）', value: 'rx' },
  { label: '仅上行（Tx）', value: 'tx' },
  { label: '双向取较大值', value: 'max' },
]

// 流量重置周期：与计费周期解耦（「年付但每月重置流量」是常见组合）。
// 空串 = 跟随计费周期；其余取值与后端 services.NICResetPeriodValues 一一对应。
const resetPeriodOptions = [
  { label: '跟随计费周期', value: '' },
  { label: '每月（按到期日）', value: '1m' },
  { label: '每季（按到期日）', value: '3m' },
  { label: '每半年（按到期日）', value: '6m' },
  { label: '自然月（每月 1 日）', value: 'natural_month' },
]

const form = reactive({
  name: '',
  host: '',
  location: '',
  remark: '',
  expire_at: '' as string | null,
  billing_cycle: '',
  price: '',
  idc_address: '',
  nic_reset_period: '',
  nic_cycle_start: '' as string | null,
  nic_cycle_days: 0 as number | null,
  nic_quota_gb: 0 as number | null,
  nic_quota_direction: 'total' as NicQuotaDirection,
})

// 旧数据可能存着非枚举的自定义周期：作为「旧值」保留在选项里，避免编辑时被静默清空。
const billingOptions = computed(() => {
  const current = form.billing_cycle.trim()
  if (current && !baseBillingOptions.some((o) => o.value === current)) {
    return [...baseBillingOptions, { label: `${current}（旧值，请改选）`, value: current }]
  }
  return baseBillingOptions
})

/** 已有自定义周期配置的机器，打开设置页时直接展开高级区，避免「看不见的生效配置」。 */
const hasCustomCycle = computed(
  () => Boolean(props.server.nic_cycle_start) || Number(props.server.nic_cycle_days ?? 0) > 0,
)

/** 选了预设周期 → 清空高级里的「自定义起点 + 天数」，界面上只留一套生效配置。 */
function setResetPeriod(v: string) {
  form.nic_reset_period = v
  if (v) {
    form.nic_cycle_start = null
    form.nic_cycle_days = 0
  }
}

// 反向互斥：只要用户真的填了自定义起点/天数，就清掉预设周期。
// 用「与快照比对」而不是单纯的 watch，避免打开存量配置时把库里的值顺手改掉。
watch(
  () => [form.nic_cycle_start, form.nic_cycle_days] as const,
  ([start, days]) => {
    const touched =
      norm(start) !== norm(props.server.nic_cycle_start) ||
      Number(days ?? 0) !== Number(props.server.nic_cycle_days ?? 0)
    if (touched && (norm(start).trim() || Number(days) > 0) && form.nic_reset_period) {
      form.nic_reset_period = ''
    }
  },
)

const errors = reactive({ name: '', host: '' })
const saving = ref(false)

function resetForm() {
  Object.assign(form, {
    name: props.server.name ?? '',
    host: props.server.host ?? '',
    location: props.server.location ?? '',
    remark: props.server.remark ?? '',
    expire_at: props.server.expire_at || null,
    billing_cycle: props.server.billing_cycle ?? '',
    price: props.server.price ?? '',
    idc_address: props.server.idc_address ?? '',
    nic_reset_period: props.server.nic_reset_period ?? '',
    nic_cycle_start: props.server.nic_cycle_start || null,
    nic_cycle_days: props.server.nic_cycle_days ?? 0,
    nic_quota_gb: props.server.nic_quota_gb ?? 0,
    nic_quota_direction: props.server.nic_quota_direction ?? 'total',
  })
  errors.name = ''
  errors.host = ''
}

// 弹窗关闭即卸载，重新打开时按 server 重新初始化；页签往返不重置，避免丢掉未保存的输入。
watch(() => props.server.id, resetForm, { immediate: true })

function norm(v: unknown) {
  return v === undefined || v === null ? '' : String(v)
}

/** 表单与服务器快照的差异，用于 footer 的「有未保存的修改」与保存按钮可用性。 */
const dirty = computed(
  () =>
    norm(form.name) !== norm(props.server.name) ||
    norm(form.host) !== norm(props.server.host) ||
    norm(form.location) !== norm(props.server.location) ||
    norm(form.remark) !== norm(props.server.remark) ||
    norm(form.expire_at) !== norm(props.server.expire_at) ||
    norm(form.billing_cycle) !== norm(props.server.billing_cycle) ||
    norm(form.price) !== norm(props.server.price) ||
    norm(form.idc_address) !== norm(props.server.idc_address) ||
    norm(form.nic_reset_period) !== norm(props.server.nic_reset_period) ||
    norm(form.nic_cycle_start) !== norm(props.server.nic_cycle_start) ||
    Number(form.nic_cycle_days ?? 0) !== Number(props.server.nic_cycle_days ?? 0) ||
    Number(form.nic_quota_gb ?? 0) !== Number(props.server.nic_quota_gb ?? 0) ||
    norm(form.nic_quota_direction) !== norm(props.server.nic_quota_direction ?? 'total'),
)

async function save() {
  errors.name = form.name.trim() ? '' : '请填写服务器名称'
  errors.host = form.host.trim() ? '' : '请填写主机地址'
  if (errors.name || errors.host) {
    toast.warning('请先补全服务器名称与主机地址')
    return false
  }
  if ((Number(form.nic_cycle_days) || 0) > 0 && !form.nic_cycle_start) {
    toast.warning('自定义周期天数需要同时填写统计周期起点')
    return false
  }

  saving.value = true
  try {
    const { data } = await updateServer(props.server.id, {
      server_type: 'xray',
      name: form.name,
      host: form.host,
      location: form.location,
      remark: form.remark,
      expire_at: form.expire_at || null,
      billing_cycle: form.billing_cycle,
      price: form.price,
      idc_address: form.idc_address,
      nic_reset_period: form.nic_reset_period,
      nic_cycle_start: form.nic_cycle_start || '',
      nic_cycle_days: Number(form.nic_cycle_days) || 0,
      nic_quota_gb: Number(form.nic_quota_gb) || 0,
      nic_quota_direction: form.nic_quota_direction,
    })
    if (data.code === 0) {
      toast.success('已保存')
      emit('changed')
      return true
    }
    toast.error(data.message)
  } catch (e) {
    toast.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
  return false
}

defineExpose({ save, saving, dirty })

const secretInfo = ref<{ node_id: string; secret: string; install_cmd?: string } | null>(null)
const resetting = ref(false)

async function resetSecret() {
  try {
    await messageBox.confirm(
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
      secretInfo.value = {
        node_id: data.data.node_id,
        secret: data.data.secret,
        install_cmd: data.data.install_cmd,
      }
      toast.success('密钥已重置')
    } else {
      toast.error(data.message)
    }
  } catch (e) {
    toast.error(errMsg(e, '重置失败'))
  } finally {
    resetting.value = false
  }
}

const deleting = ref(false)

async function removeServer() {
  try {
    await messageBox.confirm(
      `确认删除服务器「${props.server.name}」？该服务器将从列表与拓扑中移除，关联的入站、待推送配置与上报数据一并清除，Agent 将无法再连接。历史流量仍保留在统计中并标注「已删除」。`,
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
      toast.success('已删除')
      emit('removed')
    } else {
      toast.error(data.message)
    }
  } catch (e) {
    toast.error(errMsg(e, '删除失败'))
  } finally {
    deleting.value = false
  }
}

const secretItems = computed(() => {
  if (!secretInfo.value) return []
  const items = [
    { key: 'node_id', label: 'node_id', value: secretInfo.value.node_id },
    { key: 'secret', label: 'secret', value: secretInfo.value.secret },
  ]
  if (secretInfo.value.install_cmd) {
    items.push({ key: 'install_cmd', label: '一键安装指令', value: secretInfo.value.install_cmd })
  }
  return items
})

async function copyText(text: string, label: string) {
  try {
    await navigator.clipboard.writeText(text)
    toast.success(`${label}已复制`)
  } catch {
    toast.warning('复制失败，请手动复制')
  }
}
</script>

<template>
  <div class="server-tab-stack">
    <CsContainer header="基础信息">
      <CsForm>
        <div class="server-form-grid">
          <CsFormItem label="服务器名称" required :error="errors.name">
            <CsInput v-model="form.name" placeholder="如 Tokyo-01 / 广州移动 BGP" autocomplete="off" />
          </CsFormItem>
          <CsFormItem label="主机地址 / 域名" required :error="errors.host">
            <CsInput v-model="form.host" placeholder="如 tokyo01.example.com / 120.232.x.x" autocomplete="off" />
          </CsFormItem>
          <CsFormItem label="地区位置">
            <CsInput v-model="form.location" placeholder="如 日本 / 广州（选填）" />
          </CsFormItem>
          <CsFormItem label="管理备注">
            <CsInput v-model="form.remark" placeholder="选填" />
          </CsFormItem>
        </div>
      </CsForm>
    </CsContainer>

    <CsContainer header="计费与流量周期（选填）">
      <template #info>
        <TipIcon content="流量周期默认跟随计费周期（月付=每月、季付=每季…），以到期日为锚点向前推导；流量重置周期单独设置后以它为准（如年付但每月重置流量）。两者都没有或没有到期日时按自然月。每周期流量额度按同一周期计算，0 表示不限。" />
      </template>
      <CsForm>
        <div class="server-form-grid">
          <CsFormItem label="到期日期">
            <CsDatePicker
              v-model="form.expire_at"
              type="date"
              value-format="YYYY-MM-DD"
              placeholder="选择到期日期"
            />
          </CsFormItem>
          <CsFormItem label="计费周期">
            <CsSelect
              :model-value="form.billing_cycle"
              :options="billingOptions"
              filterable
              clearable
              placeholder="选择计费周期"
              @update:model-value="(v: string | number | (string | number)[] | '') => (form.billing_cycle = String(v ?? ''))"
            />
          </CsFormItem>
          <CsFormItem label="续费价格">
            <CsInput v-model="form.price" placeholder="如 ¥35.00 / $24.99 / 120/年" clearable />
          </CsFormItem>
          <CsFormItem label="IDC 控制台地址">
            <CsInput v-model="form.idc_address" placeholder="控制台网址，如 https://..." clearable />
          </CsFormItem>
          <CsFormItem label="流量重置周期">
            <CsSelect
              :model-value="form.nic_reset_period"
              :options="resetPeriodOptions"
              placeholder="选择流量重置周期"
              @update:model-value="(v: string | number | (string | number)[] | '') => setResetPeriod(String(v ?? ''))"
            />
          </CsFormItem>
          <CsFormItem label="每周期流量额度（GB）">
            <CsInputNumber
              v-model="form.nic_quota_gb"
              :min="0"
              :max="10000000"
              :controls="false"
              placeholder="0 = 不限"
            />
          </CsFormItem>
          <CsFormItem label="额度口径">
            <CsSelect
              :model-value="form.nic_quota_direction"
              :options="quotaDirectionOptions"
              @update:model-value="(v: string | number | (string | number)[] | '') => (form.nic_quota_direction = String(v || 'total') as NicQuotaDirection)"
            />
          </CsFormItem>
        </div>

        <CsExpandableSection
          header="自定义统计周期"
          header-description="存量逃生口：填了这里会清空上面的「流量重置周期」，且优先于计费周期的推导"
          class="server-nic-advanced"
          :default-expanded="hasCustomCycle"
        >
          <div class="server-form-grid">
            <CsFormItem label="统计周期起点">
              <CsDatePicker
                v-model="form.nic_cycle_start"
                type="date"
                value-format="YYYY-MM-DD"
                placeholder="留空 = 自动"
              />
            </CsFormItem>
            <CsFormItem label="周期天数">
              <CsInputNumber
                v-model="form.nic_cycle_days"
                :min="0"
                :max="3650"
                :controls="false"
                placeholder="留空 = 按计费周期"
              />
            </CsFormItem>
          </div>
        </CsExpandableSection>
      </CsForm>
    </CsContainer>

    <CsContainer header="节点通信凭据">
      <div class="server-cred-row">
        <div class="server-cred-copy">
          <strong>Node ID</strong>
          <p>Agent 与主控握手时使用的节点标识，随服务器创建生成、重装 Agent 时沿用。</p>
        </div>
        <div class="server-cred-value">
          <span class="cs-text-mono server-wrap">{{ server.node_id }}</span>
          <CsButton
            variant="inline-icon"
            icon-name="copy"
            aria-label="复制 Node ID"
            @click="copyText(server.node_id, 'Node ID')"
          />
        </div>
      </div>

      <div class="server-cred-row">
        <div class="server-cred-copy">
          <strong>重置通信密钥</strong>
          <p>旧密钥立即失效，需要在服务器 /etc/xray-agent/config.yml 中更新 secret 后重启 Agent。</p>
        </div>
        <CsButton variant="danger-outline" :loading="resetting" @click="resetSecret">重置密钥</CsButton>
      </div>

      <div v-if="secretInfo" class="server-secret">
        <CsAlert type="warning" header="新密钥已生成（仅显示这一次）">
          请更新服务器 /etc/xray-agent/config.yml 中的 secret 后重启 xray-agent 服务；或重新执行下方安装命令。
        </CsAlert>
        <div v-for="item in secretItems" :key="item.key" class="server-secret-row">
          <span class="server-cred-label">{{ item.label }}</span>
          <span class="cs-text-mono server-secret-value">{{ item.value }}</span>
          <CsButton
            variant="inline-icon"
            icon-name="copy"
            :aria-label="`复制 ${item.label}`"
            @click="copyText(item.value, item.label)"
          />
        </div>
      </div>
    </CsContainer>

    <CsContainer header="危险操作">
      <div class="server-danger-row">
        <div class="server-cred-copy">
          <strong>删除服务器</strong>
          <p>关联的入站、待推送配置与上报数据将一并删除，该服务器的 Agent 将无法再连接。</p>
        </div>
        <CsButton variant="danger" :loading="deleting" @click="removeServer">删除服务器</CsButton>
      </div>
    </CsContainer>
  </div>
</template>

<style scoped lang="scss">
.server-tab-stack {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.server-form-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px 20px;
}

/* 自定义周期是计费周期推导的逃生口：与上方常规字段留出层级间距。 */
.server-nic-advanced {
  margin-top: 16px;
}

@media (max-width: $bp-tablet-max) {
  .server-form-grid {
    grid-template-columns: 1fr;
  }
}

.server-cred-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 12px 0;
}

.server-cred-row + .server-cred-row,
.server-secret {
  border-top: 1px solid var(--color-border-divider-default-diwd4w, #dedee3);
}

.server-cred-copy {
  min-inline-size: 0;

  strong {
    font-size: 14px;
  }

  p {
    margin: 4px 0 0;
    font-size: 13px;
    line-height: 1.5;
    color: var(--color-text-body-secondary-66ygbb, #656871);
  }
}

.server-cred-value {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-inline-size: 0;
}

.server-cred-label {
  font-size: 13px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.server-wrap {
  overflow-wrap: anywhere;
}

.server-danger-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  padding: 4px 0;
}

.server-secret {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 4px;
  padding-top: 12px;
}

.server-secret-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-inline-size: 0;
}

.server-secret-value {
  overflow-wrap: anywhere;
}
</style>
