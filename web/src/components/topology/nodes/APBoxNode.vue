<script setup lang="ts">
import { computed } from 'vue'
import { Handle, Position } from '@vue-flow/core'
import type { PermissionGroup, UserAccessPoint } from '@/api/types'

const props = defineProps<{
  data: { ap: UserAccessPoint }
  editable: boolean
  permissionGroups: PermissionGroup[]
}>()

const emit = defineEmits<{
  (e: 'edit', ap: UserAccessPoint): void
}>()

const groupNameMap = computed(
  () => new Map(props.permissionGroups.map((g) => [g.id, g.name]))
)

function groupName(id: number): string {
  return groupNameMap.value.get(id) ?? `组#${id}`
}
</script>

<template>
  <div
    class="ap-node"
    :class="{ unlinked: !data.ap.target_type, disabled: !data.ap.enabled }"
    @click.stop="emit('edit', data.ap)"
  >
    <!-- 右侧源端点：拖线至目标入站（inb-tgt） -->
    <Handle
      type="source"
      :id="`ap-src-${data.ap.id}`"
      :position="Position.Right"
      :connectable="editable"
      class="ep ap-src-handle"
      title="拖线至目标用户入站"
    />

    <div class="ap-node-head">
      <span class="ap-icon">🌐</span>
      <span class="ap-name-text" :title="data.ap.name">{{ data.ap.name }}</span>
      <span v-if="!data.ap.enabled" class="x-chip gray" style="font-size: 10px; padding: 1px 4px">禁用</span>
    </div>

    <div class="ap-node-perms">
      <template v-if="data.ap.permission_group_ids && data.ap.permission_group_ids.length > 0">
        <span
          v-for="gid in data.ap.permission_group_ids.slice(0, 3)"
          :key="gid"
          class="x-chip blue"
          style="font-size: 10px; padding: 1px 5px"
        >
          {{ groupName(gid) }}
        </span>
        <span v-if="data.ap.permission_group_ids.length > 3" class="x-chip gray" style="font-size: 10px">
          +{{ data.ap.permission_group_ids.length - 3 }}
        </span>
      </template>
      <span v-else class="x-chip orange" style="font-size: 10px">未授权（全员不可见）</span>
    </div>

    <!-- 管道消费的实时端点展示 -->
    <div v-if="data.ap.resolved_host" class="ap-resolved-text">
      <span class="ap-dot online" />
      <code class="cell-mono">{{ data.ap.resolved_host }}:{{ data.ap.resolved_port }}</code>
      <span class="x-chip" :class="data.ap.resolved_protocol === 'hysteria' ? 'teal' : 'purple'" style="font-size: 10px">
        {{ data.ap.resolved_protocol === 'hysteria' ? 'Hysteria 2' : (data.ap.resolved_protocol?.toUpperCase() || 'VLESS') }}
      </span>
    </div>
    <div v-else class="ap-resolved-text waiting">
      <span class="ap-dot offline" />
      <span style="font-size: 11px">待连线：从右侧端点拖出</span>
    </div>

    <div v-if="data.ap.resolved_target_desc" class="ap-desc-text" :title="data.ap.resolved_target_desc">
      直连 ➜ {{ data.ap.resolved_target_desc }}
    </div>
  </div>
</template>

<style scoped lang="scss">
.ap-node {
  position: relative;
  width: 280px;
  background: rgba(24, 33, 47, 0.95);
  border: 1px solid rgba(56, 189, 248, 0.35);
  border-radius: 14px;
  padding: 12px 14px;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 8px;
  box-shadow:
    0 6px 24px rgba(0, 0, 0, 0.35),
    0 2px 12px rgba(56, 189, 248, 0.12),
    inset 0 1px 0 rgba(255, 255, 255, 0.1);
  transition: border-color 0.2s, box-shadow 0.2s;
  contain: paint;
  transform: translateZ(0);

  &:hover {
    border-color: rgba(56, 189, 248, 0.6);
    box-shadow:
      0 6px 24px rgba(0, 0, 0, 0.35),
      0 4px 18px rgba(56, 189, 248, 0.2),
      inset 0 1px 0 rgba(255, 255, 255, 0.1);
  }
  &.unlinked {
    border-style: dashed;
    border-color: rgba(251, 191, 36, 0.45);
    box-shadow:
      0 6px 24px rgba(0, 0, 0, 0.35),
      0 2px 10px rgba(251, 191, 36, 0.1),
      inset 0 1px 0 rgba(255, 255, 255, 0.1);
  }
  &.disabled {
    opacity: 0.6;
    filter: grayscale(0.4);
  }

  .ap-node-head {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .ap-icon {
    font-size: 14px;
    flex: none;
  }
  .ap-name-text {
    font-size: 13px;
    font-weight: 700;
    color: #f1f5f9;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 1;
    min-width: 0;
  }
  .ap-node-perms {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-wrap: wrap;
  }
  .ap-resolved-text {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 11.5px;
    color: #38bdf8;
    &.waiting {
      color: #fbbf24;
    }
  }
  .ap-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex: none;
    &.online {
      background: #10b981;
      box-shadow: 0 0 6px #10b981;
    }
    &.offline {
      background: #fbbf24;
      box-shadow: 0 0 4px #fbbf24;
    }
  }
  .ap-desc-text {
    font-size: 10.5px;
    color: #94a3b8;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
}
</style>
