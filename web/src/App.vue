<script setup lang="ts">
import { onMounted } from 'vue'
import zhCn from 'element-plus/es/locale/lang/zh-cn'
import { CsMessageBoxHost, CsToastHost } from '@/components/cloudscape'
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
    <!-- 确认/输入弹窗宿主：与 toast 同理，调用点用 ElMessageBox.confirm/prompt 即可 -->
    <CsMessageBoxHost />
  </el-config-provider>
</template>
