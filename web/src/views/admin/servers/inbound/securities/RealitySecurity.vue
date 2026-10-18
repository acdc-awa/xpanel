<script setup lang="ts">
import { ref } from 'vue'
import { Refresh, MagicStick, CopyDocument, ArrowDown, ArrowUp } from '@element-plus/icons-vue'
import { ElMessage } from '@/components/cloudscape'
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
const showAdvanced = ref(false)

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
  ElMessage.success('已随机生成 16 位 Short ID')
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
    <!-- 微卡片 1: 伪装目标与客户端指纹 -->
    <div class="x-micro-card">
      <div class="x-micro-card__head">
        <div class="head-title">
          <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
          <span>目标伪装与 TLS 模拟 (必填核心)</span>
        </div>
        <div class="head-meta">
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
      </div>
      <div class="x-micro-card__body">
        <div class="form-grid-2">
          <el-form-item label="借壳目标域名与端口 (Dest)">
            <el-input
              :model-value="modelValue.dest"
              placeholder="如 www.cloudflare.com:443"
              class="font-mono-num"
              @input="(v: string) => updateField('dest', v)"
            />
          </el-form-item>

          <el-form-item label="伪装 SNI (Server Name)">
            <el-input
              :model-value="modelValue.server_name"
              placeholder="如 www.cloudflare.com"
              class="font-mono-num"
              @input="(v: string) => updateField('server_name', v)"
            />
          </el-form-item>

          <el-form-item label="模拟客户端 TLS 指纹 (Fingerprint)">
            <el-select
              :model-value="fingerprint || 'chrome'"
              style="width: 100%"
              @update:model-value="(v: string) => emit('update:fingerprint', v)"
            >
              <el-option label="Chrome (推荐)" value="chrome" />
              <el-option label="Firefox" value="firefox" />
              <el-option label="Safari" value="safari" />
              <el-option label="Edge" value="edge" />
              <el-option label="360" value="360" />
              <el-option label="QQ" value="qq" />
              <el-option label="Random (随机)" value="random" />
              <el-option label="Randomized (随机打乱)" value="randomized" />
            </el-select>
          </el-form-item>

          <el-form-item label="Short ID (客户端认证短标识)">
            <div style="display: flex; gap: 8px; width: 100%">
              <el-input
                :model-value="modelValue.short_id"
                placeholder="16 位十六进制字符"
                class="font-mono-num"
                @input="(v: string) => updateField('short_id', v)"
              />
              <el-button title="随机生成 Short ID" @click="genShortId">
                <el-icon><Refresh /></el-icon>
              </el-button>
            </div>
          </el-form-item>
        </div>
      </div>
    </div>

    <!-- 微卡片 2: x25519 密钥认证体系 -->
    <div class="x-micro-card">
      <div class="x-micro-card__head">
        <div class="head-title">
          <span class="w-2 h-2 rounded-full bg-indigo-500"></span>
          <span>Xray x25519 密钥对认证</span>
        </div>
        <div class="head-meta">
          <el-button type="primary" plain size="small" :loading="genKeyLoading" @click="fetchRealityKeys">
            <el-icon><MagicStick /></el-icon>&nbsp;重新生成密钥对
          </el-button>
        </div>
      </div>
      <div class="x-micro-card__body">
        <div class="form-grid-2">
          <el-form-item label="服务端私钥 (Private Key)">
            <el-input
              :model-value="modelValue.private_key"
              placeholder="x25519 私钥 (仅服务器持有)"
              class="font-mono-num"
              @input="(v: string) => updateField('private_key', v)"
            >
              <template #append>
                <el-button v-if="modelValue.private_key" @click="copyText(modelValue.private_key, '私钥')">
                  <el-icon><CopyDocument /></el-icon>
                </el-button>
              </template>
            </el-input>
          </el-form-item>

          <el-form-item label="客户端公钥 (Public Key - 自动下发)">
            <el-input
              :model-value="modelValue.public_key"
              placeholder="x25519 公钥 (订阅自动注入)"
              class="font-mono-num"
              @input="(v: string) => updateField('public_key', v)"
            >
              <template #append>
                <el-button v-if="modelValue.public_key" @click="copyText(modelValue.public_key, '公钥')">
                  <el-icon><CopyDocument /></el-icon>
                </el-button>
              </template>
            </el-input>
          </el-form-item>
        </div>
      </div>
    </div>

    <!-- 微卡片 3: 客户端版本与时钟容差 (折叠微调) -->
    <div class="x-micro-card">
      <div
        class="x-micro-card__head cursor-pointer select-none"
        @click="showAdvanced = !showAdvanced"
      >
        <div class="head-title">
          <span class="w-2 h-2 rounded-full bg-amber-500"></span>
          <span>客户端版本与时间漂移容差 (选填微调)</span>
        </div>
        <div class="head-meta flex items-center gap-1 text-indigo-500">
          <span>{{ showAdvanced ? '收起配置' : '展开 3 项调优' }}</span>
          <el-icon><component :is="showAdvanced ? ArrowUp : ArrowDown" /></el-icon>
        </div>
      </div>
      <div v-show="showAdvanced" class="x-micro-card__body">
        <div class="form-grid-3">
          <el-form-item label="最低版本限制 (minClientVer)">
            <el-input
              :model-value="modelValue.min_client_ver"
              placeholder="留空不限制，如 1.8.0"
              class="font-mono-num"
              @input="(v: string) => updateField('min_client_ver', v)"
            />
          </el-form-item>
          <el-form-item label="最高版本限制 (maxClientVer)">
            <el-input
              :model-value="modelValue.max_client_ver"
              placeholder="留空不限制"
              class="font-mono-num"
              @input="(v: string) => updateField('max_client_ver', v)"
            />
          </el-form-item>
          <el-form-item label="最大允许时钟差 (ms)">
            <el-input-number
              :model-value="modelValue.max_time_diff"
              :min="0"
              style="width: 100%"
              placeholder="0 = 不限制"
              class="font-mono-num"
              @update:model-value="(v: any) => updateField('max_time_diff', Number(v || 0))"
            />
          </el-form-item>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.reality-box {
  display: flex;
  flex-direction: column;
}

:deep(.el-form-item) {
  margin-bottom: 0;
}
</style>
