<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue'
import {
  ElMessage,
  CsButton,
  CsForm,
  CsFormItem,
  CsInput,
  CsInputNumber,
  CsModal,
} from '@/components/cloudscape'
import { createServerRoutingRule } from '@/api/admin'
import { errMsg } from '@/api/http'

const props = defineProps<{
  visible: boolean
  serverId: number
  inboundTag: string
  outboundTag: string
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'changed'): void
}>()

const visibleModel = computed({
  get: () => props.visible,
  set: (v: boolean) => emit('update:visible', v),
})

const saving = ref(false)
const form = reactive({
  domain: '',
  ip: '',
  protocol: '',
  port: '',
  network: '',
  priority: 0,
})

watch(
  () => props.visible,
  (open) => {
    if (!open) return
    form.domain = ''
    form.ip = ''
    form.protocol = ''
    form.port = ''
    form.network = ''
    form.priority = 0
  }
)

async function save() {
  saving.value = true
  try {
    const { data } = await createServerRoutingRule(props.serverId, {
      inbound_tag: props.inboundTag,
      outbound_tag: props.outboundTag,
      domain: form.domain || undefined,
      ip: form.ip || undefined,
      protocol: form.protocol || undefined,
      port: form.port || undefined,
      network: form.network || undefined,
      priority: form.priority,
    })
    if (data.code === 0) {
      ElMessage.success('路由规则已创建')
      visibleModel.value = false
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '创建规则失败'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <CsModal v-model:visible="visibleModel" header="新建路由规则（拖线）" size="medium">
    <CsForm label-position="top">
      <div class="dlg-grid">
        <CsFormItem label="入站（源）">
          <CsInput :model-value="inboundTag" disabled />
        </CsFormItem>
        <CsFormItem label="出站（目标）">
          <CsInput :model-value="outboundTag" disabled />
        </CsFormItem>
      </div>

      <div class="preset-row">
        <span class="preset-label">快捷填充:</span>
        <CsButton variant="inline-link" @click="form.domain = 'geosite:cn'">国内域名 (geosite:cn)</CsButton>
        <CsButton variant="inline-link" @click="form.ip = 'geoip:cn,geoip:private'">国内/内网 IP</CsButton>
        <CsButton variant="inline-link" @click="form.protocol = 'http,tls'">HTTP/TLS 嗅探</CsButton>
        <CsButton variant="inline-link" @click="form.port = '80,443'">Web 端口 (80,443)</CsButton>
      </div>

      <p class="dlg-hint">匹配字段选填；留空 = 该入站全部流量走目标出站</p>

      <div class="dlg-grid">
        <CsFormItem label="域名匹配">
          <CsInput v-model="form.domain" placeholder="如 google.com（逗号分隔）" />
        </CsFormItem>
        <CsFormItem label="IP 匹配">
          <CsInput v-model="form.ip" placeholder="如 8.8.8.0/24" />
        </CsFormItem>
        <CsFormItem label="协议（嗅探）">
          <CsInput v-model="form.protocol" placeholder="如 tcp,http（逗号分隔）" />
        </CsFormItem>
        <CsFormItem label="端口">
          <CsInput v-model="form.port" placeholder="如 443,80" />
        </CsFormItem>
        <CsFormItem label="网络">
          <CsInput v-model="form.network" placeholder="如 tcp / udp" />
        </CsFormItem>
        <CsFormItem label="优先级">
          <CsInputNumber v-model="form.priority" :min="0" />
        </CsFormItem>
      </div>
    </CsForm>

    <template #footer>
      <CsButton @click="visibleModel = false">取消</CsButton>
      <CsButton variant="primary" :loading="saving" @click="save">创建规则</CsButton>
    </template>
  </CsModal>
</template>

<style scoped lang="scss">
.dlg-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 16px;
}
.dlg-hint {
  margin: 0 0 4px;
  font-size: 12.5px;
  line-height: 18px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}
.preset-row {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 4px 12px;
  margin-bottom: 8px;
}
.preset-label {
  font-size: 12.5px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}
@media (max-width: $bp-phone) {
  .dlg-grid {
    grid-template-columns: 1fr;
  }
}
</style>
