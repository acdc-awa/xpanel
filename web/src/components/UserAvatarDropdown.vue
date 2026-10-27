<script setup lang="ts">
/**
 * 顶栏用户菜单。原实现用 Element Plus 的 el-dropdown + el-icon，
 * 与 Cloudscape 外壳并排时是两套视觉（阴影、圆角、hover 底色都不同）。
 * 这里改用 CsPopover（官方浮层定位与 Esc/外部点击关闭）+ CsButtonDropdown 的
 * 菜单项样式，头像仍保留渐变徽标（顶部只有 32px，官方 Avatar 未移植）。
 */
import { computed, ref } from 'vue'
import { useRouter } from 'vue-router'
import { CsBadge, CsIcon, CsPopover, ElMessageBox } from '@/components/cloudscape'
import { useAuthStore } from '@/stores/auth'

const props = defineProps<{
  currentPortal?: 'client' | 'admin'
  /** 菜单头部的补充信息（管理端用于显示版本号/在线态）。 */
  footnote?: string
}>()

const auth = useAuthStore()
const router = useRouter()
const menuOpen = ref(false)

const roleLabel = computed(() => (auth.role === 'admin' ? '管理员' : '普通用户'))

const menuItems = computed(() => {
  const items = [{ id: 'account', text: '我的账户', iconName: 'user-profile' }]
  if (auth.role === 'admin' && props.currentPortal !== 'admin') {
    items.push({ id: 'admin', text: '进入管理后台', iconName: 'settings' })
  }
  if (props.currentPortal === 'admin') {
    items.push({ id: 'client', text: '返回用户中心', iconName: 'user-profile' })
  }
  items.push({ id: 'logout', text: '退出登录', iconName: 'sign-out' })
  return items
})

function onSelect(id: string) {
  menuOpen.value = false
  if (id === 'account') router.push('/account')
  else if (id === 'admin') router.push('/admin/dashboard')
  else if (id === 'client') router.push('/dashboard')
  else if (id === 'logout') void handleLogout()
}

async function handleLogout() {
  try {
    await ElMessageBox.confirm('确定退出登录？', '退出登录', {
      confirmButtonText: '退出登录',
      cancelButtonText: '取消',
      type: 'warning',
    })
  } catch {
    return
  }
  await auth.logout()
  await router.replace('/login')
}
</script>

<template>
  <CsPopover v-model:visible="menuOpen" placement="bottom-end" :width="220">
    <template #reference>
      <button
        type="button"
        class="user-avatar-badge"
        :title="`${auth.username}（${roleLabel}）`"
        :aria-label="`${auth.username}（${roleLabel}）`"
        :aria-expanded="menuOpen"
      >
        {{ auth.avatarText }}
      </button>
    </template>

    <div class="user-menu-header">
      <div class="u-name" :title="auth.username">{{ auth.username }}</div>
      <div class="u-sub">
        <CsBadge :color="auth.role === 'admin' ? 'blue' : 'grey'">{{ roleLabel }}</CsBadge>
        <span v-if="footnote" class="u-footnote">{{ footnote }}</span>
      </div>
    </div>

    <ul class="user-menu-list" role="menu">
      <li v-for="item in menuItems" :key="item.id" :class="{ 'is-separator': item.id === 'logout' }">
        <button type="button" class="user-menu-item" role="menuitem" @click="onSelect(item.id)">
          <CsIcon :name="item.iconName" size="normal" />
          <span>{{ item.text }}</span>
        </button>
      </li>
    </ul>
  </CsPopover>
</template>

<style scoped lang="scss">
.user-avatar-badge {
  inline-size: 32px;
  block-size: 32px;
  border-radius: 50%;
  border: 2px solid transparent;
  background: var(--color-background-avatar-default-2c3w8u, #0f141a);
  color: var(--color-text-inverted-fflnsl, #ffffff);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: var(--font-size-body-s-89y7qg, 12px);
  font-weight: 700;
  cursor: pointer;
  padding: 0;
  transition: border-color 0.15s ease;

  &:hover,
  &:focus-visible {
    border-color: var(--color-border-item-focused-sewkzn, #006ce0);
    outline: none;
  }
}

/* CsPopover 自带 12px 内边距；头部用负边距做通栏（分隔线到面板边缘），内部再补回 12px */
.user-menu-header {
  display: flex;
  flex-direction: column;
  gap: 6px;
  margin: -12px -12px 0;
  padding: 12px;
  border-block-end: 1px solid var(--color-border-divider-default-diwd4w, #c6c6cd);
}

.u-name {
  min-inline-size: 0;
  font-size: var(--font-size-body-m-tq5a14, 14px);
  line-height: var(--line-height-body-m-5xw6xo, 20px);
  font-weight: 700;
  color: var(--color-text-body-default-b2us04, #0f141a);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 角色徽标与版本号同行；徽标只按内容宽度（父级是 row，不会被拉成整条） */
.u-sub {
  display: flex;
  align-items: center;
  gap: var(--space-xs-m9b7jf, 8px);
  min-inline-size: 0;
}

.u-footnote {
  min-inline-size: 0;
  font-size: var(--font-size-body-s-89y7qg, 12px);
  line-height: var(--line-height-body-s-abys8r, 16px);
  color: var(--color-text-body-secondary-66ygbb, #424650);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

/* 通栏：hover 底色贴到面板两侧，与官方下拉菜单一致 */
.user-menu-list {
  list-style: none;
  margin: 0 -12px -12px;
  padding: 6px;
}

/* 退出登录前的分隔线（列表项的上下内边距由 li 承担，线才不会贴着文字） */
.user-menu-list li.is-separator {
  margin-block-start: 4px;
  padding-block-start: 4px;
  border-block-start: 1px solid var(--color-border-divider-default-diwd4w, #c6c6cd);
}

.user-menu-item {
  display: flex;
  align-items: center;
  gap: var(--space-xs-m9b7jf, 8px);
  inline-size: 100%;
  padding-block: var(--space-xs-m9b7jf, 8px);
  padding-inline: var(--space-xs-m9b7jf, 8px);
  border: 0;
  border-radius: var(--border-radius-input-k1q34z, 8px);
  background: transparent;
  color: var(--color-text-body-default-b2us04, #0f141a);
  font: inherit;
  font-size: var(--font-size-body-m-tq5a14, 14px);
  text-align: start;
  cursor: pointer;

  &:hover,
  &:focus-visible {
    background: var(--color-background-dropdown-item-hover-pvfsrg, #f2f3f3);
    outline: none;
  }

  &:focus-visible {
    box-shadow: inset 0 0 0 2px var(--color-border-item-focused-sewkzn, #006ce0);
  }
}
</style>
