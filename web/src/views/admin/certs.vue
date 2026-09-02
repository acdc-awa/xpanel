<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Plus, Delete, MagicStick, Edit } from '@element-plus/icons-vue'
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

    <BaseCard title="TLS 证书列表">
      <div v-if="loading" style="padding: 48px 0; text-align: center">
        <el-icon class="is-loading" style="font-size: 26px; color: var(--x-primary)"><Loading /></el-icon>
      </div>

      <div v-else-if="list.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
        <el-icon style="font-size: 32px; color: var(--x-text-3)"><Key /></el-icon>
        <p style="margin-top: 8px">尚无证书，点击右上角「上传证书」或「生成自签证书」</p>
      </div>

      <!-- 全局统一证书卡片网格流 (自适应 1~4 列) -->
      <div v-else class="cert-card-grid">
        <div v-for="row in list" :key="row.id" class="cert-card">
          <!-- 头部 -->
          <div class="card-head">
            <div class="head-title">
              <code class="cell-mono cert-domain">{{ row.domain }}</code>
              <el-tooltip v-if="row.self_signed" content="自签证书 · 链式代理 TLS 中转出站自动注入 pinnedPeerCertSha256" placement="top">
                <span class="x-chip green" style="font-size: 10px; padding: 1px 5px">自签·自动Pin</span>
              </el-tooltip>
            </div>
            <span class="x-chip" :class="expireTag(row).type === 'danger' ? 'red' : (expireTag(row).type === 'warning' ? 'orange' : 'green')" style="font-size: 10.5px">
              {{ expireTag(row).text }}
            </span>
          </div>

          <!-- 证书属性网格 -->
          <div class="card-grid">
            <div class="grid-item">
              <span class="item-label">证书备注</span>
              <div class="item-value">{{ row.remark || '标准 TLS 证书' }}</div>
            </div>
            <div class="grid-item">
              <span class="item-label">到期日期</span>
              <div class="item-value cell-mono font-12">{{ row.not_after || '—' }}</div>
            </div>
            <div class="grid-item full-width">
              <span class="item-label">引用接入节点</span>
              <div class="item-value">
                <template v-if="row.refs && row.refs.length > 0">
                  <span v-for="r in row.refs" :key="r.inbound_id" class="x-chip purple" style="margin: 1px 4px 1px 0; font-size: 10.5px">
                    {{ r.server_name }} / {{ r.inbound_tag }}
                  </span>
                </template>
                <span v-else class="muted font-11">未被任何入站引用</span>
              </div>
            </div>
          </div>

          <!-- 底部操作栏 -->
          <div class="card-foot-actions">
            <el-button size="small" type="primary" plain @click="openEdit(row)">
              <el-icon><Edit /></el-icon>&nbsp;编辑证书
            </el-button>
            <el-button size="small" type="danger" plain @click="remove(row)">
              <el-icon><Delete /></el-icon>&nbsp;删除
            </el-button>
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

/* ================= 全局统一证书卡片网格流 ================= */
.cert-card-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
  gap: 14px;
}

.cert-card {
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

    .cert-domain {
      font-weight: 700;
      font-size: 13.5px;
      color: var(--x-primary);
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
  .cert-card-grid {
    grid-template-columns: 1fr;
  }
}
</style>
