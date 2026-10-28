<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import {
  CsAlert,
  CsButton,
  CsForm,
  CsFormItem,
  CsModal,
  CsSelect,
  CsSpaceBetween,
  ElMessage,
} from '@/components/cloudscape'
import { getXrayVersions } from '@/api/admin'
import { errMsg } from '@/api/http'

// 批量升级 Xray 的版本选择：默认官方最新，可切到最近若干版本（降级/重装）。
// 目标低于某台当前内核时后端会自动置 force；与目标相同的默认跳过。

const props = defineProps<{
  visible: boolean
  count: number
  offlineCount?: number
}>()

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'start', target: string): void
}>()

const target = ref('')
const options = ref<{ label: string; value: string }[]>([])
const loading = ref(false)
const hint = ref('')

const canSubmit = computed(() => /^v?\d+(\.\d+)+(-[0-9A-Za-z.-]+)?$/.test(target.value.trim()))

async function loadVersions() {
  loading.value = true
  try {
    const { data } = await getXrayVersions()
    if (data.code === 0) {
      const versions = data.data.versions || []
      options.value = versions.map((v) => ({ label: v, value: v }))
      target.value = data.data.latest_version || versions[0] || ''
      hint.value = data.data.checked_at
        ? `版本列表更新于 ${data.data.checked_at}（只列最近版本，更老的版本可直接输入）`
        : ''
    }
  } catch (e) {
    hint.value = `版本列表获取失败，可直接手动输入（${errMsg(e, '网络异常')}）`
  } finally {
    loading.value = false
  }
}

watch(
  () => props.visible,
  (visible) => {
    if (!visible) return
    target.value = ''
    options.value = []
    hint.value = ''
    void loadVersions()
  },
)

function close() {
  emit('update:visible', false)
}

function submit() {
  const value = target.value.trim()
  if (!value) {
    ElMessage.warning('请选择或输入目标版本')
    return
  }
  if (!/^v?\d+(\.\d+)+(-[0-9A-Za-z.-]+)?$/.test(value)) {
    ElMessage.error('版本号格式不正确（示例：v26.10.21）')
    return
  }
  emit('start', value)
}
</script>

<template>
  <CsModal :visible="visible" header="批量升级 Xray" size="medium" @update:visible="(v: boolean) => !v && close()">
    <CsSpaceBetween size="m">
      <CsAlert type="info">
        将对 <strong>{{ count }}</strong> 台服务器切换 Xray 内核
        <template v-if="offlineCount">（其中 {{ offlineCount }} 台离线，将自动跳过）</template>。
        目标低于当前内核视为降级；与目标相同的默认跳过，如需重装请改用单台「版本与内核」页。
      </CsAlert>
      <CsForm label-position="top">
        <CsFormItem label="目标版本" :description="hint || undefined">
          <CsSelect
            v-model="target"
            :options="options"
            :loading="loading"
            filterable
            allow-create
            placeholder="选择或输入版本号，如 v26.10.21"
          />
        </CsFormItem>
      </CsForm>
    </CsSpaceBetween>
    <template #footer>
      <CsSpaceBetween direction="horizontal" size="xs">
        <CsButton @click="close">取消</CsButton>
        <CsButton variant="primary" :disabled="!canSubmit" @click="submit">开始升级</CsButton>
      </CsSpaceBetween>
    </template>
  </CsModal>
</template>
