# 模型与审查 API

模型、Issue、属性 override 与修改记录四组端点，外加模型静态资源的地址与 schema。响应统一 envelope，`code=0` 表示成功，错误码与鉴权行为见 [REST API](/reference/rest-api)。

模型 id 格式是 `m_` 加 16 位小写 hex；issue id 格式是 `i_` 加 12 位小写 hex。

## 模型

### POST /api/v1/models {#upload-model}

上传模型文件并触发异步转换。请求是 `multipart/form-data`，字段 `file`，仅接受 `.ifc` 与 `.dxf`，上限 200MB。

`.ifc` 入转换队列，初始 `status=converting`；`.dxf` 不经转换器，直接建为 `ready`。

> 用户界面的上传入口已于 2026-08-21 隐藏，模型由 agent 在项目会话内生成。本端点保留为 agent 建模型的内部链路，用户不需要直接调它。

```json
{"code":0,"message":"ok","data":{"id":"m_01J...","name":"Building-Architecture.ifc","status":"converting"}}
```

错误：`40001` 文件类型非法，`40002` 超出大小上限。

### GET /api/v1/models

模型列表，前端每 2 秒轮询，直到所有模型脱离 converting：

```json
{"code":0,"message":"ok","data":[
  {"id":"m_01J...","name":"a.ifc","size":1832140,"status":"ready","createdAt":"2026-07-27T10:00:00Z","error":""}
]}
```

`status` 取值 `converting`、`ready`、`failed`。

### GET /api/v1/models/{id} {#model-detail}

单模型详情，结构同上。

### POST /api/v1/models/{id}/retry

对 failed 模型重新入队转换，返回更新后的模型对象。

### DELETE /api/v1/models/{id} {#delete-model}

删除该模型的 IFC、XKT、metadata、状态文件及 issues、changes、overrides。

模型归属于项目时，删除会自动把它从项目里摘掉，project.json 同步更新，不会留下孤立的 modelId。

### GET /api/v1/models/{id}/download

下载原始 IFC，响应带 `Content-Disposition: attachment`。

## Issue

status 取值 `open`、`checking`、`resolved`。作者默认 `local-user`，来源默认 `UI`，创建时可覆盖。

### GET /api/v1/models/{id}/issues

返回 `data: Issue[]`，按创建时间降序。

### POST /api/v1/models/{id}/issues

`multipart/form-data` 两个部分：

- `issue`（必填）：JSON 字符串，含 `entityId`、`entityName`、`entityType`、`title`（必填）、`comment`、可选的 `author`、`provenance` 和相机参数 `camera`。
- `screenshot`（可选）：PNG，上限 5MB。

返回创建后的 `data: Issue`，含生成的 id、初始状态和截图相对路径。

### PATCH /api/v1/models/{id}/issues/{issueId} {#patch-issue}

JSON body 传 `title`、`comment`、`status` 中要更新的字段。

### DELETE /api/v1/models/{id}/issues/{issueId} {#delete-issue}

删除 Issue 及其截图。

## 属性 Override 与修改记录

属性修改走 metadata override，不改 IFC 本体。白名单字段只有 `Name`、`Description`、`Classification`、`FireRating`、`Comments`，每次修改逐字段写一条 change log。

change log 与 Issue 的 `provenance.source` 取值都是 `UI`、`AI`、`USER` 三种，**由调用方自报**，服务端只做枚举校验，不校验真实性。AI 直连编辑服务时传 `source="AI"`，登记外部用户改动走 `POST /models/{id}/user-edits` 标 `USER`。

### GET /api/v1/models/{id}/overrides

返回 `data: { [entityId]: { [field]: value } }`，无数据时是 `{}`。

### PUT /api/v1/models/{id}/entities/{entityId}/properties

```json
{"entityName":"Wall","fields":{"FireRating":"F60","Comments":"备注"}}
```

`fields` 必填且非空，字段名不在白名单返回 `40001`。空字符串表示清除该字段的 override。每个字段写一条 change log，返回该实体当前生效的 override 集合。

### GET /api/v1/models/{id}/changes

返回 `data: ChangeEntry[]`，按创建时间降序：

```json
{"code":0,"message":"ok","data":[
  {"id":"c_1a2b3c4d5e6f","entityId":"3a82-xxxx","entityName":"Wall","field":"FireRating","oldValue":"","newValue":"F60","author":"local-user","provenance":{"source":"UI"},"operation":"update","createdAt":"2026-07-29T10:00:00Z"}
]}
```

## 静态资源

以下路径直接返回文件，不走 JSON envelope：

| 路径 | 说明 |
| --- | --- |
| `GET /v1/models/{id}/model.xkt` | XKT 几何数据，支持 Range |
| `GET /v1/models/{id}/metadata.json` | xeokit 元模型，schema 见下 |
| `GET /v1/models/{id}/render.json` | CAD 渲染数据，仅 dxf 模型，schema 见下 |
| `GET /v1/models/{id}/issues/{file}` | Issue 截图，`file` 必须匹配 `i_[0-9a-f]{12}\.png` |

## metadata.json Schema

由 converter 从 IFC 提取，含空间结构树和属性集，可直接作为 `XKTLoaderPlugin.load` 的输入：

```json
{
  "projectId": "3xFoo",
  "metaObjects": [
    {"id": "1AbC...", "type": "IfcBuildingStorey", "name": "Level 1", "parent": "0Root"},
    {"id": "2XdE...", "type": "IfcWall", "name": "Wall-001", "parent": "1AbC...", "propertySetIds": ["pset_2XdE_0"]}
  ],
  "propertySets": [
    {
      "id": "pset_2XdE_0",
      "name": "Pset_WallCommon",
      "type": "Pset",
      "properties": [
        {"name": "FireRating", "value": "120min", "type": "IfcLabel"},
        {"name": "LoadBearing", "value": true, "type": "IfcBoolean"}
      ]
    }
  ]
}
```

`metaObjects[].id` 就是 IFC GlobalId，与 XKT 实体 id 一致。层级为 Site、Building、Storey 到构件。没有 pset 的构件省略 `propertySetIds`。

## render.json Schema

由 services/cad 在 run 或 save 成功后原子发布，供前端 Canvas 二维预览。坐标保留原始 DXF 坐标系：

```json
{
  "schemaVersion": 2,
  "bounds": {"min": [0, 0], "max": [100, 80]},
  "layers": [{"name": "WALL", "color": 7, "linetype": "CONTINUOUS"}],
  "entities": [
    {"key": "e_1a2b3c", "type": "LINE", "layer": "WALL", "start": [0, 0], "end": [10, 0]}
  ],
  "unsupported": [{"type": "HATCH", "handle": "1F", "coords": [5, 5]}]
}
```

约定：`bounds` 在没有可用坐标时为 null；`entities[].key` 是 XDATA 稳定 key，前端选中实体即得到 key；LWPOLYLINE 炸开成 LINE 和 ARC 条目；INSERT 只展开一层，子实体 key 为 null；白名单外的实体明面列入 `unsupported`，不静默丢弃。
