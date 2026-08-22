<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { Check, MagicStick } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import {
  createServerRoutingRule,
  updateServerRoutingRule,
  type RoutingRulePayload,
  type ServerRoutingRule,
} from '@/api/admin'
import { errMsg } from '@/api/http'

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
const selectedProtocols = ref<string[]>([])

function onProtocolsChange(vals: any) {
  form.protocol = (vals as string[]).join(',')
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

async function save() {
  jsonError.value = ''
  if (!form.outbound_tag?.trim()) {
    ElMessage.warning('请选择 OutboundTag（出站标签）')
    return
  }
  if (form.rule_json?.trim()) {
    try {
      JSON.parse(form.rule_json)
    } catch {
      jsonError.value = 'Rule JSON 不是合法 JSON'
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
      inbound_tag: form.inbound_tag?.trim() || '',
      enabled: form.enabled ?? true,
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
  set: () => {},
})

function onInboundTagsChange(vals: string[]) {
  form.inbound_tag = vals.join(',')
}

function onClosed() {
  emit('close')
}
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="rule ? `编辑路由分流规则 · ${rule.outbound_tag}` : '新增路由分流规则'"
    width="760px"
    @closed="onClosed"
  >
    <!-- 顶部常用预设气泡栏 -->
    <div class="preset-section">
      <div class="preset-title">
        <el-icon><MagicStick /></el-icon>&nbsp;常用规则一键预设：
      </div>
      <div class="preset-chips">
        <button
          v-for="p in PRESET_RULES"
          :key="p.name"
          type="button"
          class="preset-chip"
          @click="applyPreset(p)"
        >
          <span>{{ p.name }}</span>
        </button>
      </div>
    </div>

    <el-form label-position="top" class="rule-form">
      <div class="dual-panel-grid">
        <!-- ===== 左栏：入站匹配条件 ===== -->
        <div class="panel-card">
          <div class="panel-header">
            <span class="panel-title">1. 入站流量匹配条件 (Match Criteria)</span>
          </div>

          <el-form-item label="协议嗅探 Protocol">
            <el-checkbox-group v-model="selectedProtocols" @change="onProtocolsChange">
              <el-checkbox label="bittorrent">BitTorrent (BT)</el-checkbox>
              <el-checkbox label="http">HTTP</el-checkbox>
              <el-checkbox label="tls">TLS</el-checkbox>
            </el-checkbox-group>
          </el-form-item>

          <el-form-item label="域名匹配 Domain（每行一条）">
            <el-input
              v-model="form.domain"
              type="textarea"
              :rows="3"
              class="code-textarea"
              placeholder="geosite:cn&#10;geosite:netflix&#10;*.example.com"
            />
          </el-form-item>

          <el-form-item label="IP 段匹配 IP（每行一条）">
            <el-input
              v-model="form.ip"
              type="textarea"
              :rows="3"
              class="code-textarea"
              placeholder="geoip:private&#10;geoip:cn&#10;8.8.8.8"
            />
          </el-form-item>

          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 12px">
            <el-form-item label="目标端口 Port">
              <el-input v-model="form.port" placeholder="如 443 或 80,443" />
            </el-form-item>
            <el-form-item label="传输层网络">
              <el-select v-model="form.network" style="width: 100%" clearable placeholder="全部网络">
                <el-option label="TCP" value="tcp" />
                <el-option label="UDP" value="udp" />
                <el-option label="TCP, UDP" value="tcp,udp" />
              </el-select>
            </el-form-item>
          </div>

          <el-form-item label="限定入站 InboundTag" style="margin-bottom: 0">
            <el-select
              v-model="selectedInboundTags"
              style="width: 100%"
              multiple
              filterable
              allow-create
              default-first-option
              placeholder="默认匹配全部入站"
              @change="onInboundTagsChange"
            >
              <el-option v-for="t in inboundTags ?? []" :key="t" :label="t" :value="t" />
            </el-select>
          </el-form-item>
        </div>

        <!-- ===== 右栏：路由目标 ===== -->
        <div class="panel-card">
          <div class="panel-header">
            <span class="panel-title">2. 转发目标与属性 (Target & Control)</span>
          </div>

          <el-form-item label="目标出站标签 (OutboundTag)">
            <el-select
              v-model="form.outbound_tag"
              style="width: 100%"
              filterable
              allow-create
              default-first-option
              placeholder="请选择转发到的出站"
            >
              <el-option v-for="t in outboundTags ?? []" :key="t" :label="t" :value="t" />
            </el-select>
          </el-form-item>

          <el-form-item label="规则优先级 (Priority)">
            <el-input-number v-model="form.priority" :min="0" style="width: 100%" />
            <span class="muted" style="font-size: 11px; margin-top: 3px; display: block">数字越小越先被 Xray 规则链评估匹配</span>
          </el-form-item>

          <el-form-item label="启用规则">
            <el-switch v-model="form.enabled" active-text="生效此规则" />
          </el-form-item>

          <el-form-item label="规则备注">
            <el-input v-model="form.remark" placeholder="如 大陆域名直连 / 屏蔽BT下载" />
          </el-form-item>

          <div class="info-tip-box" style="margin-top: 10px">
            规则将按优先级升序生成到节点 Xray <code>routing.rules</code> 中，优先命中先执行。
          </div>
        </div>
      </div>

      <!-- ===== 高级：自定义 Rule JSON ===== -->
      <details class="json-collapse">
        <summary class="collapse-summary">
          <span>高级自定义：原始 Rule JSON（填写后直接透传覆盖上方表单）</span>
        </summary>
        <div style="margin-top: 10px">
          <el-input
            v-model="form.rule_json"
            type="textarea"
            :rows="3"
            class="code-textarea"
            placeholder='{"type":"field","inboundTag":["api"],"outboundTag":"direct"}'
          />
          <p class="muted" style="font-size: 11.5px; margin: 4px 0 0">
            填写后配置生成器将直接透传此 JSON 对象（自动补全 <code>type:"field"</code> 与 outboundTag）。
          </p>
        </div>
      </details>

      <el-alert v-if="jsonError" type="error" :closable="false" :title="jsonError" style="margin-top: 12px" />
    </el-form>

    <template #footer>
      <div style="display: flex; justify-content: space-between; align-items: center">
        <span class="muted" style="font-size: 12px">保存后主控自动编译并推送到节点</span>
        <div style="display: flex; gap: 10px">
          <el-button @click="visible = false">取消</el-button>
          <el-button type="primary" :loading="saving" @click="save">
            <el-icon><Check /></el-icon>&nbsp;保存规则
          </el-button>
        </div>
      </div>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.preset-section {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  background: var(--x-bg);
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 8px 12px;
  margin-bottom: 14px;
}
.preset-title {
  display: flex;
  align-items: center;
  font-size: 12px;
  font-weight: 600;
  color: var(--x-primary);
  white-space: nowrap;
}
.preset-chips {
  display: flex;
  gap: 6px;
  flex-wrap: wrap;
}
.preset-chip {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  background: var(--x-card-soft);
  border: 1px solid var(--x-border);
  border-radius: 6px;
  font-size: 11.5px;
  color: var(--x-text);
  cursor: pointer;
  transition: all 0.18s ease;
  &:hover {
    border-color: var(--x-primary);
    color: var(--x-primary);
    background: var(--x-primary-soft);
  }
}
.dual-panel-grid {
  display: grid;
  grid-template-columns: 1.1fr 0.9fr;
  gap: 14px;
}
@media (max-width: 680px) {
  .dual-panel-grid {
    grid-template-columns: 1fr;
  }
}
.panel-card {
  background: var(--x-card-soft);
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 12px 14px;
}
.panel-header {
  margin-bottom: 12px;
  padding-bottom: 6px;
  border-bottom: 1px solid var(--x-border);
}
.panel-title {
  font-size: 12.5px;
  font-weight: 600;
  color: var(--x-primary);
}
.code-textarea {
  font-family: var(--x-font-mono, monospace);
  font-size: 12px;
  line-height: 1.5;
}
.info-tip-box {
  font-size: 11.5px;
  color: var(--x-text-2);
  line-height: 1.5;
  background: var(--x-primary-soft);
  border: 1px solid var(--x-border);
  border-radius: 6px;
  padding: 8px 10px;
}
.json-collapse {
  margin-top: 12px;
  border: 1px dashed var(--x-border);
  border-radius: 8px;
  padding: 8px 12px;
}
.collapse-summary {
  font-size: 12px;
  color: var(--x-text-2);
  cursor: pointer;
  user-select: none;
  font-weight: 500;
}
.muted {
  color: var(--x-text-3);
}
</style>

