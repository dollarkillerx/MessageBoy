<script setup lang="ts">
import { ref, onMounted, onUnmounted } from 'vue'
import {
  NGrid,
  NGi,
  NCard,
  NStatistic,
  NSpin,
  NSpace,
  NText,
  NDataTable,
  NTag
} from 'naive-ui'
import type { DataTableColumns } from 'naive-ui'
import {
  getClientList,
  getForwardRuleList,
  getProxyGroupList,
  getTotalTraffic,
  getTrafficSummary
} from '../api/rpc'
import type { Client, ForwardRule, TotalTraffic, TrafficSummary } from '../types'

const loading = ref(true)
const stats = ref({
  totalClients: 0,
  onlineClients: 0,
  totalRules: 0,
  enabledRules: 0,
  totalGroups: 0
})

const trafficStats = ref<TotalTraffic>({
  bytes_in: 0,
  bytes_out: 0,
  total_bytes: 0,
  bytes_in_str: '0 B',
  bytes_out_str: '0 B',
  total_bytes_str: '0 B',
  active_connections: 0
})

const trafficSummary = ref<TrafficSummary[]>([])

const trafficColumns: DataTableColumns<TrafficSummary> = [
  {
    title: '规则',
    key: 'rule_name',
    ellipsis: { tooltip: true }
  },
  {
    title: '客户端',
    key: 'client_name',
    ellipsis: { tooltip: true }
  },
  {
    title: '入站流量',
    key: 'bytes_in_str',
    width: 100
  },
  {
    title: '出站流量',
    key: 'bytes_out_str',
    width: 100
  },
  {
    title: '总流量',
    key: 'total_bytes_str',
    width: 100
  },
  {
    title: '活跃连接',
    key: 'active_conns',
    width: 100,
    render(row) {
      return row.active_conns > 0
        ? h(NTag, { type: 'success', size: 'small' }, () => row.active_conns)
        : h(NTag, { type: 'default', size: 'small' }, () => '0')
    }
  }
]

import { h } from 'vue'

let refreshTimer: ReturnType<typeof setInterval> | null = null

async function loadData() {
  try {
    const [clients, rules, groups, traffic, summary] = await Promise.all([
      getClientList(),
      getForwardRuleList(),
      getProxyGroupList(),
      getTotalTraffic(),
      getTrafficSummary()
    ])

    const clientList = Array.isArray(clients) ? clients : []
    const ruleList = Array.isArray(rules) ? rules : []
    const groupList = Array.isArray(groups) ? groups : []

    stats.value = {
      totalClients: clientList.length,
      onlineClients: clientList.filter((c: Client) => c.status === 'online').length,
      totalRules: ruleList.length,
      enabledRules: ruleList.filter((r: ForwardRule) => r.enabled).length,
      totalGroups: groupList.length
    }

    trafficStats.value = traffic || trafficStats.value
    trafficSummary.value = Array.isArray(summary) ? summary : []
  } catch (error) {
    console.error('Failed to load dashboard data:', error)
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  loadData()
  // Auto refresh every 10 seconds
  refreshTimer = setInterval(loadData, 10000)
})

onUnmounted(() => {
  if (refreshTimer) {
    clearInterval(refreshTimer)
  }
})
</script>

<template>
  <div>
    <NSpace vertical size="large">
      <NText tag="h2" style="margin: 0">仪表盘</NText>

      <NSpin :show="loading">
        <!-- 基础统计 -->
        <NGrid :x-gap="16" :y-gap="16" cols="1 s:2 m:3 l:5" responsive="screen">
          <NGi>
            <NCard>
              <NStatistic label="客户端总数" :value="stats.totalClients" />
            </NCard>
          </NGi>
          <NGi>
            <NCard>
              <NStatistic label="在线客户端" :value="stats.onlineClients">
                <template #suffix>
                  <NText depth="3"> / {{ stats.totalClients }}</NText>
                </template>
              </NStatistic>
            </NCard>
          </NGi>
          <NGi>
            <NCard>
              <NStatistic label="转发规则" :value="stats.totalRules" />
            </NCard>
          </NGi>
          <NGi>
            <NCard>
              <NStatistic label="已启用规则" :value="stats.enabledRules">
                <template #suffix>
                  <NText depth="3"> / {{ stats.totalRules }}</NText>
                </template>
              </NStatistic>
            </NCard>
          </NGi>
          <NGi>
            <NCard>
              <NStatistic label="代理组" :value="stats.totalGroups" />
            </NCard>
          </NGi>
        </NGrid>

        <!-- 流量统计 -->
        <NText tag="h3" style="margin: 24px 0 16px">流量统计</NText>
        <NGrid :x-gap="16" :y-gap="16" cols="1 s:2 m:3 l:4" responsive="screen">
          <NGi>
            <NCard>
              <NStatistic label="总流量" :value="trafficStats.total_bytes_str" />
            </NCard>
          </NGi>
          <NGi>
            <NCard>
              <NStatistic label="入站流量" :value="trafficStats.bytes_in_str" />
            </NCard>
          </NGi>
          <NGi>
            <NCard>
              <NStatistic label="出站流量" :value="trafficStats.bytes_out_str" />
            </NCard>
          </NGi>
          <NGi>
            <NCard>
              <NStatistic label="活跃连接" :value="trafficStats.active_connections" />
            </NCard>
          </NGi>
        </NGrid>

        <!-- 规则流量明细 -->
        <NText tag="h3" style="margin: 24px 0 16px">规则流量明细</NText>
        <NCard>
          <NDataTable
            :columns="trafficColumns"
            :data="trafficSummary"
            :bordered="false"
            size="small"
            :pagination="trafficSummary.length > 10 ? { pageSize: 10 } : false"
          />
        </NCard>
      </NSpin>
    </NSpace>
  </div>
</template>
