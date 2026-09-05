<script setup lang="ts">
import { computed, nextTick, onMounted, reactive, ref } from 'vue'
import { Plus, Check, Refresh, CopyDocument, Document, Edit, Delete, Loading, FolderChecked, ArrowUp, ArrowDown } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import BaseCard from '@/components/base/BaseCard.vue'
import {
  createPermissionGroup,
  deletePermissionGroup,
  getPermissionGroups,
  getAccessPoints,
  setPermissionGroupAccessPoints,
  updatePermissionGroup,
  previewPermissionGroupTemplate,
  getSubTemplates,
  createSubTemplate,
  updateSubTemplate,
  deleteSubTemplate,
  type PermissionGroup,
  type TemplatePreviewResult,
  type SubTemplate,
} from '@/api/admin'
import type { UserAccessPoint } from '@/api/types'
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

// ===== 组内接入点优先级编辑器（2026-09-03：订阅节点输出顺序同此）=====
const apDialogOpen = ref(false)
const apTarget = ref<PermissionGroup | null>(null)
const orderingSaving = ref(false)
const orderedApIds = ref<number[]>([])
const selectedApId = ref<number | undefined>(undefined)
const allAPs = ref<UserAccessPoint[]>([])

const apNameMap = computed(() => {
  const m = new Map<number, string>()
  for (const ap of allAPs.value) m.set(ap.id, ap.name)
  return m
})
// 可选未加入列表的接入点（仅启用项可加入）
const availableAPs = computed(() => allAPs.value.filter((ap) => ap.enabled && !orderedApIds.value.includes(ap.id)))

async function openAPEditor(row: any) {
  apTarget.value = row
  orderedApIds.value = [...(row.access_point_ids || [])]
  apDialogOpen.value = true
  if (allAPs.value.length === 0) {
    try {
      const { data } = await getAccessPoints()
      if (data.code === 0) allAPs.value = data.data.items
    } catch (e) {
      ElMessage.error(errMsg(e, '加载接入点失败'))
    }
  }
}

function addAP() {
  const id = Number(selectedApId.value)
  if (!id || orderedApIds.value.includes(id)) return
  orderedApIds.value.push(id)
  selectedApId.value = undefined
}

function moveAP(idx: number, dir: -1 | 1) {
  const j = idx + dir
  if (j < 0 || j >= orderedApIds.value.length) return
  const arr = orderedApIds.value
  ;[arr[idx], arr[j]] = [arr[j], arr[idx]]
}

function removeAP(idx: number) {
  orderedApIds.value.splice(idx, 1)
}

async function saveOrdering() {
  if (!apTarget.value) return
  orderingSaving.value = true
  try {
    const { data } = await setPermissionGroupAccessPoints(apTarget.value.id, orderedApIds.value)
    if (data.code === 0) {
      ElMessage.success('接入点优先级已保存，订阅节点顺序即此排列')
      apDialogOpen.value = false
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存优先级失败'))
  } finally {
    orderingSaving.value = false
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

// 基础预设模板（系统推荐起点；高级模板已删，复杂配置请存入「我的模板」）
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

// ---- 我的模板库（命名模板：跨权限组保存与快速载入）----
const subTemplates = ref<SubTemplate[]>([])
const selectedTemplateId = ref<number | undefined>(undefined)

async function loadSubTemplates() {
  try {
    const { data } = await getSubTemplates()
    if (data.code === 0) subTemplates.value = data.data || []
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载模板库失败'))
  }
}

const selectedTemplate = computed(() => subTemplates.value.find((t) => t.id === selectedTemplateId.value))

function applySelectedTemplate() {
  const tpl = selectedTemplate.value
  if (!tpl) return
  templateCode.value = tpl.content
  ElMessage.success(`已载入「${tpl.name}」`)
}

async function saveAsTemplate() {
  const name = (await ElMessageBox.prompt('为当前编辑器中的模板起个名字', '另存为模板', {
    confirmButtonText: '保存',
    cancelButtonText: '取消',
    inputPattern: /\S+/,
    inputErrorMessage: '模板名不能为空',
  }).then((r) => r.value as string).catch(() => null))
  if (name === null) return
  try {
    const { data } = await createSubTemplate({ name: name.trim(), content: templateCode.value })
    if (data.code === 0) {
      ElMessage.success('已保存到模板库')
      await loadSubTemplates()
      selectedTemplateId.value = data.data.id
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存模板失败'))
  }
}

async function overwriteSelectedTemplate() {
  const tpl = selectedTemplate.value
  if (!tpl) return
  try {
    await ElMessageBox.confirm(`用编辑器当前内容覆盖模板库中的「${tpl.name}」？`, '更新模板', { type: 'warning' })
  } catch {
    return
  }
  try {
    const { data } = await updateSubTemplate(tpl.id, { name: tpl.name, content: templateCode.value })
    if (data.code === 0) {
      ElMessage.success('模板已更新')
      loadSubTemplates()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '更新模板失败'))
  }
}

async function removeSelectedTemplate() {
  const tpl = selectedTemplate.value
  if (!tpl) return
  try {
    await ElMessageBox.confirm(`删除模板库中的「${tpl.name}」？（已应用到权限组的模板不受影响）`, '删除模板', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deleteSubTemplate(tpl.id)
    if (data.code === 0) {
      ElMessage.success('模板已删除')
      selectedTemplateId.value = undefined
      loadSubTemplates()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除模板失败'))
  }
}

function openTemplateEditor(row: any) {
  templateTarget.value = row
  templateCode.value = row.clash_template || ''
  activeTab.value = 'edit'
  previewData.value = null
  selectedTemplateId.value = undefined
  templateDialogOpen.value = true
  if (subTemplates.value.length === 0) loadSubTemplates()
}

function loadPreset(type: 'basic' | 'clear') {
  if (type === 'basic') {
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
        <span class="muted" style="font-size: 12px">权限组用于组织与分发用户接入点；每个权限组可自由定制专属 Clash / Mihomo 订阅模板与分流策略。</span>
      </div>
    </div>

    <BaseCard title="权限分组列表">
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <el-icon class="is-loading" style="font-size: 26px; color: var(--x-primary)"><Loading /></el-icon>
      </div>

      <div v-else-if="list.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
        <el-icon style="font-size: 32px; color: var(--x-text-3)"><FolderChecked /></el-icon>
        <p style="margin-top: 8px">尚无权限组，点击右上角「新增权限组」</p>
      </div>

      <!-- 全局统一权限组卡片网格流 (自适应 1~4 列) -->
      <div v-else class="group-card-grid">
        <div v-for="row in list" :key="row.id" class="group-card">
          <!-- 头部 -->
          <div class="card-head">
            <div class="head-title">
              <span class="cell-mono muted" style="font-size: 11px">#{{ row.id }}</span>
              <span class="group-name" title="点击编辑权限组" @click="openEdit(row)">{{ row.name }}</span>
            </div>
            <span v-if="row.clash_template && row.clash_template.trim()" class="x-chip purple" style="font-size: 10.5px">
              自定义模板
            </span>
            <span v-else class="x-chip gray" style="font-size: 10.5px">
              系统默认
            </span>
          </div>

          <!-- 属性网格 -->
          <div class="card-grid">
            <div class="grid-item full-width">
              <span class="item-label">备注说明</span>
              <div class="item-value">{{ row.remark || '—' }}</div>
            </div>
            <div class="grid-item full-width">
              <span class="item-label">包含接入点 (Endpoints)</span>
              <div class="item-value">
                <template v-if="row.access_point_names && row.access_point_names.length">
                  <span
                    v-for="(name, idx) in row.access_point_names"
                    :key="name"
                    class="x-chip blue"
                    style="margin-right: 4px; margin-bottom: 2px; font-size: 10.5px"
                  >
                    {{ idx + 1 }}. {{ name }}
                  </span>
                  <el-button size="small" text type="primary" style="font-size: 11.5px" @click="openAPEditor(row)">
                    调整顺序
                  </el-button>
                </template>
                <span v-else class="muted font-11">暂无接入点（在拓扑画布中绑定该组）</span>
              </div>
            </div>
          </div>

          <!-- 底部操作栏 -->
          <div class="card-foot-actions">
            <el-button size="small" type="warning" plain @click="openTemplateEditor(row)">
              <el-icon><Document /></el-icon>&nbsp;订阅模板
            </el-button>
            <el-button size="small" type="primary" plain @click="openEdit(row)">
              <el-icon><Edit /></el-icon>&nbsp;编辑
            </el-button>
            <el-button size="small" type="danger" plain @click="remove(row)">
              <el-icon><Delete /></el-icon>&nbsp;删除
            </el-button>
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

    <!-- ===== 组内接入点优先级编辑弹窗 ===== -->
    <el-dialog
      v-model="apDialogOpen"
      :title="`接入点优先级 · ${apTarget?.name || ''}`"
      width="560px"
      :append-to-body="true"
    >
      <el-alert type="info" :closable="false" style="margin-bottom: 12px"
        title="列表顺序即该权限组订阅中节点的输出顺序（$PROXIES$ 注入与客户端显示同序）。同组调整不影响接入点在其他权限组中的优先级。"
      />
      <el-form label-position="top">
        <el-form-item label="添加接入点（下方列表调整顺序）">
          <div style="display: flex; gap: 8px; width: 100%">
            <el-select
              v-model="selectedApId"
              placeholder="选择启用中的接入点"
              style="flex: 1"
              :disabled="availableAPs.length === 0"
              filterable
            >
              <el-option v-for="ap in availableAPs" :key="ap.id" :label="ap.name" :value="ap.id" />
            </el-select>
            <el-button type="primary" :disabled="!selectedApId" @click="addAP">
              <el-icon><Plus /></el-icon>&nbsp;加入
            </el-button>
          </div>
        </el-form-item>

        <div v-if="orderedApIds.length === 0" class="muted" style="font-size: 12.5px; padding: 16px 0; text-align: center">
          当前权限组未绑定接入点（订阅将无节点）
        </div>
        <div v-else class="order-list">
          <div v-for="(apId, idx) in orderedApIds" :key="apId" class="order-row">
            <span class="order-idx">{{ idx + 1 }}</span>
            <span class="order-name">{{ apNameMap.get(apId) || ('#' + apId) }}</span>
            <div class="order-actions">
              <el-button size="small" text :icon="ArrowUp" :disabled="idx === 0" @click="moveAP(idx, -1)" />
              <el-button size="small" text :icon="ArrowDown" :disabled="idx === orderedApIds.length - 1" @click="moveAP(idx, 1)" />
              <el-button size="small" text type="danger" :icon="Delete" @click="removeAP(idx)" />
            </div>
          </div>
        </div>
      </el-form>
      <template #footer>
        <el-button @click="apDialogOpen = false">取消</el-button>
        <el-button type="primary" :loading="orderingSaving" @click="saveOrdering">保存顺序</el-button>
      </template>
    </el-dialog>

    <!-- ===== 订阅模板配置与实时预览弹窗 ===== -->
    <el-dialog
      v-model="templateDialogOpen"
      :title="`配置订阅模板 · ${templateTarget?.name || ''}`"
      width="820px"
      :append-to-body="true"
    >
      <!-- 顶部模板库与占位符栏 -->
      <div class="preset-section">
        <div class="preset-row">
          <span class="preset-label">我的模板：</span>
          <el-select
            v-model="selectedTemplateId"
            placeholder="选择已保存的模板"
            style="width: 220px"
            size="small"
            clearable
          >
            <el-option v-for="t in subTemplates" :key="t.id" :label="t.name" :value="t.id" />
          </el-select>
          <el-button size="small" type="primary" plain :disabled="!selectedTemplate" @click="applySelectedTemplate">载入</el-button>
          <el-button size="small" plain :disabled="!selectedTemplate" @click="overwriteSelectedTemplate">覆盖保存</el-button>
          <el-button size="small" type="danger" plain :disabled="!selectedTemplate" @click="removeSelectedTemplate">删除</el-button>
          <el-button size="small" @click="saveAsTemplate">另存为…</el-button>
        </div>

        <div class="preset-row" style="margin-top: 8px">
          <span class="preset-label">快捷载入：</span>
          <div class="preset-chips">
            <button type="button" class="preset-chip primary" @click="loadPreset('basic')">
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
            <button type="button" class="preset-chip code" @mousedown.prevent @click="insertPlaceholder('$FILTER_PROXIES(关键词)$')">
              + $FILTER_PROXIES(关键词)$
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
.order-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  max-height: 320px;
  overflow-y: auto;
  border: 1px solid var(--x-border);
  border-radius: 8px;
  padding: 8px;
}
.order-row {
  display: flex;
  align-items: center;
  gap: 10px;
  background: var(--x-bg);
  border: 1px solid var(--x-border-light);
  border-radius: 6px;
  padding: 6px 10px;
}
.order-idx {
  width: 22px;
  height: 22px;
  border-radius: 50%;
  background: var(--x-primary-soft);
  color: var(--x-primary);
  font-size: 11.5px;
  font-weight: 600;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}
.order-name {
  flex: 1;
  font-size: 12.5px;
  color: var(--x-text);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}
.order-actions {
  display: flex;
  gap: 2px;
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

/* ================= 全局统一权限组卡片网格流 ================= */
.group-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}

.group-card {
  background: var(--x-card, #ffffff);
  border: 1px solid var(--x-border, #e5e7eb);
  border-radius: var(--x-radius, 10px);
  padding: 14px;
  transition: all 0.2s cubic-bezier(0.2, 0, 0, 1);
  box-shadow: 0 1px 3px rgba(0, 0, 0, 0.04);
  display: flex;
  flex-direction: column;
  justify-content: space-between;

  &:hover {
    border-color: var(--x-border-hover, #cbd5e1);
    box-shadow: var(--x-shadow-md);
    transform: translateY(-1px);
  }

  .card-head {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding-bottom: 10px;
    border-bottom: 1px dashed var(--x-border, #e5e7eb);

    .head-title {
      display: flex;
      align-items: center;
      gap: 6px;
      flex-wrap: wrap;
    }

    .group-name {
      font-weight: 600;
      font-size: 13.5px;
      color: var(--x-text, #111827);
      cursor: pointer;
      &:hover {
        color: var(--x-primary);
      }
    }
  }

  .card-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px 12px;
    padding: 10px 0;

    .grid-item {
      display: flex;
      flex-direction: column;
      gap: 2px;

      &.full-width {
        grid-column: 1 / -1;
      }

      .item-label {
        font-size: 11px;
        color: var(--x-text-3, #9ca3af);
      }

      .item-value {
        font-size: 12.5px;
        color: var(--x-text, #1f2937);
        font-weight: 500;
      }
    }
  }

  .card-foot-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    padding-top: 10px;
    border-top: 1px solid var(--x-border-light, #f1f5f9);
    margin-top: 6px;

    .el-button {
      flex: 1;
      margin: 0;
      font-size: 12px;
      padding: 6px 8px;
      height: 30px;
    }
  }
}

@media (max-width: 640px) {
  .group-card-grid {
    grid-template-columns: 1fr;
  }
}
</style>

