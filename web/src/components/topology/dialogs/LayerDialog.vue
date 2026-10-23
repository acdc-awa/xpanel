<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import {
  ElMessage,
  CsAlert,
  CsButton,
  CsForm,
  CsFormItem,
  CsInput,
  CsInputNumber,
  CsModal,
  CsSelect,
} from '@/components/cloudscape'
import { createLayer, updateLayer } from '@/api/admin'
import type { AccessLayer } from '@/api/types'
import { errMsg } from '@/api/http'

const props = defineProps<{
  visible: boolean
  serverId: number
  layer: AccessLayer | null
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'changed'): void
  (e: 'delete', layer: AccessLayer): void
}>()

const visibleModel = computed({
  get: () => props.visible,
  set: (v: boolean) => emit('update:visible', v),
})

const SECURITY_OPTIONS = [
  { label: 'TLS（反代/CDN 卸载，默认）', value: 'tls' },
  { label: '明文 none', value: 'none' },
]

const saving = ref(false)
const form = reactive({
  name: '',
  host: '',
  port: 443,
  security: 'tls',
  remark: '',
})

watch(
  () => props.visible,
  (open) => {
    if (!open) return
    const layer = props.layer
    form.name = layer?.name ?? ''
    form.host = layer?.host ?? ''
    form.port = layer?.port ?? 443
    form.security = layer?.security ?? 'tls'
    form.remark = layer?.remark ?? ''
  }
)

async function save() {
  if (!form.name.trim() || !form.host.trim()) {
    ElMessage.warning('请填写层名称与对外 Host')
    return
  }
  saving.value = true
  try {
    const payload = {
      name: form.name.trim(),
      host: form.host.trim(),
      port: form.port || 443,
      security: form.security,
      remark: form.remark,
    }
    const { data } = props.layer
      ? await updateLayer(props.serverId, props.layer.id, payload)
      : await createLayer(props.serverId, payload)
    if (data.code === 0) {
      ElMessage.success(props.layer ? '对外层已更新' : '对外层已创建')
      visibleModel.value = false
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

function requestDelete() {
  if (props.layer) emit('delete', props.layer)
}
</script>

<template>
  <CsModal
    v-model:visible="visibleModel"
    :header="layer ? '编辑对外接入层' : '新建对外接入层'"
    size="medium"
  >
    <CsForm label-position="top">
      <CsAlert type="info">
        对外接入层定义订阅对外的 host/port/安全层。挂载的入站在订阅与画布中一律消费本层端点；内部实现（直连
        TLS / Caddy 反代 / CDN）对客户端不可见。未挂层入站沿用自身分享地址（直连端点）。
      </CsAlert>

      <div class="dlg-grid dlg-grid-2-1">
        <CsFormItem label="层名称" required>
          <CsInput v-model="form.name" placeholder="如 HK 443 反代层" />
        </CsFormItem>
        <CsFormItem label="对外安全层">
          <CsSelect v-model="form.security" :options="SECURITY_OPTIONS" />
        </CsFormItem>
      </div>

      <div class="dlg-grid dlg-grid-2-1">
        <CsFormItem label="对外 Host" required>
          <CsInput v-model="form.host" placeholder="域名或 IP，如 hk.edge.example.com" />
        </CsFormItem>
        <CsFormItem label="对外 Port">
          <CsInputNumber v-model="form.port" :min="1" :max="65535" />
        </CsFormItem>
      </div>

      <CsFormItem label="备注说明">
        <CsInput v-model="form.remark" placeholder="选填，如 Caddy 前置 / CDN / 直连域名" />
      </CsFormItem>
    </CsForm>

    <template #footer>
      <div class="dlg-footer">
        <div>
          <CsButton v-if="layer" variant="danger-outline" @click="requestDelete">删除对外层</CsButton>
        </div>
        <div class="dlg-footer-right">
          <CsButton @click="visibleModel = false">取消</CsButton>
          <CsButton variant="primary" :loading="saving" @click="save">保存</CsButton>
        </div>
      </div>
    </template>
  </CsModal>
</template>

<style scoped>
.dlg-grid {
  display: grid;
  gap: 0 16px;
}
.dlg-grid-2-1 {
  grid-template-columns: 2fr 1fr;
}
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
@media (max-width: 640px) {
  .dlg-grid-2-1 {
    grid-template-columns: 1fr;
  }
}
</style>
