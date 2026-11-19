<script setup lang="ts">
import { nextTick, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import {
  CsAlert,
  CsButton,
  CsForm,
  CsFormItem,
  CsInput,
  CsLink,
  ElMessage,
} from '@/components/cloudscape'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import { forgotPassword, resetPassword } from '@/api/auth'
import { errMsg } from '@/api/http'
import { useSiteStore } from '@/stores/site'
import AuthShell from './AuthShell.vue'

const site = useSiteStore()
const router = useRouter()

const step = ref<'email' | 'reset'>('email')
const email = ref('')
const form = reactive({ code: '', password: '', confirm: '' })
const turnstileToken = ref('')
const loading = ref(false)
const submitting = ref(false)
const hint = ref('')
const errors = reactive({ email: '', code: '', password: '', confirm: '', captcha: '' })
const emailInputRef = ref<{ focus: () => void } | null>(null)
const codeInputRef = ref<{ focus: () => void } | null>(null)
// 两步各有一个 widget（v-if/v-else 互斥渲染），各自持有重置句柄。
const turnstileEmailRef = ref<{ reset: () => void } | null>(null)
const turnstileResetRef = ref<{ reset: () => void } | null>(null)

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

// 人机验证 token 一次性：提交失败后必须换新 token，否则重试会被后端判为重复消费。
// 两步共用同一个 turnstileToken，因此重置当前步骤的 widget 即可。
function resetCaptcha(current: 'email' | 'reset') {
  if (!site.captchaEnable) return
  turnstileToken.value = ''
  const target = current === 'email' ? turnstileEmailRef.value : turnstileResetRef.value
  target?.reset()
}

function onCaptchaToken(token: string) {
  turnstileToken.value = token
  if (token) errors.captcha = ''
}

function goRoute(path: string, event: Event) {
  event.preventDefault()
  void router.push(path)
}

onMounted(async () => {
  nextTick(() => emailInputRef.value?.focus())
  await site.fetchConfig()
})

async function submitEmail() {
  if (loading.value) return
  errors.email = !email.value
    ? '请输入邮箱'
    : EMAIL_RE.test(email.value.trim())
      ? ''
      : '请输入正确的邮箱地址'
  errors.captcha = site.captchaEnable && !turnstileToken.value ? '请先完成人机验证' : ''
  if (errors.email || errors.captcha) return

  loading.value = true
  try {
    const { data } = await forgotPassword(email.value.trim().toLowerCase(), turnstileToken.value || undefined)
    if (data.code === 0) {
      hint.value = data.data.message
      step.value = 'reset'
      // 第一步的验证码已消费，第二步的 widget 会重新取 token，先清掉旧值避免误用。
      turnstileToken.value = ''
      await nextTick()
      codeInputRef.value?.focus()
    } else {
      ElMessage.error(data.message)
      resetCaptcha('email')
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '提交失败，请稍后重试'))
    resetCaptcha('email')
  } finally {
    loading.value = false
  }
}

async function submitReset() {
  if (submitting.value) return
  errors.code = form.code.trim() ? '' : '请输入动态验证码或恢复码'
  errors.password = !form.password
    ? '请输入新密码'
    : form.password.length < 8
      ? '新密码至少 8 位'
      : ''
  errors.confirm = !form.confirm
    ? '请再次输入新密码'
    : form.password !== form.confirm
      ? '两次输入的新密码不一致'
      : ''
  errors.captcha = site.captchaEnable && !turnstileToken.value ? '请先完成人机验证' : ''
  if (errors.code || errors.password || errors.confirm || errors.captcha) return

  submitting.value = true
  try {
    const { data } = await resetPassword(
      email.value.trim().toLowerCase(),
      form.code.trim(),
      form.password,
      turnstileToken.value || undefined,
    )
    if (data.code === 0) {
      ElMessage.success('密码已重置，请重新登录')
      router.replace('/login')
    } else {
      ElMessage.error(data.message)
      resetCaptcha('reset')
    }
  } catch (e) {
    ElMessage.error(errMsg(e, '重置失败，请稍后重试'))
    resetCaptcha('reset')
  } finally {
    submitting.value = false
  }
}
</script>

<template>
  <AuthShell
    title="重置密码"
    description="需邮箱和两步验证动态码（未开启两步验证的账号请联系管理员）"
  >
    <!-- 第一步：邮箱 -->
    <CsForm v-if="step === 'email'" @submit="submitEmail">
      <CsFormItem label="注册邮箱" :error="errors.email">
        <CsInput
          ref="emailInputRef"
          v-model="email"
          type="email"
          prefix-icon="envelope"
          autocomplete="email"
          placeholder="you@example.com"
          :invalid="Boolean(errors.email)"
          @update:model-value="errors.email = ''"
        />
      </CsFormItem>

      <CsFormItem v-if="site.captchaEnable && site.turnstileSiteKey" :error="errors.captcha">
        <TurnstileWidget ref="turnstileEmailRef" :site-key="site.turnstileSiteKey" @token="onCaptchaToken" />
      </CsFormItem>

      <div class="auth-submit-wrap">
        <CsButton variant="primary" form-action="submit" full-width :loading="loading">
          下一步
        </CsButton>
      </div>
    </CsForm>

    <!-- 第二步：验证码 + 新密码 -->
    <CsForm v-else @submit="submitReset">
      <CsAlert v-if="hint" type="info" class="reset-hint">{{ hint }}</CsAlert>

      <CsFormItem label="两步验证动态码 / 恢复码" :error="errors.code">
        <CsInput
          ref="codeInputRef"
          v-model="form.code"
          class="code-input"
          prefix-icon="key"
          autocomplete="one-time-code"
          placeholder="验证器动态码或恢复码"
          :maxlength="16"
          :invalid="Boolean(errors.code)"
          @update:model-value="errors.code = ''"
        />
      </CsFormItem>

      <CsFormItem label="新密码" :error="errors.password">
        <CsInput
          v-model="form.password"
          type="password"
          show-password
          prefix-icon="lock-private"
          autocomplete="new-password"
          placeholder="至少 8 位字符"
          :invalid="Boolean(errors.password)"
          @update:model-value="errors.password = ''"
        />
      </CsFormItem>

      <CsFormItem label="确认新密码" :error="errors.confirm">
        <CsInput
          v-model="form.confirm"
          type="password"
          show-password
          prefix-icon="lock-private"
          autocomplete="new-password"
          placeholder="再次输入新密码"
          :invalid="Boolean(errors.confirm)"
          @update:model-value="errors.confirm = ''"
        />
      </CsFormItem>

      <CsFormItem v-if="site.captchaEnable && site.turnstileSiteKey" :error="errors.captcha">
        <TurnstileWidget ref="turnstileResetRef" :site-key="site.turnstileSiteKey" @token="onCaptchaToken" />
      </CsFormItem>

      <div class="auth-submit-wrap">
        <CsButton variant="primary" form-action="submit" full-width :loading="submitting">
          重置密码
        </CsButton>
      </div>
    </CsForm>

    <template #footer>
      <div class="auth-footer auth-footer-centered">
        <CsLink href="/login" font-size="body-m" @follow="(_, e) => goRoute('/login', e)">
          返回登录
        </CsLink>
      </div>
    </template>
  </AuthShell>
</template>

<style scoped lang="scss">
.reset-hint {
  margin-block-end: 16px;
}

.code-input :deep(input) {
  font-family: var(--x-font-mono);
  font-variant-numeric: tabular-nums;
  letter-spacing: 0.08em;
}
</style>
