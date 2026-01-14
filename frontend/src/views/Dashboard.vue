<script setup lang="ts">
import { ref, onMounted } from 'vue'
import {
  NGrid,
  NGi,
  NCard,
  NStatistic,
  NSpin,
  NSpace,
  NText
} from 'naive-ui'
import { getClientList, getForwardRuleList, getProxyGroupList } from '../api/rpc'
import type { Client, ForwardRule } from '../types'

const loading = ref(true)
const stats = ref({
  totalClients: 0,
  onlineClients: 0,
  totalRules: 0,
  enabledRules: 0,
  totalGroups: 0
})

onMounted(async () => {
  try {
    const [clients, rules, groups] = await Promise.all([
      getClientList(),
      getForwardRuleList(),
      getProxyGroupList()
    ])

    stats.value = {
      totalClients: clients.length,
      onlineClients: clients.filter((c: Client) => c.status === 'online').length,
      totalRules: rules.length,
      enabledRules: rules.filter((r: ForwardRule) => r.enabled).length,
      totalGroups: groups.length
    }
  } catch (error) {
    console.error('Failed to load dashboard data:', error)
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div>
    <NSpace vertical size="large">
      <NText tag="h2" style="margin: 0">Dashboard</NText>

      <NSpin :show="loading">
        <NGrid :x-gap="16" :y-gap="16" cols="1 s:2 m:3 l:4" responsive="screen">
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
      </NSpin>
    </NSpace>
  </div>
</template>
