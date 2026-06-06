<template>
  <n-config-provider :theme="darkTheme" :theme-overrides="themeOverrides" :locale="zhCN" :date-locale="dateZhCN">
    <n-loading-bar-provider>
      <n-message-provider>
        <n-dialog-provider>
          <n-notification-provider>
            <router-view />
          </n-notification-provider>
        </n-dialog-provider>
      </n-message-provider>
    </n-loading-bar-provider>
  </n-config-provider>
</template>

<script setup>
import { darkTheme, zhCN, dateZhCN } from 'naive-ui'

const themeOverrides = {
  common: {
    primaryColor: '#4dd0e1',
    primaryColorHover: '#5dd9e8',
    primaryColorPressed: '#3fb8c8',
    primaryColorSuppl: '#4dd0e1',
    borderRadius: '8px',
    bodyColor: '#0f172a',
    cardColor: '#1e293b',
    modalColor: '#1e293b',
    popoverColor: '#1e293b'
  },
  Card: {
    borderRadius: '10px',
    paddingMedium: '20px 22px',
    titleFontSizeMedium: '15px'
  },
  Layout: {
    siderColor: '#111827',
    color: '#0f172a'
  },
  Menu: {
    itemColorActive: 'rgba(77, 208, 225, 0.15)',
    itemColorActiveHover: 'rgba(77, 208, 225, 0.2)',
    itemTextColorActive: '#4dd0e1',
    itemTextColorActiveHover: '#5dd9e8',
    itemIconColorActive: '#4dd0e1'
  }
}
</script>
