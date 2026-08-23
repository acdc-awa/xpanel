<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Plus, Delete, MagicStick } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import BaseCard from '@/components/base/BaseCard.vue'
import { createCert, deleteCert, generateSelfSignedCert, getCerts, updateCert, type CertItem } from '@/api/admin'
import { errMsg } from '@/api/http'

const list = ref<CertItem[]>([])
const loading = ref(false)

async function load() {
  loading.value = true
  try {
    const { data } = await getCerts()
    if (data.code === 0) list.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载证书失败'))
  } finally {
    loading.value = false
  }
}
onMounted(load)

// ---- 一键自签（链式代理 TLS：自动 pin 防 MITM）----
const ssOpen = ref(false)
const ssSaving = ref(false)
const ssForm = reactive({ domain: '', remark: '' })

function openSelfSigned() {
  ssForm.domain = ''
  ssForm.remark = ''
  ssOpen.value = true
}

async function saveSelfSigned() {
  if (!ssForm.domain.trim()) {
    ElMessage.warning('请填写域名/标识')
    return
  }
  ssSaving.value = true
  try {
    const { data } = await generateSelfSignedCert({ domain: ssForm.domain.trim(), remark: ssForm.remark })
    if (data.code === 0) {
      ElMessage.success('自签证书已生成并推送引用节点；中转链路将自动 pin 该证书')
      ssOpen.value = false
      load()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '生成失败'))
  } finally {
    ssSaving.value = false
  }
}

// ---- 上传/编辑 ----
const formOpen = ref(false)
const editing = ref<CertItem | null>(null)
const saving = ref(false)
const form = reactive({ domain: '', cert_pem: '', key_pem: '', remark: '' })

function openCreate() {
  editing.value = null
  form.domain = ''
  form.cert_pem = ''
  form.key_pem = ''
  form.remark = ''
  formOpen.value = true
}

function openEdit(row: any) {
  editing.value = row
  form.domain = row.domain
  form.remark = row.remark
  form.cert_pem = ''
  form.key_pem = ''
  formOpen.value = true
}

async function save() {
  if (!form.domain.trim() || !form.cert_pem.trim() || !form.key_pem.trim()) {
    ElMessage.warning('请填写域名与 PEM 内容（编辑时可仅改备注）')
    return
  }
  saving.value = true
  try {
    const { data } = editing.value
      ? await updateCert(editing.value.id, { remark: form.remark })
      : await createCert({ domain: form.domain.trim(), cert_pem: form.cert_pem, key_pem: form.key_pem, remark: form.remark })
    if (data.code === 0) {
      ElMessage.success(editing.value ? '证书已更新' : '证书已上传并推送引用节点')
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
    await ElMessageBox.confirm(`确认删除证书「${row.domain}」？`, '删除证书', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deleteCert(row.id)
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

function daysLeft(notAfter: string): number {
  const t = new Date(notAfter.replace(' ', 'T'))
  return Math.ceil((t.getTime() - Date.now()) / 86400000)
}

function expireTag(row: any) {
  const d = daysLeft(row.not_after)
  if (d < 0) return { type: 'danger' as const, text: `已过期 ${-d} 天` }
  if (d <= 30) return { type: 'warning' as const, text: `剩 ${d} 天` }
  return { type: 'success' as const, text: `剩 ${d} 天` }
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <el-button type="primary" @click="openCreate"><el-icon><Plus /></el-icon>&nbsp;上传证书</el-button>
        <el-button type="success" plain @click="openSelfSigned"><el-icon><MagicStick /></el-icon>&nbsp;生成自签证书</el-button>
        <span class="muted" style="font-size: 12px">证书由主控下发（push_cert）到引用它的节点，落盘 /etc/xray/certs/&lt;domain&gt;/，xray 每小时热重载不重启；自签证书用于链式代理 TLS，中转出站自动 pin 防 MITM</span>
      </div>
    </div>

    <BaseCard>
      <!-- 桌面端表格视图 -->
      <div class="desktop-table-view">
        <el-table v-loading="loading" :data="list" size="small">
          <el-table-column prop="domain" label="域名" min-width="200">
            <template #default="{ row }">
              <code class="cell-mono">{{ row.domain }}</code>
              <el-tooltip v-if="row.self_signed" content="自签证书 · 链式代理 TLS 中转出站自动注入 pinnedPeerCertSha256" placement="top">
                <span class="x-chip green" style="margin-left: 6px">自签·自动Pin</span>
              </el-tooltip>
            </template>
          </el-table-column>
          <el-table-column label="到期" width="170">
            <template #default="{ row }">
              <span class="x-chip" :class="expireTag(row).type === 'danger' ? 'red' : (expireTag(row).type === 'warning' ? 'orange' : 'green')">
                {{ expireTag(row).text }}
              </span>
            </template>
          </el-table-column>
          <el-table-column prop="remark" label="备注" min-width="140" />
          <el-table-column label="引用节点（服务器 / 入站）" min-width="200">
            <template #default="{ row }">
              <template v-if="row.refs && row.refs.length > 0">
                <span v-for="r in row.refs" :key="r.inbound_id" class="x-chip purple" style="margin: 1px 4px 1px 0">
                  {{ r.server_name }} / {{ r.inbound_tag }}
                </span>
              </template>
              <span v-else class="muted">未引用</span>
            </template>
          </el-table-column>
          <el-table-column prop="created_at" label="上传时间" width="130" />
          <el-table-column label="操作" width="140" fixed="right">
            <template #default="{ row }">
              <el-button size="small" text type="primary" @click="openEdit(row)">编辑</el-button>
              <el-button size="small" text type="danger" @click="remove(row)"><el-icon><Delete /></el-icon></el-button>
            </template>
          </el-table-column>
          <template #empty><div class="table-empty">尚无证书，点击右上角「上传证书」</div></template>
        </el-table>
      </div>

      <!-- 移动端卡片流视图 -->
      <div class="mobile-cards-view">
        <div v-if="list.length === 0" style="text-align: center; padding: 36px 0; color: var(--x-text-3); font-size: 13.5px">
          尚无证书，点击右上角「上传证书」
        </div>
        <div v-else class="mobile-data-card-list">
          <div v-for="row in list" :key="row.id" class="mobile-data-card">
            <div class="card-head">
              <div class="head-title">
                <code class="cell-mono" style="font-weight: 700; color: var(--x-primary)">{{ row.domain }}</code>
                <span v-if="row.self_signed" class="x-chip green" style="margin-left: 6px">自签·自动Pin</span>
              </div>
              <el-tag :type="expireTag(row).type" size="small">{{ expireTag(row).text }}</el-tag>
            </div>

            <div class="card-grid">
              <div class="grid-item">
                <span class="item-label">到期日期</span>
                <div class="item-value cell-mono font-12">{{ row.not_after || '—' }}</div>
              </div>
              <div class="grid-item">
                <span class="item-label">备注</span>
                <div class="item-value">{{ row.remark || '—' }}</div>
              </div>
              <div class="grid-item full-width">
                <span class="item-label">引用节点</span>
                <div class="item-value">
                  <template v-if="row.refs && row.refs.length > 0">
                    <el-tag v-for="r in row.refs" :key="r.inbound_id" size="small" style="margin: 1px 4px 1px 0">
                      {{ r.server_name }} / {{ r.inbound_tag }}
                    </el-tag>
                  </template>
                  <span v-else class="muted font-12">未被任何入站引用</span>
                </div>
              </div>
            </div>

            <div class="card-foot-actions">
              <el-button size="small" type="primary" plain @click="openEdit(row)">
                <el-icon><Edit /></el-icon>&nbsp;编辑
              </el-button>
              <el-button size="small" type="danger" plain @click="remove(row)">
                <el-icon><Delete /></el-icon>&nbsp;删除
              </el-button>
            </div>
          </div>
        </div>
      </div>
    </BaseCard>

    <el-dialog v-model="ssOpen" title="生成自签证书（链式代理 TLS 专用）" width="520px" :append-to-body="true">
      <el-alert type="success" :closable="false" show-icon style="margin-bottom: 14px">
        <p>生成 ECDSA P-256 十年期自签证书，主控自动计算 pin（SHA-256）并下发落地节点。</p>
        <p>拓扑中引用该证书入站的中转出站将<b>自动注入 pinnedPeerCertSha256</b>——pin 命中即验证通过，自签亦可防 MITM；换证时两端配置自动联动重推。</p>
      </el-alert>
      <el-form label-position="top">
        <el-form-item label="域名 / 节点标识（唯一）">
          <el-input v-model="ssForm.domain" placeholder="如 relay-jp-01 或 relay.example.com" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="ssForm.remark" placeholder="选填" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="ssOpen = false">取消</el-button>
        <el-button type="success" :loading="ssSaving" @click="saveSelfSigned">生成并下发</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="formOpen" :title="editing ? '编辑证书（' + form.domain + '）' : '上传证书'" width="640px" :append-to-body="true">
      <el-form label-position="top">
        <el-form-item label="域名（唯一）">
          <el-input v-model="form.domain" :disabled="!!editing" placeholder="如 cdn.example.com" />
        </el-form-item>
        <template v-if="!editing">
          <el-form-item label="证书链 PEM（fullchain）">
            <el-input v-model="form.cert_pem" type="textarea" :rows="5" placeholder="-----BEGIN CERTIFICATE-----..." />
          </el-form-item>
          <el-form-item label="私钥 PEM">
            <el-input v-model="form.key_pem" type="textarea" :rows="5" placeholder="-----BEGIN PRIVATE KEY-----..." />
          </el-form-item>
        </template>
        <el-form-item label="备注">
          <el-input v-model="form.remark" placeholder="选填" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="formOpen = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.muted { color: var(--x-text-3); }
.cell-mono { font-family: var(--x-mono); font-size: 12px; }
.table-empty { padding: 30px 0; text-align: center; color: var(--x-text-3); font-size: 13px; }
</style>
