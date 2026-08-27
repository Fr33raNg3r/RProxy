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
  },
  // 无 type（默认态）按钮原本背景全透明、只有一圈细边框，跟 Tag 状态标签视觉权重太接近。
  // 给默认按钮一层可见底色 + 更清楚的边框/圆角，一眼就能看出是可点击控件。
  Button: {
    borderRadiusTiny: '5px',
    borderRadiusSmall: '5px',
    borderRadiusMedium: '6px',
    borderRadiusLarge: '6px',
    fontWeight: '600',
    color: 'rgba(255, 255, 255, 0.07)',
    colorHover: 'rgba(77, 208, 225, 0.16)',
    colorPressed: 'rgba(77, 208, 225, 0.26)',
    border: '1px solid rgba(255, 255, 255, 0.24)',
    borderHover: '1px solid rgba(77, 208, 225, 0.75)',
    borderPressed: '1px solid rgba(77, 208, 225, 0.9)',
    textColor: 'rgba(255, 255, 255, 0.92)',
    textColorHover: '#4dd0e1',
    textColorPressed: '#4dd0e1'
  },
  // Tag 反过来去掉描边，纯色块圆角药丸，跟按钮的"实边框+方角"形成对比。
  Tag: {
    border: '1px solid transparent',
    borderPrimary: '1px solid transparent',
    borderInfo: '1px solid transparent',
    borderSuccess: '1px solid transparent',
    borderWarning: '1px solid transparent',
    borderError: '1px solid transparent'
  }
}
</script>
