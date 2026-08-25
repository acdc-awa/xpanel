import { fileURLToPath, URL } from 'node:url'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import AutoImport from 'unplugin-auto-import/vite'
import Components from 'unplugin-vue-components/vite'
import { ElementPlusResolver } from 'unplugin-vue-components/resolvers'

export default defineConfig({
  plugins: [
    vue(),
    AutoImport({ resolvers: [ElementPlusResolver()] }),
    Components({ resolvers: [ElementPlusResolver()] }),
  ],
  resolve: {
    alias: { '@': fileURLToPath(new URL('./src', import.meta.url)) },
  },
  css: {
    preprocessorOptions: {
      scss: { api: 'modern' },
    },
  },
  server: {
    port: 5173,
    host: '0.0.0.0',
    proxy: {
      // 开发期把 /api 转发到主控后端（避免 CORS）；生产由反代同域反代
      // 三端口模型：SPA 与 API 合并监听 APP_PORT（默认 18080），故直接指面板端口
      '/api': {
        target: 'http://127.0.0.1:18080',
        changeOrigin: true,
      },
    },
  },
})