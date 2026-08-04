<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Check } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import { getSettings, updateSettings } from '@/api/admin'
import { errMsg } from '@/api/http'

const form = reactive({ web_base: '' })
const loading = ref(false)
const saving = ref(false)

async function load() {
  loading.value = true
  try {
    const { data } = await getSettings()
    if (data.code === 0) form.web_base = data.data.web_base
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载设置失败'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

async function save() {
  saving.value = true
  try {
    const { data } = await updateSettings({ web_base: form.web_base })
    if (data.code === 0) {
      ElMessage.success('已保存，刷新页面后按新路径访问')
      form.web_base = data.data.web_base
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="x-page">
    <BaseCard v-loading="loading" style="max-width: 760px">
      <div class="sec-title">站点设置</div>

      <el-form label-position="top" style="max-width: 560px">
        <el-form-item label="Web Base（自定义访问路径前缀）">
          <el-input v-model="form.web_base" placeholder="留空为根路径，如 /panel" />
        </el-form-item>
        <p class="muted tip">
          让面板（管理端 + 用户端 + API + 订阅）挂载在自定义子路径下，例如 https://example.com/panel/。
          留空 = 根路径。保存后立即生效（刷新页面即按新路径加载）。
        </p>
        <p class="muted tip">
          使用子路径时，反向代理需把该前缀（以及 /assets 静态资源）转发到主控；部署说明见 docs/部署指南.md。
        </p>
      </el-form>

      <div class="actions">
        <el-button type="primary" :loading="saving" :icon="Check" @click="save">保存</el-button>
      </div>
    </BaseCard>
  </div>
</template>

<style scoped lang="scss">
.sec-title { font-weight: 600; font-size: 14px; margin-bottom: 16px; }
.tip { font-size: 12.5px; margin: 4px 0; line-height: 1.7; }
.actions { margin-top: 16px; }
</style>