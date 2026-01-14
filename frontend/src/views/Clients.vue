<script setup lang="ts">
import { ref, onMounted, h } from 'vue'
import {
  NSpace,
  NText,
  NButton,
  NDataTable,
  NTag,
  NModal,
  NForm,
  NFormItem,
  NInput,
  NPopconfirm,
  useMessage,
  NIcon,
  NCode
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import { AddOutline, RefreshOutline, CopyOutline, TerminalOutline } from '@vicons/ionicons5'
import {
  getClientList,
  createClient,
  deleteClient,
  regenerateClientToken,
  getClientInstallCommand
} from '../api/rpc'
import type { Client } from '../types'

const message = useMessage()
const loading = ref(true)
const clients = ref<Client[]>([])

// Modal state
const showCreateModal = ref(false)
const showInstallModal = ref(false)
const createLoading = ref(false)
const newClientName = ref('')
const installCommand = ref('')

const columns: DataTableColumns<Client> = [
  { title: '名称', key: 'name' },
  {
    title: '状态',
    key: 'status',
    render(row) {
      return h(NTag, {
        type: row.status === 'online' ? 'success' : 'default',
        size: 'small'
      }, { default: () => row.status === 'online' ? '在线' : '离线' })
    }
  },
  {
    title: '令牌',
    key: 'token',
    render(row) {
      return h(NSpace, { align: 'center' }, {
        default: () => [
          h(NCode, { code: row.token.substring(0, 16) + '...', language: 'text' }),
          h(NButton, {
            size: 'tiny',
            quaternary: true,
            onClick: () => copyToken(row.token)
          }, { icon: () => h(NIcon, null, { default: () => h(CopyOutline) }) })
        ]
      })
    }
  },
  {
    title: '最后在线',
    key: 'last_seen_at',
    render(row) {
      return row.last_seen_at ? new Date(row.last_seen_at).toLocaleString() : '-'
    }
  },
  {
    title: '操作',
    key: 'actions',
    render(row) {
      return h(NSpace, null, {
        default: () => [
          h(NButton, {
            size: 'small',
            onClick: () => showInstall(row.id)
          }, {
            default: () => '安装',
            icon: () => h(NIcon, null, { default: () => h(TerminalOutline) })
          }),
          h(NPopconfirm, {
            onPositiveClick: () => handleRegenToken(row.id)
          }, {
            trigger: () => h(NButton, { size: 'small', secondary: true }, { default: () => '重置令牌' }),
            default: () => '确定重置令牌？客户端需要重新连接。'
          }),
          h(NPopconfirm, {
            onPositiveClick: () => handleDelete(row.id)
          }, {
            trigger: () => h(NButton, { size: 'small', type: 'error', secondary: true }, { default: () => '删除' }),
            default: () => '确定删除此客户端？'
          })
        ]
      })
    }
  }
]

async function loadClients() {
  loading.value = true
  try {
    const data = await getClientList()
    clients.value = Array.isArray(data) ? data : []
  } catch (error: unknown) {
    message.error((error as Error).message)
  } finally {
    loading.value = false
  }
}

async function handleCreate() {
  if (!newClientName.value) {
    message.warning('请输入客户端名称')
    return
  }

  createLoading.value = true
  try {
    await createClient(newClientName.value)
    message.success('客户端已创建')
    showCreateModal.value = false
    newClientName.value = ''
    await loadClients()
  } catch (error: unknown) {
    message.error((error as Error).message)
  } finally {
    createLoading.value = false
  }
}

async function handleDelete(id: string) {
  try {
    await deleteClient(id)
    message.success('客户端已删除')
    await loadClients()
  } catch (error: unknown) {
    message.error((error as Error).message)
  }
}

async function handleRegenToken(id: string) {
  try {
    await regenerateClientToken(id)
    message.success('令牌已重置')
    await loadClients()
  } catch (error: unknown) {
    message.error((error as Error).message)
  }
}

async function showInstall(id: string) {
  try {
    const result = await getClientInstallCommand(id)
    installCommand.value = result.command
    showInstallModal.value = true
  } catch (error: unknown) {
    message.error((error as Error).message)
  }
}

function copyToken(token: string) {
  navigator.clipboard.writeText(token)
  message.success('令牌已复制')
}

function copyInstallCommand() {
  navigator.clipboard.writeText(installCommand.value)
  message.success('命令已复制')
}

onMounted(loadClients)
</script>

<template>
  <NSpace vertical size="large">
    <NSpace justify="space-between" align="center">
      <NText tag="h2" style="margin: 0">客户端</NText>
      <NSpace>
        <NButton @click="loadClients" :loading="loading">
          <template #icon><NIcon><RefreshOutline /></NIcon></template>
          刷新
        </NButton>
        <NButton type="primary" @click="showCreateModal = true">
          <template #icon><NIcon><AddOutline /></NIcon></template>
          添加客户端
        </NButton>
      </NSpace>
    </NSpace>

    <NDataTable
      :columns="columns"
      :data="clients"
      :loading="loading"
      :row-key="(row: Client) => row.id"
    />

    <!-- 创建弹窗 -->
    <NModal
      v-model:show="showCreateModal"
      title="创建客户端"
      preset="card"
      style="width: 400px"
    >
      <NForm>
        <NFormItem label="名称">
          <NInput v-model:value="newClientName" placeholder="客户端名称" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showCreateModal = false">取消</NButton>
          <NButton type="primary" :loading="createLoading" @click="handleCreate">
            创建
          </NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- 安装弹窗 -->
    <NModal
      v-model:show="showInstallModal"
      title="安装客户端"
      preset="card"
      style="width: 600px"
    >
      <NSpace vertical>
        <NText>在目标机器上执行以下命令：</NText>
        <NCode :code="installCommand" language="bash" word-wrap />
        <NButton @click="copyInstallCommand">
          <template #icon><NIcon><CopyOutline /></NIcon></template>
          复制命令
        </NButton>
      </NSpace>
    </NModal>
  </NSpace>
</template>
