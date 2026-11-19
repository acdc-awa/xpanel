<script setup lang="ts">
import { nextTick, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  CsButton,
  CsForm,
  CsFormItem,
  CsInput,
  CsLink,
  CsModal,
  CsSpaceBetween,
  ElMessage,
} from '@/components/cloudscape'
import { useAuthStore } from '@/stores/auth'
import { errMsg } from '@/api/http'
import { useSiteStore } from '@/stores/site'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import AuthShell from './AuthShell.vue'

const auth = useAuthStore()
const site = useSiteStore()
const router = useRouter()
const route = useRoute()

const form = reactive({ email: '', password: '', turnstile_token: '' })
const errors = reactive({ email: '', password: '', captcha: '' })
const loading = ref(false)
const emailInputRef = ref<{ focus: () => void } | null>(null)
const turnstileRef = ref<{ reset: () => void } | null>(null)

// 2FA 二次验证
const twofaOpen = ref(false)
const twofaCode = ref('')
const twofaError = ref('')
const twofaLoading = ref(false)

onMounted(async () => {
  nextTick(() => emailInputRef.value?.focus())
  await site.fetchConfig()
})

// 人机验证 token 一次性：任何一次提交失败后都必须换新 token，
// 否则用户带旧 token 重试会被后端判为重复消费，看到的是「人机验证未通过」而非真实原因。
function resetCaptcha() {
  if (!site.captchaEnable) return
  form.turnstile_token = ''
  turnstileRef.value?.reset()
}

function onCaptchaToken(token: string) {
  form.turnstile_token = token
  if (token) errors.captcha = ''
}

function goRoute(path: string, event: Event) {
  event.preventDefault()
  void router.push(path)
}

async function onSubmit() {
  if (loading.value) return
  errors.email = form.email.trim() ? '' : '请输入邮箱'
  errors.password = form.password ? '' : '请输入密码'
  errors.captcha = site.captchaEnable && !form.turnstile_token ? '请先完成人机验证' : ''
  if (errors.email || errors.password || errors.captcha) return

  loading.value = true
  try {
    const result = await auth.login(form.email.trim(), form.password, form.turnstile_token)
    if (result === '2fa') {
      twofaCode.value = ''
      twofaError.value = ''
      twofaOpen.value = true
      return
    }
    gotoHome()
  } catch (e) {
    ElMessage.error(errMsg(e, '登录失败'))
    resetCaptcha()
  } finally {
    loading.value = false
  }
}

async function confirm2fa() {
  if (twofaLoading.value) return
  if (!twofaCode.value.trim()) {
    twofaError.value = '请输入动态验证码或恢复码'
    return
  }
  twofaError.value = ''
  twofaLoading.value = true
  try {
    await auth.verify2fa(twofaCode.value.trim())
    twofaOpen.value = false
    gotoHome()
  } catch (e) {
    ElMessage.error(errMsg(e, '验证失败'))
  } finally {
    twofaLoading.value = false
  }
}

function gotoHome() {
  // 强制改密（J8）：初始 admin123 等账号首次登录必须先改密码
  if (auth.user?.must_change_pwd) {
    ElMessage.warning('首次登录请先修改初始密码')
    router.replace('/account')
    return
  }
  const redirect = (route.query.redirect as string) || auth.homePath()
  router.replace(redirect)
}
</script>

<template>
  <AuthShell title="登录">
    <CsForm @submit="onSubmit">
      <CsFormItem label="邮箱" :error="errors.email">
        <CsInput
          ref="emailInputRef"
          v-model="form.email"
          type="email"
          prefix-icon="envelope"
          autocomplete="username"
          placeholder="you@example.com"
          :invalid="Boolean(errors.email)"
          @update:model-value="errors.email = ''"
        />
      </CsFormItem>

      <CsFormItem label="密码" :error="errors.password">
        <CsInput
          v-model="form.password"
          type="password"
          show-password
          prefix-icon="lock-private"
          autocomplete="current-password"
          placeholder="请输入密码"
          :invalid="Boolean(errors.password)"
          @update:model-value="errors.password = ''"
        />
      </CsFormItem>

      <CsFormItem v-if="site.captchaEnable && site.turnstileSiteKey" :error="errors.captcha">
        <TurnstileWidget
          ref="turnstileRef"
          :site-key="site.turnstileSiteKey"
          @token="onCaptchaToken"
        />
      </CsFormItem>

      <div class="auth-submit-wrap">
        <CsButton variant="primary" form-action="submit" full-width :loading="loading">
          登录
        </CsButton>
      </div>
    </CsForm>

    <template #footer>
      <div class="auth-footer">
        <CsLink href="/forgot" font-size="body-m" @follow="(_, e) => goRoute('/forgot', e)">
          忘记密码
        </CsLink>
        <span v-if="!site.stopRegister" class="auth-register-hint">
          <span>还没有账号？</span>
          <CsLink href="/register" font-size="body-m" @follow="(_, e) => goRoute('/register', e)">
            使用邀请码注册
          </CsLink>
        </span>
      </div>
    </template>
  </AuthShell>

  <!-- 2FA 二次验证 -->
  <CsModal
    v-model:visible="twofaOpen"
    header="两步验证"
    description="该账号已开启两步验证，请输入验证器动态码或恢复码"
    size="small"
    :close-on-click-overlay="false"
    initial-focus=".twofa-code-input input"
  >
    <CsForm @submit="confirm2fa">
      <CsFormItem label="动态验证码 / 恢复码" :error="twofaError" class="twofa-form-item">
        <CsInput
          v-model="twofaCode"
          class="twofa-code-input"
          prefix-icon="key"
          autocomplete="one-time-code"
          placeholder="6 位动态验证码"
          :maxlength="16"
          :invalid="Boolean(twofaError)"
          @update:model-value="twofaError = ''"
        />
      </CsFormItem>
    </CsForm>
    <template #footer>
      <CsSpaceBetween direction="horizontal" size="xs">
        <CsButton :disabled="twofaLoading" @click="twofaOpen = false">取消</CsButton>
        <CsButton variant="primary" :loading="twofaLoading" @click="confirm2fa">验证</CsButton>
      </CsSpaceBetween>
    </template>
  </CsModal>
</template>

<style scoped lang="scss">
.auth-submit-wrap {
  margin-block-start: 4px;
}

.auth-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 8px 16px;
  font-size: 13px;
  line-height: 1.5;
  color: var(--color-text-body-secondary-66ygbb, #656871);
}

.auth-register-hint {
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.twofa-form-item {
  margin-block-end: 0;
}

.twofa-code-input :deep(input) {
  font-family: var(--x-font-mono);
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.08em;
}
</style>
