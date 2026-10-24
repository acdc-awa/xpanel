<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { ElMessage, ElMessageBox, CsButton, CsModal } from '@/components/cloudscape'
import {
  createInbound,
  deleteInbound,
  updateInbound,
  type InboundPayload,
} from '@/api/admin'
import type { InboundItem } from '@/api/types'
import { errMsg } from '@/api/http'
import InboundConfigEditor, {
  type InboundEditorChangePayload,
} from '@/views/admin/servers/InboundConfigEditor.vue'

const props = defineProps<{
  visible: boolean
  serverId: number
  inbound: InboundItem | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'changed'): void
}>()

const visibleModel = computed({
  get: () => props.visible,
  set: (v: boolean) => emit('update:visible', v),
})

const formType = ref('user')
const formCertId = ref(0)
const changePayload = ref<InboundEditorChangePayload | null>(null)
const saving = ref(false)

watch(
  () => props.visible,
  (open) => {
    if (!open) return
    formType.value = props.inbound?.type ?? 'user'
    formCertId.value = props.inbound?.cert_id ?? 0
    changePayload.value = null
  }
)

/** 把入库行还原成编辑器认识的 `{tag, port, settings, streamSettings, _panel}` 结构。 */
function editorModelValue(): string {
  const inb = props.inbound
  if (!inb) return '{}'
  let settingsObj: Record<string, unknown> = {}
  let streamObj: Record<string, unknown> = {}
  let sniffObj: Record<string, unknown> | undefined
  try {
    settingsObj = JSON.parse(inb.settings_json || '{}')
    streamObj = JSON.parse(inb.stream_settings || '{}')
    sniffObj = inb.sniffing ? JSON.parse(inb.sniffing) : undefined
  } catch {
    /* 存量行 JSON 非法时按空对象渲染，由编辑器校验兜底 */
  }
  return JSON.stringify({
    tag: inb.tag,
    port: inb.port,
    listen: inb.listen,
    protocol: inb.protocol || 'vless',
    settings: settingsObj,
    streamSettings: streamObj,
    ...(sniffObj ? { sniffing: sniffObj } : {}),
    _panel: {
      remark: inb.remark || '',
      traffic_reset: inb.traffic_reset || 'never',
      flow: inb.flow || '',
      ratio: inb.ratio,
      total_gb: inb.total_gb ?? 0,
      expiry_time: inb.expiry_time ?? null,
      type: inb.type || 'user',
      target_inbound_id: inb.target_inbound_id,
      target_address: inb.target_address,
      target_port: inb.target_port,
      cert_id: inb.cert_id || 0,
      share_addr_strategy: inb.share_addr_strategy,
      share_addr: inb.share_addr,
      share_port: inb.share_port,
      share_sni: inb.share_sni,
      share_allow_insecure: inb.share_allow_insecure,
      layer_id: inb.layer_id || 0,
    },
  })
}

async function save() {
  const c = changePayload.value
  if (!c) {
    ElMessage.warning('请先在表单中编辑入站配置')
    return
  }
  if (!c.tag.trim() || !c.port) {
    ElMessage.warning('请填写标签与端口')
    return
  }
  if (c.validationError) {
    ElMessage.warning(`请先修正配置：${c.validationError}`)
    return
  }
  saving.value = true
  try {
    const payload: Partial<InboundPayload> = {
      server_id: props.serverId,
      tag: c.tag,
      // 这一路径和 nodes.vue 共用同一套终态语义：0 / 空串必须显式送达。
      remark: c.remark ?? '',
      traffic_reset: c.traffic_reset || 'never',
      protocol: c.protocol,
      port: c.port,
      listen: c.listen,
      settings_json: c.settingsJson,
      stream_settings: c.streamSettings,
      sniffing: c.sniffing,
      ratio: c.ratio,
      total_gb: c.total_gb,
      expiry_time: c.expiry_time && c.expiry_time.trim() ? c.expiry_time : null,
      type: c.type || formType.value,
      target_inbound_id: c.targetInboundId ?? 0,
      target_address: c.targetAddress ?? '',
      target_port: c.targetPort ?? 0,
      flow: c.flow,
      cert_id: formCertId.value,
      share_addr_strategy: c.shareAddrStrategy || 'node',
      share_addr: c.shareAddr ?? '',
      share_port: c.sharePort ?? 0,
      share_sni: c.shareSni ?? '',
      share_allow_insecure: c.shareAllowInsecure,
      layer_id: c.layerId ?? 0,
    }
    const { data } = props.inbound
      ? await updateInbound(props.inbound.id, payload)
      : await createInbound(payload as InboundPayload)
    if (data.code === 0) {
      ElMessage.success(props.inbound ? '入站已保存' : '入站已创建')
      visibleModel.value = false
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, props.inbound ? '保存入站失败' : '创建入站失败'))
  } finally {
    saving.value = false
  }
}

async function remove() {
  const inb = props.inbound
  if (!inb) return
  try {
    await ElMessageBox.confirm(`删除入站「${inb.tag}」？`, '删除入站', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deleteInbound(inb.id)
    if (data.code === 0) {
      ElMessage.success('入站已删除')
      visibleModel.value = false
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}
</script>

<template>
  <CsModal
    v-model:visible="visibleModel"
    :header="inbound ? '编辑入站' : '为服务器新建入站'"
    size="large"
    scrollable
    :close-on-click-overlay="false"
  >
    <InboundConfigEditor
      :key="inbound ? `edit-${inbound.id}` : `create-${serverId}`"
      :server-id="serverId"
      :inbound-id="inbound?.id || 0"
      :inbound-type="formType"
      :target-inbound-id="inbound?.target_inbound_id"
      :target-address="inbound?.target_address || ''"
      :target-port="inbound?.target_port || 0"
      :protocol="inbound?.protocol || 'vless'"
      :cert-id="formCertId"
      :model-value="editorModelValue()"
      @change="changePayload = $event"
      @update:inbound-type="formType = $event"
      @update:cert-id="formCertId = $event || 0"
    />

    <template #footer>
      <div class="dlg-footer">
        <div>
          <CsButton v-if="inbound" variant="danger-outline" @click="remove">删除入站</CsButton>
        </div>
        <div class="dlg-footer-right">
          <CsButton @click="visibleModel = false">取消</CsButton>
          <CsButton variant="primary" :loading="saving" @click="save">
            {{ inbound ? '保存入站' : '创建入站' }}
          </CsButton>
        </div>
      </div>
    </template>
  </CsModal>
</template>

<style scoped>
.dlg-footer {
  display: flex;
  justify-content: space-between;
  align-items: center;
  width: 100%;
}
.dlg-footer-right {
  display: flex;
  gap: 8px;
}
</style>
