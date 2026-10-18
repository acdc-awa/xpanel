<script setup lang="ts">
import { onMounted } from 'vue'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import { CsToastHost } from '@/components/cloudscape'
import { useSiteStore } from '@/stores/site'

const site = useSiteStore()
onMounted(() => {
  site.fetchConfig()
})
</script>

<template>
  <el-config-provider :locale="zhCn">
    <router-view />
    <!-- 瞬时提示宿主：挂在这里而不是各布局里，登录/注册/找回密码这类无布局页面同样能显示 -->
    <CsToastHost />
  </el-config-provider>
</template>
