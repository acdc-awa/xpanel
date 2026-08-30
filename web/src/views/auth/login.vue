<script setup lang="ts">
import { onMounted, reactive, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { User, Lock, Key } from '@element-plus/icons-vue'
import { useAuthStore } from '@/stores/auth'
import { errMsg } from '@/api/http'
import { useSiteStore } from '@/stores/site'
import TurnstileWidget from '@/components/TurnstileWidget.vue'

const auth = useAuthStore()
const site = useSiteStore()
const router = useRouter()
const route = useRoute()

const form = reactive({ username: '', password: '', turnstile_token: '' })
const loading = ref(false)

// 2FA 二次验证
const twofaOpen = ref(false)
const twofaCode = ref('')
const twofaLoading = ref(false)

onMounted(async () => {
  await site.fetchConfig()
})

async function onSubmit() {
  if (!form.username || !form.password) {
    ElMessage.warning('请输入邮箱和密码')
    return
  }
  if (site.captchaEnable && !form.turnstile_token) {
    ElMessage.warning('请完成人机验证')
    return
  }
  loading.value = true
  try {
    const result = await auth.login(form.username.trim(), form.password, form.turnstile_token)
    if (result === '2fa') {
      twofaOpen.value = true
      twofaCode.value = ''
      return
    }
    gotoHome()
  } catch (e) {
    ElMessage.error(errMsg(e, '登录失败'))
  } finally {
    loading.value = false
  }
}

async function confirm2fa() {
  if (!twofaCode.value) {
    ElMessage.warning('请输入动态验证码')
    return
  }
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
  <div class="auth-stage">
    <div class="auth-card">
      <div class="auth-brand">
        <span v-if="site.logo" class="auth-logo-img"><img :src="site.logo" alt="logo" /></span>
        <span v-else class="auth-logo">X</span>
        <div>
          <div class="auth-title">{{ site.appName }}</div>
          <div class="auth-sub">{{ site.appDescription }}</div>
        </div>
      </div>

      <el-form label-position="top" size="large" @submit.prevent="onSubmit">
        <el-form-item label="邮箱（用户名）">
          <el-input v-model="form.username" placeholder="you@example.com" :prefix-icon="User" autofocus />
        </el-form-item>
        <el-form-item label="密码">
          <el-input
            v-model="form.password"
            type="password"
            show-password
            placeholder="请输入密码"
            :prefix-icon="Lock"
            @keyup.enter="onSubmit"
          />
        </el-form-item>
        <TurnstileWidget
          v-if="site.captchaEnable && site.turnstileSiteKey"
          :site-key="site.turnstileSiteKey"
          @token="(t) => (form.turnstile_token = t)"
        />
        <el-button
          type="primary"
          size="large"
          class="auth-submit"
          :loading="loading"
          native-type="submit"
        >
          登 录
        </el-button>
      </el-form>

      <div class="auth-foot">
        <router-link class="auth-link" to="/forgot">忘记密码？</router-link>
        <template v-if="!site.stopRegister">
          <span style="margin: 0 8px">·</span>
          还没有账号？
          <router-link class="auth-link" to="/register">使用邀请码注册</router-link>
        </template>
      </div>
    </div>

    <!-- 2FA 二次验证 -->
    <el-dialog v-model="twofaOpen" title="两步验证" width="360px" :close-on-click-modal="false" append-to-body>
      <p style="font-size: 13px; color: var(--x-text-2); margin-bottom: 14px">
        该账号已开启两步验证，请输入 Google Authenticator 动态验证码或恢复码
      </p>
      <el-input
        v-model="twofaCode"
        placeholder="6 位动态验证码"
        :prefix-icon="Key"
        size="large"
        maxlength="16"
        style="font-family: var(--x-font-mono)"
        @keyup.enter="confirm2fa"
      />
      <template #footer>
        <el-button type="primary" style="width: 100%" :loading="twofaLoading" @click="confirm2fa">
          验 证
        </el-button>
      </template>
    </el-dialog>
  </div>
</template>

<style scoped lang="scss">
.auth-stage {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 24px;
  background:
    radial-gradient(1200px 600px at 20% -10%, rgba(99, 102, 241, 0.35), transparent 60%),
    radial-gradient(1000px 500px at 110% 110%, rgba(168, 85, 247, 0.28), transparent 60%),
    var(--x-bg);
}
.auth-card {
  width: 100%;
  max-width: 400px;
  background: var(--x-card);
  border: 1px solid var(--x-border);
  border-radius: 16px;
  box-shadow: var(--x-shadow-lg);
  padding: 32px 28px;
}
.auth-brand { display: flex; align-items: center; gap: 12px; margin-bottom: 24px; }
.auth-logo {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  background: linear-gradient(135deg, #6366f1, #a855f7);
  color: #fff;
  font-size: 20px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex: none;
  box-shadow: 0 4px 14px rgba(99, 102, 241, 0.35);
}
.auth-logo-img {
  width: 44px;
  height: 44px;
  border-radius: 12px;
  overflow: hidden;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--x-bg, #f8fafc);
  border: 1px solid var(--x-border, #e2e8f0);
  flex: none;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);

  img {
    width: 100%;
    height: 100%;
    object-fit: contain;
  }
}
.auth-title { font-size: 19px; font-weight: 700; color: var(--x-text, #0f172a); }
.auth-sub { font-size: 12px; color: var(--x-text-3, #64748b); margin-top: 2px; }
.auth-submit { width: 100%; margin-top: 6px; font-weight: 600; letter-spacing: 4px; }
.auth-foot { margin-top: 20px; text-align: center; font-size: 13px; color: var(--x-text-2); }
.auth-link { color: var(--x-primary); font-weight: 600; }
</style>