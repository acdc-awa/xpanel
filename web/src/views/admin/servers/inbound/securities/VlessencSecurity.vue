<script setup lang="ts">
import { ref } from 'vue'
import { MagicStick, CopyDocument } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { getXrayVlessEnc } from '@/api/admin'
import { errMsg } from '@/api/http'

defineProps<{
  modelValue: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
}>()

const veGenLoading = ref(false)

async function fetchVlessencKey() {
  veGenLoading.value = true
  try {
    const { data } = await getXrayVlessEnc()
    if (data.code === 0 && data.data) {
      emit('update:modelValue', data.data.decryption)
      ElMessage.success('已生成 VLESS Encryption 密钥')
    } else {
      ElMessage.error(data.message || '生成失败')
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '生成密钥失败'))
  } finally {
    veGenLoading.value = false
  }
}

function copyText(txt: string, label = '内容') {
  if (!txt) return
  navigator.clipboard.writeText(txt)
  ElMessage.success(`已复制 ${label}`)
}
</script>

<template>
  <div class="vlessenc-box">
    <el-alert
      type="info"
      :closable="false"
      show-icon
      style="margin-bottom: 12px"
      title="VLESS 协议层加密 (ML-KEM-768)"
      description="基于后量子算法的协议内加密方案，适用于节点间私有链路。引用此入站的出站将自动派生解密参数并启用 Vision 流控。"
    />
    <el-form-item label="解密密钥 (decryption)">
      <div style="display: flex; gap: 8px; width: 100%">
        <el-input :model-value="modelValue" readonly placeholder="点击「生成密钥」自动填充" />
        <el-button v-if="modelValue" title="复制密钥" @click="copyText(modelValue, '密钥')">
          <el-icon><CopyDocument /></el-icon>
        </el-button>
        <el-button type="primary" plain :loading="veGenLoading" @click="fetchVlessencKey">
          <el-icon><MagicStick /></el-icon>&nbsp;生成密钥
        </el-button>
      </div>
    </el-form-item>
  </div>
</template>

<style scoped lang="scss">
.vlessenc-box {
  display: flex;
  flex-direction: column;
}
</style>
