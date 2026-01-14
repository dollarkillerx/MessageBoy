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
  { title: 'Name', key: 'name' },
  {
    title: 'Status',
    key: 'status',
    render(row) {
      return h(NTag, {
        type: row.status === 'online' ? 'success' : 'default',
        size: 'small'
      }, { default: () => row.status })
    }
  },
  {
    title: 'Token',
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
    title: 'Last Seen',
    key: 'last_seen_at',
    render(row) {
      return row.last_seen_at ? new Date(row.last_seen_at).toLocaleString() : '-'
    }
  },
  {
    title: 'Actions',
    key: 'actions',
    render(row) {
      return h(NSpace, null, {
        default: () => [
          h(NButton, {
            size: 'small',
            onClick: () => showInstall(row.id)
          }, {
            default: () => 'Install',
            icon: () => h(NIcon, null, { default: () => h(TerminalOutline) })
          }),
          h(NPopconfirm, {
            onPositiveClick: () => handleRegenToken(row.id)
          }, {
            trigger: () => h(NButton, { size: 'small', secondary: true }, { default: () => 'Regen Token' }),
            default: () => 'Regenerate token? Client will need to reconnect.'
          }),
          h(NPopconfirm, {
            onPositiveClick: () => handleDelete(row.id)
          }, {
            trigger: () => h(NButton, { size: 'small', type: 'error', secondary: true }, { default: () => 'Delete' }),
            default: () => 'Delete this client?'
          })
        ]
      })
    }
  }
]

async function loadClients() {
  loading.value = true
  try {
    clients.value = await getClientList()
  } catch (error: unknown) {
    message.error((error as Error).message)
  } finally {
    loading.value = false
  }
}

async function handleCreate() {
  if (!newClientName.value) {
    message.warning('Please enter client name')
    return
  }

  createLoading.value = true
  try {
    await createClient(newClientName.value)
    message.success('Client created')
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
    message.success('Client deleted')
    await loadClients()
  } catch (error: unknown) {
    message.error((error as Error).message)
  }
}

async function handleRegenToken(id: string) {
  try {
    await regenerateClientToken(id)
    message.success('Token regenerated')
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
  message.success('Token copied')
}

function copyInstallCommand() {
  navigator.clipboard.writeText(installCommand.value)
  message.success('Command copied')
}

onMounted(loadClients)
</script>

<template>
  <NSpace vertical size="large">
    <NSpace justify="space-between" align="center">
      <NText tag="h2" style="margin: 0">Clients</NText>
      <NSpace>
        <NButton @click="loadClients" :loading="loading">
          <template #icon><NIcon><RefreshOutline /></NIcon></template>
          Refresh
        </NButton>
        <NButton type="primary" @click="showCreateModal = true">
          <template #icon><NIcon><AddOutline /></NIcon></template>
          Add Client
        </NButton>
      </NSpace>
    </NSpace>

    <NDataTable
      :columns="columns"
      :data="clients"
      :loading="loading"
      :row-key="(row: Client) => row.id"
    />

    <!-- Create Modal -->
    <NModal
      v-model:show="showCreateModal"
      title="Create Client"
      preset="card"
      style="width: 400px"
    >
      <NForm>
        <NFormItem label="Name">
          <NInput v-model:value="newClientName" placeholder="Client name" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showCreateModal = false">Cancel</NButton>
          <NButton type="primary" :loading="createLoading" @click="handleCreate">
            Create
          </NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- Install Modal -->
    <NModal
      v-model:show="showInstallModal"
      title="Install Client"
      preset="card"
      style="width: 600px"
    >
      <NSpace vertical>
        <NText>Run this command on the target machine:</NText>
        <NCode :code="installCommand" language="bash" word-wrap />
        <NButton @click="copyInstallCommand">
          <template #icon><NIcon><CopyOutline /></NIcon></template>
          Copy Command
        </NButton>
      </NSpace>
    </NModal>
  </NSpace>
</template>
