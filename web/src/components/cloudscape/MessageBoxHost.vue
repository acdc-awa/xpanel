<script setup lang="ts">
/**
 * `ElMessageBox` 兼容壳的渲染宿主：挂在 App.vue，由 messagebox.ts 的命令式 API 驱动。
 *
 * 保留 Element Plus 的调用语义，所以现有 50 处 confirm 与 2 处 prompt 只换 import
 * 即可，不改业务逻辑；视觉换成 Cloudscape Modal + 官方按钮。删除类操作（type='error'
 * 或 confirmButtonClass 含 danger）用红实心确认 + 红描边取消，其余走主色按钮。
 */
import { computed, ref, watch } from 'vue'
import Modal from './Modal.vue'
import Button from './Button.vue'
import Input from './Input.vue'
import { CS } from './classes'
import { messageBoxState, resolveMessageBox } from './messagebox'

const request = computed(() => messageBoxState.current)

const inputValue = ref('')
const inputError = ref('')

watch(
  () => request.value?.id,
  () => {
    inputValue.value = request.value?.inputValue ?? ''
    inputError.value = ''
  }
)

/** prompt 打开时直接把焦点落到输入框（Modal 默认聚焦首个可聚焦元素＝关闭按钮） */
const initialFocus = computed(() => (request.value?.kind === 'prompt' ? '.cs-messagebox-input input' : undefined))

/** 调用点的 message 里用 \n\n 分段，这里按段落渲染以保留原有的视觉分隔 */
const paragraphs = computed(() =>
  (request.value?.message ?? '')
    .split(/\n{2,}/)
    .map((part) => part.trim())
    .filter(Boolean)
)

const closeOnInteract = computed(() => request.value?.showClose ?? true)

function dismiss(action: 'cancel' | 'close') {
  const current = request.value
  if (!current) return
  resolveMessageBox(current.id, action)
}

function confirm() {
  const current = request.value
  if (!current) return
  if (current.kind === 'prompt' && current.inputPattern && !current.inputPattern.test(inputValue.value)) {
    inputError.value = current.inputErrorMessage
    return
  }
  resolveMessageBox(current.id, 'confirm', inputValue.value)
}
</script>

<template>
  <Modal
    v-if="request"
    :visible="true"
    :header="request.title"
    size="small"
    :hide-dismiss-button="!closeOnInteract"
    :close-on-click-overlay="closeOnInteract"
    :close-on-escape="closeOnInteract"
    :initial-focus="initialFocus"
    @update:visible="(v: boolean) => !v && dismiss('close')"
  >
    <div :class="CS.box.box" class="cs-messagebox-body">
      <p v-for="(part, index) in paragraphs" :key="index" class="cs-messagebox-line">{{ part }}</p>

      <div v-if="request.kind === 'prompt'" class="cs-messagebox-input">
        <Input
          v-model="inputValue"
          :invalid="Boolean(inputError)"
          :placeholder="request.inputPlaceholder"
          :aria-label="request.title"
          autocomplete="off"
          @keyup="(e: KeyboardEvent) => e.key === 'Enter' && confirm()"
        />
        <p v-if="inputError" class="cs-messagebox-error">{{ inputError }}</p>
      </div>
    </div>

    <template #footer>
      <Button
        v-if="request.showCancel"
        :variant="request.danger ? 'danger-outline' : 'normal'"
        @click="dismiss('cancel')"
      >
        {{ request.cancelText }}
      </Button>
      <Button :variant="request.danger ? 'danger' : 'primary'" @click="confirm">
        {{ request.confirmText }}
      </Button>
    </template>
  </Modal>
</template>

<style scoped>
.cs-messagebox-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.cs-messagebox-line {
  margin: 0;
  font-size: 14px;
  line-height: 20px;
  white-space: pre-line;
  color: var(--color-text-body-default-b2us04, #0f141a);
}

.cs-messagebox-input {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.cs-messagebox-error {
  margin: 0;
  font-size: 12px;
  line-height: 16px;
  color: var(--color-text-status-error-ri38gt, #db0000);
}
</style>
