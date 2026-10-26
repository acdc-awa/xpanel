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
import TipIcon from '@/components/base/TipIcon.vue'
import type { TlsConfig } from '../types'

const props = defineProps<{
  modelValue: TlsConfig
  /** 固定 ALPN（Hysteria 2 必须是 h3）：非空时禁用输入并强制该值。 */
  alpnFixed?: string
  /** QUIC 传输（Hysteria 2）：只支持 TLS 1.3，版本范围与加密套件无意义。 */
  quic?: boolean
  /** 订阅是否跳过证书校验（面板字段 share_allow_insecure，唯一真源）。 */
  allowInsecure?: boolean
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: TlsConfig): void
  (e: 'update:certId', value: number): void
  (e: 'update:allowInsecure', value: boolean): void
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

const selectedCert = computed(() => certs.value.find((ct) => ct.id === props.modelValue.cert_id) || null)

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
      <div class="form-grid-2">
        <CsFormItem label="面板托管证书">
          <template v-if="selectedCert?.self_signed" #info>
            <TipIcon content="自签证书：引用本入站的中转出站将自动绑定指纹校验 (SHA-256)，防止中间人攻击。" />
          </template>
          <CsSelect
            :model-value="modelValue.cert_id || undefined"
            :options="certOptions"
            style="width: 100%"
            clearable
            placeholder="不绑定（使用本地证书文件路径）"
            @change="onCertChange"
          />
        </CsFormItem>

        <CsFormItem label="分享/客户端 SNI (serverName)">
          <template #info>
            <TipIcon content="服务端不读取该字段；作为订阅与机间连接的默认 SNI 使用。" />
          </template>
          <CsInput
            :model-value="modelValue.server_name"
            placeholder="如 example.com"
            class="mono"
            @update:model-value="(v: string) => updateField('server_name', v)"
          />
        </CsFormItem>

        <CsFormItem
          label="ALPN (逗号分隔)"
          :constraint-text="props.alpnFixed ? `Hysteria 2 走 QUIC，服务端 ALPN 必须为 ${props.alpnFixed}；填其它值会导致握手 no application protocol。` : ''"
        >
          <CsInput
            :model-value="props.alpnFixed || modelValue.alpn"
            :disabled="!!props.alpnFixed"
            :placeholder="props.alpnFixed || 'h2,http/1.1'"
            class="mono"
            @update:model-value="(v: string) => updateField('alpn', v)"
          />
        </CsFormItem>

        <CsFormItem label="证书文件路径 (Cert File)">
          <CsInput
            :model-value="modelValue.cert_file"
            placeholder="如 /etc/xray/cert.pem（未绑定托管证书时必填）"
            class="mono"
            @update:model-value="(v: string) => updateField('cert_file', v)"
          />
        </CsFormItem>

        <CsFormItem
          label="私钥文件路径 (Key File)"
          :constraint-text="'未绑定面板托管证书时，必须填写成对的证书/私钥路径，且文件需真实存在于节点上。'"
        >
          <CsInput
            :model-value="modelValue.key_file"
            placeholder="如 /etc/xray/key.pem（与证书路径成对填写）"
            class="mono"
            @update:model-value="(v: string) => updateField('key_file', v)"
          />
        </CsFormItem>
      </div>
    </CsContainer>

    <!-- 折叠区必须自带容器：ExpandableSection 的 default 变体没有左右内边距，
         直接放在面板上会让内容贴到页签边缘（与上方卡片对不齐）。 -->
    <CsContainer class="section-gap" :disable-content-paddings="true">
      <CsExpandableSection
        :header="props.quic ? '服务端 TLS 策略（QUIC 仅 TLS 1.3）' : '服务端 TLS 策略（选填调优）'"
        variant="container"
      >
        <div class="form-grid-2">
          <template v-if="!props.quic">
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
          </template>

          <CsFormItem label="拒绝未知 SNI 握手" :class="{ 'span-two': props.quic }">
            <div class="control-cell">
              <CsToggle
                :model-value="modelValue.reject_unknown_sni"
                label="rejectUnknownSni"
                @update:model-value="(v: boolean) => updateField('reject_unknown_sni', v)"
              />
            </div>
          </CsFormItem>
        </div>
      </CsExpandableSection>
    </CsContainer>

    <!-- 微卡片 3: 订阅透传（仅影响用户订阅，服务端不生效） -->
    <CsContainer class="section-gap" :disable-content-paddings="true">
      <CsExpandableSection header="订阅透传（仅影响用户订阅）" variant="container">
        <div class="form-grid-2">
          <CsFormItem label="跳过客户端证书校验 (allowInsecure)">
            <template #info>
              <TipIcon content="写入面板字段 share_allow_insecure，仅影响用户订阅；机间出站的证书校验由落地方自动决定（托管 CA → CA 验签，自签 → 指纹 Pin），不读取本开关。" />
            </template>
            <div class="control-cell">
              <CsToggle
                :model-value="Boolean(allowInsecure)"
                label="订阅下发 skip-cert-verify"
                @update:model-value="(v: boolean) => emit('update:allowInsecure', v)"
              />
            </div>
          </CsFormItem>
        </div>
      </CsExpandableSection>
    </CsContainer>
  </div>
</template>

<style scoped lang="scss">
.tls-box {
  display: flex;
  flex-direction: column;
}

.section-gap {
  margin-top: 12px;
}

.span-two {
  grid-column: span 2;
}

.version-row {
  display: flex;
  gap: 8px;
  width: 100%;
}

.mono {
  font-family: var(--font-family-monospace-842lr2, ui-monospace, Menlo, Consolas, monospace);
  font-variant-numeric: tabular-nums;
}
</style>
