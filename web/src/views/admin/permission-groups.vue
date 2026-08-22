<script setup lang="ts">
import { nextTick, onMounted, reactive, ref } from 'vue'
import { Plus, Delete, Edit, Document, Check, Refresh, CopyDocument, MagicStick } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import BaseCard from '@/components/base/BaseCard.vue'
import {
  createPermissionGroup,
  deletePermissionGroup,
  getPermissionGroups,
  updatePermissionGroup,
  previewPermissionGroupTemplate,
  type PermissionGroup,
  type TemplatePreviewResult,
} from '@/api/admin'
import { errMsg } from '@/api/http'

const list = ref<PermissionGroup[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const { data } = await getPermissionGroups()
    if (data.code === 0) list.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载权限组失败'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

// ---- 创建/编辑基础信息 ----
const formOpen = ref(false)
const editing = ref<PermissionGroup | null>(null)
const saving = ref(false)
const form = reactive({ name: '', remark: '' })

function openCreate() {
  editing.value = null
  form.name = ''
  form.remark = ''
  formOpen.value = true
}

function openEdit(row: any) {
  editing.value = row
  form.name = row.name
  form.remark = row.remark
  formOpen.value = true
}

async function save() {
  if (!form.name.trim()) {
    ElMessage.warning('请填写名称')
    return
  }
  saving.value = true
  try {
    const { data } = editing.value
      ? await updatePermissionGroup(editing.value.id, { name: form.name.trim(), remark: form.remark })
      : await createPermissionGroup({ name: form.name.trim(), remark: form.remark })
    if (data.code === 0) {
      ElMessage.success(editing.value ? '权限组已更新' : '权限组已创建')
      formOpen.value = false
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

async function remove(row: any) {
  try {
    await ElMessageBox.confirm(`确认删除权限组「${row.name}」？关联的套餐和节点权限引用将一并解除。`, '删除权限组', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deletePermissionGroup(row.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

// ===== 订阅模板编辑器 =====
const templateDialogOpen = ref(false)
const templateTarget = ref<PermissionGroup | null>(null)
const templateCode = ref('')
const templateSaving = ref(false)
const activeTab = ref('edit')

// 预览状态
const previewLoading = ref(false)
const previewData = ref<TemplatePreviewResult | null>(null)

// 推荐预设模板（以 docs/example.yaml 标准生产配置为准）
const ADVANCED_TEMPLATE = `mixed-port: 7890
allow-lan: true
ipv6: true
bind-address: '*'
mode: rule
log-level: info
unified-delay: true
tcp-concurrent: true
geodata-mode: true
geo-auto-update: true
geo-update-interval: 24
geox-url:
    geoip: 'https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geoip.dat'
    geosite: 'https://github.com/MetaCubeX/meta-rules-dat/releases/download/latest/geosite.dat'
dns:
    enable: true
    ipv6: true
    prefer-h3: false
    use-hosts: true
    use-system-hosts: true
    respect-rules: true
    enhanced-mode: fake-ip
    fake-ip-range: 198.18.0.1/16
    default-nameserver: [223.5.5.5, 119.29.29.29]
    proxy-server-nameserver: [223.5.5.5, 119.29.29.29]
    direct-nameserver: [223.5.5.5, 119.29.29.29]
    direct-nameserver-follow-policy: true
    fake-ip-filter: ['geosite:private', 'geosite:cn', +.lan, +.local, +.localhost, +.home.arpa, '*.msftncsi.com', '*.msftconnecttest.com', '+.stun.*', '+.stun.*.*', lens.l.google.com]
    nameserver-policy: { +.lan: system, +.local: system, +.home.arpa: system, 'geosite:cn': [223.5.5.5, 119.29.29.29], 'geosite:geolocation-!cn': ['tcp://1.1.1.1#节点选择', 'tcp://8.8.8.8#节点选择'] }
    nameserver: ['tcp://1.1.1.1#节点选择', 'tcp://8.8.8.8#节点选择']
proxies:
$PROXIES$
proxy-groups:
    - { name: 节点选择, type: select, proxies: [DIRECT, $ALL_PROXIES$] }
    - { name: 自动选择, type: url-test, url: http://cp.cloudflare.com/generate_204, interval: 300, proxies: [$ALL_PROXIES$] }
    - { name: 香港节点, type: select, proxies: [$FILTER_PROXIES(HK|香港)$] }
    - { name: 日本节点, type: select, proxies: [$FILTER_PROXIES(JP|日本)$] }
    - { name: 美国节点, type: select, proxies: [$FILTER_PROXIES(US|美国)$] }
    - { name: 台湾节点, type: select, proxies: [$FILTER_PROXIES(TW|台湾)$] }
    - { name: 新加坡节点, type: select, proxies: [$FILTER_PROXIES(SG|新加坡)$] }
    - { name: Anthropic, type: select, proxies: [节点选择, $FILTER_PROXIES(家宽|台湾|日本|香港)$] }
    - { name: Google, type: select, proxies: [节点选择, $ALL_PROXIES$] }
    - { name: OpenAI, type: select, proxies: [节点选择, $ALL_PROXIES$] }
rule-providers:
    google: { type: http, behavior: classical, format: yaml, url: 'https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/Google/Google.yaml', path: ./ruleset/ios-rule-script/google.yaml, interval: 86400, proxy: 节点选择 }
    anthropic: { type: http, behavior: classical, format: yaml, url: 'https://raw.githubusercontent.com/jinxinkai/clash-ai-rules/refs/heads/master/Anthropic.yaml', path: ./ruleset/clash-ai-rules/anthropic.yaml, interval: 86400, proxy: 节点选择, size-limit: 0 }
    gemini: { type: http, behavior: classical, format: yaml, url: 'https://raw.githubusercontent.com/jinxinkai/clash-ai-rules/refs/heads/master/Gemini.yaml', path: ./ruleset/clash-ai-rules/gemini.yaml, interval: 86400, proxy: 节点选择, size-limit: 0 }
    openai: { type: http, behavior: classical, format: yaml, url: 'https://raw.githubusercontent.com/jinxinkai/clash-ai-rules/refs/heads/master/OpenAI.yaml', path: ./ruleset/clash-ai-rules/openai.yaml, interval: 86400, proxy: 节点选择, size-limit: 0 }
    tiktok: { type: http, behavior: classical, format: yaml, url: 'https://raw.githubusercontent.com/blackmatrix7/ios_rule_script/master/rule/Clash/TikTok/TikTok.yaml', path: ./ruleset/ios-rule-script/tiktok.yaml, interval: 86400, proxy: 节点选择 }
rules:
    - 'DOMAIN,$PANEL_HOST$,DIRECT'
    - 'GEOIP,private,DIRECT,no-resolve'
    - 'GEOSITE,private,DIRECT'
    - 'DOMAIN,localhost,DIRECT'
    - 'DOMAIN-SUFFIX,lan,DIRECT'
    - 'DOMAIN-SUFFIX,local,DIRECT'
    - 'DOMAIN-SUFFIX,home.arpa,DIRECT'
    - 'IP-CIDR,0.0.0.0/8,DIRECT,no-resolve'
    - 'IP-CIDR,10.0.0.0/8,DIRECT,no-resolve'
    - 'IP-CIDR,100.64.0.0/10,DIRECT,no-resolve'
    - 'IP-CIDR,127.0.0.0/8,DIRECT,no-resolve'
    - 'IP-CIDR,169.254.0.0/16,DIRECT,no-resolve'
    - 'IP-CIDR,172.16.0.0/12,DIRECT,no-resolve'
    - 'IP-CIDR,192.168.0.0/16,DIRECT,no-resolve'
    - 'IP-CIDR6,::1/128,DIRECT,no-resolve'
    - 'IP-CIDR6,fc00::/7,DIRECT,no-resolve'
    - 'IP-CIDR6,fe80::/10,DIRECT,no-resolve'
    - 'IP-CIDR6,ff00::/8,DIRECT,no-resolve'
    - 'DOMAIN-SUFFIX,daily-cloudcode-pa.googleapis.com,Google'
    - 'DOMAIN-SUFFIX,daily-cloudcode-pa.sandbox.googleapis.com,Google'
    - 'DOMAIN-SUFFIX,googleapis.com,Google'
    - 'DOMAIN-SUFFIX,www.googleapis.com,Google'
    - 'DOMAIN-SUFFIX,play.googleapis.com,Google'
    - 'DOMAIN-SUFFIX,oauth2.googleapis.com,Google'
    - 'DOMAIN-SUFFIX,antigravity-unleash.goog,Google'
    - 'DOMAIN-SUFFIX,lh3.googleusercontent.com,Google'
    - 'RULE-SET,anthropic,Anthropic'
    - 'RULE-SET,openai,OpenAI'
    - 'RULE-SET,gemini,Google'
    - 'RULE-SET,google,Google'
    - 'DOMAIN-SUFFIX,cdn.bootcdn.net,DIRECT'
    - 'GEOSITE,cn,DIRECT'
    - 'GEOIP,CN,DIRECT'
    - 'MATCH,节点选择'
`

const BASIC_TEMPLATE = `mixed-port: 7890
allow-lan: true
mode: rule
log-level: info
ipv6: false

dns:
  enable: true
  listen: 0.0.0.0:1053
  enhanced-mode: fake-ip
  nameserver:
    - 223.5.5.5
    - 119.29.29.29

proxies:
$PROXIES$

proxy-groups:
  - { name: 节点选择, type: select, proxies: [DIRECT, $ALL_PROXIES$] }
  - { name: 自动选择, type: url-test, url: http://cp.cloudflare.com/generate_204, interval: 300, proxies: [$ALL_PROXIES$] }

rules:
  - 'DOMAIN,$PANEL_HOST$,DIRECT'
  - 'MATCH,节点选择'
`

function openTemplateEditor(row: any) {
  templateTarget.value = row
  templateCode.value = row.clash_template || ''
  activeTab.value = 'edit'
  previewData.value = null
  templateDialogOpen.value = true
}

function loadPreset(type: 'advanced' | 'basic' | 'clear') {
  if (type === 'advanced') {
    templateCode.value = ADVANCED_TEMPLATE
    ElMessage.success('已载入「多地区与流媒体高级模板」')
  } else if (type === 'basic') {
    templateCode.value = BASIC_TEMPLATE
    ElMessage.success('已载入「极简基础模板」')
  } else {
    templateCode.value = ''
    ElMessage.info('已清空模板（将使用系统内置默认模板）')
  }
}

const editorTextareaRef = ref<any>(null)

function insertPlaceholder(placeholder: string) {
  const el = (editorTextareaRef.value?.textarea ||
    editorTextareaRef.value?.$el?.querySelector('textarea')) as HTMLTextAreaElement | undefined

  if (!el) {
    // 降级：未获取到 textarea 元素时追加到末尾
    templateCode.value += (templateCode.value.endsWith('\n') ? '' : '\n') + placeholder + '\n'
    ElMessage.success(`已插入占位符 ${placeholder}`)
    return
  }

  const start = el.selectionStart ?? templateCode.value.length
  const end = el.selectionEnd ?? templateCode.value.length
  const text = templateCode.value

  const before = text.substring(0, start)
  const after = text.substring(end)
  templateCode.value = before + placeholder + after

  ElMessage.success(`已在光标处插入 ${placeholder}`)

  // 恢复光标至插入内容之后并保持聚焦
  nextTick(() => {
    el.focus()
    const newPos = start + placeholder.length
    el.setSelectionRange(newPos, newPos)
  })
}

async function fetchPreview() {
  if (!templateTarget.value) return
  previewLoading.value = true
  try {
    const { data } = await previewPermissionGroupTemplate(templateTarget.value.id, templateCode.value)
    if (data.code === 0) {
      previewData.value = data.data
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '编译预览失败'))
  } finally {
    previewLoading.value = false
  }
}

function onTabChange(tab: any) {
  if (tab === 'preview') {
    fetchPreview()
  }
}

async function saveTemplate() {
  if (!templateTarget.value) return
  templateSaving.value = true
  try {
    const { data } = await updatePermissionGroup(templateTarget.value.id, {
      clash_template: templateCode.value,
    })
    if (data.code === 0) {
      ElMessage.success('订阅模板已保存')
      templateDialogOpen.value = false
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存模板失败'))
  } finally {
    templateSaving.value = false
  }
}

async function copyPreview() {
  if (!previewData.value?.rendered) return
  try {
    await navigator.clipboard.writeText(previewData.value.rendered)
    ElMessage.success('预览 YAML 配置已复制到剪贴板')
  } catch {
    ElMessage.warning('复制失败')
  }
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-button type="primary" @click="openCreate"><el-icon><Plus /></el-icon>&nbsp;新增权限组</el-button>
        <span class="muted" style="font-size: 12px">节点入站定义所属权限组；每个权限组可自由定制专属 Clash / Mihomo 订阅模板与分流策略。</span>
      </div>
    </div>

    <BaseCard>
      <!-- 桌面端表格视图 -->
      <div class="desktop-table-view">
        <el-table v-loading="loading" :data="list">
          <el-table-column prop="id" label="ID" width="70">
            <template #default="{ row }"><code class="cell-mono">#{{ row.id }}</code></template>
          </el-table-column>
          <el-table-column prop="name" label="权限组名称" min-width="160">
            <template #default="{ row }"><span style="font-weight: 600">{{ row.name }}</span></template>
          </el-table-column>
          <el-table-column prop="remark" label="备注说明" min-width="180" />
          <el-table-column label="包含入站节点" min-width="200">
            <template #default="{ row }">
              <template v-if="row.inbound_tags && row.inbound_tags.length">
                <span
                  v-for="tag in row.inbound_tags"
                  :key="tag"
                  class="x-chip blue"
                  style="margin-right: 4px; margin-bottom: 2px"
                >
                  {{ tag }}
                </span>
              </template>
              <span v-else class="muted" style="font-size: 12px">暂无节点（在「节点」页编辑入站加入）</span>
            </template>
          </el-table-column>
          <el-table-column label="订阅模板" width="130">
            <template #default="{ row }">
              <span v-if="row.clash_template && row.clash_template.trim()" class="x-chip purple">
                自定义模板
              </span>
              <span v-else class="x-chip gray">
                系统默认
              </span>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="220" fixed="right">
            <template #default="{ row }">
              <el-button size="small" text type="warning" @click="openTemplateEditor(row)">
                订阅模板
              </el-button>
              <el-button size="small" text type="primary" @click="openEdit(row)">
                编辑
              </el-button>
              <el-button size="small" text type="danger" @click="remove(row)">
                删除
              </el-button>
            </template>
          </el-table-column>
          <template #empty><div class="table-empty">尚无权限组，点击右上角「新增权限组」</div></template>
        </el-table>
      </div>

      <!-- 移动端卡片流视图 -->
      <div class="mobile-cards-view">
        <div v-if="list.length === 0" style="text-align: center; padding: 36px 0; color: var(--x-text-3); font-size: 13.5px">
          尚无权限组，点击右上角「新增权限组」
        </div>
        <div v-else class="mobile-data-card-list">
          <div v-for="row in list" :key="row.id" class="mobile-data-card">
            <div class="card-head">
              <div class="head-title">
                <span class="cell-mono muted font-12">#{{ row.id }}</span>
                <span style="font-weight: 700">{{ row.name }}</span>
              </div>
              <el-tag v-if="row.clash_template && row.clash_template.trim()" size="small" type="success" effect="light">
                自定义模板
              </el-tag>
              <el-tag v-else size="small" type="info" effect="plain">
                系统默认
              </el-tag>
            </div>

            <div class="card-grid">
              <div class="grid-item full-width">
                <span class="item-label">备注说明</span>
                <div class="item-value">{{ row.remark || '—' }}</div>
              </div>
              <div class="grid-item full-width">
                <span class="item-label">包含入站节点</span>
                <div class="item-value">
                  <template v-if="row.inbound_tags && row.inbound_tags.length">
                    <el-tag
                      v-for="tag in row.inbound_tags"
                      :key="tag"
                      size="small"
                      effect="plain"
                      style="margin-right: 4px; margin-bottom: 2px"
                    >
                      {{ tag }}
                    </el-tag>
                  </template>
                  <span v-else class="muted font-12">暂无节点</span>
                </div>
              </div>
            </div>

            <div class="card-foot-actions">
              <el-button size="small" type="warning" plain @click="openTemplateEditor(row)">
                订阅模板
              </el-button>
              <el-button size="small" type="primary" plain @click="openEdit(row)">
                编辑
              </el-button>
              <el-button size="small" type="danger" plain @click="remove(row)">
                删除
              </el-button>
            </div>
          </div>
        </div>
      </div>
    </BaseCard>

    <!-- ===== 权限组基础信息编辑弹窗 ===== -->
    <el-dialog v-model="formOpen" :title="editing ? '编辑权限组' : '新增权限组'" width="440px" :append-to-body="true">
      <el-form label-position="top">
        <el-form-item label="名称"><el-input v-model="form.name" placeholder="如 VIP 1 / 基础套餐组" /></el-form-item>
        <el-form-item label="备注说明"><el-input v-model="form.remark" placeholder="选填，如 普通节点权限组" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="formOpen = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>

    <!-- ===== 订阅模板配置与实时预览弹窗 ===== -->
    <el-dialog
      v-model="templateDialogOpen"
      :title="`配置订阅模板 · ${templateTarget?.name || ''}`"
      width="820px"
      :append-to-body="true"
    >
      <!-- 顶部常用预设与占位符栏 -->
      <div class="preset-section">
        <div class="preset-row">
          <span class="preset-label">载入推荐模板：</span>
          <div class="preset-chips">
            <button type="button" class="preset-chip primary" @click="loadPreset('advanced')">
              多地区与流媒体高级模板
            </button>
            <button type="button" class="preset-chip" @click="loadPreset('basic')">
              极简基础模板
            </button>
            <button type="button" class="preset-chip danger" @click="loadPreset('clear')">
              清空（恢复系统默认）
            </button>
          </div>
        </div>

        <div class="preset-row" style="margin-top: 8px">
          <span class="preset-label">常用占位符：</span>
          <div class="preset-chips">
            <button type="button" class="preset-chip code" @mousedown.prevent @click="insertPlaceholder('$PROXIES$')">
              + $PROXIES$（节点池）
            </button>
            <button type="button" class="preset-chip code" @mousedown.prevent @click="insertPlaceholder('$ALL_PROXIES$')">
              + $ALL_PROXIES$（全部节点）
            </button>
            <button type="button" class="preset-chip code" @mousedown.prevent @click="insertPlaceholder('$FILTER_PROXIES(HK|香港)$')">
              + 香港过滤
            </button>
            <button type="button" class="preset-chip code" @mousedown.prevent @click="insertPlaceholder('$FILTER_PROXIES(JP|日本)$')">
              + 日本过滤
            </button>
            <button type="button" class="preset-chip code" @mousedown.prevent @click="insertPlaceholder('$FILTER_PROXIES(TW|台湾)$')">
              + 台湾过滤
            </button>
            <button type="button" class="preset-chip code" @mousedown.prevent @click="insertPlaceholder('$FILTER_PROXIES(US|美国)$')">
              + 美国过滤
            </button>
            <button type="button" class="preset-chip code" @mousedown.prevent @click="insertPlaceholder('$FILTER_PROXIES(SG|新加坡)$')">
              + 新加坡过滤
            </button>
            <button type="button" class="preset-chip code" @mousedown.prevent @click="insertPlaceholder('$FILTER_PROXIES(NF|流媒体)$')">
              + 流媒体过滤
            </button>
            <button type="button" class="preset-chip code" @mousedown.prevent @click="insertPlaceholder('$PANEL_HOST$')">
              + $PANEL_HOST$（面板防回环）
            </button>
          </div>
        </div>
      </div>

      <el-tabs v-model="activeTab" @tab-change="onTabChange">
        <!-- TAB 1: 模板编辑 -->
        <el-tab-pane label="模板代码 (YAML)" name="edit">
          <el-input
            ref="editorTextareaRef"
            v-model="templateCode"
            type="textarea"
            :rows="15"
            class="code-textarea"
            placeholder="留空则使用系统内置默认模板。填写后将在 proxies 和 proxy-groups 处按占位符注入该权限组的节点。"
          />
          <div class="tip-banner" style="margin-top: 10px">
            占位符说明：<code>$PROXIES$</code> 自动展开为当前权限组所有可用 VLESS 节点；<code>$ALL_PROXIES$</code> 展开为全部节点名称；<code>$FILTER_PROXIES(关键词)$</code> 自动过滤匹配该地区的节点名称（匹配为空时自动兜底）。
          </div>
        </el-tab-pane>

        <!-- TAB 2: 实时编译预览 -->
        <el-tab-pane label="实时编译预览 (Preview)" name="preview">
          <div v-loading="previewLoading">
            <div v-if="previewData" class="preview-header">
              <div class="preview-stats">
                <span class="stat-badge">
                  注入节点数：<strong>{{ previewData.proxy_count }}</strong>
                </span>
                <span v-if="previewData.is_sample_nodes" class="stat-badge warning">
                  该组暂无入站节点，已使用样例节点模拟编译
                </span>
              </div>
              <div style="display: flex; gap: 8px">
                <el-button size="small" :icon="Refresh" @click="fetchPreview">刷新预览</el-button>
                <el-button size="small" type="primary" plain :icon="CopyDocument" @click="copyPreview">复制预览配置</el-button>
              </div>
            </div>

            <!-- 匹配到的节点芯片预览 -->
            <div v-if="previewData?.proxy_names && previewData.proxy_names.length" class="matched-nodes-box">
              <span class="box-label">组内可用节点池：</span>
              <span v-for="name in previewData.proxy_names" :key="name" class="node-chip cell-mono">
                {{ name }}
              </span>
            </div>

            <el-input
              :model-value="previewData?.rendered || '正在编译渲染...'"
              type="textarea"
              :rows="13"
              readonly
              class="code-textarea preview-box"
              style="margin-top: 10px"
            />
          </div>
        </el-tab-pane>
      </el-tabs>

      <template #footer>
        <div style="display: flex; justify-content: space-between; align-items: center">
          <span class="muted" style="font-size: 12px">保存后该权限组用户请求订阅时将直接返回此定制配置</span>
          <div style="display: flex; gap: 10px">
            <el-button @click="templateDialogOpen = false">取消</el-button>
            <el-button type="primary" :loading="templateSaving" @click="saveTemplate">
              <el-icon><Check /></el-icon>&nbsp;保存模板
            </el-button>
          </div>
        </div>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.muted { color: var(--x-text-3); }
.cell-mono {
  font-family: var(--x-font-mono, monospace);
  font-size: 12px;
}
.table-empty { padding: 30px 0; text-align: center; color: var(--x-text-3); font-size: 13px; }

.preset-section {
  background: var(--x-bg);
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 10px 14px;
  margin-bottom: 12px;
}
.preset-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.preset-label {
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
  background: var(--x-card, #fff);
  border: 1px solid var(--x-border);
  padding: 3px 8px;
  border-radius: 6px;
  font-size: 11.5px;
  color: var(--x-text);
  cursor: pointer;
  transition: all 0.2s ease;
  &:hover {
    border-color: var(--x-primary);
    color: var(--x-primary);
    background: rgba(99, 102, 241, 0.06);
  }
  &.primary {
    border-color: var(--x-primary);
    color: var(--x-primary);
    background: var(--x-primary-soft);
  }
  &.danger {
    color: var(--x-danger);
    &:hover {
      border-color: var(--x-danger);
      background: var(--x-danger-soft);
    }
  }
  &.code {
    font-family: var(--x-font-mono, monospace);
    font-size: 11px;
  }
}
.code-textarea {
  font-family: var(--x-font-mono, 'JetBrains Mono', Consolas, monospace);
  font-size: 12px;
  line-height: 1.5;
}
.preview-box {
  background: #1e1e1e;
  color: #d4d4d4;
}
.tip-banner {
  font-size: 11.5px;
  color: var(--x-text-2);
  line-height: 1.5;
  background: rgba(99, 102, 241, 0.06);
  border-left: 3px solid var(--x-primary);
  padding: 6px 10px;
  border-radius: 0 4px 4px 0;
}
.preview-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 8px;
  flex-wrap: wrap;
  gap: 8px;
}
.preview-stats {
  display: flex;
  gap: 8px;
  align-items: center;
}
.stat-badge {
  font-size: 12px;
  background: var(--x-bg);
  border: 1px solid var(--x-border);
  padding: 3px 8px;
  border-radius: 6px;
  color: var(--x-text);
  &.warning {
    background: var(--x-warning-soft);
    border-color: #fde68a;
    color: #92400e;
  }
}
.matched-nodes-box {
  display: flex;
  align-items: center;
  gap: 6px;
  flex-wrap: wrap;
  background: var(--x-bg);
  border: 1px solid var(--x-border);
  border-radius: 6px;
  padding: 6px 10px;
  font-size: 11.5px;
}
.box-label {
  color: var(--x-text-2);
  font-weight: 500;
}
.node-chip {
  background: var(--x-card, #fff);
  border: 1px solid var(--x-border);
  border-radius: 4px;
  padding: 1px 6px;
  font-size: 11px;
}
</style>

