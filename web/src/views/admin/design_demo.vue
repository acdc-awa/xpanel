<script setup lang="ts">
import { ref } from 'vue'
import { ElMessageBox, ElMessage } from 'element-plus'
import {
  Monitor,
  User,
  Connection,
  Tickets,
  Sunny,
  Moon,
  Opportunity,
  Setting,
  Share,
  Lock,
  Plus,
  Refresh,
} from '@element-plus/icons-vue'
import BaseCard from '@/components/base/BaseCard.vue'
import StatCard from '@/components/base/StatCard.vue'
import { useThemeStore } from '@/stores/theme'

const theme = useThemeStore()

// 模拟表单与设置数据
const sampleInput = ref('xray.node.example.com')
const sampleSelect = ref('vless')
const toggleAutoPush = ref(true)
const toggleDeviceLimit = ref(false)

// 模拟表格数据
const sampleTableData = ref([
  {
    id: 1,
    name: '香港 01 · BGP 专线',
    protocol: 'VLESS',
    stream: 'TCP + XTLS Vision',
    port: 443,
    ratio: '1.2x',
    status: 'online',
    latency: '18 ms',
    traffic: '1.42 TB',
  },
  {
    id: 2,
    name: '日本 02 · 软银原生',
    protocol: 'VLESS',
    stream: 'XHTTP + TLS',
    port: 8443,
    ratio: '1.0x',
    status: 'online',
    latency: '42 ms',
    traffic: '840.6 GB',
  },
  {
    id: 3,
    name: '美国 03 · 洛杉矶 CN2',
    protocol: 'VLESS',
    stream: 'WS + TLS',
    port: 2053,
    ratio: '0.8x',
    status: 'offline',
    latency: '--',
    traffic: '320.1 GB',
  },
])

function testConfirm() {
  ElMessageBox.confirm('这是一个统一风格的确认弹窗测试，请检查背景、文字与边框的暗色对比度。', '系统操作确认', {
    confirmButtonText: '确定提交',
    cancelButtonText: '取消',
    type: 'warning',
  })
    .then(() => {
      ElMessage.success('操作已执行成功！')
    })
    .catch(() => {})
}

function testToast() {
  ElMessage.success('Element Plus Toast 消息提示组件正常联动！')
}
</script>

<template>
  <div class="x-page">
    <!-- 顶部 Banner: 主题切换控制器 -->
    <div class="theme-hero-card">
      <div class="hero-left">
        <div class="hero-icon-wrap">
          <el-icon :size="24"><Opportunity /></el-icon>
        </div>
        <div class="hero-text">
          <h2 class="hero-title">设计系统与双模规范体验中心 (Design Demo)</h2>
          <p class="hero-desc">
            当前处于
            <strong class="highlight-mode">{{ theme.isDark ? '🌙 深色模式 (Dark)' : '☀️ 浅色模式 (Light)' }}</strong>
            ，所有卡片、胶囊、状态点、表格均已实现框架级自动适配。
          </p>
        </div>
      </div>
      <div class="hero-actions">
        <el-radio-group :model-value="theme.mode" size="large" class="theme-segmented" @change="(val) => theme.setMode(val as any)">
          <el-radio-button value="light">
            <el-icon><Sunny /></el-icon>&nbsp;浅色
          </el-radio-button>
          <el-radio-button value="dark">
            <el-icon><Moon /></el-icon>&nbsp;深色
          </el-radio-button>
          <el-radio-button value="auto">
            <el-icon><Setting /></el-icon>&nbsp;跟随系统
          </el-radio-button>
        </el-radio-group>
      </div>
    </div>

    <!-- 1. 统一指标统计卡片 (<StatCard>) -->
    <div class="section-title">
      <el-icon><Monitor /></el-icon>&nbsp;1. 指标统计卡片 (&lt;StatCard&gt;)
    </div>
    <div class="x-stat-grid">
      <StatCard :icon="Monitor" value="18 / 20" label="在线服务器数" tone="green" />
      <StatCard :icon="User" value="1,248" label="本月有效用户" tone="purple" />
      <StatCard :icon="Tickets" value="¥ 12,850" label="本月卡密营收" tone="blue" />
      <StatCard :icon="Connection" value="2.84 TB" label="今日系统总吞吐" tone="orange" />
    </div>

    <!-- 2. 统一胶囊芯片与状态点 (.x-chip & .x-status-dot) -->
    <BaseCard title="2. 统一胶囊芯片体系 (.x-chip) 与在线呼吸状态点">
      <div class="chips-demo-wrap">
        <div class="chip-group">
          <span class="group-label">协议/类型标签:</span>
          <span class="x-chip purple">VLESS</span>
          <span class="x-chip blue">TCP + REALITY</span>
          <span class="x-chip green">XHTTP + TLS</span>
          <span class="x-chip orange">Shadowsocks</span>
          <span class="x-chip gray">未分配组</span>
        </div>

        <div class="chip-group">
          <span class="group-label">状态与属性标签:</span>
          <span class="x-chip green"><span class="x-status-dot online" />在线 · 18 ms</span>
          <span class="x-chip red"><span class="x-status-dot offline" />节点离线</span>
          <span class="x-chip orange"><span class="x-status-dot connecting" />待补推配置</span>
          <span class="x-chip blue cell-mono">倍率 1.2x</span>
          <span class="x-chip purple cell-mono">端口: 443</span>
        </div>
      </div>
    </BaseCard>

    <!-- 3. 标准业务卡片与数据表格 (<BaseCard> + el-table) -->
    <BaseCard title="3. 业务数据卡片与表格 (&lt;BaseCard&gt; + el-table)">
      <template #extra>
        <el-button type="primary" size="small" :icon="Plus" @click="testToast">
          新建接入点
        </el-button>
        <el-button size="small" :icon="Refresh" @click="testConfirm">
          弹窗测试
        </el-button>
      </template>

      <el-table :data="sampleTableData" style="width: 100%">
        <el-table-column label="节点名称" min-width="180">
          <template #default="{ row }">
            <div class="node-cell">
              <span class="x-status-dot" :class="row.status" />
              <span class="node-name">{{ row.name }}</span>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="协议" width="110">
          <template #default="{ row }">
            <span class="x-chip purple">{{ row.protocol }}</span>
          </template>
        </el-table-column>
        <el-table-column label="传输与安全" min-width="150">
          <template #default="{ row }">
            <span class="x-chip blue">{{ row.stream }}</span>
          </template>
        </el-table-column>
        <el-table-column label="分享端口" width="110">
          <template #default="{ row }">
            <span class="cell-mono">:{{ row.port }}</span>
          </template>
        </el-table-column>
        <el-table-column label="倍率" width="100">
          <template #default="{ row }">
            <span class="x-chip orange cell-mono">{{ row.ratio }}</span>
          </template>
        </el-table-column>
        <el-table-column label="本月流量" width="130">
          <template #default="{ row }">
            <span class="cell-mono font-medium">{{ row.traffic }}</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="130" fixed="right">
          <template #default>
            <el-button link type="primary" size="small">编辑</el-button>
            <el-button link type="danger" size="small" @click="testConfirm">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </BaseCard>

    <!-- 4. 统一独立开关卡片 (.x-toggle-card) & 表单控件 -->
    <BaseCard title="4. 开关卡片 (.x-toggle-card) 与基础表单输入">
      <div class="form-demo-grid">
        <div class="x-toggle-card">
          <div class="toggle-info">
            <span class="toggle-title">自动推送配置变更</span>
            <span class="toggle-desc">当节点出站、入站或路由规则发生修改时，实时触发非阻塞推送到在线 Agent。</span>
          </div>
          <el-switch v-model="toggleAutoPush" />
        </div>

        <div class="x-toggle-card">
          <div class="toggle-info">
            <span class="toggle-title">在线设备限制 (Device Limit)</span>
            <span class="toggle-desc">开启后对该入站应用 IP 维度同时在线并发数管控。</span>
          </div>
          <el-switch v-model="toggleDeviceLimit" />
        </div>
      </div>

      <div class="form-inputs-row">
        <el-form label-position="top" inline class="demo-form">
          <el-form-item label="分享域名 / Host">
            <el-input v-model="sampleInput" placeholder="请输入服务器地址" style="width: 240px" />
          </el-form-item>
          <el-form-item label="协议类型">
            <el-select v-model="sampleSelect" style="width: 180px">
              <el-option label="VLESS" value="vless" />
              <el-option label="Shadowsocks" value="ss" />
              <el-option label="Trojan" value="trojan" />
            </el-select>
          </el-form-item>
          <el-form-item label="快捷弹窗交互">
            <el-button type="primary" plain @click="testConfirm">打开 MessageBox 确认框</el-button>
          </el-form-item>
        </el-form>
      </div>
    </BaseCard>
  </div>
</template>

<style scoped lang="scss">
.theme-hero-card {
  background: var(--x-card);
  border: 1px solid var(--x-border);
  border-radius: var(--x-radius);
  padding: 20px 24px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  margin-bottom: 24px;
  box-shadow: var(--x-shadow);
  transition: all 0.24s ease;
  flex-wrap: wrap;

  .hero-left {
    display: flex;
    align-items: center;
    gap: 16px;
    min-width: 0;
  }

  .hero-icon-wrap {
    width: 46px;
    height: 46px;
    border-radius: 12px;
    background: var(--x-primary-soft);
    color: var(--x-primary);
    display: flex;
    align-items: center;
    justify-content: center;
    flex: none;
  }

  .hero-title {
    font-size: 16px;
    font-weight: 700;
    color: var(--x-text);
    margin: 0 0 4px 0;
  }

  .hero-desc {
    font-size: 13px;
    color: var(--x-text-2);
    margin: 0;

    .highlight-mode {
      color: var(--x-primary);
    }
  }

  .theme-segmented {
    :deep(.el-radio-button__inner) {
      display: flex;
      align-items: center;
    }
  }
}

.section-title {
  font-size: 14px;
  font-weight: 700;
  color: var(--x-text);
  margin-bottom: 12px;
  display: flex;
  align-items: center;
}

.chips-demo-wrap {
  padding: 16px 20px;
  display: flex;
  flex-direction: column;
  gap: 14px;

  .chip-group {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;

    .group-label {
      font-size: 13px;
      color: var(--x-text-2);
      min-width: 110px;
      font-weight: 500;
    }
  }
}

.node-cell {
  display: flex;
  align-items: center;
  gap: 8px;

  .node-name {
    font-weight: 600;
    color: var(--x-text);
  }
}

.font-medium {
  font-weight: 600;
}

.form-demo-grid {
  padding: 18px 20px 0 20px;
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}

.form-inputs-row {
  padding: 18px 20px 4px 20px;
}

@media (max-width: 900px) {
  .form-demo-grid {
    grid-template-columns: 1fr;
  }
  .theme-hero-card {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
