// Client types
export interface Client {
  id: string
  name: string
  token: string
  status: 'online' | 'offline'
  last_seen_at: string
  created_at: string
  updated_at: string
}

// Forward Rule types
export type ForwardType = 'direct' | 'relay'

export interface ForwardRule {
  id: string
  name: string
  type: ForwardType
  listen_addr: string
  listen_client: string
  relay_chain: string[]
  exit_addr: string
  enabled: boolean
  created_at: string
  updated_at: string
}

// Proxy Group types
export type LoadBalanceMethod = 'round_robin' | 'random' | 'least_conn' | 'ip_hash'
export type NodeStatus = 'healthy' | 'unhealthy' | 'unknown'

export interface ProxyGroup {
  id: string
  name: string
  load_balance_method: LoadBalanceMethod
  health_check_enabled: boolean
  health_check_interval: number
  nodes?: ProxyGroupNode[]
  created_at: string
  updated_at: string
}

export interface ProxyGroupNode {
  id: string
  group_id: string
  client_id: string
  priority: number
  weight: number
  status: NodeStatus
  active_conns: number
  total_conns: number
  last_check_at: string
  created_at: string
}

// RPC types
export interface RpcRequest {
  jsonrpc: '2.0'
  method: string
  params: Record<string, unknown>
  id: number
}

export interface RpcResponse<T = unknown> {
  jsonrpc: '2.0'
  result?: T
  error?: {
    code: number
    message: string
  }
  id: number
}
