<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { getCerts, type CertItem } from '@/api/admin'
import type { TlsConfig } from '../types'

const props = defineProps<{
  modelValue: TlsConfig
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: TlsConfig): void
  (e: 'update:certId', value: number): void
}>()

const certs = ref<CertItem[]>([])

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

onMounted(loadCerts)

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
    <div class="form-grid">
      <el-form-item label="托管证书">
        <el-select
          :model-value="modelValue.cert_id || undefined"
          style="width: 100%"
          clearable
          placeholder="不绑定（使用本地证书文件）"
          @change="onCertChange"
        >
          <el-option v-for="ct in certs" :key="ct.id" :value="ct.id" :label="`${ct.domain}（到期 ${ct.not_after}）`">
            <span>{{ ct.domain }}（到期 {{ ct.not_after }}）</span>
            <span v-if="ct.self_signed" class="x-chip green" style="margin-left: 6px; font-size: 11px">自签·中转自动Pin</span>
          </el-option>
        </el-select>
        <div v-if="certs.find(ct => ct.id === modelValue.cert_id)?.self_signed" class="muted" style="font-size: 12px; margin-top: 4px">
          自签证书：引用本入站的中转出站将自动绑定指纹校验 (SHA-256)，防止中间人攻击
        </div>
      </el-form-item>

      <el-form-item label="SNI (server_name)">
        <el-input
          :model-value="modelValue.server_name"
          placeholder="example.com"
          @input="(v: string) => updateField('server_name', v)"
        />
      </el-form-item>

      <el-form-item label="ALPN (逗号分隔)">
        <el-input
          :model-value="modelValue.alpn"
          placeholder="h2,http/1.1"
          @input="(v: string) => updateField('alpn', v)"
        />
      </el-form-item>

      <el-form-item label="证书路径 (Cert File)">
        <el-input
          :model-value="modelValue.cert_file"
          placeholder="/etc/xray/cert.pem"
          @input="(v: string) => updateField('cert_file', v)"
        />
      </el-form-item>

      <el-form-item label="私钥路径 (Key File)">
        <el-input
          :model-value="modelValue.key_file"
          placeholder="/etc/xray/key.pem"
          @input="(v: string) => updateField('key_file', v)"
        />
      </el-form-item>

      <el-form-item label="加密套件 (Cipher Suites)">
        <el-input
          :model-value="modelValue.cipher_suites"
          placeholder="留空默认"
          @input="(v: string) => updateField('cipher_suites', v)"
        />
      </el-form-item>

      <el-form-item label="TLS 版本范围">
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

      <el-form-item label="跳过证书校验 (allowInsecure)">
        <div style="padding-top: 6px">
          <el-switch
            :model-value="modelValue.allow_insecure"
            active-text="允许不安全证书"
            @update:model-value="(v: any) => updateField('allow_insecure', Boolean(v))"
          />
        </div>
      </el-form-item>
    </div>
  </div>
</template>

<style scoped lang="scss">
.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 18px;

  :deep(.el-form-item) {
    margin-bottom: 14px;
  }
}

.x-chip {
  display: inline-block;
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 11px;
  &.green {
    background: rgba(16, 185, 129, 0.15);
    color: #10b981;
  }
}

.muted {
  color: var(--x-text-3);
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
