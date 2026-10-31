<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import {
  CsAlert,
  CsBadge,
  CsButton,
  CsCard,
  CsCardFact,
  CsCardFacts,
  CsCardGrid,
  CsConfirmDialog,
  CsContainer,
  CsEmpty,
  CsForm,
  CsFormItem,
  CsIconButton,
  CsInput,
  CsInputNumber,
  CsModal,
  CsPage,
  CsSelect,
  CsSpinner,
  CsTabs,
  CsToolbar,
  ElMessage,
} from '@/components/cloudscape'
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

const activeTab = ref('server')
const certTabs = [
  { id: 'server', label: 'TLS 服务端证书' },
  { id: 'ca', label: 'CA 根证书体系' },
]

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

const issueCaOptions = computed(() =>
  caList.value
    .filter((ca) => ca.has_key)
    .map((ca) => ({
      label: `${ca.name} (${ca.common_name}) · ${ca.key_type}`,
      value: ca.id,
    })),
)

// ================= 确认弹窗 =================
const confirmState = reactive({
  visible: false,
  header: '',
  warning: '',
  message: '',
  confirmText: '确认',
  action: null as null | (() => void | Promise<void>),
})

function askConfirm(opts: {
  header: string
  warning: string
  message?: string
  confirmText?: string
  action: () => void | Promise<void>
}) {
  confirmState.header = opts.header
  confirmState.warning = opts.warning
  confirmState.message = opts.message ?? ''
  confirmState.confirmText = opts.confirmText ?? '确认'
  confirmState.action = opts.action
  confirmState.visible = true
}

async function runConfirmed() {
  const action = confirmState.action
  confirmState.visible = false
  confirmState.action = null
  if (action) await action()
}

// ---- 一键重签/续期 ----
function handleReissue(row: CertItem) {
  askConfirm({
    header: '重新签发这张证书？',
    warning: '签发后将自动下发到落地节点，并重新渲染中转出站配置。',
    message: `证书「${row.domain}」将按现有 SAN 重新签发/续期。`,
    confirmText: '立即重签',
    action: async () => {
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
    },
  })
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

function splitSan(value: string): string[] {
  return value
    .split(/[,，\n]/)
    .map((s) => s.trim())
    .filter(Boolean)
}

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
    const { data } = await generateSelfSignedCert({
      domain: ssForm.domain.trim(),
      san_dns: splitSan(ssForm.san_dns),
      san_ip: splitSan(ssForm.san_ip),
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
  const available = issueCaOptions.value
  if (available.length === 0) {
    ElMessage.warning('当前暂无可用于签发的 CA（需持有私钥），请先生成或导入完整 CA')
    return
  }
  const preset = available.find((ca) => ca.value === preselectedCaId)
  issueForm.ca_id = preset ? preselectedCaId : available[0].value
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
    const { data } = await issueCertFromCA(issueForm.ca_id, {
      domain: issueForm.domain.trim(),
      san_dns: splitSan(issueForm.san_dns),
      san_ip: splitSan(issueForm.san_ip),
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
      ElMessage.success(
        importCAForm.ca_key_pem.trim() ? '完整 CA 已导入（支持签发与验签）' : '纯公钥 CA 已导入（仅供节点验签）',
      )
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

function closeViewCA() {
  viewCAOpen.value = false
  viewingCA.value = null
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

function removeCert(row: CertItem) {
  askConfirm({
    header: '删除这张证书？',
    warning: '此操作不可恢复。',
    message: `确认删除证书「${row.domain}」？若正被入站引用，需先解绑。`,
    confirmText: '删除',
    action: async () => {
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
    },
  })
}

function removeCA(row: CACertItem) {
  askConfirm({
    header: '删除这个 CA 根证书？',
    warning: '若已签发证书，需先将签发的证书解绑删除。',
    message: `确认删除 CA 根证书「${row.name}」？`,
    confirmText: '删除',
    action: async () => {
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
    },
  })
}

function daysLeft(notAfter: string): number {
  if (!notAfter) return 0
  const t = new Date(notAfter.replace(' ', 'T'))
  return Math.ceil((t.getTime() - Date.now()) / 86400000)
}

type ExpireColor = 'red' | 'severity-medium' | 'green' | 'grey'

function expireBadge(notAfter: string): { color: ExpireColor; text: string } {
  if (!notAfter) return { color: 'grey', text: '—' }
  const d = daysLeft(notAfter)
  if (d < 0) return { color: 'red', text: `已过期 ${-d} 天` }
  if (d <= 30) return { color: 'severity-medium', text: `剩 ${d} 天` }
  return { color: 'green', text: `剩 ${d} 天` }
}
</script>

<template>
  <CsPage>
    <CsTabs v-model="activeTab" :tabs="certTabs" aria-label="证书管理">
      <!-- ================= Tab 1: TLS 服务端证书 ================= -->
      <div v-if="activeTab === 'server'" class="cert-tab-body">
        <CsToolbar>
          <CsButton icon-name="refresh" @click="loadCerts">刷新</CsButton>
          <TipIcon
            content="服务端证书由主控推送到引用服务器落盘（/etc/xray/certs/域名/），xray 每小时热重载；支持 CA 签发完整链、多算法自签与外部证书导入。"
          />
          <template #actions>
            <CsButton variant="primary" icon-name="security" @click="openIssue()">基于 CA 签发证书</CsButton>
            <CsButton icon-name="lock-private" @click="openSelfSigned">生成自签证书</CsButton>
            <CsButton icon-name="upload" @click="openCreate">上传外部证书</CsButton>
          </template>
        </CsToolbar>

        <CsContainer header="TLS 服务端证书列表">
          <div v-if="certLoading" class="cert-loading">
            <CsSpinner size="large" />
          </div>

          <CsEmpty
            v-else-if="certList.length === 0"
            icon-name="key"
            description="暂无 TLS 证书，可「基于 CA 签发」、「生成自签」或「上传证书」"
          />

          <CsCardGrid v-else>
            <CsCard v-for="row in certList" :key="row.id">
              <template #header>
                <div class="cert-head">
                  <code class="cert-domain">{{ row.domain }}</code>
                  <div class="cert-badges">
                    <CsBadge v-if="row.ca_id" color="blue">CA: {{ row.ca_name || '托管CA' }}</CsBadge>
                    <CsBadge v-else-if="row.self_signed" color="green">自签 · 自动 Pin</CsBadge>
                    <CsBadge v-else color="grey">外部导入</CsBadge>
                    <CsBadge v-if="row.key_type" color="grey">{{ row.key_type }}</CsBadge>
                  </div>
                </div>
              </template>
              <template #actions>
                <CsBadge :color="expireBadge(row.not_after).color">{{ expireBadge(row.not_after).text }}</CsBadge>
              </template>

              <CsCardFacts>
                <CsCardFact label="证书备注" :value="row.remark || '标准 TLS 证书'" />
                <CsCardFact label="到期时间" :value="row.not_after || '—'" mono />
                <CsCardFact v-if="row.pin_sha256" label="证书指纹 (Pin SHA-256)" full-width>
                  <div class="pin-row">
                    <code class="pin-text" :title="row.pin_sha256">{{ row.pin_sha256 }}</code>
                    <CsIconButton icon-name="copy" label="复制证书指纹" @click="copyText(row.pin_sha256, '证书指纹')" />
                  </div>
                </CsCardFact>
                <CsCardFact label="引用服务器" full-width>
                  <div v-if="row.refs && row.refs.length > 0" class="cert-refs">
                    <CsBadge v-for="r in row.refs" :key="r.inbound_id" color="grey">
                      {{ r.server_name }} / {{ r.inbound_tag }}
                    </CsBadge>
                  </div>
                  <span v-else class="cert-muted">未被任何入站引用</span>
                </CsCardFact>
              </CsCardFacts>

              <template #footer>
                <CsButton v-if="row.ca_id || row.self_signed" icon-name="refresh" @click="handleReissue(row)">
                  一键重签/续期
                </CsButton>
                <CsButton icon-name="edit" @click="openEdit(row)">编辑</CsButton>
                <CsButton variant="danger-outline" icon-name="remove" @click="removeCert(row)">删除</CsButton>
              </template>
            </CsCard>
          </CsCardGrid>
        </CsContainer>
      </div>

      <!-- ================= Tab 2: CA 根证书体系 ================= -->
      <div v-else-if="activeTab === 'ca'" class="cert-tab-body">
        <CsToolbar>
          <CsButton icon-name="refresh" @click="loadCAs">刷新</CsButton>
          <TipIcon
            content="CA 根证书体系：完整 CA（含私钥）可无限签发节点 TLS 服务端证书；纯公钥 CA 可直接内联下发到出入站作为验签根证书。"
          />
          <template #actions>
            <CsButton variant="primary" icon-name="add-plus" @click="openGenerateCA">生成新根 CA</CsButton>
            <CsButton icon-name="file-open" @click="openImportCA">导入外部 CA</CsButton>
          </template>
        </CsToolbar>

        <CsContainer header="托管 CA 根证书列表">
          <div v-if="caLoading" class="cert-loading">
            <CsSpinner size="large" />
          </div>

          <CsEmpty
            v-else-if="caList.length === 0"
            icon-name="security"
            description="暂无 CA 证书，点击上方「生成新根 CA」或「导入外部 CA」"
          />

          <CsCardGrid v-else>
            <CsCard v-for="ca in caList" :key="ca.id">
              <template #header>
                <div class="cert-head">
                  <code class="cert-domain">{{ ca.name }}</code>
                  <div class="cert-badges">
                    <CsBadge :color="ca.has_key ? 'green' : 'severity-medium'">
                      {{ ca.has_key ? '完整 · 可签发' : '纯公钥 · 仅验签' }}
                    </CsBadge>
                    <CsBadge v-if="ca.key_type" color="grey">{{ ca.key_type }}</CsBadge>
                  </div>
                </div>
              </template>
              <template #actions>
                <CsBadge :color="expireBadge(ca.not_after).color">{{ expireBadge(ca.not_after).text }}</CsBadge>
              </template>

              <CsCardFacts>
                <CsCardFact label="通用名称 (CN)" :value="ca.common_name || ca.name" />
                <CsCardFact
                  label="组织 / 国家"
                  :value="[ca.organization, ca.country].filter(Boolean).join(' · ') || '—'"
                />
                <CsCardFact label="已签发证书">
                  <CsBadge color="grey">{{ ca.issued_count }} 张证书</CsBadge>
                </CsCardFact>
                <CsCardFact label="到期时间" :value="ca.not_after || '—'" mono />
                <CsCardFact v-if="ca.remark" label="备注" :value="ca.remark" full-width />
              </CsCardFacts>

              <template #footer>
                <CsButton v-if="ca.has_key" icon-name="security" @click="openIssue(ca.id)">签发证书</CsButton>
                <CsButton icon-name="view-vertical" @click="openViewCA(ca)">查看公钥</CsButton>
                <CsButton variant="danger-outline" icon-name="remove" @click="removeCA(ca)">删除</CsButton>
              </template>
            </CsCard>
          </CsCardGrid>
        </CsContainer>
      </div>
    </CsTabs>

    <!-- ===== 弹窗 1: 基于 CA 签发证书 ===== -->
    <CsModal
      v-model:visible="issueOpen"
      header="基于 CA 签发服务端 TLS 证书"
      size="medium"
      :close-on-click-overlay="false"
    >
      <CsForm label-position="top">
        <CsFormItem label="签发根 CA" required>
          <CsSelect
            v-model="issueForm.ca_id"
            :options="issueCaOptions"
            placeholder="选择签发根 CA"
            style="width: 100%"
          />
        </CsFormItem>
        <div class="form-grid-2">
          <CsFormItem label="域名 / 通用名称 (CN)" required>
            <CsInput v-model="issueForm.domain" placeholder="如 relay.example.com" />
          </CsFormItem>
          <CsFormItem label="密钥算法">
            <CsSelect v-model="issueForm.key_type" :options="keyTypeOptions" style="width: 100%" />
          </CsFormItem>
        </div>
        <CsFormItem label="SAN DNS 备用域名（逗号分隔，可选通配符）">
          <CsInput v-model="issueForm.san_dns" placeholder="如 node-01.local, *.relay.example.com" />
        </CsFormItem>
        <CsFormItem label="SAN IP 备用地址（逗号分隔）">
          <CsInput v-model="issueForm.san_ip" placeholder="如 192.168.1.100, 10.0.0.1" />
        </CsFormItem>
        <div class="form-grid-2">
          <CsFormItem label="组织名称 (O)">
            <CsInput v-model="issueForm.organization" placeholder="选填，如 Internal Nodes" />
          </CsFormItem>
          <CsFormItem label="有效天数">
            <CsInputNumber v-model="issueForm.valid_days" :min="1" :max="36500" style="width: 100%" />
          </CsFormItem>
        </div>
        <CsFormItem label="备注说明">
          <CsInput v-model="issueForm.remark" placeholder="选填" />
        </CsFormItem>
      </CsForm>
      <template #footer>
        <CsButton @click="issueOpen = false">取消</CsButton>
        <CsButton variant="primary" :loading="issueSaving" @click="saveIssue">立即签发</CsButton>
      </template>
    </CsModal>

    <!-- ===== 弹窗 2: 生成自签证书（高级全面自定义） ===== -->
    <CsModal
      v-model:visible="ssOpen"
      header="生成自签证书（全面自定义）"
      size="medium"
      :close-on-click-overlay="false"
    >
      <CsAlert type="info">
        生成具备 IsCA=true 锚点属性的自签证书，主控自动计算 leaf 指纹 Pin（SHA-256）并支持出站节点验证。
      </CsAlert>
      <CsForm label-position="top" class="modal-form">
        <div class="form-grid-2">
          <CsFormItem label="域名 / 服务器标识 (CN)" required>
            <CsInput v-model="ssForm.domain" placeholder="如 relay-jp-01 或 relay.example.com" />
          </CsFormItem>
          <CsFormItem label="密钥算法">
            <CsSelect v-model="ssForm.key_type" :options="keyTypeOptions" style="width: 100%" />
          </CsFormItem>
        </div>
        <CsFormItem label="SAN DNS 备用域名（逗号分隔）">
          <CsInput v-model="ssForm.san_dns" placeholder="如 relay-01.local, *.relay.local" />
        </CsFormItem>
        <CsFormItem label="SAN IP 备用地址（逗号分隔）">
          <CsInput v-model="ssForm.san_ip" placeholder="如 127.0.0.1, 10.0.0.1" />
        </CsFormItem>
        <div class="form-grid-2">
          <CsFormItem label="组织名称 (O)">
            <CsInput v-model="ssForm.organization" placeholder="选填" />
          </CsFormItem>
          <CsFormItem label="有效天数">
            <CsInputNumber v-model="ssForm.valid_days" :min="1" :max="36500" style="width: 100%" />
          </CsFormItem>
        </div>
        <CsFormItem label="备注说明">
          <CsInput v-model="ssForm.remark" placeholder="选填" />
        </CsFormItem>
      </CsForm>
      <template #footer>
        <CsButton @click="ssOpen = false">取消</CsButton>
        <CsButton variant="primary" :loading="ssSaving" @click="saveSelfSigned">生成并保存</CsButton>
      </template>
    </CsModal>

    <!-- ===== 弹窗 3: 生成根 CA ===== -->
    <CsModal
      v-model:visible="genCAOpen"
      header="生成新根 CA"
      size="medium"
      :close-on-click-overlay="false"
    >
      <CsForm label-position="top">
        <CsFormItem label="CA 标识名称（唯一）" required>
          <CsInput v-model="genCAForm.name" placeholder="如 root-ca-prod 或 internal-cluster-ca" />
        </CsFormItem>
        <div class="form-grid-2">
          <CsFormItem label="通用名称 (CN)">
            <CsInput v-model="genCAForm.common_name" placeholder="如 Internal Cluster Root CA" />
          </CsFormItem>
          <CsFormItem label="密钥算法">
            <CsSelect v-model="genCAForm.key_type" :options="keyTypeOptions" style="width: 100%" />
          </CsFormItem>
        </div>
        <div class="form-grid-3">
          <CsFormItem label="组织名称 (O)">
            <CsInput v-model="genCAForm.organization" placeholder="选填，如 XPanel Cluster" />
          </CsFormItem>
          <CsFormItem label="国家代码 (C)">
            <CsInput v-model="genCAForm.country" placeholder="如 CN" :maxlength="2" />
          </CsFormItem>
          <CsFormItem label="有效天数">
            <CsInputNumber v-model="genCAForm.valid_days" :min="30" :max="36500" style="width: 100%" />
          </CsFormItem>
        </div>
        <CsFormItem label="备注说明">
          <CsInput v-model="genCAForm.remark" placeholder="选填" />
        </CsFormItem>
      </CsForm>
      <template #footer>
        <CsButton @click="genCAOpen = false">取消</CsButton>
        <CsButton variant="primary" :loading="genCASaving" @click="saveGenerateCA">立即生成</CsButton>
      </template>
    </CsModal>

    <!-- ===== 弹窗 4: 导入外部 CA ===== -->
    <CsModal
      v-model:visible="importCAOpen"
      header="导入外部 CA 根证书"
      size="medium"
      :close-on-click-overlay="false"
    >
      <CsForm label-position="top">
        <CsFormItem label="CA 标识名称（唯一）" required>
          <CsInput v-model="importCAForm.name" placeholder="如 custom-org-ca" />
        </CsFormItem>
        <CsFormItem label="CA 根证书 PEM" required>
          <CsInput
            v-model="importCAForm.ca_cert_pem"
            type="textarea"
            :rows="5"
            class="pem-input"
            placeholder="-----BEGIN CERTIFICATE-----..."
          />
        </CsFormItem>
        <CsFormItem label="CA 私钥 PEM" description="选填；留空则为纯公钥验签 CA，不可签发">
          <CsInput
            v-model="importCAForm.ca_key_pem"
            type="textarea"
            :rows="4"
            class="pem-input"
            placeholder="-----BEGIN PRIVATE KEY-----... (可选)"
          />
        </CsFormItem>
        <CsFormItem label="备注说明">
          <CsInput v-model="importCAForm.remark" placeholder="选填" />
        </CsFormItem>
      </CsForm>
      <template #footer>
        <CsButton @click="importCAOpen = false">取消</CsButton>
        <CsButton variant="primary" :loading="importCASaving" @click="saveImportCA">导入 CA</CsButton>
      </template>
    </CsModal>

    <!-- ===== 弹窗 5: 查看 / 复制 CA 公钥 ===== -->
    <CsModal
      v-model:visible="viewCAOpen"
      :header="`CA 根证书公钥（${viewingCA?.name || ''}）`"
      size="medium"
      :close-on-click-overlay="false"
    >
      <div v-if="viewingCA" class="modal-stack">
        <p class="modal-hint">可直接复制以下根公钥用于客户端验签、出站节点 TLS 校验或下发外部设备：</p>
        <CsInput :model-value="viewingCA.ca_cert_pem" type="textarea" :rows="10" read-only class="pem-input" />
      </div>
      <template #footer>
        <CsButton @click="closeViewCA">关闭</CsButton>
        <CsButton
          variant="primary"
          icon-name="copy"
          @click="copyText(viewingCA?.ca_cert_pem || '', 'CA 根证书公钥')"
        >
          复制公钥
        </CsButton>
      </template>
    </CsModal>

    <!-- ===== 弹窗 6: 上传外部证书 / 编辑备注 ===== -->
    <CsModal
      v-model:visible="formOpen"
      :header="editing ? `编辑证书（${form.domain}）` : '上传外部证书'"
      size="medium"
      :close-on-click-overlay="false"
    >
      <CsForm label-position="top">
        <CsFormItem label="域名（唯一）" required>
          <CsInput v-model="form.domain" :disabled="!!editing" placeholder="如 cdn.example.com" />
        </CsFormItem>
        <template v-if="!editing">
          <CsFormItem label="证书链 PEM（fullchain）" required>
            <CsInput
              v-model="form.cert_pem"
              type="textarea"
              :rows="5"
              class="pem-input"
              placeholder="-----BEGIN CERTIFICATE-----..."
            />
          </CsFormItem>
          <CsFormItem label="私钥 PEM" required>
            <CsInput
              v-model="form.key_pem"
              type="textarea"
              :rows="5"
              class="pem-input"
              placeholder="-----BEGIN PRIVATE KEY-----..."
            />
          </CsFormItem>
        </template>
        <CsFormItem label="备注">
          <CsInput v-model="form.remark" placeholder="选填" />
        </CsFormItem>
      </CsForm>
      <template #footer>
        <CsButton @click="formOpen = false">取消</CsButton>
        <CsButton variant="primary" :loading="saving" @click="save">保存</CsButton>
      </template>
    </CsModal>

    <CsConfirmDialog
      v-model:visible="confirmState.visible"
      :header="confirmState.header"
      :warning="confirmState.warning"
      :message="confirmState.message"
      :confirm-text="confirmState.confirmText"
      @confirm="runConfirmed"
    />
  </CsPage>
</template>

<style scoped lang="scss">
.cert-tab-body {
  display: flex;
  flex-direction: column;
  gap: 20px;
  min-inline-size: 0;
}

.cert-loading {
  padding: 48px 0;
  text-align: center;
}

.cert-head {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-inline-size: 0;
}

.cert-domain {
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: 16px;
  line-height: 20px;
  font-weight: 700;
  color: var(--color-text-body-default-b2us04, #0f141a);
  overflow-wrap: anywhere;
}

.cert-badges {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 6px;
}

.pin-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-inline-size: 0;
}

.pin-text {
  flex: 1 1 auto;
  min-inline-size: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: 12px;
  line-height: 16px;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.cert-refs {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.cert-muted {
  color: var(--color-text-body-secondary-66ygbb, #656871);
  font-size: var(--font-size-body-s-89y7qg, 12px);
}

.form-grid-2,
.form-grid-3 {
  display: grid;
  gap: 0 12px;
}

.form-grid-2 {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.form-grid-3 {
  grid-template-columns: minmax(0, 1fr) 120px 140px;
}

.modal-stack {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.modal-form {
  margin-top: 16px;
}

.modal-hint {
  margin: 0;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  font-size: var(--font-size-body-s-89y7qg, 12px);
  line-height: var(--line-height-body-s-gako33, 16px);
}

.pem-input :deep(textarea) {
  font-family: var(--font-family-monospace, ui-monospace, Menlo, Consolas, monospace);
  font-size: 12px;
  line-height: 16px;
}

@media (max-width: $bp-tablet-max) {
  .form-grid-2,
  .form-grid-3 {
    grid-template-columns: minmax(0, 1fr);
  }
}
</style>
