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
  NInputNumber,
  NPopconfirm,
  useMessage,
  NIcon,
  NCard
} from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import { AddOutline, RefreshOutline, TrashOutline } from '@vicons/ionicons5'
import {
  getProxyGroupList,
  getClientList,
  createProxyGroup,
  updateProxyGroup,
  deleteProxyGroup,
  addProxyGroupNode,
  removeProxyGroupNode
} from '../api/rpc'
import type { ProxyGroup, ProxyGroupNode, Client, LoadBalanceMethod } from '../types'

const message = useMessage()
const loading = ref(true)
const groups = ref<ProxyGroup[]>([])
const clients = ref<Client[]>([])

// Group Modal
const showGroupModal = ref(false)
const groupModalLoading = ref(false)
const editingGroupId = ref<string | null>(null)
const groupForm = ref({
  name: '',
  load_balance_method: 'round_robin' as LoadBalanceMethod,
  health_check_enabled: true,
  health_check_interval: 30
})

// Node Modal
const showNodeModal = ref(false)
const nodeModalLoading = ref(false)
const selectedGroupId = ref('')
const nodeForm = ref({
  client_id: '',
  priority: 100,
  weight: 100
})

const clientOptions = computed<SelectOption[]>(() =>
  clients.value.map(c => ({ label: `${c.name} (${c.status})`, value: c.id }))
)

const lbMethodOptions: SelectOption[] = [
  { label: 'Round Robin', value: 'round_robin' },
  { label: 'Random', value: 'random' },
  { label: 'Least Connections', value: 'least_conn' },
  { label: 'IP Hash', value: 'ip_hash' }
]

const nodeColumns: DataTableColumns<ProxyGroupNode> = [
  {
    title: 'Client',
    key: 'client_id',
    render(row) {
      const client = clients.value.find(c => c.id === row.client_id)
      return client?.name || row.client_id
    }
  },
  {
    title: 'Status',
    key: 'status',
    render(row) {
      const typeMap: Record<string, 'success' | 'error' | 'default'> = {
        healthy: 'success',
        unhealthy: 'error',
        unknown: 'default'
      }
      return h(NTag, { type: typeMap[row.status] || 'default', size: 'small' }, { default: () => row.status })
    }
  },
  { title: 'Priority', key: 'priority' },
  { title: 'Weight', key: 'weight' },
  { title: 'Active Conns', key: 'active_conns' },
  { title: 'Total Conns', key: 'total_conns' },
  {
    title: 'Actions',
    key: 'actions',
    render(row) {
      return h(NPopconfirm, {
        onPositiveClick: () => handleRemoveNode(row.id)
      }, {
        trigger: () => h(NButton, { size: 'small', type: 'error', quaternary: true }, {
          icon: () => h(NIcon, null, { default: () => h(TrashOutline) })
        }),
        default: () => 'Remove this node?'
      })
    }
  }
]

async function loadData() {
  loading.value = true
  try {
    const [groupsData, clientsData] = await Promise.all([
      getProxyGroupList(),
      getClientList()
    ])
    // Load nodes for each group
    groups.value = groupsData
    clients.value = clientsData
  } catch (error: unknown) {
    message.error((error as Error).message)
  } finally {
    loading.value = false
  }
}

function openCreateGroup() {
  editingGroupId.value = null
  groupForm.value = {
    name: '',
    load_balance_method: 'round_robin',
    health_check_enabled: true,
    health_check_interval: 30
  }
  showGroupModal.value = true
}

function openEditGroup(group: ProxyGroup) {
  editingGroupId.value = group.id
  groupForm.value = {
    name: group.name,
    load_balance_method: group.load_balance_method,
    health_check_enabled: group.health_check_enabled,
    health_check_interval: group.health_check_interval
  }
  showGroupModal.value = true
}

async function handleGroupSubmit() {
  if (!groupForm.value.name) {
    message.warning('Please enter group name')
    return
  }

  groupModalLoading.value = true
  try {
    if (editingGroupId.value) {
      await updateProxyGroup(editingGroupId.value, groupForm.value)
      message.success('Group updated')
    } else {
      await createProxyGroup(groupForm.value)
      message.success('Group created')
    }
    showGroupModal.value = false
    await loadData()
  } catch (error: unknown) {
    message.error((error as Error).message)
  } finally {
    groupModalLoading.value = false
  }
}

async function handleDeleteGroup(id: string) {
  try {
    await deleteProxyGroup(id)
    message.success('Group deleted')
    await loadData()
  } catch (error: unknown) {
    message.error((error as Error).message)
  }
}

function openAddNode(groupId: string) {
  selectedGroupId.value = groupId
  nodeForm.value = {
    client_id: '',
    priority: 100,
    weight: 100
  }
  showNodeModal.value = true
}

async function handleAddNode() {
  if (!nodeForm.value.client_id) {
    message.warning('Please select a client')
    return
  }

  nodeModalLoading.value = true
  try {
    await addProxyGroupNode(
      selectedGroupId.value,
      nodeForm.value.client_id,
      nodeForm.value.priority,
      nodeForm.value.weight
    )
    message.success('Node added')
    showNodeModal.value = false
    await loadData()
  } catch (error: unknown) {
    message.error((error as Error).message)
  } finally {
    nodeModalLoading.value = false
  }
}

async function handleRemoveNode(nodeId: string) {
  try {
    await removeProxyGroupNode(nodeId)
    message.success('Node removed')
    await loadData()
  } catch (error: unknown) {
    message.error((error as Error).message)
  }
}

function getLBMethodLabel(method: string) {
  const option = lbMethodOptions.find(o => o.value === method)
  return option?.label || method
}

onMounted(loadData)
</script>

<template>
  <NSpace vertical size="large">
    <NSpace justify="space-between" align="center">
      <NText tag="h2" style="margin: 0">Proxy Groups</NText>
      <NSpace>
        <NButton @click="loadData" :loading="loading">
          <template #icon><NIcon><RefreshOutline /></NIcon></template>
          Refresh
        </NButton>
        <NButton type="primary" @click="openCreateGroup">
          <template #icon><NIcon><AddOutline /></NIcon></template>
          Add Group
        </NButton>
      </NSpace>
    </NSpace>

    <NSpace vertical>
      <NCard v-for="group in groups" :key="group.id" :title="group.name">
        <template #header-extra>
          <NSpace>
            <NTag size="small">{{ getLBMethodLabel(group.load_balance_method) }}</NTag>
            <NTag size="small" :type="group.health_check_enabled ? 'success' : 'default'">
              Health Check: {{ group.health_check_enabled ? 'ON' : 'OFF' }}
            </NTag>
            <NButton size="small" @click="openEditGroup(group)">Edit</NButton>
            <NButton size="small" @click="openAddNode(group.id)">Add Node</NButton>
            <NPopconfirm @positive-click="handleDeleteGroup(group.id)">
              <template #trigger>
                <NButton size="small" type="error" secondary>Delete</NButton>
              </template>
              Delete this group?
            </NPopconfirm>
          </NSpace>
        </template>

        <NText depth="3" style="margin-bottom: 12px; display: block">
          Reference: <NTag size="small">@{{ group.name }}</NTag>
        </NText>

        <NDataTable
          v-if="group.nodes && group.nodes.length > 0"
          :columns="nodeColumns"
          :data="group.nodes"
          :row-key="(row: ProxyGroupNode) => row.id"
          size="small"
        />
        <NText v-else depth="3">No nodes in this group</NText>
      </NCard>

      <NText v-if="groups.length === 0 && !loading" depth="3">
        No proxy groups. Create one to get started.
      </NText>
    </NSpace>

    <!-- Group Modal -->
    <NModal
      v-model:show="showGroupModal"
      :title="editingGroupId ? 'Edit Group' : 'Create Group'"
      preset="card"
      style="width: 500px"
    >
      <NForm label-placement="left" label-width="150">
        <NFormItem label="Name" required>
          <NInput v-model:value="groupForm.name" placeholder="Group name" />
        </NFormItem>
        <NFormItem label="Load Balance">
          <NSelect v-model:value="groupForm.load_balance_method" :options="lbMethodOptions" />
        </NFormItem>
        <NFormItem label="Health Check">
          <NSwitch v-model:value="groupForm.health_check_enabled" />
        </NFormItem>
        <NFormItem v-if="groupForm.health_check_enabled" label="Check Interval (s)">
          <NInputNumber v-model:value="groupForm.health_check_interval" :min="5" :max="300" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showGroupModal = false">Cancel</NButton>
          <NButton type="primary" :loading="groupModalLoading" @click="handleGroupSubmit">
            {{ editingGroupId ? 'Update' : 'Create' }}
          </NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- Node Modal -->
    <NModal
      v-model:show="showNodeModal"
      title="Add Node"
      preset="card"
      style="width: 400px"
    >
      <NForm label-placement="left" label-width="80">
        <NFormItem label="Client" required>
          <NSelect
            v-model:value="nodeForm.client_id"
            :options="clientOptions"
            placeholder="Select client"
            filterable
          />
        </NFormItem>
        <NFormItem label="Priority">
          <NInputNumber v-model:value="nodeForm.priority" :min="1" :max="1000" />
        </NFormItem>
        <NFormItem label="Weight">
          <NInputNumber v-model:value="nodeForm.weight" :min="1" :max="1000" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showNodeModal = false">Cancel</NButton>
          <NButton type="primary" :loading="nodeModalLoading" @click="handleAddNode">
            Add
          </NButton>
        </NSpace>
      </template>
    </NModal>
  </NSpace>
</template>
