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
  NTooltip,
  useMessage,
  NIcon
} from 'naive-ui'
import type { DataTableColumns, SelectOption } from 'naive-ui'
import { AddOutline, RefreshOutline } from '@vicons/ionicons5'
import {
  getForwardRuleList,
  getForwardRule,
  getClientList,
  getProxyGroupList,
  createForwardRule,
  updateForwardRule,
  deleteForwardRule,
  toggleForwardRule,
  getTrafficSummary
} from '../api/rpc'
import type { ForwardRule, Client, ProxyGroup, TrafficSummary } from '../types'

const message = useMessage()
const loading = ref(true)
const rules = ref<ForwardRule[]>([])
const clients = ref<Client[]>([])
const groups = ref<ProxyGroup[]>([])
const trafficMap = ref<Map<string, TrafficSummary>>(new Map())

// Modal state
const showModal = ref(false)
const modalLoading = ref(false)
const editingId = ref<string | null>(null)
const formData = ref({
  name: '',
  type: 'direct' as 'direct' | 'relay',
  listen_addr: '',
  listen_client: '',
  target_addr: '',
  relay_chain: [] as string[],
  exit_addr: '',
  enabled: true
})

const clientOptions = computed<SelectOption[]>(() =>
  clients.value.map(c => ({ label: `${c.name} (${c.status === 'online' ? '在线' : '离线'})`, value: c.id }))
)

const relayOptions = computed<SelectOption[]>(() => [
  ...groups.value.map(g => ({ label: `@${g.name} (代理组)`, value: `@${g.name}` })),
  ...clients.value.map(c => ({ label: c.name, value: c.id }))
])

const typeOptions: SelectOption[] = [
  { label: '直连', value: 'direct' },
  { label: '中继', value: 'relay' }
]

const columns: DataTableColumns<ForwardRule> = [
  { title: '名称', key: 'name' },
  {
    title: '类型',
    key: 'type',
    render(row) {
      return h(NTag, {
        type: row.type === 'relay' ? 'info' : 'default',
        size: 'small'
      }, { default: () => row.type === 'relay' ? '中继' : '直连' })
    }
  },
  { title: '监听地址', key: 'listen_addr' },
  {
    title: '监听客户端',
    key: 'listen_client',
    render(row) {
      const client = clients.value.find(c => c.id === row.listen_client)
      return client?.name || row.listen_client
    }
  },
  {
    title: '中继链',
    key: 'relay_chain',
    render(row) {
      const chain = row.relay_chain
      if (!chain?.length) return '-'
      return h(NSpace, { size: 'small' }, {
        default: () => chain.map(r =>
          h(NTag, { size: 'small', type: r.startsWith('@') ? 'success' : 'default' }, { default: () => r })
        )
      })
    }
  },
  {
    title: '目标地址',
    key: 'target_addr',
    render(row) {
      return row.type === 'direct' ? row.target_addr : row.exit_addr
    }
  },
  {
    title: '流量',
    key: 'traffic',
    width: 120,
    render(row) {
      const traffic = trafficMap.value.get(row.id)
      if (!traffic) return '-'
      return h(NTooltip, null, {
        trigger: () => h(NText, null, { default: () => traffic.total_bytes_str }),
        default: () => `上行: ${traffic.bytes_out_str} / 下行: ${traffic.bytes_in_str}`
      })
    }
  },
  {
    title: '状态',
    key: 'status',
    width: 100,
    render(row) {
      const statusConfig: Record<string, { type: 'success' | 'error' | 'warning' | 'default', label: string }> = {
        running: { type: 'success', label: '运行中' },
        error: { type: 'error', label: '错误' },
        pending: { type: 'warning', label: '等待中' },
        stopped: { type: 'default', label: '已停止' }
      }
      const defaultStatus = { type: 'warning' as const, label: '等待中' }
      const currentStatus = statusConfig[row.status || 'pending'] || defaultStatus

      if (row.status === 'error' && row.last_error) {
        return h(NTooltip, null, {
          trigger: () => h(NTag, { type: currentStatus.type, size: 'small' }, { default: () => currentStatus.label }),
          default: () => row.last_error
        })
      }
      return h(NTag, { type: currentStatus.type, size: 'small' }, { default: () => currentStatus.label })
    }
  },
  {
    title: '启用',
    key: 'enabled',
    render(row) {
      return h(NSwitch, {
        value: row.enabled,
        onUpdateValue: (val: boolean) => handleToggle(row.id, val)
      })
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
            onClick: () => openEdit(row)
          }, { default: () => '编辑' }),
          h(NPopconfirm, {
            onPositiveClick: () => handleDelete(row.id)
          }, {
            trigger: () => h(NButton, { size: 'small', type: 'error', secondary: true }, { default: () => '删除' }),
            default: () => '确定删除此规则？'
          })
        ]
      })
    }
  }
]

async function loadData() {
  loading.value = true
  try {
    const [rulesData, clientsData, groupsData, trafficData] = await Promise.all([
      getForwardRuleList(),
      getClientList(),
      getProxyGroupList(),
      getTrafficSummary()
    ])
    rules.value = Array.isArray(rulesData) ? rulesData : []
    clients.value = Array.isArray(clientsData) ? clientsData : []
    groups.value = Array.isArray(groupsData) ? groupsData : []

    // 构建流量映射 (按 rule_id)
    const map = new Map<string, TrafficSummary>()
    if (Array.isArray(trafficData)) {
      for (const t of trafficData) {
        map.set(t.rule_id, t)
      }
    }
    trafficMap.value = map
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
    target_addr: '',
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
    target_addr: rule.target_addr || '',
    relay_chain: rule.relay_chain || [],
    exit_addr: rule.exit_addr || '',
    enabled: rule.enabled
  }
  showModal.value = true
}

async function handleSubmit() {
  if (!formData.value.name || !formData.value.listen_client || !formData.value.listen_addr) {
    message.warning('请填写必填字段')
    return
  }

  if (formData.value.type === 'direct' && !formData.value.target_addr) {
    message.warning('直连模式需要填写目标地址')
    return
  }

  if (formData.value.type === 'relay') {
    if (!formData.value.relay_chain?.length) {
      message.warning('中继模式需要选择中继链')
      return
    }
    if (!formData.value.exit_addr) {
      message.warning('中继模式需要填写出口地址')
      return
    }
  }

  modalLoading.value = true
  try {
    if (editingId.value) {
      await updateForwardRule(editingId.value, formData.value)
      message.success('规则已更新')
    } else {
      const result = await createForwardRule(formData.value) as { id: string }
      message.success('规则已创建，正在启动...')

      // 等待客户端启动规则，然后检查状态
      await new Promise(resolve => setTimeout(resolve, 2000))
      const rule = await getForwardRule(result.id)
      if (rule.status === 'error') {
        message.error(`规则启动失败: ${rule.last_error || '未知错误'}`)
      } else if (rule.status === 'running') {
        message.success('规则启动成功')
      }
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
    message.success('规则已删除')
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
      <NText tag="h2" style="margin: 0">转发规则</NText>
      <NSpace>
        <NButton @click="loadData" :loading="loading">
          <template #icon><NIcon><RefreshOutline /></NIcon></template>
          刷新
        </NButton>
        <NButton type="primary" @click="openCreate">
          <template #icon><NIcon><AddOutline /></NIcon></template>
          添加规则
        </NButton>
      </NSpace>
    </NSpace>

    <NDataTable
      :columns="columns"
      :data="rules"
      :loading="loading"
      :row-key="(row: ForwardRule) => row.id"
    />

    <!-- 创建/编辑弹窗 -->
    <NModal
      v-model:show="showModal"
      :title="editingId ? '编辑规则' : '创建规则'"
      preset="card"
      style="width: 600px"
    >
      <NForm label-placement="left" label-width="120">
        <NFormItem label="名称" required>
          <NInput v-model:value="formData.name" placeholder="规则名称" />
        </NFormItem>
        <NFormItem label="类型" required>
          <NSelect v-model:value="formData.type" :options="typeOptions" />
        </NFormItem>
        <NFormItem label="监听地址" required>
          <NInput v-model:value="formData.listen_addr" placeholder="0.0.0.0:8080" />
        </NFormItem>
        <NFormItem label="监听客户端" required>
          <NSelect
            v-model:value="formData.listen_client"
            :options="clientOptions"
            placeholder="选择客户端"
            filterable
          />
        </NFormItem>
        <NFormItem v-if="formData.type === 'direct'" label="目标地址" required>
          <NInput v-model:value="formData.target_addr" placeholder="目标主机:端口" />
        </NFormItem>
        <NFormItem v-if="formData.type === 'relay'" label="中继链" required>
          <NSelect
            v-model:value="formData.relay_chain"
            :options="relayOptions"
            placeholder="选择中继节点"
            multiple
            filterable
          />
        </NFormItem>
        <NFormItem v-if="formData.type === 'relay'" label="出口地址" required>
          <NInput v-model:value="formData.exit_addr" placeholder="目标主机:端口" />
        </NFormItem>
        <NFormItem label="启用">
          <NSwitch v-model:value="formData.enabled" />
        </NFormItem>
      </NForm>
      <template #footer>
        <NSpace justify="end">
          <NButton @click="showModal = false">取消</NButton>
          <NButton type="primary" :loading="modalLoading" @click="handleSubmit">
            {{ editingId ? '更新' : '创建' }}
          </NButton>
        </NSpace>
      </template>
    </NModal>
  </NSpace>
</template>
