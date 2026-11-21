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
  toast,
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
    toast.error(errMsg(e, '登录失败'))
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
    const msg = errMsg(e, '验证失败')
    twofaError.value = msg
    toast.error(msg)
  } finally {
    twofaLoading.value = false
  }
}

function cancel2fa() {
  twofaOpen.value = false
  twofaError.value = ''
  resetCaptcha()
}

function gotoHome() {
  // 强制改密（J8）：初始 admin123 等账号首次登录必须先改密码
  if (auth.user?.must_change_pwd) {
    toast.warning('首次登录请先修改初始密码')
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
        <span v-if="!site.stopRegister" class="auth-footer-hint">
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
    @update:visible="(v) => !v && resetCaptcha()"
  >
    <div class="twofa-body">
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
    </div>
    <template #footer>
      <CsSpaceBetween direction="horizontal" size="xs">
        <CsButton class="twofa-btn" :disabled="twofaLoading" @click="cancel2fa">取消</CsButton>
        <CsButton class="twofa-btn" variant="primary" :loading="twofaLoading" @click="confirm2fa">
          验证
        </CsButton>
      </CsSpaceBetween>
    </template>
  </CsModal>
</template>

<style scoped lang="scss">
.twofa-form-item {
  margin-block-end: 0;
}

/* 弹窗挂在 AuthShell 之外，控件尺寸需单独对齐认证页的 44px/16px */
.twofa-body {
  :deep(input) {
    min-block-size: 44px !important;
    padding-block: 10px !important;
    font-size: 16px !important;
  }

  :deep(.cs-input-root [class*='input-icon-start'] [class*='awsui_icon']),
  :deep(.cs-input-root [class*='input-icon-start'] svg),
  :deep(.cs-input-root [class*='input-icon-end'] svg) {
    inline-size: 20px !important;
    block-size: 20px !important;
    padding-block: 0 !important;
  }

  :deep(.cs-input-root .cs-icon-button) {
    inline-size: 24px !important;
    block-size: 24px !important;
  }
}

.twofa-btn {
  min-block-size: 44px !important;
  padding-block: 10px !important;
  font-size: 16px !important;
}

.twofa-code-input :deep(input) {
  font-family: var(--x-font-mono);
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.08em;
}
</style>
