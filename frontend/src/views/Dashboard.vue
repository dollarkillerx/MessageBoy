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
  total_connections: 0,
  active_connections: 0
})

const trafficSummary = ref<TrafficSummary[]>([])

const trafficColumns: DataTableColumns<TrafficSummary> = [
  {
    title: 'Rule',
    key: 'rule_name',
    ellipsis: { tooltip: true }
  },
  {
    title: 'Client',
    key: 'client_name',
    ellipsis: { tooltip: true }
  },
  {
    title: 'Bytes In',
    key: 'bytes_in_str',
    width: 100
  },
  {
    title: 'Bytes Out',
    key: 'bytes_out_str',
    width: 100
  },
  {
    title: 'Total',
    key: 'total_bytes_str',
    width: 100
  },
  {
    title: 'Active',
    key: 'active_conns',
    width: 80,
    render(row) {
      return row.active_conns > 0
        ? h(NTag, { type: 'success', size: 'small' }, () => row.active_conns)
        : h(NTag, { type: 'default', size: 'small' }, () => '0')
    }
  },
  {
    title: 'Total Conns',
    key: 'total_connections',
    width: 100
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

    stats.value = {
      totalClients: clients.length,
      onlineClients: clients.filter((c: Client) => c.status === 'online').length,
      totalRules: rules.length,
      enabledRules: rules.filter((r: ForwardRule) => r.enabled).length,
      totalGroups: groups.length
    }

    trafficStats.value = traffic
    trafficSummary.value = summary || []
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
      <NText tag="h2" style="margin: 0">Dashboard</NText>

      <NSpin :show="loading">
        <!-- Basic Stats -->
        <NGrid :x-gap="16" :y-gap="16" cols="1 s:2 m:3 l:5" responsive="screen">
          <NGi>
            <NCard>
              <NStatistic label="Total Clients" :value="stats.totalClients" />
            </NCard>
          </NGi>
          <NGi>
            <NCard>
              <NStatistic label="Online Clients" :value="stats.onlineClients">
                <template #suffix>
                  <NText depth="3"> / {{ stats.totalClients }}</NText>
                </template>
              </NStatistic>
            </NCard>
          </NGi>
          <NGi>
            <NCard>
              <NStatistic label="Forward Rules" :value="stats.totalRules" />
            </NCard>
          </NGi>
          <NGi>
            <NCard>
              <NStatistic label="Enabled Rules" :value="stats.enabledRules">
                <template #suffix>
                  <NText depth="3"> / {{ stats.totalRules }}</NText>
                </template>
              </NStatistic>
            </NCard>
          </NGi>
          <NGi>
            <NCard>
              <NStatistic label="Proxy Groups" :value="stats.totalGroups" />
            </NCard>
          </NGi>
        </NGrid>

        <!-- Traffic Stats -->
        <NText tag="h3" style="margin: 24px 0 16px">Traffic Statistics</NText>
        <NGrid :x-gap="16" :y-gap="16" cols="1 s:2 m:3 l:4" responsive="screen">
          <NGi>
            <NCard>
              <NStatistic label="Total Traffic" :value="trafficStats.total_bytes_str" />
            </NCard>
          </NGi>
          <NGi>
            <NCard>
              <NStatistic label="Bytes In" :value="trafficStats.bytes_in_str" />
            </NCard>
          </NGi>
          <NGi>
            <NCard>
              <NStatistic label="Bytes Out" :value="trafficStats.bytes_out_str" />
            </NCard>
          </NGi>
          <NGi>
            <NCard>
              <NStatistic label="Active Connections" :value="trafficStats.active_connections">
                <template #suffix>
                  <NText depth="3"> / {{ trafficStats.total_connections }} total</NText>
                </template>
              </NStatistic>
            </NCard>
          </NGi>
        </NGrid>

        <!-- Traffic Summary Table -->
        <NText tag="h3" style="margin: 24px 0 16px">Traffic by Rule</NText>
        <NCard>
          <NDataTable
            :columns="trafficColumns"
            :data="trafficSummary"
            :bordered="false"
            size="small"
            :pagination="{ pageSize: 10 }"
          />
        </NCard>
      </NSpin>
    </NSpace>
  </div>
</template>
