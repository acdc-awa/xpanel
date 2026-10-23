<script setup lang="ts">
import { computed, ref, onMounted } from 'vue'
import {
  CsContainer,
  CsExpandableSection,
  CsFormItem,
  CsInput,
  CsSelect,
  CsToggle,
} from '@/components/cloudscape'
import { getCerts, type CertItem } from '@/api/admin'
import type { TlsConfig } from '../types'

const props = defineProps<{
  modelValue: TlsConfig
  /** 固定 ALPN（Hysteria 2 必须是 h3）：非空时禁用输入并强制该值。 */
  alpnFixed?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: TlsConfig): void
  (e: 'update:certId', value: number): void
}>()

const certs = ref<CertItem[]>([])

const certOptions = computed(() =>
  certs.value.map((cert) => ({
    value: cert.id,
    label: `${cert.domain}（到期 ${cert.not_after}）${
      cert.ca_name ? ` · CA: ${cert.ca_name}` : cert.self_signed ? ' · 自签·自动Pin' : ' · 外部导入'
    }`,
  }))
)

const tlsVersionOptions = [
  { label: 'TLS 1.2', value: '1.2' },
  { label: 'TLS 1.3', value: '1.3' },
]

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

onMounted(() => {
  loadCerts()
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
    <CsContainer header="服务端 TLS 证书与 SNI 绑定 (必填核心)">
      <template #actions>
        <span class="cs-text-secondary">Xray 标准 TLS 加密</span>
      </template>
      <div class="form-grid-2">
        <CsFormItem label="面板托管证书">
          <CsSelect
            :model-value="modelValue.cert_id || undefined"
            :options="certOptions"
            style="width: 100%"
            clearable
            placeholder="不绑定（使用本地证书文件路径）"
            @change="onCertChange"
          />
          <div
            v-if="certs.find(ct => ct.id === modelValue.cert_id)?.self_signed"
            class="field-hint success-hint"
          >
            自签证书：引用本入站的中转出站将自动绑定指纹校验 (SHA-256)，防止中间人攻击
          </div>
        </CsFormItem>

        <CsFormItem label="分享/客户端 SNI (serverName)">
          <CsInput
            :model-value="modelValue.server_name"
            placeholder="如 example.com"
            class="mono"
            @update:model-value="(v: string) => updateField('server_name', v)"
          />
          <div class="field-hint cs-text-secondary">服务端不读取该字段；作为订阅与机间连接的默认 SNI 使用。</div>
        </CsFormItem>

        <CsFormItem label="ALPN (逗号分隔)">
          <CsInput
            :model-value="props.alpnFixed || modelValue.alpn"
            :disabled="!!props.alpnFixed"
            :placeholder="props.alpnFixed || 'h2,http/1.1'"
            class="mono"
            @update:model-value="(v: string) => updateField('alpn', v)"
          />
          <div v-if="props.alpnFixed" class="field-hint cs-text-secondary">
            Hysteria 2 走 QUIC，服务端 ALPN 必须为 {{ props.alpnFixed }}（填其它值会导致握手 no application protocol）。
          </div>
        </CsFormItem>

        <CsFormItem label="证书文件路径 (Cert File)">
          <CsInput
            :model-value="modelValue.cert_file"
            placeholder="如 /etc/xray/cert.pem（未绑定托管证书时必填）"
            class="mono"
            @update:model-value="(v: string) => updateField('cert_file', v)"
          />
        </CsFormItem>

        <CsFormItem label="私钥文件路径 (Key File)">
          <CsInput
            :model-value="modelValue.key_file"
            placeholder="如 /etc/xray/key.pem（与证书路径成对填写）"
            class="mono"
            @update:model-value="(v: string) => updateField('key_file', v)"
          />
          <div class="field-hint cs-text-secondary">
            未绑定面板托管证书时，必须填写成对的证书/私钥路径，且文件需真实存在于节点上。
          </div>
        </CsFormItem>
      </div>
    </CsContainer>

    <!-- 微卡片 2: 服务端 TLS 策略（仅服务端生效，折叠） -->
    <CsExpandableSection header="服务端 TLS 策略（选填调优）" class="section-gap">
      <div class="form-grid-2">
        <CsFormItem label="TLS 协议版本范围">
          <div class="version-row">
            <CsSelect
              :model-value="modelValue.min_version"
              :options="tlsVersionOptions"
              placeholder="最低版本"
              clearable
              style="width: 50%"
              @update:model-value="(v: any) => updateField('min_version', String(v))"
            />
            <CsSelect
              :model-value="modelValue.max_version"
              :options="tlsVersionOptions"
              placeholder="最高版本"
              clearable
              style="width: 50%"
              @update:model-value="(v: any) => updateField('max_version', String(v))"
            />
          </div>
        </CsFormItem>

        <CsFormItem label="加密套件 (Cipher Suites)">
          <CsInput
            :model-value="modelValue.cipher_suites"
            placeholder="留空使用标准套件；多个用逗号分隔"
            class="mono"
            @update:model-value="(v: string) => updateField('cipher_suites', v)"
          />
        </CsFormItem>

        <CsFormItem label="拒绝未知 SNI 握手">
          <div class="toggle-pad">
            <CsToggle
              :model-value="modelValue.reject_unknown_sni"
              label="rejectUnknownSni"
              @update:model-value="(v: boolean) => updateField('reject_unknown_sni', v)"
            />
          </div>
        </CsFormItem>
      </div>
    </CsExpandableSection>

    <!-- 微卡片 3: 订阅透传（仅影响用户订阅，服务端不生效） -->
    <CsExpandableSection header="订阅透传（仅影响用户订阅）" class="section-gap">
      <div class="form-grid-2">
        <CsFormItem label="跳过客户端证书校验 (allowInsecure)">
          <div class="toggle-pad">
            <CsToggle
              :model-value="modelValue.allow_insecure"
              label="订阅下发 skip-cert-verify"
              @update:model-value="(v: boolean) => updateField('allow_insecure', v)"
            />
            <div class="field-hint cs-text-secondary">
              仅写入用户订阅；xray 服务端已移除该字段，不会进入节点配置。
              机间出站的证书校验由落地方自动决定（托管 CA → CA 验签，自签 → 指纹 Pin），不读取本页配置。
            </div>
          </div>
        </CsFormItem>
      </div>
    </CsExpandableSection>
  </div>
</template>

<style scoped lang="scss">
.tls-box {
  display: flex;
  flex-direction: column;
}

.section-gap {
  margin-top: 16px;
}

.version-row {
  display: flex;
  gap: 8px;
  width: 100%;
}

.toggle-pad {
  padding-top: 4px;
}

.field-hint {
  margin-top: 4px;
  font-size: 11px;
  line-height: 1.35;
}

.success-hint {
  color: var(--color-text-status-success-1i2jf8, #037f0c);
}

.mono {
  font-family: var(--font-family-monospace-842lr2, ui-monospace, Menlo, Consolas, monospace);
  font-variant-numeric: tabular-nums;
}
</style>
