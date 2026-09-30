<script setup lang="ts">
import { ref } from 'vue'
import { Refresh, MagicStick, CopyDocument } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { getXrayKeys } from '@/api/admin'
import { errMsg } from '@/api/http'
import type { RealityConfig } from '../types'

const props = defineProps<{
  modelValue: RealityConfig
  fingerprint?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: RealityConfig): void
  (e: 'update:fingerprint', value: string): void
}>()

const genKeyLoading = ref(false)

const REALITY_PRESETS = [
  { label: 'Cloudflare (www.cloudflare.com:443)', dest: 'www.cloudflare.com:443', sni: 'www.cloudflare.com' },
  { label: 'Amazon AWS (aws.amazon.com:443)', dest: 'aws.amazon.com:443', sni: 'aws.amazon.com' },
  { label: 'Yahoo Japan (www.yahoo.co.jp:443)', dest: 'www.yahoo.co.jp:443', sni: 'www.yahoo.co.jp' },
  { label: 'Docker Registry (registry-1.docker.io:443)', dest: 'registry-1.docker.io:443', sni: 'registry-1.docker.io' },
]

function updateField<K extends keyof RealityConfig>(key: K, val: RealityConfig[K]) {
  emit('update:modelValue', {
    ...props.modelValue,
    [key]: val,
  })
}

function applyPreset(preset: { dest: string; sni: string }) {
  emit('update:modelValue', {
    ...props.modelValue,
    dest: preset.dest,
    server_name: preset.sni,
  })
  ElMessage.success(`已应用目标预设：${preset.sni}`)
}

function genShortId() {
  const bytes = new Uint8Array(8)
  crypto.getRandomValues(bytes)
  const hex = Array.from(bytes)
    .map((b) => b.toString(16).padStart(2, '0'))
    .join('')
  updateField('short_id', hex)
}

async function fetchRealityKeys() {
  genKeyLoading.value = true
  try {
    const { data } = await getXrayKeys()
    if (data.code === 0 && data.data) {
      emit('update:modelValue', {
        ...props.modelValue,
        private_key: data.data.private_key,
        public_key: data.data.public_key,
      })
      ElMessage.success('已生成 x25519 密钥对')
    } else {
      ElMessage.error(data.message || '生成失败')
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '请求后端生成密钥失败'))
  } finally {
    genKeyLoading.value = false
  }
}

function copyText(txt: string, label = '内容') {
  if (!txt) return
  navigator.clipboard.writeText(txt)
  ElMessage.success(`已复制 ${label}`)
}
</script>

<template>
  <div class="reality-box">
    <div class="card-head-flex">
      <span class="sub-title">REALITY 伪装参数</span>
      <el-dropdown trigger="click" @command="applyPreset">
        <el-button size="small" text type="primary">
          <span>常用目标预设</span>
        </el-button>
        <template #dropdown>
          <el-dropdown-menu>
            <el-dropdown-item v-for="p in REALITY_PRESETS" :key="p.dest" :command="p">
              {{ p.label }}
            </el-dropdown-item>
          </el-dropdown-menu>
        </template>
      </el-dropdown>
    </div>

    <div class="form-grid">
      <el-form-item label="借壳目标 (Dest)">
        <el-input
          :model-value="modelValue.dest"
          placeholder="如 www.cloudflare.com:443"
          @input="(v: string) => updateField('dest', v)"
        />
      </el-form-item>

      <el-form-item label="伪装域名 (SNI)">
        <el-input
          :model-value="modelValue.server_name"
          placeholder="如 www.cloudflare.com"
          @input="(v: string) => updateField('server_name', v)"
        />
      </el-form-item>

      <el-form-item label="客户端指纹">
        <el-select
          :model-value="fingerprint || 'chrome'"
          style="width: 100%"
          @update:model-value="(v: string) => emit('update:fingerprint', v)"
        >
          <el-option label="Chrome" value="chrome" />
          <el-option label="Firefox" value="firefox" />
          <el-option label="Safari" value="safari" />
          <el-option label="Edge" value="edge" />
          <el-option label="360" value="360" />
          <el-option label="QQ" value="qq" />
          <el-option label="Random" value="random" />
          <el-option label="Randomized (随机)" value="randomized" />
        </el-select>
      </el-form-item>

      <el-form-item label="Short ID">
        <div style="display: flex; gap: 8px">
          <el-input
            :model-value="modelValue.short_id"
            placeholder="16 位十六进制"
            @input="(v: string) => updateField('short_id', v)"
          />
          <el-button title="重新生成 Short ID" @click="genShortId">
            <el-icon><Refresh /></el-icon>
          </el-button>
        </div>
      </el-form-item>
    </div>

    <div class="key-box">
      <div class="key-row">
        <el-form-item label="私钥 (Private Key)" style="margin-bottom: 0">
          <el-input
            :model-value="modelValue.private_key"
            placeholder="x25519 私钥"
            @input="(v: string) => updateField('private_key', v)"
          />
        </el-form-item>
        <el-form-item label="公钥 (Public Key)" style="margin-bottom: 0">
          <el-input
            :model-value="modelValue.public_key"
            placeholder="x25519 公钥"
            @input="(v: string) => updateField('public_key', v)"
          />
        </el-form-item>
      </div>
      <div class="key-actions">
        <el-button type="primary" plain size="small" :loading="genKeyLoading" @click="fetchRealityKeys">
          <el-icon><MagicStick /></el-icon>&nbsp;生成 x25519 密钥对
        </el-button>
        <el-button
          v-if="modelValue.public_key"
          size="small"
          text
          @click="copyText(modelValue.public_key, '公钥')"
        >
          <el-icon><CopyDocument /></el-icon>&nbsp;复制公钥
        </el-button>
        <el-button
          v-if="modelValue.private_key"
          size="small"
          text
          @click="copyText(modelValue.private_key, '私钥')"
        >
          <el-icon><CopyDocument /></el-icon>&nbsp;复制私钥
        </el-button>
      </div>
    </div>

    <div class="form-grid" style="margin-top: 14px">
      <el-form-item label="最低客户端版本 (minClientVer)">
        <el-input
          :model-value="modelValue.min_client_ver"
          placeholder="留空不限，如 1.8.0"
          @input="(v: string) => updateField('min_client_ver', v)"
        />
      </el-form-item>
      <el-form-item label="最高客户端版本 (maxClientVer)">
        <el-input
          :model-value="modelValue.max_client_ver"
          placeholder="留空不限"
          @input="(v: string) => updateField('max_client_ver', v)"
        />
      </el-form-item>
      <el-form-item label="最大时间差 (ms)">
        <el-input-number
          :model-value="modelValue.max_time_diff"
          :min="0"
          style="width: 100%"
          placeholder="0 = 不限制"
          @update:model-value="(v: any) => updateField('max_time_diff', Number(v || 0))"
        />
      </el-form-item>
    </div>
  </div>
</template>

<style scoped lang="scss">
.card-head-flex {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.sub-title {
  font-size: 13px;
  font-weight: 600;
  color: var(--x-text);
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 18px;

  :deep(.el-form-item) {
    margin-bottom: 14px;
  }
}

.key-box {
  margin-top: 8px;
  background: var(--x-card);
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 14px 16px;
}

.key-row {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 16px;
}

.key-actions {
  display: flex;
  align-items: center;
  gap: 12px;
  margin-top: 10px;
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
  .key-row {
    grid-template-columns: 1fr;
  }
}
</style>
