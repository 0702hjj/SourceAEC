# IFC 编辑服务（services/ifc）

`services/ifc/` 是 Python FastAPI 服务，基于 ifcopenshell 和 ifcdiff，默认端口 8100。它提供 IFC 脚本沙箱执行、版本快照、ScriptMap 定位和语义 diff。原 L1 直改链路已退役。DXF 侧的同构服务见 [CAD 编辑服务](/development/cad-service)。

## 运行与配置

```bash
cd services/ifc
uv sync
VIEWER_DATA_DIR="$(realpath -m ../../data)" uv run uvicorn app.main:app --port 8100
```

依赖全部来自 PyPI 官方发布，`uv sync` 直接安装。环境变量（`VIEWER_DATA_DIR`、`EDIT_SERVICE_PORT`、`AIIFC_FLOWS_DIR`、`EDIT_SERVICE_MAX_MODELS` 等）与沙箱变量集中在[配置说明](/guide/configuration)，沙箱机制见[沙箱执行环境](/development/sandbox)。

## 端点

| 分组 | 端点 | 契约 |
| --- | --- | --- |
| 脚本编辑 | `GET/PUT /models/{id}/script`、`script/params`、`script/undo\|redo\|discard` | [IFC 编辑 API](/reference/edit-api) |
| 沙箱与版本 | `POST /models/{id}/script/run`、`save`、`rollback` | 同上 |
| 版本与对比 | `GET /models/{id}/scripts`、`POST .../script/diff`、`GET .../script/staging/diff` | 同上 |
| 定位与改写 | `GET /models/{id}/script/locate?guid=`、`POST .../script/edit-call` | 同上 |
| 语义 diff | `GET /models/{id}/versions`、`POST /models/{id}/diff`、`.../diff/upload` | 同上 |
| 运行簿记 | `GET/DELETE /models/{id}/pending`、`GET /models/{id}/history` | 同上的「版本与语义对比」 |
| 外部修改登记 | `POST /models/{id}/user-edits` | 同上 |
| 直改遗产 | `PUT/DELETE /models/{id}/entities/{guid}`、`POST /models/{id}/commit` | **已退役**，直连返回 410 |

`pending` 在直改退役后只作运行簿记；`history` 只读保留，新记录来自外部修改登记。

## 实现要点

- 沙箱执行、暂存、版本、diff 这些领域逻辑的单一事实源在共享包 `aibim_sandbox`，REST 编辑面在 `aibim_editapi`，本服务只留薄适配层，详见[沙箱执行环境](/development/sandbox)。
- `app/ifc_materialize.py` 负责按需重建历史版本 IFC，结果进 LRU 缓存。重建产物只保证语义相等，比较一律走 diff。
- `app/route_common.py` 统一请求解析；业务校验住在各 `verify*` 函数里，这是仓库 `AGENTS.md` 的硬规则。
- `app/registry.py` 做模型缓存、原子保存和每路径文件锁。
- `app/diffing.py` 适配 IfcDiff：只比较属性和属性集，changed 的字段级明细自算，快照间结果缓存。

## 测试

```bash
cd services/ifc
uv run --group dev pytest
```

## 独立部署与移植

这个服务可以脱离 Go server、web、converter、PostgreSQL 单独部署，也可以整体搬到新宿主。它是和 aiifc skill 配对的服务端：skill 产出脚本，它负责执行、存版本和算 diff。

移植的最小步骤：

1. 拷贝 `services/ifc/` 和 `skills/aiifc/`，沙箱执行需要后者 flows 里的 `script_lib.py`。
2. 在 `services/ifc/` 下执行 `uv sync`。
3. 把 `VIEWER_DATA_DIR` 配成绝对路径，指向你的模型数据根目录。
4. 启动服务，打开 `/docs` 的 Swagger UI 自检。

其他组件都是可缺省的：

| 缺少的组件 | 还能做什么 | 少了什么 |
| --- | --- | --- |
| Go server | 全部编辑、diff、版本端点直连可用 | 统一 envelope、对外入口、鉴权、XKT 重转、浏览器桥接 |
| web | 纯 API 调用不受影响 | 可视化界面 |
| converter | 编辑与 diff 完全可用 | IFC 转 XKT 的渲染链路 |
| PostgreSQL | 默认文件存储即可 | Issue 等数据的 PG 持久化 |

数据约定只有一条：任何组件按同一个 `VIEWER_DATA_DIR` 写 `uploads/{id}.ifc`，编辑服务就能使用它。

两点边界要注意。8100 直连没有鉴权，务必只监听 127.0.0.1，要对外请走 Go server。provenance 的 `UI`、`AI`、`USER` 是调用方自报的声明字段，服务端只校验枚举，语义见[模型与审查 API](/reference/api-model)。
