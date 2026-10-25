<script setup lang="ts">
import { computed, ref } from 'vue'
import { Handle, Position } from '@vue-flow/core'
import type { PermissionGroup, UserAccessPoint } from '@/api/types'
import { protocolDisplayName } from '@/utils/protocol'
import { useGridHeight } from '../useGridHeight'

const props = defineProps<{
  data: { ap: UserAccessPoint }
  editable: boolean
  permissionGroups: PermissionGroup[]
}>()

const emit = defineEmits<{
  (e: 'edit', ap: UserAccessPoint): void
}>()

const boxEl = ref<HTMLElement | null>(null)
useGridHeight(boxEl)

const groupNameMap = computed(
  () => new Map(props.permissionGroups.map((g) => [g.id, g.name]))
)

function groupName(id: number): string {
  return groupNameMap.value.get(id) ?? `组#${id}`
}
</script>

<template>
  <div
    ref="boxEl"
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
      <span v-if="!data.ap.enabled" class="ap-chip gray">禁用</span>
    </div>

    <div class="ap-node-perms">
      <template v-if="data.ap.permission_group_ids && data.ap.permission_group_ids.length > 0">
        <span v-for="gid in data.ap.permission_group_ids.slice(0, 3)" :key="gid" class="ap-chip blue">
          {{ groupName(gid) }}
        </span>
        <span v-if="data.ap.permission_group_ids.length > 3" class="ap-chip gray">
          +{{ data.ap.permission_group_ids.length - 3 }}
        </span>
      </template>
      <span v-else class="ap-chip orange">未授权（全员不可见）</span>
    </div>

    <!-- 管道消费的实时端点展示 -->
    <div v-if="data.ap.resolved_host" class="ap-resolved-text">
      <span class="ap-dot online" />
      <code class="cell-mono">{{ data.ap.resolved_host }}:{{ data.ap.resolved_port }}</code>
      <span class="ap-chip" :class="data.ap.resolved_protocol === 'hysteria' ? 'teal' : 'purple'">
        {{ data.ap.resolved_protocol === 'hysteria' ? 'Hysteria 2' : (protocolDisplayName(data.ap.resolved_protocol).toUpperCase() || 'VLESS') }}
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
  background: var(--topo-card-bg, rgba(24, 33, 47, 0.95));
  border: 1px solid rgba(56, 189, 248, 0.35);
  border-radius: var(--topo-radius, 16px);
  padding: 16px;
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
    gap: 8px;
    min-height: 24px;
  }
  .ap-icon {
    font-size: 14px;
    flex: none;
  }
  .ap-name-text {
    font-size: 13px;
    font-weight: 700;
    color: var(--topo-text-1, #e2e8f0);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    flex: 1;
    min-width: 0;
  }
  /* 固定 24px 高、不换行：权限组数量再多也不改变卡片高度，保证底边落在 8pt 网格上 */
  .ap-node-perms {
    display: flex;
    align-items: center;
    gap: 4px;
    flex-wrap: nowrap;
    overflow: hidden;
    height: 24px;

    .ap-chip {
      flex: none;
      max-width: 120px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
  }
  .ap-resolved-text {
    display: flex;
    align-items: center;
    gap: 6px;
    min-height: 24px;
    font-size: 11.5px;
    color: var(--topo-accent, #38bdf8);
    &.waiting {
      color: var(--topo-warning, #fbbf24);
    }
  }
  .ap-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex: none;
    &.online {
      background: var(--topo-success, #34d399);
      box-shadow: 0 0 6px var(--topo-success, #34d399);
    }
    &.offline {
      background: var(--topo-warning, #fbbf24);
      box-shadow: 0 0 4px var(--topo-warning, #fbbf24);
    }
  }
  .ap-desc-text {
    font-size: 10.5px;
    line-height: 16px;
    color: var(--topo-text-2, #94a3b8);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* 画布内自有色板：不跟随全局亮/暗主题，保证深色卡片上始终可读 */
  .ap-chip {
    display: inline-flex;
    align-items: center;
    height: 18px;
    padding: 0 6px;
    border-radius: 9px;
    border: 1px solid transparent;
    font-size: 10px;
    line-height: 1;
    font-weight: 500;
    white-space: nowrap;

    &.blue {
      background: rgba(56, 189, 248, 0.14);
      border-color: rgba(56, 189, 248, 0.28);
      color: #7dd3fc;
    }
    &.gray {
      background: rgba(148, 163, 184, 0.12);
      border-color: rgba(148, 163, 184, 0.22);
      color: #94a3b8;
    }
    &.orange {
      background: rgba(251, 191, 36, 0.12);
      border-color: rgba(251, 191, 36, 0.28);
      color: #fbbf24;
    }
    &.teal {
      background: rgba(45, 212, 191, 0.14);
      border-color: rgba(45, 212, 191, 0.28);
      color: #2dd4bf;
    }
    &.purple {
      background: rgba(192, 132, 252, 0.14);
      border-color: rgba(192, 132, 252, 0.28);
      color: #c084fc;
    }
  }
}
</style>
