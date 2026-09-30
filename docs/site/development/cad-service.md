# CAD 编辑服务（services/cad）

`services/cad/` 是 Python FastAPI 服务，基于 ezdxf，默认端口 8200，与 `services/ifc` 同构：脚本沙箱、10 步暂存、大版本快照、定位、标量改写、diff 一应俱全，另外负责发布 render.json 给前端画布。

## 与 IFC 侧的差异

- 模型文件是 `uploads/{id}.dxf`，产物是 `out.dxf`。
- **没有内存实体缓存**，也没有直改时代的遗留概念（无 `pending`、`history`）。
- 版本与对比齐备，但对齐口径不同：定位按实体 key 查而非 guid，key 存在 DXF 的 XDATA 扩展数据里；diff 是实体级的，IFC 侧是属性级。
- run/save 成功后**原子发布 render.json**，这是 IFC 侧没有的一步。

## 运行与配置

```bash
cd services/cad
uv sync
VIEWER_DATA_DIR="$(realpath -m ../../data)" uv run uvicorn app.main:app --port 8200
```

环境变量（`VIEWER_DATA_DIR`、`CAD_SERVICE_PORT`、`AIDXF_FLOWS_DIR`、`AIDXF_DRAWLIB_DIR`、`CAD_SERVICE_MAX_MODELS` 等）与沙箱变量集中在[配置说明](/guide/configuration)，沙箱机制见[沙箱执行环境](/development/sandbox)。

## 端点

脚本编辑面与 IFC 侧是同一套，语义见 [IFC 编辑 API](/reference/edit-api)：

| 分组 | 端点 |
| --- | --- |
| 读取与暂存 | `GET/PUT /models/{id}/script`、`script/params`、`script/undo\|redo\|discard` |
| 沙箱与版本 | `POST /models/{id}/script/run`、`save`、`rollback`。成功后原子发布 render.json |
| 版本与对比 | `GET /models/{id}/scripts`、`script/diff`、`script/staging/diff`、`GET /models/{id}/versions`、`POST /models/{id}/diff` |
| 定位与改写 | `GET /models/{id}/script/locate?key=`、`POST /models/{id}/script/edit-call`（仅直连） |
| 渲染载荷 | `GET /models/{id}/render.json`。已发布时直接读文件，未发布时即时生成 |

Go server 按模型 kind 分流：dxf 模型的请求代理到 8200，对外路径不变。render.json 另外经 `GET /v1/models/{id}/render.json` 只读直挂，schema 见[模型与审查 API](/reference/api-model#render-json-schema)。

render.json 的发布不阻断主流程：生成或写盘失败只记 warning，并**主动删除旧的 render.json**，防止旧 payload 与新的 uploads 文件错位。所以前端拿不到渲染数据时，看服务日志有没有这条 warning。

## 实现要点

- 七个领域模块来自共享包 `aibim_sandbox`，REST 编辑面来自 `aibim_editapi`，与 services/ifc 同一事实源，本服务只留薄适配。
- `app/dxf_diffing.py` 做实体级 diff；`app/dxf_materialize.py` 按需重建历史版本；`app/render.py` 把 DXF 转成 render.json，坐标保持原始 DXF 坐标系。
- 契约层在 `services/cad/flows/`，提供脚本校验和确定性 key。

## 测试

```bash
cd services/cad
uv run --group dev pytest
```
