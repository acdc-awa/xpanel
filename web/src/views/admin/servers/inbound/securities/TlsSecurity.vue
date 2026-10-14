<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ArrowDown, ArrowUp } from '@element-plus/icons-vue'
import { getCerts, getCACerts, type CertItem, type CACertItem } from '@/api/admin'
import type { TlsConfig } from '../types'

const props = defineProps<{
  modelValue: TlsConfig
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: TlsConfig): void
  (e: 'update:certId', value: number): void
}>()

const certs = ref<CertItem[]>([])
const caCerts = ref<CACertItem[]>([])
const showAdvanced = ref(false)

async function loadCerts() {
  try {
    const { data } = await getCerts()
    if (data.code === 0 && data.data) {
      certs.value = data.data.items || []
    }
  } catch {
    certs.value = []
  }
}

async function loadCAs() {
  try {
    const { data } = await getCACerts()
    if (data.code === 0 && data.data) {
      caCerts.value = data.data.items || []
    }
  } catch {
    caCerts.value = []
  }
}

onMounted(() => {
  loadCerts()
  loadCAs()
})

function updateField<K extends keyof TlsConfig>(key: K, val: TlsConfig[K]) {
  emit('update:modelValue', {
    ...props.modelValue,
    [key]: val,
  })
}

function onCertChange(val: any) {
  const cid = Number(val || 0)
  updateField('cert_id', cid)
  emit('update:certId', cid)
}
</script>

<template>
  <div class="tls-box">
    <!-- 微卡片 1: 托管证书与 SNI 核心 -->
    <div class="x-micro-card">
      <div class="x-micro-card__head">
        <div class="head-title">
          <span class="w-2 h-2 rounded-full bg-emerald-500"></span>
          <span>服务端 TLS 证书与 SNI 绑定 (必填核心)</span>
        </div>
        <div class="head-meta">
          <span>Xray 标准 TLS 加密</span>
        </div>
      </div>
      <div class="x-micro-card__body">
        <div class="form-grid-2">
          <el-form-item label="面板托管证书">
            <el-select
              :model-value="modelValue.cert_id || undefined"
              style="width: 100%"
              clearable
              placeholder="不绑定（使用本地证书文件路径）"
              @change="onCertChange"
            >
              <el-option v-for="ct in certs" :key="ct.id" :value="ct.id" :label="`${ct.domain}（到期 ${ct.not_after}）`">
                <span>{{ ct.domain }}（到期 {{ ct.not_after }}）</span>
                <span v-if="ct.ca_name" class="x-chip purple" style="margin-left: 6px; font-size: 11px">CA: {{ ct.ca_name }}</span>
                <span v-else-if="ct.self_signed" class="x-chip green" style="margin-left: 6px; font-size: 11px">自签·自动Pin</span>
                <span v-else class="x-chip blue" style="margin-left: 6px; font-size: 11px">外部导入</span>
              </el-option>
            </el-select>
            <div v-if="certs.find(ct => ct.id === modelValue.cert_id)?.self_signed" class="field-hint" style="color: var(--x-success)">
              自签证书：引用本入站的中转出站将自动绑定指纹校验 (SHA-256)，防止中间人攻击
            </div>
          </el-form-item>

          <el-form-item label="SNI (server_name)">
            <el-input
              :model-value="modelValue.server_name"
              placeholder="如 example.com"
              class="font-mono-num"
              @input="(v: string) => updateField('server_name', v)"
            />
          </el-form-item>

          <el-form-item label="ALPN (逗号分隔)">
            <el-input
              :model-value="modelValue.alpn"
              placeholder="h2,http/1.1"
              class="font-mono-num"
              @input="(v: string) => updateField('alpn', v)"
            />
          </el-form-item>

          <el-form-item label="跳过客户端证书校验 (allowInsecure)">
            <div style="padding-top: 4px">
              <el-switch
                :model-value="modelValue.allow_insecure"
                active-text="允许不安全证书 (客户端直连/订阅下发)"
                @update:model-value="(v: any) => updateField('allow_insecure', Boolean(v))"
              />
            </div>
          </el-form-item>

          <el-form-item label="证书文件路径 (Cert File)">
            <el-input
              :model-value="modelValue.cert_file"
              placeholder="/etc/xray/cert.pem"
              class="font-mono-num"
              @input="(v: string) => updateField('cert_file', v)"
            />
          </el-form-item>

          <el-form-item label="私钥文件路径 (Key File)">
            <el-input
              :model-value="modelValue.key_file"
              placeholder="/etc/xray/key.pem"
              class="font-mono-num"
              @input="(v: string) => updateField('key_file', v)"
            />
          </el-form-item>
        </div>
      </div>
    </div>

    <!-- 微卡片 2: 高级安全策略与验签 (折叠) -->
    <div class="x-micro-card">
      <div
        class="x-micro-card__head cursor-pointer select-none"
        @click="showAdvanced = !showAdvanced"
      >
        <div class="head-title">
          <span class="w-2 h-2 rounded-full bg-amber-500"></span>
          <span>高级 TLS 策略、CA 验签与证书锁定 (选填调优)</span>
        </div>
        <div class="head-meta flex items-center gap-1 text-indigo-500">
          <span>{{ showAdvanced ? '收起高级配置' : '展开 6 项高级调优' }}</span>
          <el-icon><component :is="showAdvanced ? ArrowUp : ArrowDown" /></el-icon>
        </div>
      </div>
      <div v-show="showAdvanced" class="x-micro-card__body">
        <div class="form-grid-2">
          <el-form-item label="TLS 协议版本范围">
            <div style="display: flex; gap: 8px; width: 100%">
              <el-select
                :model-value="modelValue.min_version"
                placeholder="最低版本"
                clearable
                style="width: 50%"
                @update:model-value="(v: string) => updateField('min_version', v)"
              >
                <el-option label="TLS 1.2" value="1.2" />
                <el-option label="TLS 1.3" value="1.3" />
              </el-select>
              <el-select
                :model-value="modelValue.max_version"
                placeholder="最高版本"
                clearable
                style="width: 50%"
                @update:model-value="(v: string) => updateField('max_version', v)"
              >
                <el-option label="TLS 1.2" value="1.2" />
                <el-option label="TLS 1.3" value="1.3" />
              </el-select>
            </div>
          </el-form-item>

          <el-form-item label="加密套件 (Cipher Suites)">
            <el-input
              :model-value="modelValue.cipher_suites"
              placeholder="留空默认标准套件"
              class="font-mono-num"
              @input="(v: string) => updateField('cipher_suites', v)"
            />
          </el-form-item>

          <el-form-item label="证书指纹固定 (Pin SHA-256)">
            <el-input
              :model-value="modelValue.pinned_peer_cert_sha256"
              placeholder="64位 Hex 散列"
              class="font-mono-num"
              clearable
              @input="(v: string) => updateField('pinned_peer_cert_sha256', v)"
            />
          </el-form-item>

          <el-form-item label="从托管 CA 选择验签证书">
            <el-select
              placeholder="选择已保存的 CA"
              clearable
              style="width: 100%"
              @change="(id: any) => {
                const ca = caCerts.find(c => c.id === id)
                if (ca) updateField('ca_cert_content', ca.ca_cert_pem)
              }"
            >
              <el-option
                v-for="ca in caCerts"
                :key="ca.id"
                :value="ca.id"
                :label="`${ca.name} (${ca.common_name})`"
              />
            </el-select>
          </el-form-item>

          <el-form-item label="CA 证书内容 (PEM 直接粘贴)">
            <el-input
              :model-value="modelValue.ca_cert_content"
              type="textarea"
              :rows="2"
              class="font-mono-num"
              placeholder="-----BEGIN CERTIFICATE-----..."
              @input="(v: string) => updateField('ca_cert_content', v)"
            />
          </el-form-item>

          <el-form-item label="证书与握手保护">
            <div style="display: flex; flex-direction: column; gap: 8px; padding-top: 4px">
              <el-switch
                :model-value="modelValue.disable_system_root"
                active-text="禁用系统内置根证书"
                @update:model-value="(v: any) => updateField('disable_system_root', Boolean(v))"
              />
              <el-switch
                :model-value="modelValue.reject_unknown_sni"
                active-text="拒绝未知 SNI 握手"
                @update:model-value="(v: any) => updateField('reject_unknown_sni', Boolean(v))"
              />
            </div>
          </el-form-item>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped lang="scss">
.tls-box {
  display: flex;
  flex-direction: column;
}

:deep(.el-form-item) {
  margin-bottom: 0;
}

.field-hint {
  font-size: 11px;
  color: var(--x-text-3);
  margin-top: 3px;
  line-height: 1.35;
}
</style>
