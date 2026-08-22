import { createApp } from 'vue'
import { createPinia } from 'pinia'
import 'element-plus/dist/index.css'
import 'element-plus/theme-chalk/dark/css-vars.css'
import App from './App.vue'
import router from './router'
import { appName } from '@/config/site'
import './styles/index.scss'

// 站点标题（index.html 已注入 <title>；SPA 内路由切换不重载页面，这里兜底同步一次）
document.title = appName

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(router)
app.mount('#app')
