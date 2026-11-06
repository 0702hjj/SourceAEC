// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
import { defineConfig } from 'vitepress'
import { withMermaid } from 'vitepress-plugin-mermaid'
import { navigation } from './navigation.mts'

export default withMermaid(defineConfig({
  base: '/SourceAEC/',
  title: 'SourceAEC',
  description: 'Self-hosted, agent-friendly IFC operations',
  cleanUrls: true,
  lastUpdated: true,
  locales: {
    root: {
      label: '简体中文', lang: 'zh-CN',
      description: '开源、自托管、面向 Agent 的 IFC 操作接口',
      themeConfig: {
        ...navigation(false),
        outline: { label: '本页目录', level: [2, 3] },
        lastUpdated: { text: '最后更新于' },
        docFooter: { prev: '上一篇', next: '下一篇' },
        returnToTopLabel: '返回顶部', sidebarMenuLabel: '菜单',
        darkModeSwitchLabel: '外观', langMenuLabel: '切换语言',
      },
    },
    en: {
      label: 'English', lang: 'en', link: '/en/',
      description: 'Self-hosted, agent-friendly IFC operations',
      themeConfig: { ...navigation(true), outline: { level: [2, 3] } },
    },
  },
  head: [
    ['meta', { name: 'theme-color', content: '#3fb950' }],
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/SourceAEC/favicon.svg' }],
  ],
  themeConfig: {
    search: {
      provider: 'local',
      options: { locales: { root: { translations: {
        button: { buttonText: '搜索文档', buttonAriaLabel: '搜索文档' },
        modal: {
          noResultsText: '未找到相关结果', resetButtonTitle: '清除查询条件',
          footer: { selectText: '选择', navigateText: '切换', closeText: '关闭' },
        },
      } } } },
    },
    socialLinks: [{ icon: 'github', link: 'https://github.com/0702hjj/SourceAEC' }],
    footer: {
      message: 'Apache-2.0 · See NOTICE for frontend dependency licenses',
      copyright: 'Copyright © 2026 0702hjj',
    },
  },
}))
