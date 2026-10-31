# 测试与边界检查

Go 测试位于实现旁。编辑领域测试覆盖暂存与冲突、版本/Git、失败不发布、
格式 diff、DXF render 以及真实 bubblewrap 执行和资源限制。
Python 只作为部分集成测试使用的外部几何运行时；测试本身用 Go 编写。

```bash
cd server
go test ./...
go vet ./...
```

设置独立测试数据库的 `VIEWER_PG_DSN`、`VIEWER_TEST_PG_DSN`。
几何库端到端测试可设置 `SOURCEAEC_TEST_GEOMETRY=1`，并通过
`SOURCEAEC_PYTHON` 提供安装 IfcOpenShell 与 ezdxf 的环境。
执行测试需要 bubblewrap 和 prlimit；无相应运行时的测试会说明跳过原因。

根目录的 `scripts/test_go_integration.sh` 是严格入口：必须显式提供专用数据库的
`VIEWER_TEST_PG_DSN`，并准备 Go、Git、bubblewrap、prlimit、uv、Node 和
`converter` 的 npm 依赖。它启用真实 IFC/DXF 几何及 IFC 转换器测试，验证转换
快照、过期结果、预览 generation、进程崩溃恢复及脚本读取快照，并重复运行聚焦 race 检查。

构建输入快照测试覆盖缺失/落后投影、其他事务的未提交项目归属、调用者只读快照、
单连接池复用、请求取消，以及 IFC/DXF 保存与回滚使用相同的执行输入和归档输入。
严格入口将 editruntime 的输入采集用例纳入五轮聚焦 race。

基准、profile 与容量测量见[性能与容量验证](/development/performance)。

覆盖关系可从以下测试入口定位；确定性协调测试与真实外部运行时测试分别执行：

| 契约 | 测试入口 |
| --- | --- |
| 原生网关、鉴权、envelope、IFC/DXF | `api/native_editing_test.go`、`api/auth_test.go` |
| 乐观修改与输入/分支/回滚过期冲突 | `editing/concurrency_test.go`、`editing/detached_test.go` |
| 独立进程冲突与进程终止恢复 | `editing/detached_process_test.go`、`editing/crash_test.go` |
| 断开数据库、提交应答丢失、恢复重试 | `editruntime/disconnect_test.go`、`editruntime/commit_test.go` |
| 旧进程回滚不覆盖新提交 | `editruntime/interrupted_process_test.go` |
| 版本权威、遗留迁移、投影缺失 | `editing/version_catalog_test.go` |
| 连接池/命名空间/共同准入 | `editing/isolation_test.go`、`editing/derived_admission_test.go` |
| 沙箱隔离、取消、环境与产物限制 | `editing/sandbox_isolation_test.go`、`editing/geometry_test.go` |
| 转换过期结果与独立队列 | `store/conversion_test.go`、`store/conversion_process_test.go`、`convert/cross_queue_test.go` |
| 缓存回收、进程间在途读取、模型删除 | `store/viewer_cache_test.go`、`store/viewer_process_test.go`、`store/deletion_test.go` |
| 已提交下载/Range 与 DXF 预览 | `api/source_snapshot_test.go` |
| Agent 原生冲突与 MCP 认证/人工来源 | `agent/tools_native_revision_test.go`、`api/native_mcp_test.go` |

MCP 当前提供上下文读取与人工修改文件比较，未提供脚本保存工具；脚本工具由 Agent
复用同一原生 editing 生命周期。不能把这两类集成误写成两套独立保存实现。

其他组件：`web` 执行 npm test/lint/build；`converter` 执行 npm test；
`docs` 执行 npm run docs:build。

根目录执行 `scripts/check_file_size.sh`、`scripts/check_public_snapshot.sh` 和
`scripts/check_go_business.sh`。业务边界检查禁止提交 Python 源码或应用清单，
skill 只维护声明和参考，不捆绑执行库。
