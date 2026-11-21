import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import router from './router'
import { appName } from '@/config/site'
import './styles/index.scss'

// 站点标题（index.html 已注入 <title>；SPA 内路由切换不重载页面，这里兜底同步一次）
document.title = appName

// 激活 Cloudscape vendor 的 `body[data-awsui-focus-visible="true"]` 键盘焦点环规则
const FOCUS_VISIBLE_KEYS = new Set(['Tab', 'ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight', 'Enter', ' '])
document.addEventListener(
  'keydown',
  (event) => {
    if (FOCUS_VISIBLE_KEYS.has(event.key)) {
      document.body?.setAttribute('data-awsui-focus-visible', 'true')
    }
  },
  true
)
document.addEventListener(
  'pointerdown',
  () => {
    document.body?.removeAttribute('data-awsui-focus-visible')
  },
  true
)

const app = createApp(App)
const pinia = createPinia()
app.use(pinia)
app.use(router)
app.mount('#app')
