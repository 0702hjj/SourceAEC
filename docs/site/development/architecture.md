# 总体架构

```mermaid
graph LR
  subgraph 客户端层
    UI[浏览器<br/>React 19 + xeokit / web-ifc<br/>web]
    AI[AI Agent]
  end

  subgraph 服务层
    GO[Go server :8090<br/>server<br/>编排 / REST / 存储抽象<br/>Eino chat agent]
    PY[Python edit-service :8100<br/>services/ifc<br/>FastAPI + IfcOpenShell]
    CAD[Python cad-edit-service :8200<br/>services/cad<br/>FastAPI + ezdxf]
    CV[Node converter<br/>converter<br/>IFC → XKT + metadata.json]
    SB[共享包 aibim_sandbox<br/>services/sandbox<br/>bwrap 沙箱 + script-as-source 领域模块]
  end

  subgraph 存储层
    PG[(PostgreSQL<br/>issues / changes / overrides)]
    FS[(文件系统<br/>uploads/*.ifc, models/{id}/)]
  end

  UI -->|REST envelope + chat SSE| GO
  AI -->|同一套编辑 API| PY
  AI -->|或经 Go 代理| GO
  GO -->|/api/v1/models/{id}/script/* 代理 + 编排| PY
  GO -->|dxf kind 分流| CAD
  GO -->|子进程 node convert.js| CV
  GO -->|pgx/v5，可选| PG
  GO --> FS
  PY -->|脚本沙箱执行 / 版本快照 / history| FS
  CAD -->|DXF 脚本沙箱 / 版本 / render.json| FS
  CV -->|model.xkt + metadata.json| FS
  PY -.->|import（uv path editable）| SB
  CAD -.->|import（uv path editable）| SB
```

## 组件职责

| 组件 | 技术 | 职责 | 选型原因 |
| --- | --- | --- | --- |
| web | React 19 + TS + Vite + zustand + xeokit-sdk + web-ifc/three | 全部交互界面 | xeokit 提供 XKT 加载与 BIM 工具链；web-ifc 直读 IFC，两条链路并存 |
| server | Go 1.26，stdlib net/http + pgx/v5 + cloudwego/eino | 上传转换队列、REST、编辑编排、存储抽象、chat agent | 静态编译、并发模型 |
| converter | Node CLI，web-ifc + xeokit-convert | IFC 转 XKT，提取语义元数据 | xeokit-convert 只有 npm 形态 |
| edit-service | Python 3.10 + FastAPI + ifcopenshell + ifcdiff | IFC 脚本沙箱、版本、定位、语义 diff | IfcOpenShell 是 IFC 编辑的事实标准 |
| cad-edit-service | Python 3.10 + FastAPI + ezdxf | DXF 脚本沙箱、版本、diff、render.json | ezdxf 是 DXF 编辑的事实标准 |
| services/sandbox | 共享包 aibim_sandbox | bwrap 沙箱后端、PEP 723 依赖环境、script-as-source 领域模块 | 两个服务同构，沙箱逻辑只维护一份，见[沙箱执行环境](/development/sandbox) |
| PostgreSQL | 可选 | Issue、修改记录、override 三张表 | 不配置时全部落文件 |

各组件内部结构见[Web 前端](/development/web)、[Go Server](/development/server)、[IFC 编辑服务](/development/edit-service)、[CAD 编辑服务](/development/cad-service)。

## chat agent

对话由 `server/internal/agent/` 里的 Eino agent 驱动，完全跑在 Go 进程内：

- **事件流**：agent 事件翻译成 SSE 帧推给前端，会话记录追加写 JSONL 日志，重启可回放。旧的 opencode serve 外部进程已退役。
- **领域工具**：LLM 拿不到 shell 和任意文件写权限，只有平台工具——列模型、读脚本、定位、暂存、试运行、保存、看版本、看 diff、建项目、跑 skill 命令等。工具报错以文本返回给模型自己纠正。
- **主子编排**：orchestrator 负责对话和路由，按任务派 ifc-agent 或 cad-agent 子 agent。子 agent 有独立模型实例，输出带 subagentId 标签，前端分组展示。
- **skill 接入**：从 `skills/dist` 加载正式 skill 集合。文件工具只允许读参考文档和执行白名单命令，写文件只允许写项目工作区。
- **会话连续性**：跨轮次回填历史，超预算时压缩。agent 随时可以读当前脚本，保证修改是增量而不是重写。
- **向用户提问**：agent 可以中断并提问，回答后继续；LLM key 为空时用确定性脚本模型，不依赖真实 LLM 也能跑通流程。

## 仓库结构

```
SourceAEC/
├── skills/                   # ① AI 生成 skill 封装（agent-agnostic，可分发）
│   ├── aiifc/                #   IFC 生成/修改（ifcopenshell）
│   ├── aiplan/               #   plan 阶段（外部资料 → plan.json + bim_supplement.json）
│   ├── aidxf/                #   plan→cad 建筑平面管线正式版（plan.json → building.json + 各层 DXF）
│   ├── aiblueprint-mcp/      #   CAD 交互微调 MCP（MIT）
│   └── aibim-orchestrator/   #   主 Agent 编排提示词包（意图路由 + 子 Agent 分工契约）
├── services/                 # ② 业务逻辑核心（沙箱执行 + diff + 编辑 API）
│   ├── ifc/                  #   IFC 段（FastAPI + IfcOpenShell，:8100）
│   ├── cad/                  #   CAD 段（FastAPI + ezdxf，:8200，与 ifc 同构）
│   ├── sandbox/              #   共享包 aibim_sandbox（bwrap 沙箱 + script-as-source 领域模块）
│   └── editapi/              #   共享包 aibim_editapi（REST 编辑面单一源 + 共享测试套件）
├── web/                      # ③ 共享可选运行时 · 前端（React 19 + xeokit/web-ifc + Fabric，:5173）
├── server/                   #   · Go 网关（:8090——REST 入口 + 编排 + chat agent + 静态托管 web/dist）
├── converter/                #   · Node 转换器（IFC → XKT）
├── mcp/                      #   · MCP 桥（可选，薄包编辑服务，stdio）
├── scripts/smoke.sh          #   · 端到端冒烟
├── data/                     #   · 运行时数据（gitignored，server 与两个 Python 服务共享）
├── tools/                    # skill 打包器（skill_pack.py）+ agent TUI 调试工具
├── examples/                 # IFC 示例脚本与 buildingSMART 样例（CC BY 4.0）
├── docs/
│   ├── site/                 # 唯一公开文档站源（VitePress：guide / development / reference）
│   ├── internal/             # 内部计划、团队同步、阶段评估（不发布）
│   └── work/                 # 工作项看板
├── .github/workflows/        # CI 与 docs（构建 + Pages 部署）
├── LICENSE                   # Apache-2.0
└── NOTICE                    # 三方组件与归档代码边界
```

历史沿革：原 `viewer/` 目录已拆分为顶层组件；`skills/aidxfv/` 已删除，`skills/aidxf/` 是唯一迭代基线；SCAD 遗产代码已移至私有归档仓；设计 spec 与实施计划（`docs/superpowers/`）和 `.opencode/` 存档已移出仓库，仅本地保留。

**文档边界**：`docs/site/` 是唯一公开文档站源。各服务 README 只留最小启动提示，详细说明链接到文档站，不复制第二份。

## 核心数据流

### 上传转换流

```
上传 .ifc → Go 校验并存 uploads/{id}.ifc（status=converting）
  → 转换队列（2 worker，dedup + dirty 重跑）→ node convert.js
  → models/{id}/model.xkt（几何）+ metadata.json（空间树/pset）
  → status=ready → 前端 XKTLoaderPlugin 同时加载几何与语义
```

关键不变量：XKT 构件 id、metadata 里的 metaObject id、IFC GlobalId 三者一致。选中、着色、diff 全靠这条链对齐。重转由上传、重试以及脚本的 run/save/rollback 触发，对同一模型的在途任务做 dirty 重跑。

### 编辑流

web 和 AI 的修改统一为改构建脚本，IFC 是脚本执行的产物。原 L1 直改链路已退役：直连编辑服务返回 410，经 Go 代理的对应路由已注销、返回 404。

```
PUT  /models/{id}/script        → 契约静态校验（失败 422 零副作用）→ 暂存一步
POST /models/{id}/script/run    → 沙箱试运行预览（无版本）
POST /models/{id}/script/save   → 沙箱跑脚本生成 IFC
                                → 大版本 v{n}：scripts/v{n}.py + v{n}.map.json 成对快照
                                → versions/v{n}.ifc 只物化最新，历史按需重建
```

概念与版本语义见[编辑与版本](/guide/editing)，端点契约见 [IFC 编辑 API](/reference/edit-api)。

### 版本与 diff 流

大版本三件成对：脚本和定位 map 全量保留，产物文件只物化最新。历史版本 diff 或下载时从脚本重建，结果进 LRU 缓存。确定性 GlobalId 保证重建结果语义可对齐，**字节不做断言**。

`POST /models/{id}/diff` 做属性级语义对比：added 和 removed 是 guid 列表，changed 是字段级的旧值新值。快照之间的对比结果会缓存；对比外部上传的文件则不落盘不缓存。

## 版本模型

change log 条目记录作者、时间、操作和来源标签。来源分 UI、AI、USER 三种（语义见[模型与审查 API](/reference/api-model)）。版本是线性序列，没有分支合并；回滚等于恢复历史脚本重跑，不改写历史。

已知技术债：Go 侧 change log 和 edit-service 的编辑历史并存，粒度与用途不同；Python 侧存储只有文件模式。
