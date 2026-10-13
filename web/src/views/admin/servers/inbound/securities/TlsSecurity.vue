<script setup lang="ts">
import { ref, onMounted } from 'vue'
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
            <span v-if="ct.ca_name" class="x-chip purple" style="margin-left: 6px; font-size: 11px">CA: {{ ct.ca_name }}</span>
            <span v-else-if="ct.self_signed" class="x-chip green" style="margin-left: 6px; font-size: 11px">自签·自动Pin</span>
            <span v-else class="x-chip blue" style="margin-left: 6px; font-size: 11px">外部导入</span>
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

      <el-form-item label="证书指纹固定 (Pin SHA-256)">
        <el-input
          :model-value="modelValue.pinned_peer_cert_sha256"
          placeholder="64位 Hex 散列，多个以逗号分隔"
          clearable
          @input="(v: string) => updateField('pinned_peer_cert_sha256', v)"
        />
        <div class="field-hint">
          固定远端或 CA 根证书的 SHA-256 散列值；命中即验证通过，自签证书亦防中间人劫持
        </div>
      </el-form-item>

      <el-form-item label="CA 根证书路径 (CA File)">
        <el-input
          :model-value="modelValue.ca_cert_file"
          placeholder="/etc/xray/ca.crt"
          clearable
          @input="(v: string) => updateField('ca_cert_file', v)"
        />
        <div class="field-hint">
          用于验签对端证书的 CA 根证书（usage: verify，对端证书须由该 CA 签发）
        </div>
      </el-form-item>

      <el-form-item label="校验证书域名 (verifyPeerCertByName)">
        <el-input
          :model-value="modelValue.verify_peer_cert_by_name"
          placeholder="留空默认匹配 SNI，逗号分隔多个"
          clearable
          @input="(v: string) => updateField('verify_peer_cert_by_name', v)"
        />
      </el-form-item>

      <el-form-item label="从托管 CA 选择验签证书（快速填充）">
        <el-select
          placeholder="从已保存的 CA 中选择"
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
            :label="`${ca.name} (${ca.common_name}) · ${ca.has_key ? '完整CA' : '纯公钥'}`"
          />
        </el-select>
        <div class="field-hint">选择后将自动提取并填充下方 CA 证书 PEM 内容</div>
      </el-form-item>

      <el-form-item label="CA 证书内容 (PEM 直接粘贴)">
        <el-input
          :model-value="modelValue.ca_cert_content"
          type="textarea"
          :rows="2"
          placeholder="-----BEGIN CERTIFICATE-----&#10;...&#10;-----END CERTIFICATE-----"
          @input="(v: string) => updateField('ca_cert_content', v)"
        />
      </el-form-item>

      <el-form-item label="证书与握手保护">
        <div style="display: flex; flex-direction: column; gap: 8px; padding-top: 4px">
          <el-switch
            :model-value="modelValue.disable_system_root"
            active-text="禁用系统内置根证书 (disableSystemRoot)"
            @update:model-value="(v: any) => updateField('disable_system_root', Boolean(v))"
          />
          <el-switch
            :model-value="modelValue.reject_unknown_sni"
            active-text="拒绝未知 SNI 握手 (rejectUnknownSni)"
            @update:model-value="(v: any) => updateField('reject_unknown_sni', Boolean(v))"
          />
        </div>
      </el-form-item>

      <el-form-item label="跳过证书校验 (allowInsecure)">
        <div style="padding-top: 6px">
          <el-switch
            :model-value="modelValue.allow_insecure"
            active-text="允许不安全证书 (客户端/订阅)"
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

.field-hint {
  font-size: 11px;
  color: var(--x-text-3);
  margin-top: 3px;
  line-height: 1.35;
}

@media (max-width: 640px) {
  .form-grid {
    grid-template-columns: 1fr;
  }
}
</style>
