# 项目介绍

SourceAEC 是一个自托管、开源的建筑 3D 建模平台，目标是让 AI 和设计师高效协作。它围绕一个核心构想构建：

> AI 读 skill，写 Python 构建脚本；脚本经 IfcOpenShell 或 ezdxf 执行，产出 IFC 模型或 DXF 图纸。Python 脚本是模型的唯一事实源，前端的一切修改组件最终改的都是脚本。

这样做的好处很直接。模型文件可以随时从脚本重建，存储很轻；版本控制就是脚本的版本控制；两个模型的差异就是两份脚本的差异，可以精确算出来。完整概念与操作见[编辑与版本](/guide/editing)。

## 两条对等管线，外加一个推荐项

**逻辑一：AI 生成 IFC，已交付。** `skills/aiifc` 是给 AI 的建模参考包，`services/ifc` 是服务端运行时，负责脚本沙箱执行、版本快照、语义对比和编辑 API。

**逻辑二：AI 生成 CAD，已交付。** `skills/aiplan` 把外部资料整理成任务书，`skills/aidxf` 把任务书画成逐层 DXF 图纸；`services/cad` 与 ifc 侧同构。另有 `skills/aiblueprint-mcp` 支持交互式微调。

**推荐项：Agent 工作流，已落地。** 平台内置 Eino chat agent，按项目类型派发 ifc 或 cad 子 agent。网页右侧的 AI 对话栏就是它驱动的，不需要外部 agent 服务。

可复用性是设计原则：两个 skill、两个业务服务都可以单独拿出来用，前端和 PostgreSQL 都是可选的。

## 面向谁

- 要**自托管 BIM 工具链**的团队：数据不出自己的机器，不依赖任何云服务。
- 做 **IFC 或 CAD 工具**的开发者：`services/ifc` 与 `services/cad` 可脱离前端单独部署和移植。
- 需要**「AI 可接入的编辑底座」**的研究者：人和 AI 共用同一套编辑 API，来源用 provenance 字段区分。

当前端到端可用：创建项目 → AI 生成模型 → 三维或二维审查 → 提 Issue → 定位脚本并修改 → 沙箱验证 → 保存大版本 → 对比版本差异。走一遍见[创建第一个项目](/guide/first-project)。

## 组件一览

| 组件 | 技术 | 职责 |
| --- | --- | --- |
| `web` | React 19，xeokit 与 web-ifc 双引擎，DXF 用 Fabric Canvas | 项目库、查看器、编辑面板、Issue、Diff、AI 对话栏 |
| `server` | Go 1.26 | 唯一对外入口（:8090）：REST API、转换队列、编辑编排、chat agent，并托管前端构建产物 |
| `converter` | Node CLI | 把 IFC 转成 XKT 几何和元数据，由 server 当子进程调用 |
| `services/ifc` | Python FastAPI + IfcOpenShell | IFC 脚本沙箱、版本、定位、语义对比（:8100） |
| `services/cad` | Python FastAPI + ezdxf | DXF 脚本沙箱、版本、对比、render.json 发布（:8200） |
| `services/sandbox` | 共享包 `aibim_sandbox` | bwrap 沙箱后端与 script-as-source 领域模块，被两个服务引用 |
| `skills/` | SKILL.md 加参考文档 | aiifc、aiplan、aidxf 等 skill，AI 侧入口，可脱离平台单独分发 |

三个语言并存是生态现实：Go 适合网关，Python 绑定 IfcOpenShell 和 ezdxf，Node 绑定 xeokit 转换器。服务之间用 REST 和子进程解耦，任何组件都可以单独替换。详见[总体架构](/development/architecture)。

## 部署形态

宿主机直接运行，不用 Docker。生产环境只有 8090 一个端口：Go server 托管前端构建产物，浏览器只访问它。不配 PostgreSQL 也能跑，数据默认落文件；**鉴权默认关闭，生产环境必须设置 token**。步骤见[生产部署与运维](/guide/deploy)。

## 许可

SourceAEC 以 Apache-2.0 发布，见 [LICENSE](https://github.com/0702hjj/SourceAEC/blob/main/LICENSE)。例外目录保留原许可：`skills/aiplan`、`skills/aidxf` 为 MIT。完整清单见根目录 [NOTICE](https://github.com/0702hjj/SourceAEC/blob/main/NOTICE)。

需要特别注意 xeokit：`web/` 前端依赖 AGPL-3.0 的 `@xeokit/xeokit-sdk`，分发包含它的前端构建产物时，整个产物受 AGPL 约束，网络使用即触发。闭源或商用场景请改走 three.js 加 web-ifc 的路径。converter 以子进程方式使用 xeokit-convert，它输出的 XKT 数据不受 AGPL 覆盖。
