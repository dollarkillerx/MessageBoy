import axios from 'axios'
import type { RpcRequest, RpcResponse } from '../types'
import router from '../router'

const API_BASE = import.meta.env.VITE_API_BASE || ''

// RPC 错误码
const ErrCodeAuthRequired = -32001

let requestId = 0

export async function rpcCall<T>(method: string, params: Record<string, unknown> = {}): Promise<T> {
  const request: RpcRequest = {
    jsonrpc: '2.0',
    method,
    params,
    id: String(++requestId)
  }

  const token = localStorage.getItem('token')
  const headers: Record<string, string> = {
    'Content-Type': 'application/json'
  }
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  const response = await axios.post<RpcResponse<T>>(`${API_BASE}/api/rpc`, request, { headers })

  if (response.data.error) {
    // Token 失效或未认证，跳转到登录页面
    if (response.data.error.code === ErrCodeAuthRequired) {
      localStorage.removeItem('token')
      router.push('/login')
    }
    throw new Error(response.data.error.message)
  }

  return response.data.result as T
}

// Auth
export const adminLogin = (username: string, password: string) =>
  rpcCall<{ token: string }>('adminLogin', { username, password })

// Clients
export const getClientList = async () => {
  const result = await rpcCall<{ clients: import('../types').Client[] }>('getClientList', {})
  return result.clients || []
}

export const getClient = (id: string) =>
  rpcCall<import('../types').Client>('getClient', { id })

export const createClient = (params: {
  name: string
  relay_ip?: string
  ssh_host?: string
  ssh_port?: number
  ssh_user?: string
  ssh_password?: string
}) =>
  rpcCall<import('../types').Client>('createClient', params)

export const updateClient = (id: string, params: {
  name?: string
  description?: string
  relay_ip?: string
  ssh_host?: string
  ssh_port?: number
  ssh_user?: string
  ssh_password?: string
}) =>
  rpcCall<{ success: boolean }>('updateClient', { id, ...params })

export const deleteClient = (id: string) =>
  rpcCall<void>('deleteClient', { id })

export const regenerateClientToken = (id: string) =>
  rpcCall<import('../types').Client>('regenerateClientToken', { id })

export const getClientInstallCommand = async (id: string) => {
  const result = await rpcCall<{ command: string }>('getClientInstallCommand', { id })
  return result
}

// Forward Rules
export const getForwardRuleList = async () => {
  const result = await rpcCall<{ rules: import('../types').ForwardRule[] }>('getForwardRuleList', {})
  return result.rules || []
}

export const getForwardRule = (id: string) =>
  rpcCall<import('../types').ForwardRule>('getForwardRule', { id })

export const createForwardRule = (rule: Partial<import('../types').ForwardRule>) =>
  rpcCall<import('../types').ForwardRule>('createForwardRule', rule as Record<string, unknown>)

export const updateForwardRule = (id: string, rule: Partial<import('../types').ForwardRule>) =>
  rpcCall<import('../types').ForwardRule>('updateForwardRule', { id, ...rule } as Record<string, unknown>)

export const deleteForwardRule = (id: string) =>
  rpcCall<void>('deleteForwardRule', { id })

export const toggleForwardRule = (id: string, enabled: boolean) =>
  rpcCall<import('../types').ForwardRule>('toggleForwardRule', { id, enabled })

// Proxy Groups
export const getProxyGroupList = async () => {
  const result = await rpcCall<{ groups: import('../types').ProxyGroup[] }>('getProxyGroupList', {})
  return result.groups || []
}

export const getProxyGroup = (id: string) =>
  rpcCall<import('../types').ProxyGroup>('getProxyGroup', { id })

export const createProxyGroup = (group: Partial<import('../types').ProxyGroup>) =>
  rpcCall<import('../types').ProxyGroup>('createProxyGroup', group as Record<string, unknown>)

export const updateProxyGroup = (id: string, group: Partial<import('../types').ProxyGroup>) =>
  rpcCall<import('../types').ProxyGroup>('updateProxyGroup', { id, ...group } as Record<string, unknown>)

export const deleteProxyGroup = (id: string) =>
  rpcCall<void>('deleteProxyGroup', { id })

export const addProxyGroupNode = (groupId: string, clientId: string, priority = 100, weight = 100) =>
  rpcCall<import('../types').ProxyGroupNode>('addProxyGroupNode', {
    group_id: groupId,
    client_id: clientId,
    priority,
    weight
  })

export const removeProxyGroupNode = (id: string) =>
  rpcCall<void>('removeProxyGroupNode', { id })

export const updateProxyGroupNode = (id: string, priority: number, weight: number) =>
  rpcCall<import('../types').ProxyGroupNode>('updateProxyGroupNode', { id, priority, weight })

// Traffic Stats
export const getTrafficSummary = () =>
  rpcCall<import('../types').TrafficSummary[]>('getTrafficSummary', {})

export const getTotalTraffic = () =>
  rpcCall<import('../types').TotalTraffic>('getTotalTraffic', {})

export const getTodayTraffic = () =>
  rpcCall<import('../types').TotalTraffic>('getTodayTraffic', {})

// Client Bandwidth
export interface ClientBandwidth {
  client_id: string
  bandwidth_in: number
  bandwidth_out: number
  bandwidth_in_str: string
  bandwidth_out_str: string
}

export const getClientBandwidth = () =>
  rpcCall<ClientBandwidth[]>('getClientBandwidth', {})
