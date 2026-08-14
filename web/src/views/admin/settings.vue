<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { Check } from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import { getSettings, updateSettings, type SiteGroup, type CaptchaGroup } from '@/api/admin'
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
onMounted(load)

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
</style>
