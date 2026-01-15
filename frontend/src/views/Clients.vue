<script setup lang="ts">
import { ref, onMounted, h, onUnmounted } from 'vue'
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
  NInputNumber,
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
  getClientInstallCommand,
  getClientBandwidth,
  type ClientBandwidth
} from '../api/rpc'
import type { Client } from '../types'

const message = useMessage()
const loading = ref(true)
const clients = ref<Client[]>([])
const bandwidthMap = ref<Map<string, ClientBandwidth>>(new Map())
let bandwidthTimer: ReturnType<typeof setInterval> | null = null

// Modal state
const showCreateModal = ref(false)
const showInstallModal = ref(false)
const showSSHModal = ref(false)
const createLoading = ref(false)
const installCommand = ref('')
const currentSSHClient = ref<Client | null>(null)

// 创建表单
const createForm = ref({
  name: '',
  relay_ip: '',
  ssh_host: '',
  ssh_port: 22,
  ssh_user: 'root',
  ssh_password: ''
})

// SSH 终端相关
const terminalRef = ref<HTMLDivElement | null>(null)
let terminal: any = null
let ws: WebSocket | null = null

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
    title: 'SSH 地址',
    key: 'ssh_host',
    render(row) {
      if (!row.ssh_host) return '-'
      return `${row.ssh_host}:${row.ssh_port || 22}`
    }
  },
  {
    title: '连接 IP',
    key: 'last_ip',
    render(row) {
      return row.last_ip || '-'
    }
  },
  {
    title: '中继地址',
    key: 'relay_ip',
    render(row) {
      if (!row.relay_ip) return h(NText, { depth: 3 }, { default: () => '使用连接IP' })
      return row.relay_ip
    }
  },
  {
    title: '主机名',
    key: 'hostname',
    render(row) {
      return row.hostname || '-'
    }
  },
  {
    title: '最后在线',
    key: 'last_seen',
    render(row) {
      return row.last_seen ? new Date(row.last_seen).toLocaleString() : '-'
    }
  },
  {
    title: '上行带宽',
    key: 'bandwidth_out',
    width: 100,
    render(row) {
      const bw = bandwidthMap.value.get(row.id)
      return bw?.bandwidth_out_str || '-'
    }
  },
  {
    title: '下行带宽',
    key: 'bandwidth_in',
    width: 100,
    render(row) {
      const bw = bandwidthMap.value.get(row.id)
      return bw?.bandwidth_in_str || '-'
    }
  },
  {
    title: '操作',
    key: 'actions',
    width: 280,
    render(row) {
      return h(NSpace, null, {
        default: () => [
          h(NButton, {
            size: 'small',
            type: 'primary',
            disabled: !row.ssh_host,
            onClick: () => openSSH(row)
          }, {
            default: () => 'SSH',
            icon: () => h(NIcon, null, { default: () => h(TerminalOutline) })
          }),
          h(NButton, {
            size: 'small',
            onClick: () => showInstall(row.id)
          }, {
            default: () => '安装'
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

async function refreshBandwidth() {
  try {
    const data = await getClientBandwidth()
    const map = new Map<string, ClientBandwidth>()
    if (Array.isArray(data)) {
      for (const bw of data) {
        map.set(bw.client_id, bw)
      }
    }
    bandwidthMap.value = map
  } catch {
    // 静默处理带宽刷新错误
  }
}

function startBandwidthTimer() {
  if (bandwidthTimer) return
  refreshBandwidth()
  bandwidthTimer = setInterval(refreshBandwidth, 1000)
}

function stopBandwidthTimer() {
  if (bandwidthTimer) {
    clearInterval(bandwidthTimer)
    bandwidthTimer = null
  }
}

function openCreateModal() {
  createForm.value = {
    name: '',
    relay_ip: '',
    ssh_host: '',
    ssh_port: 22,
    ssh_user: 'root',
    ssh_password: ''
  }
  showCreateModal.value = true
}

async function handleCreate() {
  if (!createForm.value.name) {
    message.warning('请输入客户端名称')
    return
  }

  createLoading.value = true
  try {
    await createClient(createForm.value)
    message.success('客户端已创建')
    showCreateModal.value = false
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

function copyInstallCommand() {
  navigator.clipboard.writeText(installCommand.value)
  message.success('命令已复制')
}

// WebSSH 相关
async function openSSH(client: Client) {
  currentSSHClient.value = client
  showSSHModal.value = true

  // 等待 DOM 更新后初始化终端
  setTimeout(() => {
    initTerminal(client.id)
  }, 100)
}

async function initTerminal(clientId: string) {
  if (!terminalRef.value) return

  // 动态导入 xterm
  const { Terminal } = await import('xterm')
  const { FitAddon } = await import('xterm-addon-fit')

  // 清理旧的终端
  if (terminal) {
    terminal.dispose()
  }
  if (ws) {
    ws.close()
  }

  // 清空容器
  terminalRef.value.innerHTML = ''

  // 创建终端
  terminal = new Terminal({
    cursorBlink: true,
    fontSize: 14,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    theme: {
      background: '#1e1e1e',
      foreground: '#d4d4d4',
      cursor: '#d4d4d4'
    }
  })

  const fitAddon = new FitAddon()
  terminal.loadAddon(fitAddon)
  terminal.open(terminalRef.value)
  fitAddon.fit()

  // 连接 WebSocket
  const token = localStorage.getItem('token')
  const apiBase = import.meta.env.VITE_API_BASE || ''
  let wsUrl: string
  if (apiBase) {
    // 使用环境变量配置的地址
    const wsBase = apiBase.replace(/^http/, 'ws')
    wsUrl = `${wsBase}/api/ws/ssh/${clientId}?token=${token}`
  } else {
    // 回退到当前页面地址
    const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    wsUrl = `${protocol}//${window.location.host}/api/ws/ssh/${clientId}?token=${token}`
  }

  ws = new WebSocket(wsUrl)

  ws.onopen = () => {
    terminal.writeln('\x1b[32m连接成功，正在建立 SSH 会话...\x1b[0m')
    // 发送终端大小
    ws?.send(JSON.stringify({
      type: 'resize',
      cols: terminal.cols,
      rows: terminal.rows
    }))
  }

  ws.onmessage = (event) => {
    const data = JSON.parse(event.data)
    if (data.type === 'output') {
      terminal.write(data.data)
    } else if (data.type === 'error') {
      terminal.writeln(`\x1b[31m错误: ${data.data}\x1b[0m`)
    }
  }

  ws.onclose = () => {
    terminal.writeln('\x1b[33m\r\n连接已断开\x1b[0m')
  }

  ws.onerror = () => {
    terminal.writeln('\x1b[31m连接错误\x1b[0m')
  }

  // 处理终端输入
  terminal.onData((data: string) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({
        type: 'input',
        data: data
      }))
    }
  })

  // 处理终端大小变化
  terminal.onResize(({ cols, rows }: { cols: number; rows: number }) => {
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify({
        type: 'resize',
        cols,
        rows
      }))
    }
  })
}

function closeSSH() {
  if (ws) {
    ws.close()
    ws = null
  }
  if (terminal) {
    terminal.dispose()
    terminal = null
  }
  showSSHModal.value = false
  currentSSHClient.value = null
}

onMounted(() => {
  loadClients()
  startBandwidthTimer()
})

onUnmounted(() => {
  stopBandwidthTimer()
  if (ws) ws.close()
  if (terminal) terminal.dispose()
})
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
        <NButton type="primary" @click="openCreateModal">
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
      style="width: 500px"
    >
      <NForm label-placement="left" label-width="100">
        <NFormItem label="名称" required>
          <NInput v-model:value="createForm.name" placeholder="客户端名称" />
        </NFormItem>
        <NFormItem label="中继地址">
          <NInput v-model:value="createForm.relay_ip" placeholder="中继时使用的 IP 地址（可选，为空则使用连接 IP）" />
        </NFormItem>
        <NFormItem label="SSH 主机">
          <NInput v-model:value="createForm.ssh_host" placeholder="IP 或域名（可选）" />
        </NFormItem>
        <NFormItem label="SSH 端口">
          <NInputNumber v-model:value="createForm.ssh_port" :min="1" :max="65535" style="width: 100%" />
        </NFormItem>
        <NFormItem label="SSH 用户">
          <NInput v-model:value="createForm.ssh_user" placeholder="root" />
        </NFormItem>
        <NFormItem label="SSH 密码">
          <NInput v-model:value="createForm.ssh_password" type="password" placeholder="SSH 密码（可选）" show-password-on="click" />
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

    <!-- SSH 终端弹窗 -->
    <NModal
      v-model:show="showSSHModal"
      :title="`SSH - ${currentSSHClient?.name || ''}`"
      preset="card"
      style="width: 90vw; max-width: 1400px; height: 85vh"
      :on-after-leave="closeSSH"
    >
      <div ref="terminalRef" style="width: 100%; height: calc(85vh - 120px); background: #1e1e1e; border-radius: 4px;"></div>
    </NModal>
  </NSpace>
</template>

<style>
@import 'xterm/css/xterm.css';
</style>
