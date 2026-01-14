<script setup lang="ts">
import { ref, onMounted, h, computed } from 'vue'
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
  NSelect,
  NSwitch,
  NPopconfirm,
  useMessage,
  NIcon
} from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import { AddOutline, RefreshOutline } from '@vicons/ionicons5'
import {
  getForwardRuleList,
  getClientList,
  getProxyGroupList,
  createForwardRule,
  updateForwardRule,
  deleteForwardRule,
  toggleForwardRule
} from '../api/rpc'
import type { ForwardRule, Client, ProxyGroup } from '../types'

const message = useMessage()
const loading = ref(true)
const rules = ref<ForwardRule[]>([])
const clients = ref<Client[]>([])
const groups = ref<ProxyGroup[]>([])

// Modal state
const showModal = ref(false)
const modalLoading = ref(false)
const editingId = ref<string | null>(null)
const formData = ref({
  name: '',
  type: 'direct' as 'direct' | 'relay',
  listen_addr: '',
  listen_client: '',
  relay_chain: [] as string[],
  exit_addr: '',
  enabled: true
})

const clientOptions = computed<SelectOption[]>(() =>
  clients.value.map(c => ({ label: `${c.name} (${c.status})`, value: c.id }))
)

const relayOptions = computed<SelectOption[]>(() => [
  ...groups.value.map(g => ({ label: `@${g.name} (Group)`, value: `@${g.name}` })),
  ...clients.value.map(c => ({ label: c.name, value: c.id }))
])

const typeOptions: SelectOption[] = [
  { label: 'Direct', value: 'direct' },
  { label: 'Relay', value: 'relay' }
]

const columns: DataTableColumns<ForwardRule> = [
  { title: 'Name', key: 'name' },
  {
    title: 'Type',
    key: 'type',
    render(row) {
      return h(NTag, {
        type: row.type === 'relay' ? 'info' : 'default',
        size: 'small'
      }, { default: () => row.type })
    }
  },
  { title: 'Listen', key: 'listen_addr' },
  {
    title: 'Listen Client',
    key: 'listen_client',
    render(row) {
      const client = clients.value.find(c => c.id === row.listen_client)
      return client?.name || row.listen_client
    }
  },
  {
    title: 'Relay Chain',
    key: 'relay_chain',
    render(row) {
      if (!row.relay_chain?.length) return '-'
      return h(NSpace, { size: 'small' }, {
        default: () => row.relay_chain.map(r =>
          h(NTag, { size: 'small', type: r.startsWith('@') ? 'success' : 'default' }, { default: () => r })
        )
      })
    }
  },
  { title: 'Exit', key: 'exit_addr' },
  {
    title: 'Enabled',
    key: 'enabled',
    render(row) {
      return h(NSwitch, {
        value: row.enabled,
        onUpdateValue: (val: boolean) => handleToggle(row.id, val)
      })
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
            onClick: () => openEdit(row)
          }, { default: () => 'Edit' }),
          h(NPopconfirm, {
            onPositiveClick: () => handleDelete(row.id)
          }, {
            trigger: () => h(NButton, { size: 'small', type: 'error', secondary: true }, { default: () => 'Delete' }),
            default: () => 'Delete this rule?'
          })
        ]
      })
    }
  }
]

async function loadData() {
  loading.value = true
  try {
    const [rulesData, clientsData, groupsData] = await Promise.all([
      getForwardRuleList(),
      getClientList(),
      getProxyGroupList()
    ])
    rules.value = rulesData
    clients.value = clientsData
    groups.value = groupsData
  } catch (error: unknown) {
    message.error((error as Error).message)
  } finally {
    loading.value = false
  }
}

function openCreate() {
  editingId.value = null
  formData.value = {
    name: '',
    type: 'direct',
    listen_addr: '0.0.0.0:',
    listen_client: '',
    relay_chain: [],
    exit_addr: '',
    enabled: true
  }
  showModal.value = true
}

function openEdit(rule: ForwardRule) {
  editingId.value = rule.id
  formData.value = {
    name: rule.name,
    type: rule.type,
    listen_addr: rule.listen_addr,
    listen_client: rule.listen_client,
    relay_chain: rule.relay_chain || [],
    exit_addr: rule.exit_addr,
    enabled: rule.enabled
  }
  showModal.value = true
}

async function handleSubmit() {
  if (!formData.value.name || !formData.value.listen_client) {
    message.warning('Please fill in required fields')
    return
  }

  modalLoading.value = true
  try {
    if (editingId.value) {
      await updateForwardRule(editingId.value, formData.value)
      message.success('Rule updated')
    } else {
      await createForwardRule(formData.value)
      message.success('Rule created')
    }
    showModal.value = false
    await loadData()
  } catch (error: unknown) {
    message.error((error as Error).message)
  } finally {
    modalLoading.value = false
  }
}

async function handleDelete(id: string) {
  try {
    await deleteForwardRule(id)
    message.success('Rule deleted')
    await loadData()
  } catch (error: unknown) {
    message.error((error as Error).message)
  }
}

async function handleToggle(id: string, enabled: boolean) {
  try {
    await toggleForwardRule(id, enabled)
    await loadData()
  } catch (error: unknown) {
    message.error((error as Error).message)
  }
}

onMounted(loadData)
</script>

<template>
  <NSpace vertical size="large">
    <NSpace justify="space-between" align="center">
      <NText tag="h2" style="margin: 0">Forward Rules</NText>
      <NSpace>
        <NButton @click="loadData" :loading="loading">
          <template #icon><NIcon><RefreshOutline /></NIcon></template>
          Refresh
        </NButton>
        <NButton type="primary" @click="openCreate">
          <template #icon><NIcon><AddOutline /></NIcon></template>
          Add Rule
        </NButton>
      </NSpace>
    </NSpace>

    <NDataTable
      :columns="columns"
      :data="rules"
      :loading="loading"
      :row-key="(row: ForwardRule) => row.id"
    />

    <!-- Create/Edit Modal -->
    <NModal
      v-model:show="showModal"
      :title="editingId ? 'Edit Rule' : 'Create Rule'"
      preset="card"
      style="width: 600px"
    >
      <NForm label-placement="left" label-width="120">
        <NFormItem label="Name" required>
          <NInput v-model:value="formData.name" placeholder="Rule name" />
        </NFormItem>
        <NFormItem label="Type" required>
          <NSelect v-model:value="formData.type" :options="typeOptions" />
        </NFormItem>
        <NFormItem label="Listen Address" required>
          <NInput v-model:value="formData.listen_addr" placeholder="0.0.0.0:8080" />
        </NFormItem>
        <NFormItem label="Listen Client" required>
          <NSelect
            v-model:value="formData.listen_client"
            :options="clientOptions"
            placeholder="Select client"
            filterable
          />
        </NFormItem>
        <NFormItem v-if="formData.type === 'relay'" label="Relay Chain">
          <NSelect
            v-model:value="formData.relay_chain"
            :options="relayOptions"
            placeholder="Select relay nodes"
            multiple
            filterable
          />
        </NFormItem>
        <NFormItem label="Exit Address" required>
          <NInput v-model:value="formData.exit_addr" placeholder="target-host:port" />
        </NFormItem>
        <NFormItem label="Enabled">
          <NSwitch v-model:value="formData.enabled" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showModal = false">Cancel</NButton>
          <NButton type="primary" :loading="modalLoading" @click="handleSubmit">
            {{ editingId ? 'Update' : 'Create' }}
          </NButton>
        </NSpace>
      </template>
    </NModal>
  </NSpace>
</template>
