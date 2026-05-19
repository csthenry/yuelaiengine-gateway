import { createRouter, createWebHistory } from 'vue-router'
import { getStoredToken } from '../stores/auth'
import ShellLayout from '../components/ShellLayout.vue'
import LoginView from '../views/LoginView.vue'
import DashboardView from '../views/DashboardView.vue'
import RoutesView from '../views/RoutesView.vue'
import ServicesView from '../views/ServicesView.vue'
import RateLimitView from '../views/RateLimitView.vue'
import ConfigView from '../views/ConfigView.vue'
import NotFoundView from '../views/NotFoundView.vue'

const router = createRouter({
  history: createWebHistory('/web/'),
  routes: [
    { path: '/login', component: LoginView },
    {
      path: '/',
      component: ShellLayout,
      children: [
        { path: '', redirect: '/dashboard' },
        { path: 'dashboard', component: DashboardView },
        { path: 'routes', component: RoutesView },
        { path: 'services', component: ServicesView },
        { path: 'ratelimit', component: RateLimitView },
        { path: 'config', component: ConfigView }
      ]
    },
    { path: '/:pathMatch(.*)*', component: NotFoundView }
  ]
})

router.beforeEach((to) => {
  const token = getStoredToken()
  if (to.path !== '/login' && !token) {
    return '/login'
  }
  if (to.path === '/login' && token) {
    return '/dashboard'
  }
  return true
})

export default router
