// SPDX-License-Identifier: Apache-2.0
// Copyright (C) 2026 0702hjj
import { defineConfig } from 'vitepress'

export default defineConfig({
  base: '/SourceAEC/',
  lang: 'zh-CN',
  title: 'SourceAEC',
  description: '开源、自托管、面向 Agent 的 IFC 操作接口',
  cleanUrls: true,
  lastUpdated: true,

  head: [
    ['meta', { name: 'theme-color', content: '#3fb950' }],
    ['link', { rel: 'icon', type: 'image/svg+xml', href: '/SourceAEC/favicon.svg' }],
  ],

  themeConfig: {
    // 入门与部署共用 /guide/ 前缀（保住 /guide/quickstart 这个最热入口 URL），
    // 所以顶部导航靠 activeMatch 正则区分二者，否则两项会同时高亮。
    nav: [
      { text: '入门', link: '/guide/intro', activeMatch: '^/guide/(intro|first-project|interface|editing|configuration|troubleshooting)' },
      { text: '部署', link: '/guide/deploy', activeMatch: '^/guide/(quickstart|deploy)' },
      { text: '开发', link: '/development/architecture', activeMatch: '^/development/' },
      { text: '参考', link: '/reference/rest-api', activeMatch: '^/reference/' },
    ],

    // 侧边栏随栏目切换：当前栏目完整展开，底部挂一组到其他栏目的入口链接
    // （模式参考 deepseek-harness：本栏目页面全列，兄弟栏目只给一个跳转链接）。
    sidebar: {
      '/guide/': [
        {
          text: '入门',
          items: [
            { text: '项目介绍', link: '/guide/intro' },
            { text: '创建第一个项目', link: '/guide/first-project' },
            { text: '界面使用', link: '/guide/interface' },
            { text: '编辑与版本', link: '/guide/editing' },
            { text: '配置说明', link: '/guide/configuration' },
            { text: '故障排查', link: '/guide/troubleshooting' },
          ],
        },
        {
          text: '部署',
          items: [
            { text: '环境要求与本地启动', link: '/guide/quickstart' },
            { text: '生产部署与运维', link: '/guide/deploy' },
          ],
        },
        {
          items: [
            { text: '开发', link: '/development/architecture' },
            { text: '参考', link: '/reference/rest-api' },
          ],
        },
      ],
      '/development/': [
        {
          text: '开发',
          items: [
            { text: '总体架构', link: '/development/architecture' },
            { text: '引擎与集成边界', link: '/development/integration-boundaries' },
            { text: 'Web 前端', link: '/development/web' },
            { text: 'Go Server', link: '/development/server' },
            { text: 'IFC 编辑服务', link: '/development/edit-service' },
            { text: 'CAD 编辑服务', link: '/development/cad-service' },
            { text: '沙箱执行环境', link: '/development/sandbox' },
            { text: '存储与前端对接', link: '/development/integration' },
            { text: '测试与调试', link: '/development/testing' },
          ],
        },
        {
          items: [
            { text: '入门', link: '/guide/intro' },
            { text: '部署', link: '/guide/deploy' },
            { text: '参考', link: '/reference/rest-api' },
          ],
        },
      ],
      '/reference/': [
        {
          text: '参考',
          items: [
            { text: 'REST API', link: '/reference/rest-api' },
            { text: '模型与审查 API', link: '/reference/api-model' },
            { text: '对话 API', link: '/reference/api-chat' },
            { text: 'IFC 编辑 API', link: '/reference/edit-api' },
            { text: '编辑 API 参考（自动生成）', link: '/reference/edit-api-reference' },
            { text: 'Agent 接入', link: '/reference/ai' },
            { text: 'Agent Skills', link: '/reference/ai-skill' },
          ],
        },
        {
          items: [
            { text: '入门', link: '/guide/intro' },
            { text: '部署', link: '/guide/deploy' },
            { text: '开发', link: '/development/architecture' },
          ],
        },
      ],
    },

    search: {
      provider: 'local',
      options: {
        translations: {
          button: { buttonText: '搜索文档', buttonAriaLabel: '搜索文档' },
          modal: {
            noResultsText: '未找到相关结果',
            resetButtonTitle: '清除查询条件',
            footer: { selectText: '选择', navigateText: '切换', closeText: '关闭' },
          },
        },
      },
    },

    outline: { label: '本页目录', level: [2, 3] },
    lastUpdated: { text: '最后更新于' },
    docFooter: { prev: '上一篇', next: '下一篇' },
    returnToTopLabel: '返回顶部',
    sidebarMenuLabel: '菜单',
    darkModeSwitchLabel: '外观',
    lightModeSwitchTitle: '切换到浅色模式',
    darkModeSwitchTitle: '切换到深色模式',


    socialLinks: [{ icon: 'github', link: 'https://github.com/0702hjj/SourceAEC' }],

    footer: {
      message: 'Apache-2.0 · xeokit AGPL 注意事项见项目介绍',
      copyright: 'Copyright © 2026 0702hjj',
    },
  },
})
