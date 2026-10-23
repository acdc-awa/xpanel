<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  ElMessage,
  CsAlert,
  CsButton,
  CsCheckboxGroup,
  CsColumnLayout,
  CsContainer,
  CsExpandableSection,
  CsForm,
  CsFormItem,
  CsIcon,
  CsInput,
  CsInputNumber,
  CsModal,
  CsSelect,
  CsSpaceBetween,
  CsToggle,
  type CheckboxGroupOption,
} from '@/components/cloudscape'
import {
  createServerRoutingRule,
  updateServerRoutingRule,
  type RoutingRulePayload,
  type ServerRoutingRule,
} from '@/api/admin'
import { errMsg } from '@/api/http'
import CodeEditor from '@/components/CodeEditor.vue'

// 自定义 Rule JSON 的占位示例（含双引号，用常量避免在模板表达式里写字面量对象）
const RULE_JSON_PLACEHOLDER = '{"type":"field","inboundTag":["api"],"outboundTag":"direct"}'

const props = defineProps<{
  serverId: number
  rule?: ServerRoutingRule | null
  outboundTags?: string[]
  inboundTags?: string[]
}>()

const emit = defineEmits<{
  (e: 'saved'): void
  (e: 'close'): void
}>()

const visible = ref(false)
const saving = ref(false)

interface RoutingRuleForm extends RoutingRulePayload {
  inbound_tag?: string
}

const form = reactive<RoutingRuleForm>({
  outbound_tag: '',
  rule_json: '',
  domain: '',
  ip: '',
  port: '',
  network: '',
  protocol: '',
  inbound_tag: '',
  enabled: true,
  priority: 0,
  remark: '',
})

const jsonError = ref('')

// 协议嗅探多选 → 逗号分隔字符串
const selectedProtocols = ref<Array<string | number>>([])

const protocolOptions: CheckboxGroupOption[] = [
  { value: 'bittorrent', label: 'BitTorrent (BT)' },
  { value: 'http', label: 'HTTP' },
  { value: 'tls', label: 'TLS' },
]

const networkOptions = [
  { label: 'TCP', value: 'tcp' },
  { label: 'UDP', value: 'udp' },
  { label: 'TCP, UDP', value: 'tcp,udp' },
]

const inboundTagOptions = computed(() => (props.inboundTags ?? []).map((tag) => ({ label: tag, value: tag })))
const outboundTagOptions = computed(() => (props.outboundTags ?? []).map((tag) => ({ label: tag, value: tag })))

function onProtocolsChange(vals: Array<string | number>) {
  form.protocol = vals.map(String).join(',')
}

// ===== 常用规则预设 =====
interface PresetRule {
  name: string
  icon?: string
  domain?: string
  ip?: string
  protocols?: string[]
  network?: string
  outbound_tag: string
  remark: string
}

const PRESET_RULES: PresetRule[] = [
  {
    name: '阻断 BT 下载',
    protocols: ['bittorrent'],
    outbound_tag: 'blocked',
    remark: '阻断 BitTorrent P2P 下载',
  },
  {
    name: '中国大陆直连',
    domain: 'geosite:cn',
    ip: 'geoip:cn',
    outbound_tag: 'direct',
    remark: '大陆域名与 IP 直连',
  },
  {
    name: '阻断私有局域网',
    ip: 'geoip:private',
    outbound_tag: 'blocked',
    remark: '阻断 10.x/172.16.x/192.168.x 等私网',
  },
  {
    name: '流媒体分流',
    domain: 'geosite:netflix\ngeosite:youtube\ngeosite:disney\ngeosite:spotify',
    outbound_tag: 'proxy',
    remark: '海外流媒体定向分流',
  },
  {
    name: '社交平台分流',
    domain: 'geosite:telegram\ngeosite:twitter\ngeosite:facebook\ngeosite:instagram',
    outbound_tag: 'proxy',
    remark: '海外通讯与社交平台分流',
  },
  {
    name: '广告与追踪拦截',
    domain: 'geosite:category-ads-all',
    outbound_tag: 'blocked',
    remark: '全局广告与跟踪拦截',
  },
]

function applyPreset(p: PresetRule) {
  if (p.domain !== undefined) form.domain = p.domain
  if (p.ip !== undefined) form.ip = p.ip
  if (p.network !== undefined) form.network = p.network
  if (p.protocols) {
    selectedProtocols.value = [...p.protocols]
    form.protocol = p.protocols.join(',')
  } else {
    selectedProtocols.value = []
    form.protocol = ''
  }
  // 查找匹配的出站标签，优先选用当前服务器已有的同名出站标签
  const matchTag = props.outboundTags?.find((t) => t === p.outbound_tag || (p.outbound_tag === 'blocked' && t === 'blackhole'))
  if (matchTag) {
    form.outbound_tag = matchTag
  } else if (!form.outbound_tag) {
    form.outbound_tag = p.outbound_tag
  }
  if (!form.remark || form.remark.includes('分流') || form.remark.includes('屏蔽')) {
    form.remark = p.remark
  }
  ElMessage.success(`已应用「${p.name}」规则预设`)
}

onMounted(() => {
  if (props.rule) {
    form.outbound_tag = props.rule.outbound_tag
    form.rule_json = props.rule.rule_json || ''
    form.domain = props.rule.domain || ''
    form.ip = props.rule.ip || ''
    form.port = props.rule.port || ''
    form.network = props.rule.network || ''
    form.protocol = props.rule.protocol || ''
    form.inbound_tag = props.rule.inbound_tag || ''
    form.enabled = props.rule.enabled
    form.priority = props.rule.priority
    form.remark = props.rule.remark
    selectedProtocols.value = form.protocol ? form.protocol.split(',').map((s) => s.trim()).filter(Boolean) : []
  } else if (props.outboundTags && props.outboundTags.length > 0 && !form.outbound_tag) {
    form.outbound_tag = props.outboundTags[0]
  }
  visible.value = true
})

const isDerived = computed(() => !!props.rule?.owner_inbound_id)

async function save() {
  jsonError.value = ''
  if (!form.outbound_tag?.trim()) {
    ElMessage.warning('请选择出站标签（OutboundTag）')
    return
  }
  if (form.rule_json?.trim()) {
    try {
      JSON.parse(form.rule_json)
    } catch {
      jsonError.value = 'Rule JSON 格式不合法'
      return
    }
  }
  saving.value = true
  try {
    const payload: RoutingRuleForm = {
      outbound_tag: form.outbound_tag.trim(),
      rule_json: form.rule_json?.trim() || '',
      domain: form.domain?.trim() || '',
      ip: form.ip?.trim() || '',
      port: form.port?.trim() || '',
      network: form.network || '',
      protocol: form.protocol?.trim() || '',
      inbound_tag: isDerived.value ? (props.rule?.inbound_tag || '') : (form.inbound_tag?.trim() || ''),
      enabled: isDerived.value ? (props.rule?.enabled ?? true) : (form.enabled ?? true),
      priority: form.priority ?? 0,
      remark: form.remark || '',
    }
    const { data } = props.rule
      ? await updateServerRoutingRule(props.serverId, props.rule.id, payload)
      : await createServerRoutingRule(props.serverId, payload)
    if (data.code === 0) {
      ElMessage.success(props.rule ? '路由规则已更新' : '路由规则已创建')
      emit('saved')
      visible.value = false
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

// InboundTag 多选下拉
const selectedInboundTags = computed<string[]>({
  get: () => {
    const raw = form.inbound_tag || ''
    return raw ? raw.split(',').map((s) => s.trim()).filter(Boolean) : []
  },
  set: (vals) => {
    form.inbound_tag = vals.join(',')
  },
})

function onClosed() {
  emit('close')
}
</script>
<template>
  <CsModal
    v-model:visible="visible"
    :header="rule ? `编辑路由规则 · ${rule.outbound_tag}` : '新增路由规则'"
    size="large"
    scrollable
    @close="onClosed"
  >
    <CsSpaceBetween size="l">
      <!-- 顶部四层直通派生规则提示 -->
      <CsAlert v-if="isDerived" type="info" header="四层直通派生转发规则">
        本规则由四层直通入站自动派生维护。来源入站与启用状态自动跟随直通本身（已锁定）；您可调整优先级（以便在直通前插入过滤规则）或调整目标出口。
      </CsAlert>

      <!-- 顶部常用预设（仅非直通派生规则展示） -->
      <div v-else class="preset-section">
        <CsSpaceBetween direction="horizontal" size="xs" align-items="center">
          <span class="preset-title"><CsIcon name="suggestions" size="small" /> 常用规则一键预设</span>
          <div class="preset-actions">
            <CsButton v-for="p in PRESET_RULES" :key="p.name" @click="applyPreset(p)">{{ p.name }}</CsButton>
          </div>
        </CsSpaceBetween>
      </div>

      <CsForm label-position="top">
        <CsSpaceBetween size="l">
          <CsColumnLayout :columns="2">
            <!-- ===== 左栏：入站匹配条件 ===== -->
            <CsContainer header="1. 流量匹配条件 (Match Criteria)">
              <CsSpaceBetween size="m">
                <CsFormItem label="协议嗅探 Protocol">
                  <CsCheckboxGroup
                    v-model="selectedProtocols"
                    :options="protocolOptions"
                    @change="onProtocolsChange"
                  />
                </CsFormItem>

                <CsFormItem label="域名匹配 Domain（每行一条）">
                  <CsInput
                    v-model="form.domain"
                    type="textarea"
                    :rows="3"
                    class="cs-text-mono"
                    placeholder="geosite:cn&#10;geosite:netflix&#10;*.example.com"
                  />
                </CsFormItem>

                <CsFormItem label="IP 段匹配 IP（每行一条）">
                  <CsInput
                    v-model="form.ip"
                    type="textarea"
                    :rows="3"
                    class="cs-text-mono"
                    placeholder="geoip:private&#10;geoip:cn&#10;8.8.8.8"
                  />
                </CsFormItem>

                <CsColumnLayout :columns="2">
                  <CsFormItem label="目标端口 Port">
                    <CsInput v-model="form.port" class="cs-text-mono" placeholder="如 443 或 80,443" />
                  </CsFormItem>
                  <CsFormItem label="传输层网络">
                    <CsSelect v-model="form.network" :options="networkOptions" clearable placeholder="全部网络" />
                  </CsFormItem>
                </CsColumnLayout>

                <CsFormItem label="限定来源入站 InboundTag">
                  <CsSelect
                    v-model="selectedInboundTags"
                    :options="inboundTagOptions"
                    :disabled="isDerived"
                    multiple
                    filterable
                    allow-create
                    placeholder="默认匹配全部入站流量"
                  />
                  <span v-if="isDerived" class="cs-text-secondary field-hint">
                    直通派生规则的来源入站由直通入站本身维护，已锁定不可更改
                  </span>
                </CsFormItem>
              </CsSpaceBetween>
            </CsContainer>

            <!-- ===== 右栏：路由目标与控制 ===== -->
            <CsSpaceBetween size="m">
              <CsContainer header="2. 转发目标与属性 (Target)">
                <template #actions>
                  <CsSpaceBetween direction="horizontal" size="xs" align-items="center">
                    <span class="cs-text-secondary">生效此规则</span>
                    <CsToggle v-model="form.enabled" :disabled="isDerived" />
                  </CsSpaceBetween>
                </template>

                <CsSpaceBetween size="m">
                  <CsFormItem label="目标出站出口 (OutboundTag)">
                    <CsSelect
                      v-model="form.outbound_tag"
                      :options="outboundTagOptions"
                      filterable
                      allow-create
                      placeholder="请选择转发到的出站"
                    />
                  </CsFormItem>

                  <CsFormItem label="规则优先级 (Priority)">
                    <CsInputNumber v-model="form.priority" :min="0" />
                    <span class="cs-text-secondary field-hint">数字越小越先被 Xray 规则链评估匹配</span>
                  </CsFormItem>

                  <CsFormItem label="规则备注说明">
                    <CsInput v-model="form.remark" placeholder="如 大陆域名直连 / 屏蔽 BT 下载" />
                  </CsFormItem>
                </CsSpaceBetween>
              </CsContainer>

              <CsAlert type="info" header="规则判定顺序">
                Xray 从上至下按 Priority 优先级顺序检索规则，首条完全满足「协议嗅探 / 域名 / IP / 端口」的规则即决定出站走向。
              </CsAlert>
            </CsSpaceBetween>
          </CsColumnLayout>

          <!-- ===== 高级：自定义 Rule JSON ===== -->
          <CsExpandableSection
            variant="container"
            header="高级自定义：原始 Rule JSON（填写后直接透传覆盖上方表单）"
          >
            <CsSpaceBetween size="xs">
              <CodeEditor
                v-model="form.rule_json"
                language="json"
                height="150px"
                :placeholder="RULE_JSON_PLACEHOLDER"
              />
              <span class="cs-text-secondary json-hint">
                填写后配置生成器将直接透传此 JSON 对象（自动补全 <code>type:"field"</code> 与 outboundTag）。
              </span>
            </CsSpaceBetween>
          </CsExpandableSection>

          <CsAlert v-if="jsonError" type="error">{{ jsonError }}</CsAlert>
        </CsSpaceBetween>
      </CsForm>
    </CsSpaceBetween>

    <template #footer>
      <div class="modal-footer">
        <span class="cs-text-secondary footer-hint">保存后主控自动编译并推送到服务器</span>
        <CsSpaceBetween direction="horizontal" size="xs">
          <CsButton @click="visible = false">取消</CsButton>
          <CsButton variant="primary" :loading="saving" icon-name="check" @click="save">保存规则</CsButton>
        </CsSpaceBetween>
      </div>
    </template>
  </CsModal>
</template>

<style scoped lang="scss">
.preset-section {
  background: var(--color-background-container-content-mjfil9, #fff);
  border: 1px solid var(--color-border-divider-default-diwd4w, #e9ebed);
  border-radius: 8px;
  padding: 12px 16px;
}
.preset-title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-weight: 700;
  white-space: nowrap;
}
.preset-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.field-hint {
  display: block;
  margin-top: 4px;
  font-size: 12px;
}
.json-hint {
  display: block;
  font-size: 12px;
}
.modal-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.footer-hint {
  font-size: 12px;
}
</style>
