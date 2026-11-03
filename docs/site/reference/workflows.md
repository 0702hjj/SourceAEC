# 持久脚本工作流

显式选择此接口可在网关重启后继续执行脚本任务。浏览器原有 run/save 接口仍同步执行。
所有工作流接口使用网关鉴权与 `{code,message,data}` envelope；成功 `code=0`。
普通 Bearer token 必须放在请求头，工作流查询不接受 `?token=`。

## 创建与查询

先暂存脚本，再从 `GET /api/v1/models/{id}/script` 取得当前 `revision`：

```http
POST /api/v1/models/{id}/workflows
Authorization: Bearer <token>
Content-Type: application/json

{"idempotencyKey":"save-001","action":"save","baseRevision":"<current script revision>","source":"ai","note":"Update wall"}
```

`action` 为 `run` 或 `save`。`source` 为 `human`（默认）或 `ai`。
key 为 1–128 个字母、数字、点、下划线、冒号或连字符；请求最多 1 MiB，拒绝未知字段。
服务先预留捕获容量，再冻结脚本、项目输入和几何环境身份。只有不可变定义、字节内容
与 run 已提交后才返回 **202**，`data.id` 格式为 `wf_` 加 32 个十六进制字符。
提交期间断连，可用原 key 和原请求重试；服务可能尚未接受，也可能已完成提交。

相同模型、相同 key 和相同意图返回原 run（**200**），不会重新捕获或生成。
改动 action、baseRevision、source 或 note 后复用 key 返回 **409**。
相同 key 仍在捕获中返回可重试 **429**。resume 也会重新检查待处理容量，满载返回 429。其他 429 表示捕获或构建容量不足；稍后用相同请求重试。
当前模型或输入在捕获期间变化返回 409；读取最新状态后使用新 key 提交。
纯文件上传但没有源码的模型不能创建脚本工作流。

| 方法与路径 | 返回 |
| --- | --- |
| `GET /api/v1/models/{id}/workflows?limit=20` | 最新摘要 `data.workflows`，limit 为 1–100；不读取大 blob 或结果 |
| `GET /api/v1/models/{id}/workflows/{workflowId}` | 单个 run 的阶段、状态、尝试次数和已提交 `result` |
| `POST /api/v1/models/{id}/workflows/{workflowId}/cancel` | 持久取消尚未完成的阶段 |
| `POST /api/v1/models/{id}/workflows/{workflowId}/resume` | 显式恢复 failed/canceled run，保留固定定义和已完成检查点 |

阶段为 `geometry`、`publication`、`conversion`；状态为 `pending`、`running`、
`succeeded`、`failed`、`canceled`、`superseded`。IFC 发布后还需完成预览转换；DXF 发布后完成。
查询中的 `result` 是已经提交的原编辑响应，例如 `{"modelId":"...","version":"v1","staged":0}`。
列表只提供摘要；详情才提供 result。错误摘要不暴露内部租约、环境路径或存储异常，诊断见服务日志。

## 恢复与取消语义

几何执行至少一次：进程在检查点持久化前终止，恢复时可能再次执行几何；
已持久完成的候选直接进入发布。发布在共同模型锁下检查租约所有权和固定执行身份，
版本、history 和工作流阶段在同一个事务中提交。过期结果成为 superseded，不覆盖当前模型。
损坏候选最多自动重新生成一次；冻结输入缺失或环境变化会失败，不静默读取新输入。

工作流扫描 PostgreSQL 恢复任务，进程内通知只用于降低延迟。普通租约为 30 秒，
运行时每秒续租。写入候选/发布的事务在持有 run 锁时将本事务内租约延长到三分钟阶段上限，
独立心跳跳过忙行；崩溃会回滚延长，其他领取者跳过活跃行锁。每阶段最多三次自动尝试。连接、资源或事务暂态错误有限退避重试，
脚本错误或环境漂移直接失败。显式 resume 重置阶段尝试额度，不改变旧定义。
成功或 superseded 的 run 不能 resume；需要重新读取并提交新请求。

取消会撤销该 run 的发布资格；已经运行的沙箱在续租失败后停止，迟到结果不能发布。
**取消不能撤销已提交版本**，也不保证立即停止已经入队的派生转换。
IFC 转换阶段失败或取消时，之前发布的版本与 result 仍存在；resume 只恢复转换。
转换根据已提交 IFC/build 身份重新入队，不再次保存模型版本。

## 部署与资源

此契约支持单个网关、同一 PostgreSQL 命名空间及共享工作区；进程预算不是跨副本全局配额。
默认环境使用 `uv venv --managed-python`，基础解释器与包环境均来自可审计目录。
升级此环境策略会使用新的缓存键，旧缓存保留但不再作为默认环境复用。
管理员自配环境也必须有隔离的基础解释器目录；仅创建链接到系统 `/usr` Python 的 venv
不能满足此条件。配置 `SOURCEAEC_PYTHON` 时应选择基于 uv-managed Python 创建的环境。
环境摘要覆盖解释器、venv 和基础运行时文件内容，执行前后校验；这不是 OS 快照，
也不保证脚本对时间、随机数和系统资源的字节级确定性。已完成候选发布无需重跑几何环境。

每个存储命名空间最多 64 个待处理 run/捕获票据、2 GiB 逻辑输入。
捕获前保守预留 259 MiB，票据五分钟到期，提交后改按实际输入与 manifest 大小计费。
因此接近容量上限时，小任务也可能被提前拒绝。原始输入单文件最多 64 MiB、
合计 256 MiB、最多 2048 个文件，manifest/script 各最多 1 MiB。
两个扫描 worker 与同步执行共享每进程三个完整构建名额；候选字节持有期间不提前释放名额。
内容寻址 blob 仅存 PostgreSQL，恢复不会把它们批量投影到磁盘；模型删除同时清理工作流。
完成 run 与 blob 当前保留到模型删除，待处理限额不限制累计历史存储、PG 临时空间或总 RSS。

参见[执行与恢复](/development/execution-lifecycle)、[沙箱](/development/sandbox)、
[AI/MCP](/reference/ai)与 [OpenAPI](/go-server.openapi.json)。
