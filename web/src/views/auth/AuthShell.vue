<script setup lang="ts">
import { CsButton, CsContainer, CsHeader } from '@/components/cloudscape'
import { useSiteStore } from '@/stores/site'
import { useThemeStore } from '@/stores/theme'

defineProps<{
  title: string
  description?: string
}>()

const site = useSiteStore()
const theme = useThemeStore()
</script>

<template>
  <div class="auth-stage" :class="{ 'is-dark': theme.isDark }">
    <div class="auth-topbar">
      <CsButton
        variant="icon"
        icon-size="medium"
        :icon-name="theme.isDark ? 'moon' : 'sun'"
        :aria-label="theme.modeTooltip"
        @click="theme.toggle()"
      />
    </div>

    <main class="auth-main">
      <CsContainer class="auth-container" disable-header-paddings disable-content-paddings>
        <template #header>
          <div class="auth-card-head">
            <div class="auth-brand">
              <span v-if="site.logo" class="auth-logo-img">
                <img :src="site.logo" :alt="site.appName || ''" />
              </span>
              <span v-else class="auth-logo-mark" aria-hidden="true">X</span>
              <span class="auth-brand-text">
                <span class="auth-brand-name">{{ site.appName }}</span>
                <span v-if="site.appDescription" class="auth-brand-desc">
                  {{ site.appDescription }}
                </span>
              </span>
            </div>
            <CsHeader variant="h1" :description="description">
              {{ title }}
            </CsHeader>
          </div>
        </template>

        <div class="auth-card-body">
          <slot />
        </div>

        <template v-if="$slots.footer" #footer>
          <div class="auth-card-foot">
            <slot name="footer" />
          </div>
        </template>
      </CsContainer>
    </main>
  </div>
</template>

<style scoped lang="scss">
.auth-stage {
  position: relative;
  min-block-size: 100vh;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 64px 20px 40px;
  background-color: var(--color-neutral-150-44n9cw, #f6f6f9);
  background-image: radial-gradient(
    960px 420px at 50% -120px,
    rgba(0, 108, 224, 0.08),
    transparent 70%
  );
  color: var(--color-text-body-default-b2us04, #0f141a);
}

.auth-stage.is-dark {
  background-color: var(--color-neutral-950-qa1l7j, #0f141a);
  background-image: radial-gradient(
    960px 420px at 50% -120px,
    rgba(66, 180, 255, 0.1),
    transparent 70%
  );
}

.auth-topbar {
  position: absolute;
  inset-block-start: 20px;
  inset-inline-end: 20px;
}

.auth-main {
  inline-size: 100%;
  max-inline-size: 460px;
}

.auth-container {
  inline-size: 100%;
}

.auth-card-head {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 28px 32px 0;
}

.auth-card-body {
  padding: 24px 32px 28px;

  /* 认证页是单焦点短表单，比控制台密集表单放大控件触控高度（32px → 44px）与行距 */
  :deep(.cs-form-item) {
    margin-block-end: 20px;
  }

  /* 官方 input/button 规则带 :not(#\9) 提权，这里必须 !important 才能覆盖 */
  :deep(input) {
    min-block-size: 44px !important;
    padding-block: 10px !important;
    font-size: 16px !important;
  }

  :deep(button[type='submit']) {
    min-block-size: 44px !important;
    padding-block: 10px !important;
    font-size: 16px !important;
  }

  /* 输入框前缀/尾部图标按官方 medium 档（20px）放大；尾部按钮同步放大到 24px 避免裁切 */
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

  :deep(.auth-submit-wrap) {
    margin-block-start: 4px;
  }
}

.auth-card-foot {
  padding: 4px 12px;

  /* footer 槽内容由各页提供，样式在此统一，避免三页各复制一份 */
  :deep(.auth-footer) {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 8px 16px;
    font-size: 13px;
    line-height: 1.5;
    color: var(--color-text-body-secondary-66ygbb, #656871);
  }

  :deep(.auth-footer-centered) {
    justify-content: center;
  }

  :deep(.auth-footer-hint) {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
}

.auth-brand {
  display: flex;
  align-items: center;
  gap: 12px;
  min-inline-size: 0;
}

.auth-brand-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-inline-size: 0;
}

.auth-logo-mark {
  inline-size: 40px;
  block-size: 40px;
  border-radius: 8px;
  background: var(--x-primary);
  color: #ffffff;
  font-size: 20px;
  font-weight: 800;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: none;
}

.auth-logo-img {
  inline-size: 40px;
  block-size: 40px;
  border-radius: 8px;
  overflow: hidden;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  /* logo 多为带透明圆角的图片，再加边框/底色会在四角露出方形线框 */
  background: transparent;
  flex: none;

  img {
    inline-size: 100%;
    block-size: 100%;
    object-fit: contain;
  }
}

.auth-brand-name {
  font-size: 16px;
  line-height: 1.4;
  font-weight: 700;
  color: var(--color-text-body-default-b2us04, #0f141a);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.auth-brand-desc {
  font-size: 12px;
  line-height: 1.5;
  color: var(--color-text-body-secondary-66ygbb, #656871);
  overflow: hidden;
  display: -webkit-box;
  -webkit-box-orient: vertical;
  -webkit-line-clamp: 2;
}

@media (max-width: $bp-small) {
  .auth-card-head {
    padding: 24px 20px 0;
  }

  .auth-card-body {
    padding: 20px 20px 24px;
  }

  .auth-card-foot {
    padding: 2px 0;
  }
}
</style>
