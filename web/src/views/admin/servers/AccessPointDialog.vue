<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import {
  CsAlert,
  CsButton,
  CsColumnLayout,
  CsContainer,
  CsForm,
  CsFormItem,
  CsInput,
  CsInputNumber,
  CsModal,
  CsSegmentedControl,
  CsSelect,
  CsSpaceBetween,
  CsToggle,
  ElMessage,
  ElMessageBox,
  type SegmentedOption,
} from '@/components/cloudscape'
import {
  createAccessPoint,
  updateAccessPoint,
  deleteAccessPoint,
  getServers,
  getInbounds,
  getPermissionGroups,
  type ServerItem,
  type InboundItem,
  type PermissionGroup,
  type UserAccessPoint,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import TipIcon from '@/components/base/TipIcon.vue'

const props = withDefaults(
  defineProps<{
    modelValue: boolean
    accessPoint?: UserAccessPoint | null
    servers?: ServerItem[]
    inbounds?: InboundItem[]
    permissionGroups?: PermissionGroup[]
  }>(),
  {
    accessPoint: null,
    servers: () => [],
    inbounds: () => [],
    permissionGroups: () => [],
  },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'saved'): void
  (e: 'deleted'): void
}>()

const visible = computed({
  get: () => props.modelValue,
  set: (v) => emit('update:modelValue', v),
})

const isEditing = computed(() => !!props.accessPoint)

const localServers = ref<ServerItem[]>([])
const localInbounds = ref<InboundItem[]>([])
const localGroups = ref<PermissionGroup[]>([])

const effectiveServers = computed(() =>
  props.servers && props.servers.length ? props.servers : localServers.value,
)
const effectiveInbounds = computed(() =>
  props.inbounds && props.inbounds.length ? props.inbounds : localInbounds.value,
)
const effectiveGroups = computed(() =>
  props.permissionGroups && props.permissionGroups.length ? props.permissionGroups : localGroups.value,
)

const saving = ref(false)
const targetServerId = ref(0)
const form = reactive({
  name: '',
  enabled: true,
  permission_group_ids: [] as number[],
  remark: '',
  custom_host: '',
  custom_port: 0,
  target_type: '' as '' | 'inbound',
  target_inbound_id: undefined as number | undefined,
})

const availableInbounds = computed(() =>
  effectiveInbounds.value.filter(
    (i) => i.server_id === targetServerId.value && i.enabled,
  ),
)

const selectedInbound = computed(() =>
  effectiveInbounds.value.find((i) => i.id === form.target_inbound_id),
)

const groupOptions = computed(() => effectiveGroups.value.map((g) => ({ label: g.name, value: g.id })))
const serverOptions = computed(() =>
  effectiveServers.value.map((s) => ({ label: `${s.name} (${s.host})`, value: s.id })),
)
const inboundOptions = computed(() =>
  availableInbounds.value.map((i) => ({ label: inboundOptionLabel(i), value: i.id })),
)
const targetTypeOptions: SegmentedOption[] = [
  { id: '', text: '待连线 / 未绑定' },
  { id: 'inbound', text: '直连落地入站' },
]

function inboundOptionLabel(inb: InboundItem): string {
  if (inb.type === 'tunnel' || inb.type === 'forward' || inb.type === 'channel' || inb.protocol === 'dokodemo-door') {
    return `[四层直通] ${inb.tag} (:${inb.port})`
  }
  if (inb.protocol === 'hysteria') {
    return `[Hysteria 2] ${inb.tag} (:${inb.port})`
  }
  const proto = (inb.protocol || 'vless').toUpperCase()
  return `[${proto}] ${inb.tag} (:${inb.port})`
}

// CsSelect / CsSegmentedControl 的 v-model 类型是 string | number，这里显式收窄回表单类型。
type SelectValue = string | number | (string | number)[] | ''

function onPermissionGroupsChange(value: SelectValue) {
  const list = Array.isArray(value) ? value : value === '' ? [] : [value]
  form.permission_group_ids = list.map((v) => Number(v))
}

function onTargetServerChange(value: SelectValue) {
  const raw = Array.isArray(value) ? value[0] : value
  const id = Number(raw)
  targetServerId.value = Number.isFinite(id) ? id : 0
  form.target_inbound_id = undefined
}

function onTargetInboundChange(value: SelectValue) {
  const raw = Array.isArray(value) ? value[0] : value
  const id = Number(raw)
  form.target_inbound_id = Number.isFinite(id) && id > 0 ? id : undefined
}

function onTargetTypeChange(detail: { selectedId: string | number }) {
  form.target_type = detail.selectedId === 'inbound' ? 'inbound' : ''
}

async function loadAuxDataIfNeeded() {
  const tasks: Promise<any>[] = []
  if (!props.servers?.length && !localServers.value.length) {
    tasks.push(
      getServers().then((res) => {
        if (res.data.code === 0) localServers.value = res.data.data.items
      }),
    )
  }
  if (!props.inbounds?.length && !localInbounds.value.length) {
    tasks.push(
      getInbounds(undefined).then((res) => {
        if (res.data.code === 0) localInbounds.value = res.data.data.items
      }),
    )
  }
  if (!props.permissionGroups?.length && !localGroups.value.length) {
    tasks.push(
      getPermissionGroups().then((res) => {
        if (res.data.code === 0) localGroups.value = res.data.data.items
      }),
    )
  }
  if (tasks.length) {
    try {
      await Promise.all(tasks)
    } catch {
      /* 辅助信息拉取失败时允许用户继续操作 */
    }
  }
}

function initForm() {
  if (props.accessPoint) {
    const ap = props.accessPoint
    form.name = ap.name
    form.enabled = ap.enabled
    form.permission_group_ids = [...(ap.permission_group_ids || [])]
    form.remark = ap.remark || ''
    form.custom_host = ap.custom_host || ''
    form.custom_port = ap.custom_port || 0
    form.target_type = (ap.target_type || '') as '' | 'inbound'
    form.target_inbound_id = ap.target_inbound_id

    if (ap.target_type === 'inbound' && ap.target_inbound_id) {
      const inb = effectiveInbounds.value.find((i) => i.id === ap.target_inbound_id)
      targetServerId.value = inb?.server_id || effectiveServers.value[0]?.id || 0
    } else {
      targetServerId.value = effectiveServers.value[0]?.id || 0
    }
  } else {
    form.name = ''
    form.enabled = true
    form.permission_group_ids = []
    form.remark = ''
    form.custom_host = ''
    form.custom_port = 0
    form.target_type = ''
    form.target_inbound_id = undefined
    targetServerId.value = effectiveServers.value[0]?.id || 0
  }
}

watch(
  [() => props.modelValue, () => props.accessPoint],
  async ([val]) => {
    if (val) {
      await loadAuxDataIfNeeded()
      initForm()
    }
  },
  { immediate: true },
)

watch(
  () => effectiveServers.value,
  (srvs) => {
    if (!targetServerId.value && srvs.length > 0) {
      targetServerId.value = srvs[0].id
    }
  },
)

async function handleSave() {
  const name = form.name.trim()
  if (!name) {
    ElMessage.warning('请输入接入点名称')
    return
  }
  if (form.target_type === 'inbound' && !form.target_inbound_id) {
    ElMessage.warning('请选择直连落地入站')
    return
  }
  saving.value = true
  try {
    const payload = {
      name,
      enabled: form.enabled,
      permission_group_ids: form.permission_group_ids,
      remark: form.remark,
      custom_host: form.custom_host.trim(),
      custom_port: form.custom_port || 0,
      target_type: form.target_type,
      target_inbound_id: form.target_type === 'inbound' ? form.target_inbound_id : undefined,
    }
    const { data } = props.accessPoint
      ? await updateAccessPoint(props.accessPoint.id, payload)
      : await createAccessPoint(payload)
    if (data.code === 0) {
      ElMessage.success(props.accessPoint ? '接入点已更新' : '接入点已创建')
      visible.value = false
      emit('saved')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存接入点失败'))
  } finally {
    saving.value = false
  }
}

async function handleDelete() {
  if (!props.accessPoint) return
  try {
    await ElMessageBox.confirm(
      `确认删除接入点「${props.accessPoint.name}」？订阅将立即移除该入口。`,
      '删除接入点',
      { type: 'warning', confirmButtonText: '确定删除', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  try {
    const { data } = await deleteAccessPoint(props.accessPoint.id)
    if (data.code === 0) {
      ElMessage.success('接入点已删除')
      visible.value = false
      emit('deleted')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}
</script>

<template>
  <CsModal v-model:visible="visible" :header="isEditing ? '编辑接入点' : '新建接入点'" size="medium">
    <CsForm label-position="top">
      <CsSpaceBetween size="m">
        <CsAlert type="info">
          接入点是面向客户端订阅与分发的入口。定义 Tag 名称与开放权限组即可，连接配置沿拓扑链路自动继承（亦可在下方进行高级覆写）。
        </CsAlert>

        <CsColumnLayout :columns="2">
          <CsFormItem label="接入点 Tag 名称" required>
            <CsInput v-model="form.name" placeholder="如 🇭🇰 香港直连 01, 🇨🇳 广州移动 BGP" />
          </CsFormItem>
          <CsFormItem label="启用状态">
            <CsToggle v-model="form.enabled">{{ form.enabled ? '启用' : '禁用' }}</CsToggle>
          </CsFormItem>
        </CsColumnLayout>

        <CsFormItem label="开放权限组（显式白名单权限控制，勾选可见的权限组）">
          <CsSelect
            :model-value="form.permission_group_ids"
            :options="groupOptions"
            multiple
            collapse-tags
            placeholder="请勾选可见的权限组"
            aria-label="开放权限组"
            @change="onPermissionGroupsChange"
          />
        </CsFormItem>

        <CsFormItem label="目标绑定方式（亦可在拓扑画布上拖拽连线）">
          <CsSegmentedControl
            :model-value="form.target_type"
            :options="targetTypeOptions"
            label="目标绑定方式"
            full-width
            @change="onTargetTypeChange"
          />
        </CsFormItem>

        <CsContainer v-if="form.target_type === 'inbound'">
          <CsSpaceBetween size="m">
            <CsColumnLayout :columns="2">
              <CsFormItem label="目标落地服务器">
                <CsSelect
                  :model-value="targetServerId"
                  :options="serverOptions"
                  placeholder="选择 Xray 服务器"
                  aria-label="目标落地服务器"
                  @change="onTargetServerChange"
                />
              </CsFormItem>
              <CsFormItem label="目标入站 (Target Inbound)">
                <CsSelect
                  :model-value="form.target_inbound_id ?? null"
                  :options="inboundOptions"
                  placeholder="选择目标入站"
                  aria-label="目标入站"
                  @change="onTargetInboundChange"
                />
              </CsFormItem>
            </CsColumnLayout>

            <CsAlert v-if="selectedInbound?.type === 'tunnel'" type="info">
              该接入点连接四层直通管道：订阅将自动穿透下游落地入站的安全参数，并以本前置机的地址与端口对外分发。
            </CsAlert>
          </CsSpaceBetween>
        </CsContainer>

        <CsContainer>
          <template #header>
            <CsSpaceBetween direction="horizontal" size="xs" align-items="center">
              <span>订阅地址覆写</span>
              <TipIcon content="选填；留空沿链路继承：入站分享地址 / 接入层端点。" />
            </CsSpaceBetween>
          </template>

          <CsColumnLayout :columns="2">
            <CsFormItem label="自定义连接 Host">
              <CsInput v-model="form.custom_host" placeholder="留空自动继承" />
            </CsFormItem>
            <CsFormItem label="自定义连接 Port">
              <CsInputNumber
                v-model="form.custom_port"
                :min="0"
                :max="65535"
                placeholder="0 自动继承"
              />
            </CsFormItem>
          </CsColumnLayout>
        </CsContainer>

        <CsFormItem label="备注说明">
          <CsInput v-model="form.remark" placeholder="选填，如 VIP 专享入口" />
        </CsFormItem>
      </CsSpaceBetween>
    </CsForm>

    <template #footer>
      <div class="dialog-foot">
        <div>
          <CsButton v-if="isEditing" variant="danger-outline" @click="handleDelete">删除接入点</CsButton>
        </div>
        <CsSpaceBetween direction="horizontal" size="xs">
          <CsButton @click="visible = false">取消</CsButton>
          <CsButton variant="primary" :loading="saving" @click="handleSave">保存接入点</CsButton>
        </CsSpaceBetween>
      </div>
    </template>
  </CsModal>
</template>

<style scoped lang="scss">
.dialog-foot {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
}
</style>
