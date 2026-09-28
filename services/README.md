# services/ — 业务逻辑核心（可复用，接口可直接调用或移植）

平台框架（`docs/superpowers/specs/2026-08-11-platform-framework-design.md`）定义两个对等的业务逻辑核心：每个核心提供 **沙箱执行 + diff + 面向前端修改的接口协议**，与对应 skill 配对，可脱离前端/网关独立部署。

| 目录 | 业务逻辑 | 物理实现 |
|---|---|---|
| `services/ifc` | IFC 段：脚本沙箱执行 + 版本快照 + 语义 diff + script-as-source 编辑 API | FastAPI + IfcOpenShell，:8100 |
| `services/cad` | CAD 段：同构（DXF 脚本沙箱 + 版本 + diff + render.json 发布） | FastAPI + ezdxf，:8200 |
| `services/sandbox` | 共享包 `aibim_sandbox`：bwrap 沙箱后端 + PEP 723 依赖环境 + script-as-source 领域模块单一源 | 被 ifc/cad 经 uv path editable 依赖 import，不独立起服务 |

- **services/ifc 独立调用指南**：见文档站 [Edit Service 独立部署与移植](https://0702hjj.github.io/AI_IFC/development/edit-service)。
- **沙箱机制**：见文档站 [沙箱执行环境](https://0702hjj.github.io/AI_IFC/development/sandbox)。
- **共享可选运行时**：`web`（前端，可选）、`server`（Go 网关 :8090）、`converter`（转换）、PostgreSQL（可选）。
- **skill 封装**：`skills/aiifc/`（IFC）、`skills/aiplan/` + `skills/aidxf/`（CAD plan→cad 管线）。
