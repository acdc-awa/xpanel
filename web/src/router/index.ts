import { createRouter, createWebHistory } from 'vue-router'
import { clientRoutes } from './modules/client'
import { adminRoutes } from './modules/admin'
import { setupRouterGuards } from './guards'

const router = createRouter({
  history: createWebHistory(),
  routes: [...clientRoutes, ...adminRoutes],
  scrollBehavior: () => ({ top: 0 }),
})

setupRouterGuards(router)

export default router
