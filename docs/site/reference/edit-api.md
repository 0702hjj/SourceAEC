# 原生 Go 编辑 API

公共入口为 Go 网关 `/api/v1/models/{id}`，响应使用 `{code,message,data}`。
下表路径相对该模型入口。不存在独立 Python 编辑服务。

| 方法 | 路径 | 行为 |
| --- | --- | --- |
| GET / PUT | /script | 当前源脚本 / 全量源码、PARAMS 或精确 patch 修改 |
| GET | /script/params | 读取字面量 PARAMS |
| POST | /script/undo、/script/redo、/script/discard | 十步暂存历史 |
| POST | /script/run | 沙箱执行并发布当前产物 |
| POST | /script/save | 执行并保存不可变版本及源码历史引用 |
| POST | /script/rollback | 以版本源码恢复当前模型 |
| GET | /scripts | 脚本版本列表 |
| GET | /edit/versions | 产品版本及当前版本 |
| POST | /script/diff | 版本间源文本与参数对比 |
| GET | /script/staging/diff | 暂存步对比，from/to 查询参数 |
| POST | /edit/diff | 产品版本间结构对比 |
| GET | /script/locate | 可选源 map 定位，IFC 用 guid，DXF 用 key |
| POST | /script/edit-call | 标量参数绑定或覆盖编辑 |
| GET | /git | 分支与提交状态 |
| POST | /git/branches、/git/checkout、/git/merge | 分支操作 |
| POST | /edit/diff-upload | multipart file，比较上传文档并记录 USER 来源 |

PUT body 为 script、params、patch 三选一。全量源码和参数修改可携带 baseRevision；
patch 必须提供它，且基准必须与当前源码 SHA-256 完全相同，过期返回 409。
patch 包含 1–128 个 `{start,end,text}`，使用原始 UTF-8 源码的字节位置，
start 包含、end 不包含；必须有序、不重叠且不切开字符。生成源码最多 1 MiB，
失败不修改暂存；成功与普通编辑共用撤销/重做。平台不猜测相似上下文或模糊应用 patch。

例如 `PARAMS = {}\n` 的 `{}` 位于字节 `[9,11)`，可在当前 revision 上提交：

```json
{"baseRevision":"<当前源码 SHA-256>","patch":[{"start":9,"end":11,"text":"{'width': 80}"}]}
```

保存 body 可有 note、source（human/ai）；rollback body 为 version。
保存返回 `version` 与 `historyRevision`，后者是 `r_` 开头的历史 ID，不是 Git SHA。
Git 查询或分支操作会按需物化真实 Git 提交；若物化失败，保存版本仍然有效，可重试查询。
产品 diff body 为 base 与 target，target 可用 current。

运行时脚本必须有顶层字面量 PARAMS，输出路径是第一个进程参数。
无平台 Python SDK。可选源 map 在 SOURCEAEC_MAP_OUT 输出，格式为
`{"map":{"entity-key":{"line":12,"origin":"declared"}}}`；摘要由 Go 绑定。IFC map key 使用 GlobalId，DXF 使用实体 XDATA key。

IFC 对比由 Go 读取 STEP 属性与相关属性定义，按 GlobalId 匹配；这不是完整 IFC
语义/合规分析引擎。DXF 按稳定 key 或无 key 内容匹配。
脚本错误、冲突、运行限额和环境不可用分别返回相应 API 错误。

[运行时限制](/development/sandbox)、[存储](/development/model-storage)。
