# 存储与前端对接

本文面向两类集成方：想把存储层接到自己数据库的，以及自研前端要接入的。协议细节（envelope、错误码、鉴权行为）以 [REST API](/reference/rest-api) 为准，本页讲存储契约与对接面。

## 数据目录布局

Go server 和两个 Python 服务必须共享同一个数据目录，配错会 404 或改错文件。

```
{VIEWER_DATA_DIR}/
├── uploads/{id}.ifc|.dxf              # 当前模型态（script/run 或 save 时原子替换）
├── staging/{modelId}.py                # agent 写脚本的中转落点，再由 API 暂存
├── projects/{projectID}/project.json   # 项目本体（原子 tmp + rename）
├── plans/{projectID}/                  # 方案产物版本目录（plan / bim_supplement / building）
├── skill-work/{projectID}/             # agent 文件工具的写白名单根；skill 中间产物，不版本化
└── models/{id}/
    ├── model.json                      # 模型状态：name/size/status/error（Go store，原子写）
    ├── model.xkt                       # converter 产物（静态直挂路径）
    ├── metadata.json                   # converter 产物（xeokit 元模型）
    ├── render.json                     # CAD 渲染载荷（仅 dxf，run/save 后原子发布）
    ├── issues.json                     # issue 列表（仅文件存储模式）
    ├── issues/{issueId}.png            # issue 截图（文件与 PG 模式均在此）
    ├── changes.json                    # 修改记录（仅文件存储模式）
    ├── overrides.json                  # 属性 override（仅文件存储模式）
    ├── edit-history.json               # services/ifc 持久化编辑历史
    ├── pending.json                    # script-run 回放簿记（内部）
    ├── script_staging.json             # 暂存脚本链（原子写，重启恢复）
    ├── bootstrap.ifc                   # 首次暂存脚本时保留的上传原件
    ├── current.map.json                # 当前 ScriptMap 发布信封
    ├── scripts/                        # 大版本脚本快照：v{n}.py + v{n}.map.json 全留
    ├── versions/                       # 大版本产物：v{n}.ifc|.dxf 只留最新
    └── ifc_cache/                      # diff/下载时按需重建的历史版本 + .map.json sidecar
```

写入方分两组：uploads、model.json、XKT、metadata、projects、plans 和 issues/changes/overrides 由 Go server 与 converter 写；其余由 Python 服务直接读写，不经过 Go store。Go 和 Python 共享的是**目录契约**，不是同一个 store 实例。

跨语言共享目录只有一条硬约束：**先写临时文件再 rename，禁止原地截断写**。

## 两种存储实现

文件存储是默认实现，零依赖：所有状态落在数据目录里，JSON 文件加原子写。

PostgreSQL 可选：设置 `VIEWER_PG_DSN` 即切换。三张表由各自的 `pgstore.go` 在构造时自动建立。

两处不对称要知道：

1. **模型注册表不进 PG**。模型元数据和上传文件始终是文件存储，PG 只承接 issue、change、override 三个网关侧数据。
2. **Issue 截图不进 PG**。PG 模式下 PNG 仍落盘，库里只存相对路径。

这两点决定了备份要同时覆盖数据目录和 PG，缺一不可。

## 第三方整合路径

按耦合从高到低三条路：

1. **实现 Go store 接口**，用自己的数据库承接网关侧状态。三个接口分别在 `issue.go`、`change.go`、`override.go`：

   | 接口 | 方法 | 职责 |
   | --- | --- | --- |
   | `issue.Store` | `List` / `Create` / `Update` / `Delete` / `DeleteModel` / `SaveScreenshot` | issue 增删改查、按模型清理、截图落盘 |
   | `change.Store` | `List` / `Append` / `DeleteModel` | 修改记录追加与按模型清理 |
   | `override.Store` | `GetAll` / `Set` / `DeleteModel` | 属性 override 读写与按模型清理 |

   注意模型注册表 `store.Store` 是具体类型不是接口，替换它要改 server 内部装配。

2. **复用本仓 PG schema**：把三个 `pgstore.go` 里的建表语句跑到自己的 PG 实例，设 `VIEWER_PG_DSN` 指向它即可。

3. **只做文件级对接**：直接读写数据目录，耦合最低，适合只读消费 XKT、metadata 或旁路分析。写入方必须遵守上面的并发纪律。

`data/` 是运行时目录，不要手工修改。

## 前端对接契约

平台本体是 API，`web/` 只是一个可整体替换的参考实现。对接面是 Go 网关的 `/api/v1/*`（信封响应）和 `/v1/models/*`（静态直挂）。

### 协议基线

除静态文件端点外，所有响应统一 envelope，`code=0` 表示成功：

```json
{"code": 0, "message": "ok", "data": {...}}
```

八个错误码的含义、鉴权豁免端点与 CORS 规则见 [REST API](/reference/rest-api)。

SSE 事件流：`GET /api/v1/chat/sessions/{cid}/events` 返回标准 SSE 帧，服务端维护编号缓冲，断线可以用 `Last-Event-ID` 重放最近 64 条。帧类型见[对话 API 的帧表](/reference/api-chat#sse-events)。

### 编辑流程对接

一次典型修改只有四步：

```bash
BASE=http://127.0.0.1:8090/api/v1
MID=m_0123456789abcdef

# 1. 暂存脚本（整脚本或 params 增量）
curl -X PUT "$BASE/models/$MID/script" -H 'Content-Type: application/json' -d '{"script": "..."}'
# 2. 沙箱试运行（预览，无版本；成功后 Go 侧排队重转 XKT）
curl -X POST "$BASE/models/$MID/script/run"
# 3. 保存大版本 v{n}
curl -X POST "$BASE/models/$MID/script/save" -H 'Content-Type: application/json' -d '{"note": "v1"}'
# 4. 轮询模型状态，converting 变 ready 说明新 XKT 可取
curl "$BASE/models"
```

配套端点还有 `script/undo|redo|discard`、`script/rollback`、`script/staging/diff`、`GET .../scripts`、`script/locate`，以及只读的 `edit/versions`、`edit/diff`。逐个端点的 body 与响应见 [IFC 编辑 API](/reference/edit-api)；供外部 Agent 使用的完整走查见 [Agent 接入](/reference/ai)。

### 显示对接

| 端点 | 说明 |
| --- | --- |
| `GET /v1/models/{id}/model.xkt` | XKT 几何数据，支持 Range |
| `GET /v1/models/{id}/metadata.json` | xeokit 元模型，schema 见[模型与审查 API](/reference/api-model#metadata-json-schema) |
| `GET /v1/models/{id}/render.json` | DXF 渲染数据，仅 dxf 模型 |

自研前端可以用任何渲染器。用 xeokit 时 `metadata.json` 可以直接作为 `XKTLoaderPlugin.load` 的输入，实体 id 就是 IFC GlobalId，和编辑 API 的 guid 天然对齐。
