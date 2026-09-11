<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
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
    (i) => i.server_id === targetServerId.value && i.enabled && i.type === 'user',
  ),
)

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
  <el-dialog
    v-model="visible"
    :title="isEditing ? '编辑接入点' : '新建接入点'"
    width="580px"
    append-to-body
    destroy-on-close
  >
    <el-form label-position="top">
      <el-alert
        title="接入点是面向客户端订阅与分发的入口。定义 Tag 名称与开放权限组即可，连接配置沿拓扑链路自动继承（亦可在下方进行高级覆写）。"
        type="info"
        :closable="false"
        style="margin-bottom: 16px"
      />

      <div class="ap-row-top">
        <el-form-item label="接入点 Tag 名称" required style="flex: 2; margin-bottom: 0">
          <el-input v-model="form.name" placeholder="如 🇭🇰 香港直连 01, 🇨🇳 广州移动 BGP" />
        </el-form-item>
        <el-form-item label="启用状态" style="flex: 1; margin-bottom: 0">
          <el-switch v-model="form.enabled" active-text="启用" inactive-text="禁用" style="margin-top: 4px" />
        </el-form-item>
      </div>

      <el-form-item label="开放权限组（显式白名单权限控制，勾选可见的权限组）" style="margin-top: 14px">
        <el-select
          v-model="form.permission_group_ids"
          multiple
          collapse-tags
          collapse-tags-tooltip
          placeholder="请勾选可见的权限组"
          style="width: 100%"
        >
          <el-option v-for="g in effectiveGroups" :key="g.id" :label="g.name" :value="g.id" />
        </el-select>
      </el-form-item>

      <el-form-item label="目标绑定方式（亦可在拓扑画布上拖拽连线）">
        <el-radio-group v-model="form.target_type" style="width: 100%">
          <el-radio-button value="">待连线 / 未绑定</el-radio-button>
          <el-radio-button value="inbound">直连落地入站</el-radio-button>
        </el-radio-group>
      </el-form-item>

      <div v-if="form.target_type === 'inbound'" class="inbound-target-box">
        <el-form-item label="目标落地服务器" style="margin-bottom: 0">
          <el-select
            v-model="targetServerId"
            placeholder="选择 Xray 服务器"
            style="width: 100%"
            @change="form.target_inbound_id = undefined"
          >
            <el-option
              v-for="s in effectiveServers"
              :key="s.id"
              :label="`${s.name} (${s.host})`"
              :value="s.id"
            />
          </el-select>
        </el-form-item>
        <el-form-item label="目标用户入站 (Target Inbound)" style="margin-bottom: 0">
          <el-select v-model="form.target_inbound_id" placeholder="选择用户入站" style="width: 100%">
            <el-option
              v-for="inb in availableInbounds"
              :key="inb.id"
              :label="`${inb.tag} (:${inb.port})`"
              :value="inb.id"
            />
          </el-select>
        </el-form-item>
      </div>

      <div class="override-box">
        <div class="override-title">
          订阅地址覆写（选填；留空沿链路继承：入站分享地址 / 接入层端点）
        </div>
        <div class="override-grid">
          <el-form-item label="自定义连接 Host" style="margin-bottom: 0">
            <el-input v-model="form.custom_host" placeholder="留空自动继承" />
          </el-form-item>
          <el-form-item label="自定义连接 Port" style="margin-bottom: 0">
            <el-input-number
              v-model="form.custom_port"
              :min="0"
              :max="65535"
              placeholder="0 自动继承"
              style="width: 100%"
            />
          </el-form-item>
        </div>
      </div>

      <el-form-item label="备注说明" style="margin-bottom: 0">
        <el-input v-model="form.remark" placeholder="选填，如 VIP 专享入口" />
      </el-form-item>
    </el-form>

    <template #footer>
      <div class="dialog-foot">
        <div>
          <el-button v-if="isEditing" type="danger" plain @click="handleDelete">
            删除接入点
          </el-button>
        </div>
        <div style="display: flex; gap: 8px">
          <el-button @click="visible = false">取消</el-button>
          <el-button type="primary" :loading="saving" @click="handleSave">
            保存接入点
          </el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.ap-row-top {
  display: flex;
  gap: 16px;
  align-items: flex-start;
}

.inbound-target-box {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px 16px;
  background: var(--x-card-soft);
  padding: 12px;
  border-radius: 8px;
  margin-bottom: 16px;
  border: 1px dashed var(--x-border);
}

.override-box {
  background: var(--x-card-soft);
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 12px;
  margin-bottom: 16px;

  .override-title {
    font-size: 12px;
    font-weight: 600;
    color: var(--x-text-3);
    margin-bottom: 8px;
  }

  .override-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 12px 16px;
  }
}

.dialog-foot {
  display: flex;
  justify-content: space-between;
  align-items: center;
}

@media (max-width: 640px) {
  .ap-row-top {
    flex-direction: column;
    align-items: stretch;
  }

  .inbound-target-box,
  .override-box .override-grid {
    grid-template-columns: 1fr !important;
  }
}
</style>
