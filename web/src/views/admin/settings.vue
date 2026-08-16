<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Check, Download, Refresh } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import {
  getSettings,
  updateSettings,
  getBackups,
  createBackup,
  getSystemStatus,
  type SiteGroup,
  type CaptchaGroup,
  type BackupItem,
  type SystemStatus,
} from '@/api/admin'
import { apiBase } from '@/config/site'
import { errMsg } from '@/api/http'

const activeTab = ref('site')
const loading = ref(false)
const saving = ref(false)

const emptySite = (): SiteGroup => ({
  app_name: '',
  app_description: '',
  logo: '',
  favicon: '',
  subscribe_domain: '',
  tos_url: '',
  stop_register: '0',
  currency: 'CNY',
  currency_symbol: '¥',
})
const emptyCaptcha = (): CaptchaGroup => ({
  captcha_enable: '0',
  captcha_type: 'turnstile',
  turnstile_site_key: '',
  turnstile_secret_key: '',
})

const form = reactive({
  site: emptySite(),
  captcha: emptyCaptcha(),
  web_base: '',
})

async function load() {
  loading.value = true
  try {
    const { data } = await getSettings()
    if (data.code === 0) {
      Object.assign(form.site, emptySite(), data.data.site)
      Object.assign(form.captcha, emptyCaptcha(), data.data.captcha)
      form.web_base = data.data.web_base
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '加载设置失败'))
  } finally {
    loading.value = false
  }
}
onMounted(() => {
  load()
  loadBackups()
  loadSystem()
})

// ---- 备份管理（ISSUE-17） ----
const backups = ref<BackupItem[]>([])
const backupLoading = ref(false)
const backupCreating = ref(false)

async function loadBackups() {
  backupLoading.value = true
  try {
    const { data } = await getBackups()
    if (data.code === 0) backups.value = data.data.items
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载备份列表失败'))
  } finally {
    backupLoading.value = false
  }
}

async function createBackupNow() {
  backupCreating.value = true
  try {
    const { data } = await createBackup()
    if (data.code === 0) {
      ElMessage.success(`备份成功：${data.data.file}`)
      loadBackups()
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '创建备份失败'))
  } finally {
    backupCreating.value = false
  }
}

function downloadBackup(file: string) {
  window.open(`${apiBase}/admin/backup/${encodeURIComponent(file)}`, '_blank')
}

function fmtSize(n: number) {
  if (!n) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`
}

// ---- 系统状态（ISSUE-17） ----
const system = ref<SystemStatus | null>(null)
const systemLoading = ref(false)

async function loadSystem() {
  systemLoading.value = true
  try {
    const { data } = await getSystemStatus()
    if (data.code === 0) system.value = data.data
    else ElMessage.error(data.message)
  } catch (e) {
    ElMessage.error(errMsg(e, '加载系统状态失败'))
  } finally {
    systemLoading.value = false
  }
}

async function save() {
  saving.value = true
  try {
    const { data } = await updateSettings({
      site: { ...form.site },
      captcha: { ...form.captcha },
      web_base: form.web_base,
    })
    if (data.code === 0) {
      ElMessage.success('设置已保存并立即生效')
      form.web_base = data.data.web_base
      if (form.web_base !== window.__PANEL_BASE__) {
        ElMessage.warning('访问路径已变更，刷新页面后请按新路径访问')
      }
    } else {
      ElMessage.error(data.message)
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '保存失败'))
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="x-page">
    <div class="x-toolbar">
      <div class="x-toolbar-left">
        <span style="font-weight: 600">站点设置</span>
        <span class="muted" style="font-size: 12px">站点品牌、人机验证与访问路径的统一入口，保存后立即生效。</span>
      </div>
      <el-button type="primary" :loading="saving" :icon="Check" @click="save">保存全部</el-button>
    </div>

    <BaseCard v-loading="loading" style="max-width: 860px">
      <el-tabs v-model="activeTab">
        <!-- ==================== TAB 1: 站点 ==================== -->
        <el-tab-pane label="🏷️ 站点" name="site">
          <el-form label-position="top" style="max-width: 640px">
            <div class="form-grid">
              <el-form-item label="站点名称（浏览器标题 / 订阅文件名）">
                <el-input v-model="form.site.app_name" placeholder="例如：星云机场" maxlength="64" />
              </el-form-item>
              <el-form-item label="站点描述">
                <el-input v-model="form.site.app_description" placeholder="一句话描述站点" maxlength="200" />
              </el-form-item>
              <el-form-item label="LOGO URL（登录页 / 管理端品牌展示）">
                <el-input v-model="form.site.logo" placeholder="https://example.com/logo.png" />
              </el-form-item>
              <el-form-item label="Favicon URL（浏览器标签图标）">
                <el-input v-model="form.site.favicon" placeholder="https://example.com/favicon.ico" />
              </el-form-item>
              <el-form-item label="订阅域名（预留：多域名分发）">
                <el-input v-model="form.site.subscribe_domain" placeholder="sub.example.com（可多个，逗号分隔）" />
              </el-form-item>
              <el-form-item label="服务条款 URL">
                <el-input v-model="form.site.tos_url" placeholder="https://example.com/tos" />
              </el-form-item>
              <el-form-item label="货币代码">
                <el-input v-model="form.site.currency" placeholder="CNY" maxlength="8" />
              </el-form-item>
              <el-form-item label="货币符号">
                <el-input v-model="form.site.currency_symbol" placeholder="¥" maxlength="8" />
              </el-form-item>
            </div>
            <el-form-item label="关闭注册">
              <el-switch
                v-model="form.site.stop_register"
                active-value="1"
                inactive-value="0"
                active-text="已关闭（新用户无法注册）"
                inactive-text="开放注册（需邀请码）"
              />
            </el-form-item>
          </el-form>
        </el-tab-pane>

        <!-- ==================== TAB 2: 安全（人机验证） ==================== -->
        <el-tab-pane label="🛡️ 安全" name="captcha">
          <el-form label-position="top" style="max-width: 640px">
            <el-form-item label="Cloudflare Turnstile 人机验证（登录 / 注册）">
              <el-switch
                v-model="form.captcha.captcha_enable"
                active-value="1"
                inactive-value="0"
                active-text="已开启（登录与注册需通过验证）"
                inactive-text="已关闭（内网 / 开发环境可关）"
              />
            </el-form-item>
            <el-form-item label="验证类型">
              <el-select v-model="form.captcha.captcha_type" style="width: 100%">
                <el-option label="Turnstile（Cloudflare）" value="turnstile" />
              </el-select>
            </el-form-item>
            <el-form-item label="Turnstile Site Key（前端公开）">
              <el-input v-model="form.captcha.turnstile_site_key" placeholder="0x4A...（Cloudflare 控制台获取）" />
            </el-form-item>
            <el-form-item label="Turnstile Secret Key（后端校验，仅管理端可见）">
              <el-input v-model="form.captcha.turnstile_secret_key" type="password" show-password placeholder="0x4B...（请勿泄露）" />
            </el-form-item>
            <p class="muted tip">
              在 Cloudflare 控制台创建站点并添加 Turnstile 小部件后，将 Site Key / Secret Key 填入此处并开启开关即可。
              未配置密钥时即使开启也会拒绝所有请求（fail-closed）。
            </p>
          </el-form>
        </el-tab-pane>

        <!-- ==================== TAB 3: 访问路径 ==================== -->
        <el-tab-pane label="🔗 访问路径" name="web_base">
          <el-form label-position="top" style="max-width: 640px">
            <el-form-item label="Web Base（自定义访问路径前缀）">
              <el-input v-model="form.web_base" placeholder="留空为根路径，如 /panel" />
            </el-form-item>
            <p class="muted tip">
              让面板（管理端 + 用户端 + API + 订阅）挂载在自定义子路径下，例如 https://example.com/panel/。
              留空 = 根路径。保存后立即生效（刷新页面即按新路径加载）。
            </p>
            <p class="muted tip">
              使用子路径时，反向代理需把该前缀（以及 /assets 静态资源）转发到主控；部署说明见 docs/部署指南.md。
            </p>
          </el-form>
        </el-tab-pane>

        <!-- ==================== TAB 4: 备份 ==================== -->
        <el-tab-pane label="💾 备份" name="backup">
          <div style="max-width: 720px">
            <div class="x-toolbar" style="margin-bottom: 12px">
              <div class="x-toolbar-left">
                <el-button :loading="backupLoading" :icon="Refresh" @click="loadBackups">刷新列表</el-button>
              </div>
              <el-button type="primary" :loading="backupCreating" @click="createBackupNow">立即备份</el-button>
            </div>
            <el-table v-loading="backupLoading" :data="backups" size="small">
              <el-table-column prop="file" label="备份文件" min-width="220">
                <template #default="{ row }"><code class="cell-mono">{{ row.file }}</code></template>
              </el-table-column>
              <el-table-column label="大小" width="110">
                <template #default="{ row }">{{ fmtSize(row.size) }}</template>
              </el-table-column>
              <el-table-column label="创建时间" width="170">
                <template #default="{ row }">{{ String(row.created_at).replace('T', ' ').slice(0, 19) }}</template>
              </el-table-column>
              <el-table-column label="操作" width="100" fixed="right">
                <template #default="{ row }">
                  <el-button link type="primary" :icon="Download" @click="downloadBackup(row.file)">下载</el-button>
                </template>
              </el-table-column>
            </el-table>
            <p class="muted tip">备份文件保存在主控备份目录，按配置定期轮转；建议每月下载一份离线保存。</p>
          </div>
        </el-tab-pane>

        <!-- ==================== TAB 5: 系统状态 ==================== -->
        <el-tab-pane label="🩺 系统状态" name="system">
          <div v-loading="systemLoading" style="max-width: 720px; min-height: 200px">
            <template v-if="system">
              <el-descriptions :column="2" border size="small">
                <el-descriptions-item label="应用">{{ system.app_name }}（{{ system.app_env }}）</el-descriptions-item>
                <el-descriptions-item label="Go 版本">{{ system.go_version }}</el-descriptions-item>
                <el-descriptions-item label="运行时长">{{ Math.floor(system.uptime_seconds / 3600) }}h {{ Math.floor((system.uptime_seconds % 3600) / 60) }}m</el-descriptions-item>
                <el-descriptions-item label="Goroutines">{{ system.goroutines }}</el-descriptions-item>
                <el-descriptions-item label="数据库">
                  <el-tag :type="system.db_ok ? 'success' : 'danger'" size="small">{{ system.db_ok ? '正常' : '异常' }}</el-tag>
                  <span style="margin-left: 6px">{{ system.db_driver }} · {{ system.db_latency_ms }}ms</span>
                </el-descriptions-item>
                <el-descriptions-item label="备份调度">{{ system.backup_enabled ? '已启用' : '未启用' }}</el-descriptions-item>
                <el-descriptions-item label="内存占用">{{ system.mem_alloc_mb }} MB</el-descriptions-item>
                <el-descriptions-item label="服务时间">{{ system.server_time }}</el-descriptions-item>
              </el-descriptions>
              <div class="status-count-grid">
                <div v-for="(v, k) in system.counts" :key="k" class="status-count">
                  <strong>{{ v }}</strong>
                  <span>{{ { users: '用户', servers: '服务器', inbounds: '入站', orders: '订单', gift_cards: '礼品卡', audit_logs: '审计日志' }[k] ?? k }}</span>
                </div>
              </div>
              <p v-if="!system.db_ok" class="muted tip">数据库异常：{{ system.db_error }}</p>
              <div class="x-toolbar" style="margin-top: 12px">
                <el-button :icon="Refresh" @click="loadSystem">刷新状态</el-button>
              </div>
            </template>
            <el-empty v-else-if="!systemLoading" description="暂无状态数据" />
          </div>
        </el-tab-pane>
      </el-tabs>
    </BaseCard>
  </div>
</template>

<style scoped lang="scss">
.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 0 16px;
  @media (max-width: 640px) {
    grid-template-columns: 1fr;
  }
}
.tip { font-size: 12.5px; margin: 4px 0; line-height: 1.7; color: var(--x-text-3); }
.cell-mono { font-family: ui-monospace, Menlo, Consolas, monospace; font-size: 12.5px; color: var(--x-text-2); }
.status-count-grid {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 10px;
  margin-top: 14px;
}
.status-count {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding: 12px;
  border: 1px solid var(--x-border-color, var(--el-border-color-lighter));
  border-radius: 10px;
  background: var(--x-bg-2, var(--el-bg-color-page));
}
.status-count strong { font-size: 20px; font-variant-numeric: tabular-nums; }
.status-count span { font-size: 12px; color: var(--x-text-3); }
</style>
