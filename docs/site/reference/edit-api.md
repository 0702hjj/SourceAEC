# IFC 编辑 API

edit-service 是 IFC 编辑端点的唯一参考，Python FastAPI，默认端口 8100。路径参数 `id` 匹配 `^m_[0-9a-f]{16}$`，`guid` 是 IFC GlobalId。直连时错误响应是 FastAPI 形态的 `{"detail": ...}`，经 Go 代理时换成 envelope（见文末）。

定位、暂存、大版本这些概念与版本语义见[编辑与版本](/guide/editing)，本页只讲契约。DXF 侧端点为同一套，差异见 [CAD 编辑服务](/development/cad-service)。

> 直改链路已于 2026-08-08 退役：直连本服务返回 410 Gone（不是 404），经 Go 代理的对应路由已注销、返回 404。修改一律走构建脚本。历史实现可从 git 历史回捞，锚点 `fb55a8a`——当时的路径是 `viewer/edit-service/app/routes_edits.py`，后随目录重组迁到 `services/ifc/` 下。

## 构建脚本契约

每个模型对应一个完整 Python 脚本，必须满足：

- 头部 `PARAMS = {...}` 是顶层字面量 dict，所有可调参数集中在这里。
- 构件的 GlobalId 由 key 确定性派生，并写入 `Pset_AIIFC.designKey`。同一脚本跑多少次 id 都不变，跨版本 diff 才能对齐。
- 审查可见的构件必须经契约工厂 `script_lib.create_entity` 创建，工厂自动写确定性身份并记录调用点。禁止绕过工厂直接建实体。
- 需要 web 表单可编辑的参数必须是标量字面量或 PARAMS 引用，不能是任意表达式，否则定向改写会拒绝。
- 入口是 `build(params, out_path)`，产物必须过 `ifcopenshell.validate`。

## 脚本编辑端点

`GET`、`PUT` 与 undo/redo 走暂存语义，`run` 沙箱试运行不产生版本，`save` 晋升大版本。

| 端点 | 语义 |
| --- | --- |
| `GET /models/{id}/script` | 当前脚本，暂存态优先，否则最近保存的版本 |
| `PUT /models/{id}/script` | 暂存一次编辑，整体替换脚本或只改 PARAMS。plain 模型首次暂存自动保留原件为 `bootstrap.ifc` |
| `GET /models/{id}/script/params` | 当前 PARAMS，ast 提取不执行脚本 |
| `POST /models/{id}/script/undo\|redo\|discard` | 暂存导航与放弃 |
| `POST /models/{id}/script/run` | 沙箱试运行，预览用，不产生版本 |
| `POST /models/{id}/script/save` | 晋升大版本；有 bootstrap 时响应带 alignment 计数 |
| `GET /models/{id}/scripts` | 大版本脚本列表 |
| `POST /models/{id}/script/rollback` | 恢复某版脚本并重新执行 |
| `POST /models/{id}/script/diff` | 两个大版本之间的脚本 diff |
| `GET /models/{id}/script/staging/diff` | 暂存链相邻步之间的轻量行内 diff |
| `GET /models/{id}/script/locate?guid=` | guid 定位到脚本调用点，语义见下节 |
| `POST /models/{id}/script/edit-call` | libcst 标量改写，**仅直连可用**。非法输入 422 零副作用 |
| `POST /models/{id}/user-edits` | 登记外部用户修改（标 `USER` 来源），不经过沙箱。IFC 侧独有，**Go 代理未注册此路由** |

`GET /health` 返回 `{"status": "ok"}`。

## 定位链路

每次沙箱执行时，契约工厂记下每个构件的调用点，落成 map sidecar，按脚本哈希绑定发布：

```python
ScriptMap = dict[designKey, {"line": int, "col": int, "snippet": str,
                             "origin": "literal" | "params" | "traced"}]
# 发布信封（current.map.json / v{n}.map.json）：
# {"scriptHash": sha256(脚本全文), "map": ScriptMap}
```

map 的行号只对生成它的那份脚本有效。暂存了新脚本但没运行过，两者就分叉：`edit-call` 对分叉直接拒绝（409），`locate` 降级返回 `stale: true`，前端提示先运行脚本。查不到构件返回 200 加 `found: false`，不是 5xx。

`origin` 决定前端的改写策略——`params` 走参数表单，`literal` 可直接改这一行，`traced`（运行时算出来的值）只能定位不能自动改写。

## 版本与语义对比

| 端点 | 语义 |
| --- | --- |
| `GET /models/{id}/versions` | `{"versions": [{"version": "v1", "createdAt": "<ISO8601 UTC>"}, ...], "current": "v2"}`。只有最新大版本的 IFC 在盘上，历史版本按需从脚本重建 |
| `POST /models/{id}/diff` | body `{"base": "v1", "target": "v2"}`，target 可为 `"current"` |
| `POST /models/{id}/diff/upload` | multipart 字段 `file` 与当前状态做属性级对比。比普通 diff 多一个 `labels`（guid 到可读名称）。不落盘不缓存 |
| `GET /models/{id}/pending` | 当前 pending 列表。直改退役后仅作运行簿记，**不校验模型是否存在** |
| `DELETE /models/{id}/pending` | 丢弃全部 pending，返回条数；模型不存在返回 404 |
| `GET /models/{id}/history` | 持久化编辑历史，存在模型的 `edit-history.json`。直改退役后只读保留 |

diff 响应形状：

```json
{
  "base": "v1",
  "target": "v2",
  "added": ["<guid>", ...],
  "removed": ["<guid>", ...],
  "changed": [{"guid": "...", "changes": [{"field": "...", "old": ..., "new": ...}]}]
}
```

对比是属性级语义，不做几何对比。版本不存在返回 404，缺参数返回 422；上传对比的文件非法返回 422。

## 经 Go 代理

Go server 把这些端点暴露在 `/api/v1` 下：

- 脚本编辑端点一一对应，路径不变，query 透传。run、save、rollback 成功后自动排队重转 XKT。`script/edit-call` **不经代理**，仅直连可用。
- 只读与对比端点在 `edit/...` 前缀下：`edit/diff`、`edit/pending`、`edit/history`、`edit/versions`。
- 直改代理路由已随退役删除。

与直连的差异：响应统一包 envelope，错误码映射为 `40400`、`40900`、`40001`、`50200` 等，见 [REST API](/reference/rest-api#响应与错误码)。

机器可读 schema 见[编辑 API 参考](/reference/edit-api-reference)，由 FastAPI 导出，与本页同源。
