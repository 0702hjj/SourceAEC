// SPDX-License-Identifier: Apache-2.0
import type { DefaultTheme } from 'vitepress'

const sections = [
  { path: 'guide', zh: '入门', en: 'Start here', pages: [
    ['intro', '项目介绍', 'Introduction'],
    ['first-project', '第一个项目', 'First project'],
    ['interface', '界面与审查', 'Interface and review'],
    ['editing', '编辑与版本', 'Editing and versions'],
  ] },
  { path: 'architecture', zh: '架构', en: 'Architecture', pages: [
    ['overview', '架构总览', 'Overview'],
    ['data-flow', '数据流与事务', 'Data flow and transactions'],
    ['gateway', 'Go 网关', 'Go gateway'],
    ['web', 'Web 客户端', 'Web client'],
    ['editing', '编辑领域', 'Editing domain'],
    ['execution', '并发与恢复', 'Concurrency and recovery'],
    ['storage', '持久化与版本', 'Persistence and versions'],
    ['previews', '转换、预览与读取', 'Conversion, previews and reads'],
    ['sandbox', '几何沙箱', 'Geometry sandbox'],
    ['notebooks', '可选 Notebook 缓存', 'Optional notebook cache'],
    ['formats', 'IFC / DXF 适配', 'IFC / DXF adapters'],
    ['boundaries', '外部引擎边界', 'External engine boundaries'],
  ] },
  { path: 'operations', zh: '运维', en: 'Operations', pages: [
    ['quickstart', '环境与启动', 'Requirements and startup'],
    ['deployment', '生产部署', 'Deployment'],
    ['configuration', '配置', 'Configuration'],
    ['troubleshooting', '故障排查', 'Troubleshooting'],
    ['testing', '验证与测试', 'Verification and tests'],
    ['performance', '性能与容量', 'Performance and capacity'],
    ['documentation', '文档构建与托管', 'Documentation hosting'],
  ] },
  { path: 'integrations', zh: '接入', en: 'Integrations', pages: [
    ['overview', '网关接入契约', 'Gateway contract'],
    ['agents', 'Agent 与 MCP', 'Agents and MCP'],
    ['skills', '声明性 Skills', 'Declarative skills'],
  ] },
  { path: 'reference', zh: 'API 参考', en: 'API reference', pages: [
    ['rest-api', 'REST 约定', 'REST conventions'],
    ['edit-api', '脚本编辑', 'Script editing'],
    ['workflows', '持久工作流', 'Durable workflows'],
    ['api-model', '模型与审查', 'Models and review'],
    ['api-chat', '项目、对话与 SSE', 'Projects, chat and SSE'],
    ['edit-api-reference', '机器可读 Schema', 'Machine-readable schemas'],
  ] },
]

export function navigation(english: boolean): Pick<DefaultTheme.Config, 'nav' | 'sidebar'> {
  const prefix = english ? '/en' : ''
  const groups = sections.map(section => ({
    text: english ? section.en : section.zh,
    items: section.pages.map(([slug, zh, en]) => ({
      text: english ? en : zh, link: `${prefix}/${section.path}/${slug}`,
    })),
  }))
  const nav = sections.map((section, index) => ({
    text: groups[index].text,
    link: groups[index].items[0].link,
    activeMatch: `^${prefix}/${section.path}/`,
  }))
  const sidebar = Object.fromEntries(sections.map((section, index) => [
    `${prefix}/${section.path}/`,
    [groups[index], { text: english ? 'Explore' : '继续阅读', items: nav.filter((_, i) => i !== index) }],
  ]))
  return { nav, sidebar }
}
