import axios from 'axios'
import type { RpcRequest, RpcResponse } from '../types'

const API_BASE = import.meta.env.VITE_API_BASE || ''

let requestId = 0

export async function rpcCall<T>(method: string, params: Record<string, unknown> = {}): Promise<T> {
  const request: RpcRequest = {
    jsonrpc: '2.0',
    method,
    params,
    id: ++requestId
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
    throw new Error(response.data.error.message)
  }

  return response.data.result as T
}

// Auth
export const adminLogin = (username: string, password: string) =>
  rpcCall<{ token: string }>('adminLogin', { username, password })

// Clients
export const getClientList = () =>
  rpcCall<import('../types').Client[]>('getClientList', {})

export const getClient = (id: string) =>
  rpcCall<import('../types').Client>('getClient', { id })

export const createClient = (name: string) =>
  rpcCall<import('../types').Client>('createClient', { name })

export const updateClient = (id: string, name: string) =>
  rpcCall<import('../types').Client>('updateClient', { id, name })

export const deleteClient = (id: string) =>
  rpcCall<void>('deleteClient', { id })

export const regenerateClientToken = (id: string) =>
  rpcCall<import('../types').Client>('regenerateClientToken', { id })

export const getClientInstallCommand = (id: string) =>
  rpcCall<{ command: string }>('getClientInstallCommand', { id })

// Forward Rules
export const getForwardRuleList = () =>
  rpcCall<import('../types').ForwardRule[]>('getForwardRuleList', {})

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
export const getProxyGroupList = () =>
  rpcCall<import('../types').ProxyGroup[]>('getProxyGroupList', {})

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
