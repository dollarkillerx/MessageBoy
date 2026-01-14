import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { adminLogin } from '../api/rpc'
import router from '../router'

export const useAuthStore = defineStore('auth', () => {
  const token = ref<string | null>(localStorage.getItem('token'))

  const isLoggedIn = computed(() => !!token.value)

  async function login(username: string, password: string) {
    const result = await adminLogin(username, password)
    token.value = result.token
    localStorage.setItem('token', result.token)
    router.push('/')
  }

  function logout() {
    token.value = null
    localStorage.removeItem('token')
    router.push('/login')
  }

  return {
    token,
    isLoggedIn,
    login,
    logout
  }
})
