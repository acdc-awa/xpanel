<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  ElMessage,
  CsButton,
  CsForm,
  CsFormItem,
  CsModal,
  CsSelect,
} from '@/components/cloudscape'
import { updateInbound } from '@/api/admin'
import type { InboundItem } from '@/api/types'
import { errMsg } from '@/api/http'

const props = defineProps<{
  visible: boolean
  layerId: number
  /** 本机尚未挂到任何对外层的入站（由父组件按服务器过滤）。 */
  candidates: InboundItem[]
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'changed'): void
}>()

const visibleModel = computed({
  get: () => props.visible,
  set: (v: boolean) => emit('update:visible', v),
})

const inboundId = ref<number | undefined>(undefined)
const saving = ref(false)

const options = computed(() =>
  props.candidates.map((inb) => ({ label: `${inb.tag} (:${inb.port})`, value: inb.id }))
)

watch(
  () => props.visible,
  (open) => {
    if (open) inboundId.value = undefined
  }
)

async function confirm() {
  if (!inboundId.value) {
    ElMessage.warning('请选择要挂载的入站')
    return
  }
  saving.value = true
  try {
    const { data } = await updateInbound(inboundId.value, { layer_id: props.layerId })
    if (data.code === 0) {
      ElMessage.success('入站已挂载到对外层')
      visibleModel.value = false
      emit('changed')
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '挂载失败'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <CsModal v-model:visible="visibleModel" header="挂载入站到对外层" size="medium">
    <CsForm label-position="top">
      <CsFormItem label="选择本机未挂层入站">
        <CsSelect v-model="inboundId" :options="options" placeholder="选择入站" />
      </CsFormItem>
      <p class="dlg-hint">
        挂载后订阅对外端点由该层决议（host/port/security）；分流路径取入站传输设置里的 xhttp 路径，反代按同一路径转发到该入站端口。
      </p>
    </CsForm>

    <template #footer>
      <CsButton @click="visibleModel = false">取消</CsButton>
      <CsButton variant="primary" :loading="saving" @click="confirm">挂载</CsButton>
    </template>
  </CsModal>
</template>

<style scoped>
.dlg-hint {
  margin: 0;
  font-size: 12.5px;
  line-height: 18px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}
</style>
