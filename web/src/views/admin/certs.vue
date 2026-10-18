<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Plus, Delete, MagicStick, Edit, Loading, Key, Refresh, CopyDocument, Stamp, View } from '@element-plus/icons-vue'
import { ElMessage, ElMessageBox } from '@/components/cloudscape'
import BaseCard from '@/components/base/BaseCard.vue'
import TipIcon from '@/components/base/TipIcon.vue'
import {
  getCerts,
  createCert,
  generateSelfSignedCert,
  updateCert,
  deleteCert,
  reissueCert,
  getCACerts,
  createCACert,
  generateCACert,
  issueCertFromCA,
  deleteCACert,
  type CertItem,
  type CACertItem,
} from '@/api/admin'
import { errMsg } from '@/api/http'

const activeTab = ref<'server' | 'ca'>('server')

// ===== 密钥算法选项 =====
const keyTypeOptions = [
  { label: 'ECDSA P-256 (推荐，现代高效)', value: 'ecdsa_p256' },
  { label: 'ECDSA P-384 (高强度椭圆曲线)', value: 'ecdsa_p384' },
  { label: 'RSA 2048 (传统高兼容)', value: 'rsa_2048' },
  { label: 'RSA 4096 (高强度 RSA)', value: 'rsa_4096' },
  { label: 'Ed25519 (轻量高速)', value: 'ed25519' },
]

// ================= TLS 服务端证书 =================
const certList = ref<CertItem[]>([])
const certLoading = ref(false)

async function loadCerts() {
  certLoading.value = true
  try {
    const { data } = await getCerts()
    if (data.code === 0) certList.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载证书失败'))
  } finally {
    certLoading.value = false
  }
}

// ================= CA 根证书 =================
const caList = ref<CACertItem[]>([])
const caLoading = ref(false)

async function loadCAs() {
  caLoading.value = true
  try {
    const { data } = await getCACerts()
    if (data.code === 0) caList.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载 CA 证书失败'))
  } finally {
    caLoading.value = false
  }
}

function loadAll() {
  loadCerts()
  loadCAs()
}
onMounted(loadAll)

// ---- 一键重签/续期 ----
async function handleReissue(row: CertItem) {
  try {
    await ElMessageBox.confirm(
      `确定为证书「${row.domain}」一键重新签发/续期？\n签发后将自动下发到落地节点，并重新渲染中转出站配置。`,
      '证书重新签发',
      { type: 'warning', confirmButtonText: '立即重签', cancelButtonText: '取消' },
    )
  } catch {
    return
  }
  try {
    const { data } = await reissueCert(row.id)
    if (data.code === 0) {
      ElMessage.success('证书已重签并推送到节点！')
      loadCerts()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '重签失败'))
  }
}

// ---- 生成自签证书（高级弹窗）----
const ssOpen = ref(false)
const ssSaving = ref(false)
const ssForm = reactive({
  domain: '',
  san_dns: '',
  san_ip: '',
  organization: '',
  country: '',
  key_type: 'ecdsa_p256',
  valid_days: 3650,
  remark: '',
})

function openSelfSigned() {
  ssForm.domain = ''
  ssForm.san_dns = ''
  ssForm.san_ip = ''
  ssForm.organization = 'xray-panel relay'
  ssForm.country = ''
  ssForm.key_type = 'ecdsa_p256'
  ssForm.valid_days = 3650
  ssForm.remark = ''
  ssOpen.value = true
}

async function saveSelfSigned() {
  if (!ssForm.domain.trim()) {
    ElMessage.warning('请填写域名/通用名称')
    return
  }
  ssSaving.value = true
  try {
    const dnsList = ssForm.san_dns
      .split(/[,，\n]/)
      .map((s) => s.trim())
      .filter(Boolean)
    const ipList = ssForm.san_ip
      .split(/[,，\n]/)
      .map((s) => s.trim())
      .filter(Boolean)

    const { data } = await generateSelfSignedCert({
      domain: ssForm.domain.trim(),
      san_dns: dnsList,
      san_ip: ipList,
      organization: ssForm.organization.trim(),
      country: ssForm.country.trim(),
      key_type: ssForm.key_type,
      valid_days: Number(ssForm.valid_days) || 3650,
      remark: ssForm.remark.trim(),
    })
    if (data.code === 0) {
      ElMessage.success('自签证书已生成并推送引用服务器；中转链路将自动 pin 该证书')
      ssOpen.value = false
      loadCerts()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '生成失败'))
  } finally {
    ssSaving.value = false
  }
}

// ---- 基于 CA 签发证书 ----
const issueOpen = ref(false)
const issueSaving = ref(false)
const issueForm = reactive({
  ca_id: 0,
  domain: '',
  san_dns: '',
  san_ip: '',
  organization: '',
  country: '',
  key_type: 'ecdsa_p256',
  valid_days: 365,
  remark: '',
})

function openIssue(preselectedCaId = 0) {
  const availableCAs = caList.value.filter((ca) => ca.has_key)
  if (availableCAs.length === 0) {
    ElMessage.warning('当前暂无可用于签发的 CA（需持有私钥），请先生成或导入完整 CA')
    return
  }
  issueForm.ca_id = preselectedCaId || availableCAs[0].id
  issueForm.domain = ''
  issueForm.san_dns = ''
  issueForm.san_ip = ''
  issueForm.organization = ''
  issueForm.country = ''
  issueForm.key_type = 'ecdsa_p256'
  issueForm.valid_days = 365
  issueForm.remark = ''
  issueOpen.value = true
}

async function saveIssue() {
  if (!issueForm.ca_id) {
    ElMessage.warning('请选择签发 CA 根证书')
    return
  }
  if (!issueForm.domain.trim()) {
    ElMessage.warning('请填写域名/通用名称')
    return
  }
  issueSaving.value = true
  try {
    const dnsList = issueForm.san_dns
      .split(/[,，\n]/)
      .map((s) => s.trim())
      .filter(Boolean)
    const ipList = issueForm.san_ip
      .split(/[,，\n]/)
      .map((s) => s.trim())
      .filter(Boolean)

    const { data } = await issueCertFromCA(issueForm.ca_id, {
      domain: issueForm.domain.trim(),
      san_dns: dnsList,
      san_ip: ipList,
      organization: issueForm.organization.trim(),
      country: issueForm.country.trim(),
      key_type: issueForm.key_type,
      valid_days: Number(issueForm.valid_days) || 365,
      remark: issueForm.remark.trim(),
    })
    if (data.code === 0) {
      ElMessage.success('证书已由 CA 成功签发并自动组装完整证书链！')
      issueOpen.value = false
      activeTab.value = 'server'
      loadAll()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '签发失败'))
  } finally {
    issueSaving.value = false
  }
}

// ---- 生成根 CA ----
const genCAOpen = ref(false)
const genCASaving = ref(false)
const genCAForm = reactive({
  name: '',
  common_name: '',
  organization: '',
  country: '',
  key_type: 'ecdsa_p256',
  valid_days: 3650,
  remark: '',
})

function openGenerateCA() {
  genCAForm.name = ''
  genCAForm.common_name = ''
  genCAForm.organization = 'Internal Security CA'
  genCAForm.country = 'CN'
  genCAForm.key_type = 'ecdsa_p256'
  genCAForm.valid_days = 3650
  genCAForm.remark = ''
  genCAOpen.value = true
}

async function saveGenerateCA() {
  if (!genCAForm.name.trim()) {
    ElMessage.warning('请填写 CA 标识名称')
    return
  }
  genCASaving.value = true
  try {
    const { data } = await generateCACert({
      name: genCAForm.name.trim(),
      common_name: genCAForm.common_name.trim() || genCAForm.name.trim(),
      organization: genCAForm.organization.trim(),
      country: genCAForm.country.trim(),
      key_type: genCAForm.key_type,
      valid_days: Number(genCAForm.valid_days) || 3650,
      remark: genCAForm.remark.trim(),
    })
    if (data.code === 0) {
      ElMessage.success('CA 根证书已成功生成！')
      genCAOpen.value = false
      loadCAs()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '生成 CA 失败'))
  } finally {
    genCASaving.value = false
  }
}

// ---- 导入外部 CA ----
const importCAOpen = ref(false)
const importCASaving = ref(false)
const importCAForm = reactive({
  name: '',
  ca_cert_pem: '',
  ca_key_pem: '',
  remark: '',
})

function openImportCA() {
  importCAForm.name = ''
  importCAForm.ca_cert_pem = ''
  importCAForm.ca_key_pem = ''
  importCAForm.remark = ''
  importCAOpen.value = true
}

async function saveImportCA() {
  if (!importCAForm.name.trim()) {
    ElMessage.warning('请填写 CA 标识名称')
    return
  }
  if (!importCAForm.ca_cert_pem.trim()) {
    ElMessage.warning('请填写 CA 根证书 PEM')
    return
  }
  importCASaving.value = true
  try {
    const { data } = await createCACert({
      name: importCAForm.name.trim(),
      ca_cert_pem: importCAForm.ca_cert_pem.trim(),
      ca_key_pem: importCAForm.ca_key_pem.trim(),
      remark: importCAForm.remark.trim(),
    })
    if (data.code === 0) {
      ElMessage.success(importCAForm.ca_key_pem.trim() ? '完整 CA 已导入（支持签发与验签）' : '纯公钥 CA 已导入（仅供节点验签）')
      importCAOpen.value = false
      loadCAs()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '导入 CA 失败'))
  } finally {
    importCASaving.value = false
  }
}

// ---- 查看 / 复制 CA 公钥 ----
const viewCAOpen = ref(false)
const viewingCA = ref<CACertItem | null>(null)

function openViewCA(ca: CACertItem) {
  viewingCA.value = ca
  viewCAOpen.value = true
}

async function copyText(text: string, title = '内容') {
  try {
    await navigator.clipboard.writeText(text)
    ElMessage.success(`${title}已复制到剪贴板`)
  } catch {
    ElMessage.info('复制失败，请手动选择复制')
  }
}

// ---- 上传/编辑普通证书 ----
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

function openEdit(row: CertItem) {
  editing.value = row
  form.domain = row.domain
  form.remark = row.remark
  form.cert_pem = ''
  form.key_pem = ''
  formOpen.value = true
}

async function save() {
  if (!editing.value) {
    if (!form.domain.trim()) {
      ElMessage.warning('请填写域名')
      return
    }
    if (!form.cert_pem.trim() || !form.key_pem.trim()) {
      ElMessage.warning('请填写 PEM 证书链与私钥内容')
      return
    }
  }
  saving.value = true
  try {
    const { data } = editing.value
      ? await updateCert(editing.value.id, { remark: form.remark })
      : await createCert({ domain: form.domain.trim(), cert_pem: form.cert_pem, key_pem: form.key_pem, remark: form.remark })
    if (data.code === 0) {
      ElMessage.success(editing.value ? '证书备注已更新' : '证书已上传并推送引用服务器')
      formOpen.value = false
      loadCerts()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}

async function removeCert(row: CertItem) {
  try {
    await ElMessageBox.confirm(`确认删除证书「${row.domain}」？`, '删除证书', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deleteCert(row.id)
    if (data.code === 0) {
      ElMessage.success('已删除')
      loadCerts()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

async function removeCA(row: CACertItem) {
  try {
    await ElMessageBox.confirm(`确认删除 CA 根证书「${row.name}」？\n若已签发证书，需先将签发的证书解绑删除。`, '删除 CA 根证书', { type: 'error' })
  } catch {
    return
  }
  try {
    const { data } = await deleteCACert(row.id)
    if (data.code === 0) {
      ElMessage.success('CA 已删除')
      loadCAs()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '删除失败'))
  }
}

function daysLeft(notAfter: string): number {
  if (!notAfter) return 0
  const t = new Date(notAfter.replace(' ', 'T'))
  return Math.ceil((t.getTime() - Date.now()) / 86400000)
}

function expireTag(notAfter: string) {
  const d = daysLeft(notAfter)
  if (d < 0) return { type: 'danger' as const, text: `已过期 ${-d} 天` }
  if (d <= 30) return { type: 'warning' as const, text: `剩 ${d} 天` }
  return { type: 'success' as const, text: `剩 ${d} 天` }
}
</script>

<template>
  <div class="x-page">
    <!-- 顶层标签页导航 -->
    <el-tabs v-model="activeTab" class="cert-nav-tabs">
      <el-tab-pane label="TLS 服务端证书" name="server" />
      <el-tab-pane label="CA 根证书体系" name="ca" />
    </el-tabs>

    <!-- ================= Tab 1: TLS 服务端证书 ================= -->
    <div v-if="activeTab === 'server'">
      <div class="x-toolbar">
        <div class="x-toolbar-left">
          <el-button type="primary" @click="openIssue()"><el-icon><Stamp /></el-icon>&nbsp;基于 CA 签发证书</el-button>
          <el-button type="success" plain @click="openSelfSigned"><el-icon><MagicStick /></el-icon>&nbsp;生成自签证书</el-button>
          <el-button plain @click="openCreate"><el-icon><Plus /></el-icon>&nbsp;上传外部证书</el-button>
          <TipIcon content="服务端证书由主控推送到引用服务器落盘（/etc/xray/certs/域名/），xray 每小时热重载；支持 CA 签发完整链、多算法自签与外部证书导入。" />
        </div>
      </div>

      <BaseCard title="TLS 服务端证书列表">
        <div v-if="certLoading" style="padding: 48px 0; text-align: center">
          <el-icon class="is-loading" style="font-size: 26px; color: var(--x-primary)"><Loading /></el-icon>
        </div>

        <div v-else-if="certList.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
          <el-icon style="font-size: 32px; color: var(--x-text-3)"><Key /></el-icon>
          <p style="margin-top: 8px">暂无 TLS 证书，可「基于 CA 签发」、「生成自签」或「上传证书」</p>
        </div>

        <div v-else class="cert-card-grid">
          <div v-for="row in certList" :key="row.id" class="cert-card">
            <!-- 头部 -->
            <div class="card-head">
              <div class="head-title">
                <code class="cell-mono cert-domain">{{ row.domain }}</code>
                <span v-if="row.ca_id" class="x-chip purple" style="font-size: 10px; padding: 1px 5px">
                  CA: {{ row.ca_name || '托管CA' }}
                </span>
                <span v-else-if="row.self_signed" class="x-chip green" style="font-size: 10px; padding: 1px 5px">
                  自签·自动Pin
                </span>
                <span v-else class="x-chip blue" style="font-size: 10px; padding: 1px 5px">
                  外部导入
                </span>
                <span v-if="row.key_type" class="x-chip gray" style="font-size: 10px; padding: 1px 5px">
                  {{ row.key_type }}
                </span>
              </div>
              <span
                class="x-chip"
                :class="expireTag(row.not_after).type === 'danger' ? 'red' : (expireTag(row.not_after).type === 'warning' ? 'orange' : 'green')"
                style="font-size: 10.5px"
              >
                {{ expireTag(row.not_after).text }}
              </span>
            </div>

            <!-- 证书属性网格 -->
            <div class="card-grid">
              <div class="grid-item">
                <span class="item-label">证书备注</span>
                <div class="item-value">{{ row.remark || '标准 TLS 证书' }}</div>
              </div>
              <div class="grid-item">
                <span class="item-label">到期时间</span>
                <div class="item-value cell-mono font-12">{{ row.not_after || '—' }}</div>
              </div>
              <div class="grid-item full-width" v-if="row.pin_sha256">
                <span class="item-label">证书指纹 (Pin SHA-256)</span>
                <div class="item-value pin-value">
                  <code class="cell-mono font-11 text-ellipsis">{{ row.pin_sha256 }}</code>
                  <el-button link size="small" type="primary" @click="copyText(row.pin_sha256, '证书指纹')">
                    <el-icon><CopyDocument /></el-icon>
                  </el-button>
                </div>
              </div>
              <div class="grid-item full-width">
                <span class="item-label">引用服务器</span>
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
              <el-button v-if="row.ca_id || row.self_signed" size="small" type="success" plain @click="handleReissue(row)">
                <el-icon><Refresh /></el-icon>&nbsp;一键重签/续期
              </el-button>
              <el-button size="small" type="primary" plain @click="openEdit(row)">
                <el-icon><Edit /></el-icon>&nbsp;编辑
              </el-button>
              <el-button size="small" type="danger" plain @click="removeCert(row)">
                <el-icon><Delete /></el-icon>&nbsp;删除
              </el-button>
            </div>
          </div>
        </div>
      </BaseCard>
    </div>

    <!-- ================= Tab 2: CA 根证书体系 ================= -->
    <div v-else-if="activeTab === 'ca'">
      <div class="x-toolbar">
        <div class="x-toolbar-left">
          <el-button type="primary" @click="openGenerateCA"><el-icon><Plus /></el-icon>&nbsp;生成新根 CA</el-button>
          <el-button plain @click="openImportCA"><el-icon><Stamp /></el-icon>&nbsp;导入外部 CA</el-button>
          <TipIcon content="CA 根证书体系：完整 CA（含私钥）可无限签发节点 TLS 服务端证书；纯公钥 CA 可直接内联下发到出入站作为验签根证书。" />
        </div>
      </div>

      <BaseCard title="托管 CA 根证书列表">
        <div v-if="caLoading" style="padding: 48px 0; text-align: center">
          <el-icon class="is-loading" style="font-size: 26px; color: var(--x-primary)"><Loading /></el-icon>
        </div>

        <div v-else-if="caList.length === 0" style="text-align: center; padding: 48px 0; color: var(--x-text-3); font-size: 13.5px">
          <el-icon style="font-size: 32px; color: var(--x-text-3)"><Stamp /></el-icon>
          <p style="margin-top: 8px">暂无 CA 证书，点击上方「生成新根 CA」或「导入外部 CA」</p>
        </div>

        <div v-else class="cert-card-grid">
          <div v-for="ca in caList" :key="ca.id" class="cert-card">
            <!-- 头部 -->
            <div class="card-head">
              <div class="head-title">
                <code class="cell-mono cert-domain">{{ ca.name }}</code>
                <span v-if="ca.has_key" class="x-chip green" style="font-size: 10px; padding: 1px 5px">
                  完整·可签发
                </span>
                <span v-else class="x-chip orange" style="font-size: 10px; padding: 1px 5px">
                  纯公钥·仅验签
                </span>
                <span v-if="ca.key_type" class="x-chip gray" style="font-size: 10px; padding: 1px 5px">
                  {{ ca.key_type }}
                </span>
              </div>
              <span
                class="x-chip"
                :class="expireTag(ca.not_after).type === 'danger' ? 'red' : (expireTag(ca.not_after).type === 'warning' ? 'orange' : 'green')"
                style="font-size: 10.5px"
              >
                {{ expireTag(ca.not_after).text }}
              </span>
            </div>

            <!-- CA 属性网格 -->
            <div class="card-grid">
              <div class="grid-item">
                <span class="item-label">通用名称 (CN)</span>
                <div class="item-value text-ellipsis">{{ ca.common_name || ca.name }}</div>
              </div>
              <div class="grid-item">
                <span class="item-label">组织 / 国家</span>
                <div class="item-value text-ellipsis">{{ [ca.organization, ca.country].filter(Boolean).join(' · ') || '—' }}</div>
              </div>
              <div class="grid-item">
                <span class="item-label">已签发证书</span>
                <div class="item-value">
                  <span class="x-chip purple font-11" style="padding: 1px 6px">{{ ca.issued_count }} 张证书</span>
                </div>
              </div>
              <div class="grid-item">
                <span class="item-label">到期时间</span>
                <div class="item-value cell-mono font-12">{{ ca.not_after || '—' }}</div>
              </div>
              <div class="grid-item full-width" v-if="ca.remark">
                <span class="item-label">备注</span>
                <div class="item-value">{{ ca.remark }}</div>
              </div>
            </div>

            <!-- 底部操作栏 -->
            <div class="card-foot-actions">
              <el-button v-if="ca.has_key" size="small" type="primary" plain @click="openIssue(ca.id)">
                <el-icon><Stamp /></el-icon>&nbsp;签发证书
              </el-button>
              <el-button size="small" plain @click="openViewCA(ca)">
                <el-icon><View /></el-icon>&nbsp;查看公钥
              </el-button>
              <el-button size="small" type="danger" plain @click="removeCA(ca)">
                <el-icon><Delete /></el-icon>&nbsp;删除
              </el-button>
            </div>
          </div>
        </div>
      </BaseCard>
    </div>

    <!-- ===== 弹窗 1: 基于 CA 签发证书 ===== -->
    <el-dialog v-model="issueOpen" title="基于 CA 签发服务端 TLS 证书" width="620px" :append-to-body="true">
      <el-form label-position="top">
        <el-form-item label="签发根 CA">
          <el-select v-model="issueForm.ca_id" style="width: 100%">
            <el-option
              v-for="ca in caList.filter((c) => c.has_key)"
              :key="ca.id"
              :value="ca.id"
              :label="`${ca.name} (${ca.common_name}) · ${ca.key_type}`"
            />
          </el-select>
        </el-form-item>
        <div class="form-row">
          <el-form-item label="域名 / 通用名称 (CN)" style="flex: 1">
            <el-input v-model="issueForm.domain" placeholder="如 relay.example.com" />
          </el-form-item>
          <el-form-item label="密钥算法" style="width: 200px">
            <el-select v-model="issueForm.key_type">
              <el-option v-for="opt in keyTypeOptions" :key="opt.value" :value="opt.value" :label="opt.label" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item label="SAN DNS 备用域名（逗号分隔，可选通配符）">
          <el-input v-model="issueForm.san_dns" placeholder="如 node-01.local, *.relay.example.com" />
        </el-form-item>
        <el-form-item label="SAN IP 备用地址（逗号分隔）">
          <el-input v-model="issueForm.san_ip" placeholder="如 192.168.1.100, 10.0.0.1" />
        </el-form-item>
        <div class="form-row">
          <el-form-item label="组织名称 (O)" style="flex: 1">
            <el-input v-model="issueForm.organization" placeholder="选填，如 Internal Nodes" />
          </el-form-item>
          <el-form-item label="有效天数" style="width: 140px">
            <el-input-number v-model="issueForm.valid_days" :min="1" :max="36500" style="width: 100%" />
          </el-form-item>
        </div>
        <el-form-item label="备注说明">
          <el-input v-model="issueForm.remark" placeholder="选填" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="issueOpen = false">取消</el-button>
        <el-button type="primary" :loading="issueSaving" @click="saveIssue">立即签发</el-button>
      </template>
    </el-dialog>

    <!-- ===== 弹窗 2: 生成自签证书（高级全面自定义） ===== -->
    <el-dialog v-model="ssOpen" title="生成自签证书（全面自定义）" width="620px" :append-to-body="true">
      <div class="ss-note">
        生成具备 IsCA=true 锚点属性的自签证书，主控自动计算 leaf 指纹 Pin（SHA-256）并支持出站节点验证。
      </div>
      <el-form label-position="top">
        <div class="form-row">
          <el-form-item label="域名 / 服务器标识 (CN)" style="flex: 1">
            <el-input v-model="ssForm.domain" placeholder="如 relay-jp-01 或 relay.example.com" />
          </el-form-item>
          <el-form-item label="密钥算法" style="width: 200px">
            <el-select v-model="ssForm.key_type">
              <el-option v-for="opt in keyTypeOptions" :key="opt.value" :value="opt.value" :label="opt.label" />
            </el-select>
          </el-form-item>
        </div>
        <el-form-item label="SAN DNS 备用域名（逗号分隔）">
          <el-input v-model="ssForm.san_dns" placeholder="如 relay-01.local, *.relay.local" />
        </el-form-item>
        <el-form-item label="SAN IP 备用地址（逗号分隔）">
          <el-input v-model="ssForm.san_ip" placeholder="如 127.0.0.1, 10.0.0.1" />
        </el-form-item>
        <div class="form-row">
          <el-form-item label="组织名称 (O)" style="flex: 1">
            <el-input v-model="ssForm.organization" placeholder="选填" />
          </el-form-item>
          <el-form-item label="有效天数" style="width: 140px">
            <el-input-number v-model="ssForm.valid_days" :min="1" :max="36500" style="width: 100%" />
          </el-form-item>
        </div>
        <el-form-item label="备注说明">
          <el-input v-model="ssForm.remark" placeholder="选填" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="ssOpen = false">取消</el-button>
        <el-button type="success" :loading="ssSaving" @click="saveSelfSigned">生成并保存</el-button>
      </template>
    </el-dialog>

    <!-- ===== 弹窗 3: 生成根 CA ===== -->
    <el-dialog v-model="genCAOpen" title="生成新根 CA" width="560px" :append-to-body="true">
      <el-form label-position="top">
        <el-form-item label="CA 标识名称（唯一）">
          <el-input v-model="genCAForm.name" placeholder="如 root-ca-prod 或 internal-cluster-ca" />
        </el-form-item>
        <div class="form-row">
          <el-form-item label="通用名称 (CN)" style="flex: 1">
            <el-input v-model="genCAForm.common_name" placeholder="如 Internal Cluster Root CA" />
          </el-form-item>
          <el-form-item label="密钥算法" style="width: 200px">
            <el-select v-model="genCAForm.key_type">
              <el-option v-for="opt in keyTypeOptions" :key="opt.value" :value="opt.value" :label="opt.label" />
            </el-select>
          </el-form-item>
        </div>
        <div class="form-row">
          <el-form-item label="组织名称 (O)" style="flex: 1">
            <el-input v-model="genCAForm.organization" placeholder="选填，如 XPanel Cluster" />
          </el-form-item>
          <el-form-item label="国家代码 (C)" style="width: 120px">
            <el-input v-model="genCAForm.country" placeholder="如 CN" maxlength="2" />
          </el-form-item>
          <el-form-item label="有效天数" style="width: 120px">
            <el-input-number v-model="genCAForm.valid_days" :min="30" :max="36500" style="width: 100%" />
          </el-form-item>
        </div>
        <el-form-item label="备注说明">
          <el-input v-model="genCAForm.remark" placeholder="选填" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="genCAOpen = false">取消</el-button>
        <el-button type="primary" :loading="genCASaving" @click="saveGenerateCA">立即生成</el-button>
      </template>
    </el-dialog>

    <!-- ===== 弹窗 4: 导入外部 CA ===== -->
    <el-dialog v-model="importCAOpen" title="导入外部 CA 根证书" width="620px" :append-to-body="true">
      <el-form label-position="top">
        <el-form-item label="CA 标识名称（唯一）">
          <el-input v-model="importCAForm.name" placeholder="如 custom-org-ca" />
        </el-form-item>
        <el-form-item label="CA 根证书 PEM（必填）">
          <el-input v-model="importCAForm.ca_cert_pem" type="textarea" :rows="5" placeholder="-----BEGIN CERTIFICATE-----..." />
        </el-form-item>
        <el-form-item label="CA 私钥 PEM（选填；留空则为纯公钥验签 CA，不可签发）">
          <el-input v-model="importCAForm.ca_key_pem" type="textarea" :rows="4" placeholder="-----BEGIN PRIVATE KEY-----... (可选)" />
        </el-form-item>
        <el-form-item label="备注说明">
          <el-input v-model="importCAForm.remark" placeholder="选填" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="importCAOpen = false">取消</el-button>
        <el-button type="primary" :loading="importCASaving" @click="saveImportCA">导入 CA</el-button>
      </template>
    </el-dialog>

    <!-- ===== 弹窗 5: 查看 / 复制 CA 公钥 ===== -->
    <el-dialog v-model="viewCAOpen" :title="`CA 根证书公钥（${viewingCA?.name}）`" width="580px" :append-to-body="true">
      <div v-if="viewingCA">
        <div style="font-size: 12px; color: var(--x-text-2); margin-bottom: 8px">
          可直接复制以下根公钥用于客户端验签、出站节点 TLS 校验或下发外部设备：
        </div>
        <el-input :model-value="viewingCA.ca_cert_pem" type="textarea" :rows="10" readonly />
      </div>
      <template #footer>
        <el-button @click="viewCAOpen = false">关闭</el-button>
        <el-button type="primary" @click="copyText(viewingCA?.ca_cert_pem || '', 'CA 根证书公钥')">
          <el-icon><CopyDocument /></el-icon>&nbsp;复制公钥
        </el-button>
      </template>
    </el-dialog>

    <!-- ===== 弹窗 6: 上传外部证书 / 编辑备注 ===== -->
    <el-dialog v-model="formOpen" :title="editing ? '编辑证书（' + form.domain + '）' : '上传外部证书'" width="640px" :append-to-body="true">
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

.cert-nav-tabs {
  margin-bottom: 14px;
}

.ss-note {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  color: var(--x-text-2);
  line-height: 1.6;
  margin-bottom: 14px;
  background: var(--x-bg-subtle, #f8fafc);
  padding: 8px 12px;
  border-radius: var(--x-radius-sm, 6px);
  border: 1px dashed var(--x-border, #e2e8f0);
}

.form-row {
  display: flex;
  gap: 12px;
}

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

        &.pin-value {
          display: flex;
          align-items: center;
          justify-content: space-between;
          background: var(--x-bg-subtle, #f8fafc);
          padding: 2px 6px;
          border-radius: 4px;
        }
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

.text-ellipsis {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

@media (max-width: 640px) {
  .cert-card-grid {
    grid-template-columns: 1fr;
  }
  .form-row {
    flex-direction: column;
    gap: 0;
  }
}
</style>
