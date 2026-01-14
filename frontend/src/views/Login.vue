<script setup lang="ts">
import { ref } from 'vue'
import {
  NCard,
  NForm,
  NFormItem,
  NInput,
  NButton,
  NSpace,
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
    message.warning('Please enter username and password')
    return
  }

  loading.value = true
  try {
    await authStore.login(formData.value.username, formData.value.password)
    message.success('Login successful')
  } catch (error: unknown) {
    message.error((error as Error).message || 'Login failed')
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="login-container">
    <NCard title="MessageBoy Admin" style="width: 400px">
      <NForm>
        <NFormItem label="Username">
          <NInput
            v-model:value="formData.username"
            placeholder="Enter username"
            @keyup.enter="handleLogin"
          />
        </NFormItem>
        <NFormItem label="Password">
          <NInput
            v-model:value="formData.password"
            type="password"
            placeholder="Enter password"
            @keyup.enter="handleLogin"
          />
        </NFormItem>
        <NSpace justify="end">
          <NButton type="primary" :loading="loading" @click="handleLogin">
            Login
          </NButton>
        </NSpace>
      </NForm>
    </NCard>
  </div>
</template>

<style scoped>
.login-container {
  height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}
</style>
