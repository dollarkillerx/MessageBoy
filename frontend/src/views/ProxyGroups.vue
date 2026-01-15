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
  clients.value.map(c => ({ label: `${c.name} (${c.status === 'online' ? '在线' : '离线'})`, value: c.id }))
)

const lbMethodOptions: SelectOption[] = [
  { label: '轮询', value: 'round_robin' },
  { label: '随机', value: 'random' },
  { label: '最少连接', value: 'least_conn' },
  { label: 'IP 哈希', value: 'ip_hash' }
]

const statusMap: Record<string, string> = {
  healthy: '健康',
  unhealthy: '异常',
  unknown: '未知'
}

const nodeColumns: DataTableColumns<ProxyGroupNode> = [
  {
    title: '客户端',
    key: 'client_id',
    render(row) {
      const client = clients.value.find(c => c.id === row.client_id)
      return client?.name || row.client_id
    }
  },
  {
    title: '状态',
    key: 'status',
    render(row) {
      const typeMap: Record<string, 'success' | 'error' | 'default'> = {
        healthy: 'success',
        unhealthy: 'error',
        unknown: 'default'
      }
      return h(NTag, { type: typeMap[row.status] || 'default', size: 'small' }, { default: () => statusMap[row.status] || row.status })
    }
  },
  { title: '优先级', key: 'priority' },
  { title: '权重', key: 'weight' },
  { title: '活跃连接', key: 'active_conns' },
  {
    title: '操作',
    key: 'actions',
    render(row) {
      return h(NPopconfirm, {
        onPositiveClick: () => handleRemoveNode(row.id)
      }, {
        trigger: () => h(NButton, { size: 'small', type: 'error', quaternary: true }, {
          icon: () => h(NIcon, null, { default: () => h(TrashOutline) })
        }),
        default: () => '确定移除此节点？'
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
    groups.value = Array.isArray(groupsData) ? groupsData : []
    clients.value = Array.isArray(clientsData) ? clientsData : []
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
    message.warning('请输入代理组名称')
    return
  }

  groupModalLoading.value = true
  try {
    if (editingGroupId.value) {
      await updateProxyGroup(editingGroupId.value, groupForm.value)
      message.success('代理组已更新')
    } else {
      await createProxyGroup(groupForm.value)
      message.success('代理组已创建')
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
    message.success('代理组已删除')
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
    message.warning('请选择客户端')
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
    message.success('节点已添加')
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
    message.success('节点已移除')
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
      <NText tag="h2" style="margin: 0">代理组</NText>
      <NSpace>
        <NButton @click="loadData" :loading="loading">
          <template #icon><NIcon><RefreshOutline /></NIcon></template>
          刷新
        </NButton>
        <NButton type="primary" @click="openCreateGroup">
          <template #icon><NIcon><AddOutline /></NIcon></template>
          添加代理组
        </NButton>
      </NSpace>
    </NSpace>

    <NSpace vertical>
      <NCard v-for="group in groups" :key="group.id" :title="group.name">
        <template #header-extra>
          <NSpace>
            <NTag size="small">{{ getLBMethodLabel(group.load_balance_method) }}</NTag>
            <NTag size="small" :type="group.health_check_enabled ? 'success' : 'default'">
              健康检查: {{ group.health_check_enabled ? '开启' : '关闭' }}
            </NTag>
            <NButton size="small" @click="openEditGroup(group)">编辑</NButton>
            <NButton size="small" @click="openAddNode(group.id)">添加节点</NButton>
            <NPopconfirm @positive-click="handleDeleteGroup(group.id)">
              <template #trigger>
                <NButton size="small" type="error" secondary>删除</NButton>
              </template>
              确定删除此代理组？
            </NPopconfirm>
          </NSpace>
        </template>

        <NText depth="3" style="margin-bottom: 12px; display: block">
          引用标识: <NTag size="small">@{{ group.name }}</NTag>
        </NText>

        <NDataTable
          v-if="group.nodes && group.nodes.length > 0"
          :columns="nodeColumns"
          :data="group.nodes"
          :row-key="(row: ProxyGroupNode) => row.id"
          size="small"
        />
        <NText v-else depth="3">该代理组暂无节点</NText>
      </NCard>

      <NText v-if="groups.length === 0 && !loading" depth="3">
        暂无代理组，请创建一个开始使用。
      </NText>
    </NSpace>

    <!-- 代理组弹窗 -->
    <NModal
      v-model:show="showGroupModal"
      :title="editingGroupId ? '编辑代理组' : '创建代理组'"
      preset="card"
      style="width: 500px"
    >
      <NForm label-placement="left" label-width="150">
        <NFormItem label="名称" required>
          <NInput v-model:value="groupForm.name" placeholder="代理组名称" />
        </NFormItem>
        <NFormItem label="负载均衡">
          <NSelect v-model:value="groupForm.load_balance_method" :options="lbMethodOptions" />
        </NFormItem>
        <NFormItem label="健康检查">
          <NSwitch v-model:value="groupForm.health_check_enabled" />
        </NFormItem>
        <NFormItem v-if="groupForm.health_check_enabled" label="检查间隔 (秒)">
          <NInputNumber v-model:value="groupForm.health_check_interval" :min="5" :max="300" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showGroupModal = false">取消</NButton>
          <NButton type="primary" :loading="groupModalLoading" @click="handleGroupSubmit">
            {{ editingGroupId ? '更新' : '创建' }}
          </NButton>
        </NSpace>
      </template>
    </NModal>

    <!-- 节点弹窗 -->
    <NModal
      v-model:show="showNodeModal"
      title="添加节点"
      preset="card"
      style="width: 400px"
    >
      <NForm label-placement="left" label-width="80">
        <NFormItem label="客户端" required>
          <NSelect
            v-model:value="nodeForm.client_id"
            :options="clientOptions"
            placeholder="选择客户端"
            filterable
          />
        </NFormItem>
        <NFormItem label="优先级">
          <NInputNumber v-model:value="nodeForm.priority" :min="1" :max="1000" />
        </NFormItem>
        <NFormItem label="权重">
          <NInputNumber v-model:value="nodeForm.weight" :min="1" :max="1000" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showNodeModal = false">取消</NButton>
          <NButton type="primary" :loading="nodeModalLoading" @click="handleAddNode">
            添加
          </NButton>
        </NSpace>
      </template>
    </NModal>
  </NSpace>
</template>
