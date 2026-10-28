<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import {
  CsAlert,
  CsButton,
  CsContainer,
  CsDatePicker,
  CsForm,
  CsFormItem,
  CsInput,
  CsSelect,
  ElMessage,
  ElMessageBox,
} from '@/components/cloudscape'
import { deleteServer, resetServerSecret, updateServer, type ServerItem } from '@/api/admin'
import { errMsg } from '@/api/http'

// 设置页只做编辑：名称/地址/地区/备注、VPS 资产、节点通信凭据、危险操作。
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

const billingOptions = [
  { label: '月付', value: '月付' },
  { label: '季付', value: '季付' },
  { label: '半年付', value: '半年付' },
  { label: '年付', value: '年付' },
  { label: '两年付', value: '两年付' },
  { label: '三年付', value: '三年付' },
  { label: '一次性 / 永久', value: '一次性' },
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
})

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
    norm(form.idc_address) !== norm(props.server.idc_address),
)

async function save() {
  errors.name = form.name.trim() ? '' : '请填写服务器名称'
  errors.host = form.host.trim() ? '' : '请填写主机地址'
  if (errors.name || errors.host) {
    ElMessage.warning('请先补全服务器名称与主机地址')
    return
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
    })
    if (data.code === 0) {
      ElMessage.success('已保存')
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

defineExpose({ save, saving, dirty })

const secretInfo = ref<{ node_id: string; secret: string; install_cmd?: string } | null>(null)
const resetting = ref(false)

async function resetSecret() {
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
      secretInfo.value = {
        node_id: data.data.node_id,
        secret: data.data.secret,
        install_cmd: data.data.install_cmd,
      }
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
  try {
    await ElMessageBox.confirm(
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
      ElMessage.success('已删除')
      emit('removed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
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
    ElMessage.success(`${label}已复制`)
  } catch {
    ElMessage.warning('复制失败，请手动复制')
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

    <CsContainer header="VPS 采购与续费资产（选填）">
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
              allow-create
              clearable
              placeholder="选择或输入计费周期"
              @update:model-value="(v: string | number | (string | number)[] | '') => (form.billing_cycle = String(v ?? ''))"
            />
          </CsFormItem>
          <CsFormItem label="续费价格">
            <CsInput v-model="form.price" placeholder="如 ¥35.00 / $24.99 / 120/年" clearable />
          </CsFormItem>
          <CsFormItem label="IDC 控制台地址">
            <CsInput v-model="form.idc_address" placeholder="控制台网址，如 https://..." clearable />
          </CsFormItem>
        </div>
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

@media (max-width: 640px) {
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
    font-size: 13.5px;
  }

  p {
    margin: 4px 0 0;
    font-size: 12.5px;
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
  font-size: 12.5px;
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
