---
layout: home

hero:
  name: SourceAEC
  text: AI 可编辑 3D 建模平台
  tagline: AI 写 Python 脚本，脚本生成 IFC 和 DXF 模型。脚本就是模型本身：轻量、可版本化、可对比，设计师和 AI 改的都是同一份脚本。
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
    title: AI 生成 IFC
    details: AI 读取 aiifc skill 后直接编写 IfcOpenShell 构建脚本。脚本在沙箱里执行，产出 IFC 模型，浏览器里用 xeokit 或 web-ifc 引擎三维查看。
  - icon: 📐
    title: AI 生成 CAD
    details: aiplan 先把外部资料整理成设计任务书，aidxf 再把任务书画成逐层 DXF 图纸。前端用 Canvas 二维查看，选中实体能定位到脚本。
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
    title: 设计师与 AI 双角色
    details: 人和 AI 用同一套编辑 API，来源用 provenance 字段区分。平台内置 Eino chat agent，网页右侧的 AI 对话栏就是它驱动的。
---

## 从这里开始

| 你的角色 | 入口 |
| --- | --- |
| **使用者**：想先跑起来看看 | [创建第一个项目](/guide/first-project)：新建项目、对话建模、审查、提 Issue、改脚本、对比版本 |
| **部署者**：要把整套跑起来 | [环境要求与本地启动](/guide/quickstart) → [生产部署与运维](/guide/deploy) |
| **开发者**：要读代码或二次开发 | [总体架构](/development/architecture)：组件职责、仓库结构、数据流、版本模型 |
| **集成者**：要让 AI 或自研前端接进来 | [AI 接入](/reference/ai) 与 [REST API](/reference/rest-api) |

想先了解这个平台是什么、为什么这样设计，读[项目介绍](/guide/intro)；遇到问题查[故障排查](/guide/troubleshooting)。

## 界面截图

![三维查看器](/screenshots/viewer.png)

| 模型库 | 属性编辑 | 版本对比 | AI 对话 |
|---|---|---|---|
| ![模型库](/screenshots/library.png) | ![属性编辑](/screenshots/properties.png) | ![版本对比](/screenshots/diff.png) | ![AI 对话](/screenshots/chat.png) |

## 链接

- [GitHub 仓库](https://github.com/0702hjj/SourceAEC)：源码、Issue 与 PR
- [LICENSE](https://github.com/0702hjj/SourceAEC/blob/main/LICENSE) 与 [NOTICE](https://github.com/0702hjj/SourceAEC/blob/main/NOTICE)
