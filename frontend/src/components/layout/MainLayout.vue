<script setup lang="ts">
import { h } from 'vue'
import { RouterLink, useRoute } from 'vue-router'
import {
  NLayout,
  NLayoutSider,
  NLayoutContent,
  NMenu,
  NIcon,
  NButton,
  NText
} from 'naive-ui'
import type { MenuOption } from 'naive-ui'
import {
  HomeOutline,
  DesktopOutline,
  GitNetworkOutline,
  LayersOutline,
  LogOutOutline
} from '@vicons/ionicons5'
import { useAuthStore } from '../../stores/auth'

const route = useRoute()
const authStore = useAuthStore()

function renderIcon(icon: typeof HomeOutline) {
  return () => h(NIcon, null, { default: () => h(icon) })
}

const menuOptions: MenuOption[] = [
  {
    label: () => h(RouterLink, { to: '/' }, { default: () => 'Dashboard' }),
    key: 'dashboard',
    icon: renderIcon(HomeOutline)
  },
  {
    label: () => h(RouterLink, { to: '/clients' }, { default: () => 'Clients' }),
    key: 'clients',
    icon: renderIcon(DesktopOutline)
  },
  {
    label: () => h(RouterLink, { to: '/rules' }, { default: () => 'Forward Rules' }),
    key: 'rules',
    icon: renderIcon(GitNetworkOutline)
  },
  {
    label: () => h(RouterLink, { to: '/groups' }, { default: () => 'Proxy Groups' }),
    key: 'groups',
    icon: renderIcon(LayersOutline)
  }
]

function getActiveKey() {
  const path = route.path
  if (path === '/') return 'dashboard'
  if (path.startsWith('/clients')) return 'clients'
  if (path.startsWith('/rules')) return 'rules'
  if (path.startsWith('/groups')) return 'groups'
  return 'dashboard'
}
</script>

<template>
  <NLayout has-sider style="height: 100vh">
    <NLayoutSider
      bordered
      :width="220"
      :native-scrollbar="false"
      style="background: #fff"
    >
      <div class="logo">
        <NText strong style="font-size: 18px">MessageBoy</NText>
      </div>
      <NMenu
        :options="menuOptions"
        :value="getActiveKey()"
        :indent="24"
      />
      <div class="sider-footer">
        <NButton quaternary block @click="authStore.logout">
          <template #icon>
            <NIcon><LogOutOutline /></NIcon>
          </template>
          Logout
        </NButton>
      </div>
    </NLayoutSider>
    <NLayoutContent style="padding: 24px; background: #f5f7f9">
      <RouterView />
    </NLayoutContent>
  </NLayout>
</template>

<style scoped>
.logo {
  height: 64px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-bottom: 1px solid #efeff5;
}

.sider-footer {
  position: absolute;
  bottom: 16px;
  left: 16px;
  right: 16px;
}
</style>
