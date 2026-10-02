# 项目介绍

SourceAEC 是一个开源、自托管、面向 Agent 的 IFC 操作接口。它把 IFC 的创建、编辑、检查、试运行和版本管理组织成 Agent 与人都能调用的明确契约，并围绕一个核心构想构建：

> Agent 通过 REST API 或 Skill 操作 Python 构建脚本；脚本经 IfcOpenShell 执行并产出 IFC。Python 脚本是模型的唯一事实源，界面编辑最终也落到同一份脚本。规划与 DXF 制图是可选的上游工作流。

这样做的好处很直接。模型文件可以随时从脚本重建，存储很轻；版本控制就是脚本的版本控制；两个模型的差异就是两份脚本的差异，可以精确算出来。完整概念与操作见[编辑与版本](/guide/editing)。

## IFC 核心与可选扩展

**IFC 操作接口。** IFC Authoring Skill（技术标识 `skills/aiifc`）帮助 Agent 编写建模脚本；`services/ifc` 是服务端运行时，负责脚本沙箱执行、版本快照、语义对比和编辑 API。这是 SourceAEC 的核心能力。

**可选规划与 DXF 工作流。** Plan Preparation Skill（`skills/aiplan`）把外部资料整理成任务书，DXF Authoring Skill（`skills/aidxf`）把任务书转成逐层 DXF 图纸；`services/cad` 提供配套运行时。它们可以独立使用，也可以为 IFC 建模提供上游输入。

**内置 Agent 工作流。** 平台提供 Eino chat agent，按项目类型派发 IFC 或 CAD 子 Agent；外部 Agent 也可以绕过界面直接调用 REST API 和 Skill。

可复用性是设计原则：IFC 接口可以脱离前端和 PostgreSQL 独立部署，各个 Skill 与业务服务也可以按需组合。

## 面向谁

- 要**自托管 BIM 工具链**的团队：数据不出自己的机器，不依赖任何云服务。
- 做 **IFC 或 CAD 工具**的开发者：`services/ifc` 与 `services/cad` 可脱离前端单独部署和移植。
- 需要 **Agent-friendly IFC 编辑底座**的研究者：Agent 和人共用同一套编辑 API，来源用 provenance 字段区分。

当前端到端可用：创建项目 → Agent 构建模型 → 三维或二维审查 → 提 Issue → 定位脚本并修改 → 沙箱验证 → 保存大版本 → 对比版本差异。走一遍见[创建第一个项目](/guide/first-project)。

## 组件一览

| 组件 | 技术 | 职责 |
| --- | --- | --- |
| `web` | React 19，xeokit 与 web-ifc 双引擎，DXF 用 Fabric Canvas | 项目库、查看器、编辑面板、Issue、Diff、AI 对话栏 |
| `server` | Go 1.26 | 唯一对外入口（:8090）：REST API、转换队列、编辑编排、chat agent，并托管前端构建产物 |
| `converter` | Node CLI | 把 IFC 转成 XKT 几何和元数据，由 server 当子进程调用 |
| `services/ifc` | Python FastAPI + IfcOpenShell | IFC 脚本沙箱、版本、定位、语义对比（:8100） |
| `services/cad` | Python FastAPI + ezdxf | DXF 脚本沙箱、版本、对比、render.json 发布（:8200） |
| `services/sandbox` | 共享包 `aibim_sandbox` | bwrap 沙箱后端与 script-as-source 领域模块，被两个服务引用 |
| `skills/` | SKILL.md 加参考文档 | IFC、规划与 DXF 的 Agent Skill；当前技术标识为 `aiifc`、`aiplan`、`aidxf`，可独立分发 |

三个语言并存是生态现实：Go 适合网关，Python 绑定 IfcOpenShell 和 ezdxf，Node 绑定 xeokit 转换器。服务之间用 REST 和子进程解耦，任何组件都可以单独替换。详见[总体架构](/development/architecture)。

## 部署形态

宿主机直接运行，不用 Docker。生产环境只有 8090 一个端口：Go server 托管前端构建产物，浏览器只访问它。不配 PostgreSQL 也能跑，数据默认落文件；**鉴权默认关闭，生产环境必须设置 token**。步骤见[生产部署与运维](/guide/deploy)。

## 许可

SourceAEC 主体以 Apache-2.0 发布，见 [LICENSE](https://github.com/0702hjj/SourceAEC/blob/main/LICENSE)。`skills/aiplan` 与 `skills/aidxf` 目录默认采用 MIT；单个文件的 SPDX 标识优先于目录默认许可。完整清单见根目录 [NOTICE](https://github.com/0702hjj/SourceAEC/blob/main/NOTICE)。

需要特别注意 xeokit：`web/` 前端依赖 AGPL-3.0 的 `@xeokit/xeokit-sdk`。分发或通过网络提供包含它的前端构建产物，可能触发 AGPL 的源码提供和其他义务；请按具体版本许可证和部署方式进行审查，不能仅凭本页判断合规。闭源或商用场景应在发布前取得专业许可意见，或改用不含该依赖的 three.js + web-ifc 路径。converter 以子进程方式使用 xeokit-convert，其输出的 XKT 数据是否受相关许可证影响，也应按具体依赖版本和使用方式单独核查。
