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

// Traffic types
export interface TrafficSummary {
  rule_id: string
  rule_name: string
  client_id: string
  client_name: string
  bytes_in: number
  bytes_out: number
  total_bytes: number
  connections: number
  active_conns: number
  total_connections: number
  bytes_in_str: string
  bytes_out_str: string
  total_bytes_str: string
}

export interface TotalTraffic {
  bytes_in: number
  bytes_out: number
  total_bytes: number
  bytes_in_str: string
  bytes_out_str: string
  total_bytes_str: string
  total_connections: number
  active_connections: number
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
