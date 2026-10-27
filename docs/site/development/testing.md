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

原生混合负载基准在 `server` 目录运行：

```bash
go test ./internal/editing -run '^$' -bench '^BenchmarkMixedNativeEditing$' \
  -benchtime=2000x -count=5 -cpu=1,4,8,16,32
```

它轮转脚本/参数读取、乐观暂存、运行和派生状态写入，报告所有操作的成功、冲突、
过载、超时与其他错误数量、成功吞吐、p50/p95/p99 和连接池等待。
默认使用确定性执行器；设置 `SOURCEAEC_BENCH_GEOMETRY=1` 及已准备好的
`SOURCEAEC_PYTHON` 可测预热后的真实 IFC/DXF 几何，宜降低每组操作数。
Linux 下每 5ms 采样进程树 RSS 之和，可能重复计入共享页、漏掉短命子进程，
不包含 PostgreSQL，也不表示唯一内存占用或保证捕获峰值。
该基准不包含 HTTP、Node 转换或冷依赖准备，不能作为这些链路的吞吐结论。

其他组件：`web` 执行 npm test/lint/build；`converter` 执行 npm test；
`docs` 执行 npm run docs:build。

根目录执行 `scripts/check_file_size.sh`、`scripts/check_public_snapshot.sh` 和
`scripts/check_go_business.sh`。业务边界检查禁止提交 Python 源码或应用清单，
skill 只维护声明和参考，不捆绑执行库。
