# 引擎、领域服务与集成边界

SourceAEC 是面向 AEC 场景的应用平台，不是通用 CAD 内核。它把 IFC、DXF 以及其他可选的几何引擎组织成可部署、可审计、可供 Agent 调用的领域工作流。

SimpleCADAPI（下文简称 SCAD）是可以被 SourceAEC 集成的通用参数化 CAD 工程底座。SCAD 负责通用的实体建模、拓扑、特征树、零件装配、Notebook Runtime 和 `.scadpkg` 产品包；SourceAEC 负责把这些能力放进项目、版本、审阅、安全执行和 AEC 语义中。两者通过公开接口和产物协作，不通过复制内部实现协作。

## 分层模型

```text
几何内核与通用 CAD 范式
  └─ SCAD：OCP/BREP、特征、装配、Notebook、.scadpkg、通用导出

领域适配与产物转换
  └─ SourceAEC adapter：调用公开 CLI/API，读取产物，建立版本关联

AEC 领域服务
  ├─ IFC：IfcOpenShell、IFC 语义、GlobalId 和语义 diff
  ├─ DXF：ezdxf、图元/XDATA、图纸版本和渲染
  └─ DWG：独立的外部引擎或受许可约束的适配服务

应用与治理
  └─ Gateway、Web、MCP、项目、权限、沙箱、Agent、审计和版本管理
```

分层的判断标准是：脱离 AEC 项目仍然成立的通用 CAD 能力归 SCAD；回答“模型如何进入 AEC 项目并被管理、审阅和交付”的能力归 SourceAEC。

## SourceAEC 当前组件的职责

| 组件 | 责任 | 边界 |
| --- | --- | --- |
| Go Gateway | 认证、CORS、统一响应、公共 REST 入口和服务编排 | 不承载 IFC/DXF/几何运算 |
| `services/ifc` | IFC 脚本执行、版本、GlobalId 定位和语义 diff | 不实现通用机械 CAD 内核 |
| `services/cad` | DXF 脚本执行、XDATA、版本、diff 和渲染 | 当前以 `ezdxf` 为核心，不等同于 SCAD |
| `services/sandbox` | bubblewrap、限时、限额和脚本运行基础设施 | 作为共享运行库，不直接暴露业务 API |
| `services/editapi` | stage、run、save、rollback、diff 等共享编辑协议 | 不拥有 IFC、DXF 或 SCAD 的领域语义 |
| `converter` | IFC 转换和 viewer 产物生成 | 可在作业量增大时演化为异步 worker |
| `mcp` | 将 SourceAEC 能力暴露给 Agent | 不重复实现领域逻辑 |

如果未来需要在线运行 SCAD Notebook，优先新增独立的 `SimpleCAD Adapter` 或专门服务，而不是把 OCP 依赖直接加入现有 DXF 服务。

## SCAD 应负责的能力

SCAD 适合维护与行业无关的通用 CAD 能力：

- OCP/OpenCascade 几何和 BREP 拓扑；
- 参数化特征、特征树、零件、装配和约束；
- Notebook Runtime、稳定实体标识、查询和标签；
- `.scadpkg` 及其校验、重放和通用读取接口；
- STEP、STL、FreeCAD、MJCF 等通用产物；
- 通用 CAD 验证、错误诊断和 Agent 建模 Skill；
- 面向第三方工具的通用 addon 协议。

SCAD 可以提供 IFC、DXF 或 DWG 的通用交换能力，但不应承担建筑项目、楼层空间、构件属性、图纸审批或 SourceAEC 权限等行业应用语义。

## SourceAEC 应负责的能力

SourceAEC 负责 AEC 领域和应用治理：

- IFC 项目、空间、构件和属性集；
- DXF 图纸、图层、图元和建筑平面流程；
- DWG 的独立引擎适配、许可隔离和交付流程；
- 脚本暂存、试运行、保存、回滚、版本和差异审查；
- 项目、模型、权限、审计、Agent 和 MCP；
- 将外部 CAD 产物关联到 SourceAEC 的模型版本；
- CAD 到 IFC 或其他 AEC 语义的领域映射。

因此，`IfcBuildingElement` 的映射、楼层和空间管理、施工图阶段、版本审阅等内容应留在 SourceAEC，不应要求 SCAD 理解这些语义。

## 插件与适配器的归属

“插件”需要按依赖对象分为三类：

1. **通用 CAD addon**：只依赖 SCAD 公共 API 或 `.scadpkg`，例如通用分析、加工后处理和格式转换，属于 SCAD 生态。
2. **SourceAEC adapter**：调用 `sca`、公开 API 或读取 `.scadpkg`，把结果登记为 SourceAEC 产物、版本或任务，属于 SourceAEC。
3. **AEC 领域扩展**：IFC 映射、建筑构件属性、项目审查和交付工作流，属于 SourceAEC，不应进入 SCAD 核心。

SourceAEC 对 SCAD 的集成应优先采用进程或产物边界：

```text
Notebook / sca run
        ↓
     .scadpkg / STEP / JSON 摘要
        ↓
SourceAEC adapter
        ↓
项目、版本、审阅、Web、MCP
```

适配器不得依赖 SCAD 的私有模块、内部缓存目录、测试辅助代码或未文档化的数据结构。反过来，SCAD 也不应依赖 SourceAEC 的模型 ID、API envelope、权限、项目数据库或 MCP 实现。

## 何时向上游提议，何时留在本仓库

只有同时满足以下条件时，才适合向 SCAD 上游提议功能：

- 需求与行业无关；
- 多个外部集成者都会需要；
- 必须由 SCAD 内部提供才能保证正确性；
- 它属于公共 API、产品格式或运行时契约。

例如，稳定的 headless runner、机器可读的运行结果、标准化取消/超时错误和 `.scadpkg` 摘要接口，可能是上游议题。

以下内容应留在 SourceAEC：

- SCAD 产物如何进入 AEC 项目；
- `.scadpkg` 如何关联 SourceAEC 版本；
- STEP 到 IFC 的领域映射；
- IFC/DXF/DWG 的项目管理和审查；
- 权限、沙箱、Agent 和 MCP 治理。

SourceAEC 的集成案例可以向上游分享，但不应把 SourceAEC 的领域架构写成 SCAD 的内置职责。反之，SCAD 的通用 CAD 能力可以在 SourceAEC 中作为可选引擎接入，但不应改变 SourceAEC 的 AEC 应用定位。

## 当前迭代顺序

后续工作按以下顺序推进：

1. 保持现有 IFC、DXF、sandbox、edit API 和 Gateway 的边界稳定；
2. 在 SourceAEC 内验证 `.scadpkg` 的只读消费、摘要和版本关联；
3. 根据验证结果设计独立的 SCAD adapter 或服务；
4. 只有发现通用、可复现的公共契约缺口时，才向 SCAD 提交上游建议；
5. 将已经稳定的集成方式补充到 MCP、Web 和 AEC 工作流中。

核心原则：SCAD 拥有通用 CAD 语义和稳定产物契约；SourceAEC 拥有 AEC 语义、应用工作流和治理能力。
