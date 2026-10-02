---
layout: home

hero:
  name: SourceAEC
  text: 面向 Agent 的 IFC 操作接口
  tagline: 用 REST API、Agent Skill 和可审查的 Python 构建脚本创建、编辑、检查与版本化 IFC；按需接入规划和 DXF 工作流。
  actions:
    - theme: brand
      text: 开始使用
      link: /guide/quickstart
    - theme: alt
      text: 项目介绍
      link: /guide/intro
    - theme: alt
      text: GitHub
      link: https://github.com/0702hjj/SourceAEC

features:
  - icon: 🧱
    title: IFC 操作接口
    details: Agent 可通过 REST API 或 IFC Authoring Skill 创建、读取和修改 IfcOpenShell 构建脚本；人也使用同一套编辑与版本契约。
  - icon: 📐
    title: 可选 DXF 工作流
    details: Plan Preparation Skill 可整理设计输入，DXF Authoring Skill 可生成逐层图纸，并将结果作为 IFC 建模的上游资料。
  - icon: ✏️
    title: 脚本即事实源
    details: 所有修改都落在 Python 构建脚本上。选中构件即可定位到生成它的那行代码，改参数、改脚本，试运行满意后保存成不可变的大版本。
  - icon: 🔍
    title: 语义版本对比
    details: 两个版本之间按构件做属性级对比。新增、删除、修改分别着色，能展开看每个字段的旧值和新值，没有几何噪声。
  - icon: 🛡️
    title: 沙箱执行
    details: 构建脚本在 bubblewrap 沙箱里运行，只读挂载、断网。没有沙箱就拒绝执行，不做静默降级。脚本依赖按 PEP 723 声明。
  - icon: 🤖
    title: Agent 与人共用契约
    details: Agent 和人使用同一套编辑 API，来源用 provenance 字段区分。既可接入外部 Agent，也可使用内置 Eino chat agent。
---

## 从这里开始

| 你的角色 | 入口 |
| --- | --- |
| **使用者**：想先跑起来看看 | [创建第一个项目](/guide/first-project)：新建项目、对话建模、审查、提 Issue、改脚本、对比版本 |
| **部署者**：要把整套跑起来 | [环境要求与本地启动](/guide/quickstart) → [生产部署与运维](/guide/deploy) |
| **开发者**：要读代码或二次开发 | [总体架构](/development/architecture)：组件职责、仓库结构、数据流、版本模型 |
| **集成者**：要让 Agent 或自研前端接进来 | [Agent 接入](/reference/ai) 与 [REST API](/reference/rest-api) |

想先了解这个平台是什么、为什么这样设计，读[项目介绍](/guide/intro)；遇到问题查[故障排查](/guide/troubleshooting)。

## 界面截图

![三维查看器](/screenshots/viewer.png)

| 模型库 | 属性编辑 | 版本对比 | AI 对话 |
|---|---|---|---|
| ![模型库](/screenshots/library.png) | ![属性编辑](/screenshots/properties.png) | ![版本对比](/screenshots/diff.png) | ![AI 对话](/screenshots/chat.png) |

## 链接

- [GitHub 仓库](https://github.com/0702hjj/SourceAEC)：源码、Issue 与 PR
- [LICENSE](https://github.com/0702hjj/SourceAEC/blob/main/LICENSE) 与 [NOTICE](https://github.com/0702hjj/SourceAEC/blob/main/NOTICE)
