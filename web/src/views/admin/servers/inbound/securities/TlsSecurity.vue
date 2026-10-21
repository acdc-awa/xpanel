<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { ArrowDown, ArrowUp } from '@element-plus/icons-vue'
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
const showAdvanced = ref(false)
const showClient = ref(false)

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

          <el-form-item label="分享/客户端 SNI (serverName)">
            <el-input
              :model-value="modelValue.server_name"
              placeholder="如 example.com"
              class="font-mono-num"
              @input="(v: string) => updateField('server_name', v)"
            />
            <div class="field-hint">服务端不读取该字段；作为订阅与机间连接的默认 SNI 使用。</div>
          </el-form-item>

          <el-form-item label="ALPN (逗号分隔)">
            <el-input
              :model-value="modelValue.alpn"
              placeholder="h2,http/1.1"
              class="font-mono-num"
              @input="(v: string) => updateField('alpn', v)"
            />
          </el-form-item>

          <el-form-item label="证书文件路径 (Cert File)">
            <el-input
              :model-value="modelValue.cert_file"
              placeholder="如 /etc/xray/cert.pem（未绑定托管证书时必填）"
              class="font-mono-num"
              @input="(v: string) => updateField('cert_file', v)"
            />
          </el-form-item>

          <el-form-item label="私钥文件路径 (Key File)">
            <el-input
              :model-value="modelValue.key_file"
              placeholder="如 /etc/xray/key.pem（与证书路径成对填写）"
              class="font-mono-num"
              @input="(v: string) => updateField('key_file', v)"
            />
            <div class="field-hint">
              未绑定面板托管证书时，必须填写成对的证书/私钥路径，且文件需真实存在于节点上。
            </div>
          </el-form-item>
        </div>
      </div>
    </div>

    <!-- 微卡片 2: 服务端 TLS 策略（仅服务端生效，折叠） -->
    <div class="x-micro-card">
      <div
        class="x-micro-card__head cursor-pointer select-none"
        @click="showAdvanced = !showAdvanced"
      >
        <div class="head-title">
          <span class="w-2 h-2 rounded-full bg-amber-500"></span>
          <span>服务端 TLS 策略（选填调优）</span>
        </div>
        <div class="head-meta flex items-center gap-1 text-indigo-500">
          <span>{{ showAdvanced ? '收起高级配置' : '展开高级调优' }}</span>
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
              placeholder="留空使用标准套件；多个用逗号分隔"
              class="font-mono-num"
              @input="(v: string) => updateField('cipher_suites', v)"
            />
          </el-form-item>

          <el-form-item label="拒绝未知 SNI 握手">
            <div style="padding-top: 4px">
              <el-switch
                :model-value="modelValue.reject_unknown_sni"
                active-text="rejectUnknownSni"
                @update:model-value="(v: any) => updateField('reject_unknown_sni', Boolean(v))"
              />
            </div>
          </el-form-item>
        </div>
      </div>
    </div>

    <!-- 微卡片 3: 订阅透传（仅影响用户订阅，服务端不生效） -->
    <div class="x-micro-card">
      <div
        class="x-micro-card__head cursor-pointer select-none"
        @click="showClient = !showClient"
      >
        <div class="head-title">
          <span class="w-2 h-2 rounded-full bg-sky-500"></span>
          <span>订阅透传（仅影响用户订阅）</span>
        </div>
        <div class="head-meta flex items-center gap-1 text-indigo-500">
          <span>{{ showClient ? '收起' : '展开' }}</span>
          <el-icon><component :is="showClient ? ArrowUp : ArrowDown" /></el-icon>
        </div>
      </div>
      <div v-show="showClient" class="x-micro-card__body">
        <div class="form-grid-2">
          <el-form-item label="跳过客户端证书校验 (allowInsecure)">
            <div style="padding-top: 4px">
              <el-switch
                :model-value="modelValue.allow_insecure"
                active-text="订阅下发 skip-cert-verify"
                @update:model-value="(v: any) => updateField('allow_insecure', Boolean(v))"
              />
              <div class="field-hint">
                仅写入用户订阅；xray 服务端已移除该字段，不会进入节点配置。
                机间出站的证书校验由落地方自动决定（托管 CA → CA 验签，自签 → 指纹 Pin），不读取本页配置。
              </div>
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
