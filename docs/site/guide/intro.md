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

SourceAEC 自有材料以 Apache-2.0 发布，见 [LICENSE](https://github.com/0702hjj/SourceAEC/blob/main/LICENSE)。`skills/aiplan` 与 `skills/aidxf` 目录默认采用 MIT；单个文件的 SPDX 标识优先于目录默认许可。完整清单见根目录 [NOTICE](https://github.com/0702hjj/SourceAEC/blob/main/NOTICE)，项目的独立来源、开发记录和权利声明见 [PROJECT_PROVENANCE.md](https://github.com/0702hjj/SourceAEC/blob/main/PROJECT_PROVENANCE.md)。

IFC Web 查看有两套实现：xeokit 路径通过 AGPL-3.0 的 `@xeokit/xeokit-sdk` 加载服务端预转换的 XKT；web-ifc 路径使用 MPL-2.0 的 web-ifc 与 Three.js 在浏览器中直接读取 IFC，运行时不使用 XKT。两条路径在功能上独立，但当前标准 Web 构建同时包含两者并声明 xeokit 依赖，因此仅在界面切换到 web-ifc 不会从已分发构建中移除 xeokit，也不会自动消除相应的 AGPL 义务。若部署目标是不包含 xeokit，必须制作并验证不引入 xeokit 的独立构建。`converter` 另行以子进程使用 xeokit-convert 与 web-ifc 生成 XKT 和元数据。分发或通过网络提供相关构建前，应按实际包含的组件、具体版本和部署方式审查许可证义务。

## 开源背景与独立来源

SourceAEC 源于维护者对 Agent-friendly CAD/BIM 接口的独立研究与工程探索。公开技术背景包括 SimpleCADAPI 项目及其发表于 *Computer-Aided Design* 的相关研究、buildingSMART IFC 标准、公开开源接口和其他公开技术资料。它们构成问题定义与工程方向的参考；SourceAEC 面向 IFC 创建、编辑、检查与版本管理进行了独立实现，引用这些公开来源不表示双方存在隶属、背书或共同作者关系。

本项目在维护者实习合同约定的职责范围之外独立开展，并非为履行实习工作任务而开发；未利用实习单位的物质技术条件，完全基于公开资料及个人自费取得的设备、账号、算力与其他资源独立实现。项目未有意使用或收录实习单位的私有仓库、源代码、内部文档、Prompt、客户数据、图纸、模型、商业秘密或其他保密技术材料。公开 Git 历史始于首次源码快照，因此它本身不是发布前开发过程的完整记录；发布后的变更由 commit 与 Pull Request 记录，适当的同期开发材料由维护者另行保留。

上述内容是维护者依据开发记录对事实所作的善意说明，不代表任何现任或前任雇主、客户或其他第三方发言，也不改变第三方材料各自的许可证。贡献者仍须遵守 DCO 和来源披露要求；具体声明及私密权利异议渠道见 [PROJECT_PROVENANCE.md](https://github.com/0702hjj/SourceAEC/blob/main/PROJECT_PROVENANCE.md)。
