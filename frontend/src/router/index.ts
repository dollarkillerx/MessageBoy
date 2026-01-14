import { createRouter, createWebHistory } from 'vue-router'

const router = createRouter({
  history: createWebHistory(),
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('../views/Login.vue'),
      meta: { public: true }
    },
    {
      path: '/',
      component: () => import('../components/layout/MainLayout.vue'),
      children: [
        {
          path: '',
          name: 'dashboard',
          component: () => import('../views/Dashboard.vue')
        },
        {
          path: 'clients',
          name: 'clients',
          component: () => import('../views/Clients.vue')
        },
        {
          path: 'rules',
          name: 'rules',
          component: () => import('../views/ForwardRules.vue')
        },
        {
          path: 'groups',
          name: 'groups',
          component: () => import('../views/ProxyGroups.vue')
        }
      ]
    }
  ]
})

router.beforeEach((to, _from, next) => {
  const token = localStorage.getItem('token')
  if (!to.meta.public && !token) {
    next('/login')
  } else if (to.path === '/login' && token) {
    next('/')
  } else {
    next()
  }
})

export default router
