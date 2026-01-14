<script setup lang="ts">
import { ref } from 'vue'
import {
  NForm,
  NFormItem,
  NInput,
  NButton,
  useMessage
} from 'naive-ui'
import { useAuthStore } from '../stores/auth'

const authStore = useAuthStore()
const message = useMessage()

const loading = ref(false)
const formData = ref({
  username: '',
  password: ''
})

async function handleLogin() {
  if (!formData.value.username || !formData.value.password) {
    message.warning('请输入用户名和密码')
    return
  }

  loading.value = true
  try {
    await authStore.login(formData.value.username, formData.value.password)
    message.success('登录成功')
  } catch (error: unknown) {
    message.error((error as Error).message || '登录失败')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-container">
    <div class="login-card">
      <div class="login-header">
        <div class="logo">
          <svg viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
            <path d="M20 4H4C2.9 4 2 4.9 2 6V18C2 19.1 2.9 20 4 20H20C21.1 20 22 19.1 22 18V6C22 4.9 21.1 4 20 4Z" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
            <path d="M22 6L12 13L2 6" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </div>
        <h1 class="title">MessageBoy</h1>
        <p class="subtitle">管理控制台</p>
      </div>

      <NForm class="login-form">
        <NFormItem :show-label="false">
          <NInput
            v-model:value="formData.username"
            placeholder="用户名"
            size="large"
            @keyup.enter="handleLogin"
          >
            <template #prefix>
              <svg class="input-icon" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                <path d="M20 21V19C20 17.9391 19.5786 16.9217 18.8284 16.1716C18.0783 15.4214 17.0609 15 16 15H8C6.93913 15 5.92172 15.4214 5.17157 16.1716C4.42143 16.9217 4 17.9391 4 19V21" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                <circle cx="12" cy="7" r="4" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
            </template>
          </NInput>
        </NFormItem>
        <NFormItem :show-label="false">
          <NInput
            v-model:value="formData.password"
            type="password"
            placeholder="密码"
            size="large"
            show-password-on="click"
            @keyup.enter="handleLogin"
          >
            <template #prefix>
              <svg class="input-icon" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg">
                <rect x="3" y="11" width="18" height="11" rx="2" ry="2" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
                <path d="M7 11V7C7 5.67392 7.52678 4.40215 8.46447 3.46447C9.40215 2.52678 10.6739 2 12 2C13.3261 2 14.5979 2.52678 15.5355 3.46447C16.4732 4.40215 17 5.67392 17 7V11" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/>
              </svg>
            </template>
          </NInput>
        </NFormItem>
        <NButton
          type="primary"
          block
          size="large"
          :loading="loading"
          @click="handleLogin"
          class="login-button"
        >
          登 录
        </NButton>
      </NForm>

      <div class="login-footer">
        <p>安全代理管理系统</p>
      </div>
    </div>
  </div>
</template>

<style scoped>
.login-container {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #1a1c2e 0%, #2d3561 50%, #1a1c2e 100%);
  position: relative;
  overflow: hidden;
}

.login-container::before {
  content: '';
  position: absolute;
  top: -50%;
  left: -50%;
  width: 200%;
  height: 200%;
  background: radial-gradient(circle, rgba(99, 102, 241, 0.1) 0%, transparent 50%);
  animation: rotate 30s linear infinite;
}

@keyframes rotate {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

.login-card {
  background: rgba(255, 255, 255, 0.05);
  backdrop-filter: blur(20px);
  border-radius: 24px;
  padding: 48px 40px;
  width: 100%;
  max-width: 420px;
  box-shadow:
    0 25px 50px -12px rgba(0, 0, 0, 0.5),
    0 0 0 1px rgba(255, 255, 255, 0.1);
  position: relative;
  z-index: 1;
}

.login-header {
  text-align: center;
  margin-bottom: 40px;
}

.logo {
  width: 64px;
  height: 64px;
  margin: 0 auto 16px;
  background: linear-gradient(135deg, #6366f1 0%, #8b5cf6 100%);
  border-radius: 16px;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow: 0 10px 30px -5px rgba(99, 102, 241, 0.5);
}

.logo svg {
  width: 32px;
  height: 32px;
  color: white;
}

.title {
  font-size: 28px;
  font-weight: 700;
  color: white;
  margin: 0 0 8px 0;
  letter-spacing: -0.5px;
}

.subtitle {
  font-size: 14px;
  color: rgba(255, 255, 255, 0.5);
  margin: 0;
}

.login-form {
  margin-bottom: 24px;
}

.login-form :deep(.n-form-item) {
  margin-bottom: 16px;
}

.login-form :deep(.n-input) {
  --n-border: 1px solid rgba(255, 255, 255, 0.1);
  --n-border-hover: 1px solid rgba(99, 102, 241, 0.5);
  --n-border-focus: 1px solid #6366f1;
  --n-color: rgba(255, 255, 255, 0.05);
  --n-color-focus: rgba(255, 255, 255, 0.08);
  --n-text-color: white;
  --n-placeholder-color: rgba(255, 255, 255, 0.3);
  --n-caret-color: #6366f1;
  border-radius: 12px;
}

.input-icon {
  width: 18px;
  height: 18px;
  color: rgba(255, 255, 255, 0.4);
}

.login-button {
  margin-top: 8px;
  --n-color: linear-gradient(135deg, #6366f1 0%, #8b5cf6 100%);
  --n-color-hover: linear-gradient(135deg, #7c7ff7 0%, #9d78f8 100%);
  --n-color-pressed: linear-gradient(135deg, #5558e8 0%, #7a4ad4 100%);
  background: linear-gradient(135deg, #6366f1 0%, #8b5cf6 100%);
  border: none;
  border-radius: 12px;
  font-weight: 600;
  letter-spacing: 0.5px;
  transition: all 0.3s ease;
  box-shadow: 0 10px 30px -5px rgba(99, 102, 241, 0.4);
}

.login-button:hover {
  transform: translateY(-2px);
  box-shadow: 0 15px 35px -5px rgba(99, 102, 241, 0.5);
}

.login-footer {
  text-align: center;
}

.login-footer p {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.3);
  margin: 0;
}
</style>
