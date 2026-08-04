<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Check } from '@element-plus/icons-vue'
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
}>()

const emit = defineEmits<{
  (e: 'saved'): void
  (e: 'close'): void
}>()

const visible = ref(false)
const saving = ref(false)

// RoutingRulePayload 在 admin.ts 定义（不含 inbound_tag 的旧契约），这里局部扩展以携带新字段。
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
  inbound_tag: '',
  enabled: true,
  priority: 0,
  remark: '',
})

const jsonError = ref('')

onMounted(() => {
  if (props.rule) {
    form.outbound_tag = props.rule.outbound_tag
    form.rule_json = props.rule.rule_json || ''
    form.domain = props.rule.domain || ''
    form.ip = props.rule.ip || ''
    form.port = props.rule.port || ''
    form.network = props.rule.network || ''
    form.inbound_tag = props.rule.inbound_tag || ''
    form.enabled = props.rule.enabled
    form.priority = props.rule.priority
    form.remark = props.rule.remark
  }
  visible.value = true
})

async function save() {
  jsonError.value = ''
  if (!form.outbound_tag?.trim()) {
    ElMessage.warning('请填写 OutboundTag（出站标签）')
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

function onClosed() {
  emit('close')
}
</script>

<template>
  <el-dialog
    :model-value="visible"
    :title="rule ? '编辑路由规则' : '新增路由规则'"
    width="680px"
    @closed="onClosed"
  >
    <el-form label-position="top">
      <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
        <el-form-item label="OutboundTag（出站标签，必填）">
          <el-select
            v-model="form.outbound_tag"
            style="width: 100%"
            filterable
            allow-create
            default-first-option
            placeholder="选择或输入出站标签"
          >
            <el-option v-for="t in outboundTags ?? []" :key="t" :label="t" :value="t" />
          </el-select>
        </el-form-item>
        <el-form-item label="Port（端口）">
          <el-input v-model="form.port" placeholder="如 443 或 80,443" />
        </el-form-item>
        <el-form-item label="Network（网络）">
          <el-select v-model="form.network" style="width: 100%" clearable>
            <el-option label="tcp" value="tcp" />
            <el-option label="udp" value="udp" />
          </el-select>
        </el-form-item>
        <el-form-item label="优先级 Priority">
          <el-input-number v-model="form.priority" :min="0" style="width: 100%" />
        </el-form-item>
      </div>

      <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
        <el-form-item label="启用">
          <el-switch v-model="form.enabled" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="form.remark" placeholder="选填" />
        </el-form-item>
      </div>

      <div class="sec-title">匹配条件（Domain / IP / InboundTag 支持逗号或换行分隔，也可填 JSON 数组）</div>
      <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 0 16px">
        <el-form-item label="Domain（域名）">
          <el-input
            v-model="form.domain"
            type="textarea"
            :rows="5"
            placeholder="如 geosite:netflix&#10;google.com&#10;*.example.com"
          />
        </el-form-item>
        <el-form-item label="IP（地址段）">
          <el-input
            v-model="form.ip"
            type="textarea"
            :rows="5"
            placeholder="如 geoip:cn&#10;8.8.8.8&#10;10.0.0.0/8"
          />
        </el-form-item>
      </div>
      <el-form-item label="InboundTag（入站标签匹配，选填）">
        <el-input
          v-model="form.inbound_tag"
          type="textarea"
          :rows="3"
          placeholder="如 vless-in&#10;api"
        />
        <p class="muted tip">逗号/换行分隔多 tag（如 vless-in,api），生成配置后写入路由规则 inboundTag 数组。</p>
      </el-form-item>

      <div class="sec-title">Rule JSON（高级：完整规则透传，覆盖上述匹配条件）</div>
      <el-input
        v-model="form.rule_json"
        type="textarea"
        :rows="5"
        class="code-textarea"
        placeholder='{"type":"field","inboundTag":["api"],"outboundTag":"direct"}'
      />
      <p class="muted tip">
        填写后生成配置时按原样透传（自动补 <code>type:"field"</code> 与 outboundTag），Domain/IP/Port/InboundTag 等字段不再参与组装。
      </p>

      <el-alert
        v-if="jsonError"
        type="error"
        :closable="false"
        :title="jsonError"
        style="margin-top: 10px"
      />
      <el-alert
        v-else
        type="info"
        :closable="false"
        show-icon
        title="保存后将自动重新生成该节点的 Xray 配置并推送（节点离线时保留，上线自动补推）"
        style="margin-top: 10px"
      />
    </el-form>
    <template #footer>
      <el-button @click="visible = false">取消</el-button>
      <el-button type="primary" :loading="saving" @click="save">
        <el-icon><Check /></el-icon>&nbsp;保存
      </el-button>
    </template>
  </el-dialog>
</template>

<style scoped lang="scss">
.sec-title {
  font-weight: 600;
  font-size: 13px;
  margin: 6px 0 10px;
  padding-top: 6px;
  border-top: 1px dashed var(--x-border);
  color: var(--x-primary);
}
.muted {
  color: var(--x-text-3);
}
.tip {
  font-size: 12px;
  margin: 6px 0 0;
}
.code-textarea {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12.5px;
  line-height: 1.5;
}
</style>
