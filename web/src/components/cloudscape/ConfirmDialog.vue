<script setup lang="ts">
/**
 * 破坏性操作确认弹窗（AWS 控制台 ConfirmDialog 口径）。
 *
 * 官方 Cloudscape 只提供 Modal，确认框由 AWS 控制台在上层拼装：标题是问句，
 * 正文首行用红色「Warning:」开头，底部是红色描边取消 + 红色实心确认。
 * 本组件把这套组合固定下来，避免各页再各自拼 ElMessageBox（原生 confirm 无法
 * 使用 Cloudscape 的按钮与配色）。
 */
import { computed } from 'vue'
import Button from './Button.vue'
import Modal from './Modal.vue'
import { CS } from './classes'

const props = withDefaults(
  defineProps<{
    visible: boolean
    /** 问句式标题，例如「删除这条入站记录？」 */
    header: string
    /** 正文说明；不传则只显示 warning 行之外的空内容 */
    message?: string
    /** 红色「Warning:」后面的句子，AWS 用它陈述不可逆后果 */
    warning?: string
    confirmText?: string
    cancelText?: string
    loading?: boolean
  }>(),
  {
    message: undefined,
    warning: undefined,
    confirmText: '确认',
    cancelText: '取消',
    loading: false,
  }
)

const emit = defineEmits<{
  (e: 'update:visible', value: boolean): void
  (e: 'confirm'): void
}>()

const visibleProxy = computed({
  get: () => props.visible,
  set: (value: boolean) => emit('update:visible', value),
})

function cancel() {
  emit('update:visible', false)
}
</script>

<template>
  <Modal v-model:visible="visibleProxy" :header="header" size="small" :close-on-click-overlay="false" close-on-escape>
    <div :class="CS.box.box" class="cs-confirm-body">
      <p v-if="warning" class="cs-confirm-line">
        <strong class="cs-confirm-warning">Warning:</strong>
        <span>{{ warning }}</span>
      </p>
      <p v-if="message" class="cs-confirm-line">{{ message }}</p>
      <slot />
    </div>
    <template #footer>
      <Button variant="danger-outline" :disabled="loading" @click="cancel">{{ cancelText }}</Button>
      <Button variant="danger" :loading="loading" @click="emit('confirm')">{{ confirmText }}</Button>
    </template>
  </Modal>
</template>

<style scoped>
.cs-confirm-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.cs-confirm-line {
  margin: 0;
  font-size: 14px;
  line-height: 20px;
  color: var(--color-text-body-default-b2us04, #0f141a);
}

/* AWS 控制台用红色粗体「Warning:」引出不可逆后果，红色沿用状态错误色，
   深色模式下由官方 token 自动换成浅红。 */
.cs-confirm-warning {
  color: var(--color-text-status-error-ri38gt, #db0000);
  font-weight: 700;
  margin-inline-end: 4px;
}
</style>
