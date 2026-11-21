<script setup lang="ts">
import { nextTick, onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import {
  CsButton,
  CsForm,
  CsFormItem,
  CsInput,
  CsLink,
  toast,
} from '@/components/cloudscape'
import { useAuthStore } from '@/stores/auth'
import { useSiteStore } from '@/stores/site'
import { errMsg } from '@/api/http'
import TurnstileWidget from '@/components/TurnstileWidget.vue'
import AuthShell from './AuthShell.vue'

const auth = useAuthStore()
const site = useSiteStore()
const route = useRoute()
const router = useRouter()

const form = reactive({
  email: '',
  password: '',
  confirm: '',
  invite_code: '',
  turnstile_token: '',
})
const errors = reactive({ email: '', invite: '', password: '', confirm: '', captcha: '' })
const loading = ref(false)
const emailInputRef = ref<{ focus: () => void } | null>(null)
const turnstileRef = ref<{ reset: () => void } | null>(null)

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

// 人机验证 token 一次性：提交失败后必须换新 token，否则重试会被后端判为重复消费。
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

onMounted(async () => {
  nextTick(() => emailInputRef.value?.focus())
  await site.fetchConfig()
  const codeParam = (route.query.code || route.query.invite || route.query.invite_code) as string | undefined
  if (codeParam && codeParam.trim()) {
    form.invite_code = codeParam.trim()
  }
})

async function onSubmit() {
  if (loading.value) return
  errors.email = !form.email
    ? '请输入邮箱'
    : EMAIL_RE.test(form.email.trim())
      ? ''
      : '请输入正确的邮箱地址'
  errors.invite = form.invite_code.trim() ? '' : '请填写邀请码'
  errors.password = !form.password
    ? '请输入密码'
    : form.password.length < 8
      ? '密码至少 8 位'
      : ''
  errors.confirm = !form.confirm
    ? '请再次输入密码'
    : form.password !== form.confirm
      ? '两次输入的密码不一致'
      : ''
  errors.captcha = site.captchaEnable && !form.turnstile_token ? '请先完成人机验证' : ''
  if (errors.email || errors.invite || errors.password || errors.confirm || errors.captcha) return

  loading.value = true
  try {
    await auth.register({
      email: form.email.trim().toLowerCase(),
      password: form.password,
      invite_code: form.invite_code.trim(),
      turnstile_token: form.turnstile_token,
    })
    toast.success('注册成功，已自动登录')
    router.replace(auth.homePath())
  } catch (e) {
    toast.error(errMsg(e, '注册失败'))
    resetCaptcha()
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <AuthShell title="注册" description="需要邀请码才能注册">
    <CsForm @submit="onSubmit">
      <CsFormItem label="邮箱" :error="errors.email">
        <CsInput
          ref="emailInputRef"
          v-model="form.email"
          type="email"
          prefix-icon="envelope"
          autocomplete="email"
          placeholder="you@example.com"
          :invalid="Boolean(errors.email)"
          @update:model-value="errors.email = ''"
        />
      </CsFormItem>

      <CsFormItem label="邀请码" :error="errors.invite">
        <CsInput
          v-model="form.invite_code"
          prefix-icon="ticket"
          autocomplete="off"
          placeholder="请输入邀请码"
          clearable
          :invalid="Boolean(errors.invite)"
          @update:model-value="errors.invite = ''"
        />
      </CsFormItem>

      <CsFormItem label="密码" :error="errors.password">
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

      <CsFormItem label="确认密码" :error="errors.confirm">
        <CsInput
          v-model="form.confirm"
          type="password"
          show-password
          prefix-icon="lock-private"
          autocomplete="new-password"
          placeholder="再次输入密码"
          :invalid="Boolean(errors.confirm)"
          @update:model-value="errors.confirm = ''"
        />
      </CsFormItem>

      <CsFormItem v-if="site.captchaEnable && site.turnstileSiteKey" :error="errors.captcha">
        <TurnstileWidget ref="turnstileRef" :site-key="site.turnstileSiteKey" @token="onCaptchaToken" />
      </CsFormItem>

      <div class="auth-submit-wrap">
        <CsButton variant="primary" form-action="submit" full-width :loading="loading">
          注册
        </CsButton>
      </div>
    </CsForm>

    <template #footer>
      <div class="auth-footer auth-footer-centered">
        <span class="auth-footer-hint">
          <span>已有账号？</span>
          <CsLink href="/login" font-size="body-m" @follow="(_, e) => goRoute('/login', e)">
            直接登录
          </CsLink>
        </span>
      </div>
    </template>
  </AuthShell>
</template>
