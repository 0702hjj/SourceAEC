# 性能与容量验证

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

完整网关负载使用真实几何、认证 HTTP、暂存、保存/Git、产品比较、下载与 Node 转换：

```bash
SOURCEAEC_BENCH_GEOMETRY=1 go test ./internal/api -run '^$' \
  -bench '^BenchmarkNativeHTTP$' -benchtime=32x -count=5 -cpu=1,4,8,16,32
SOURCEAEC_BENCH_GEOMETRY=1 go test ./internal/api -run '^$' \
  -bench '^BenchmarkGeometryEnvironment$' -benchtime=1x -count=5 -cpu=1
```

两个 DSN 必须显式指向同一个专用测试数据库。每组创建合成模型；每个逻辑操作
可能包含多个 HTTP 请求，`http-requests` 是实际请求数。吞吐包含已接受转换任务的
完成时间；`conversions` 与 `conversion-ns` 来自真实 Node 进程，不是队列入队时间。
冷组使用新的应用环境目录，仍允许 uv 使用解释器与 wheel 缓存；热组复用同一环境。
`SOURCEAEC_PYTHON` 用于网关预热负载，冷/热环境组主动使用 uv 准备固定依赖。

报告保留全部成功、冲突、过载、超时和其他错误；`http-p99-ns` 包含拒绝请求，
`http-ok-p99-ns` 仅包含成功请求。应同时比较 `http-ok/s`、`http-build-ok/s` 及
成功数量，不能把快速拒绝形成的总请求吞吐当作几何吞吐提升。
阶段值是累计次数和纳秒；CPU 是本进程与已回收子进程的 rusage，Go 分配不包含
外部几何库的原生堆。样本量、工作负载和机器预算应随结果记录，不设置共享 CI 的绝对 QPS 门槛。

需要 profile 时，将所有生成物放到临时目录：

```bash
profile_dir=$(mktemp -d)
SOURCEAEC_BENCH_GEOMETRY=1 go test ./internal/api -run '^$' \
  -bench '^BenchmarkNativeHTTP/models=32$' -benchtime=200x -cpu=8 \
  -o "$profile_dir/api.test" -cpuprofile="$profile_dir/cpu.pprof" \
  -memprofile="$profile_dir/heap.pprof" -blockprofile="$profile_dir/block.pprof" \
  -mutexprofile="$profile_dir/mutex.pprof"
```

## 大模型与长时间运行

已提交源的整份读取与文件快照可单独比较：

```bash
go test ./internal/artifact -run '^$' -bench '^BenchmarkArtifactRead$' \
  -benchtime=1x -count=5 -benchmem
```

该基准要求专用 `VIEWER_TEST_PG_DSN`，使用 1/32/128/256 MiB 可压缩合成内容。
`B/op` 为累计分配而非峰值；PostgreSQL 导出、临时文件与慢客户端须另外观察。

版本历史成本可用以下基准独立测量：

```bash
go test ./internal/editing -run '^$' -bench '^BenchmarkSaveHistory$' \
  -benchtime=1x -count=3 -benchmem -timeout=10m
```

它通过真实保存入口准备 1/100/1000 个版本，使用极小确定性几何结果隔离历史开销。
准备过程不计入测量；`initial-bundle-bytes` 是测量开始前的仓库大小。
每次测量会追加版本，因此应使用固定操作数，并与相同历史长度、格式和环境对比。

小模型的吞吐不能证明大模型的内存效率。按产品大小、历史长度、热点/多模型和并发
分别测量，加入慢下载与构建/比较混合负载，并记录成功构建数量与冲突/过载比例。
同时观察 Go heap、Python/Node RSS、PostgreSQL、磁盘及临时目录；
采样进程树 RSS 不包含数据库，也不等同于系统唯一物理内存。

性能回归测试应使用匹配的环境与工作负载，故障注入另行运行。
运行前先完成[正确性验证](/development/testing)，避免把错误或快速拒绝当成性能收益。
