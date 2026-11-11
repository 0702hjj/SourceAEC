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

完整网关负载使用真实几何、认证 HTTP、暂存、快照保存、产品比较、下载与 Node 转换：

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

下载快照的并发与限速读取矩阵：

```bash
go test ./internal/editing -run '^$' -bench '^BenchmarkSnapshotConcurrency$' \
  -benchtime=32x -count=3 -benchmem -timeout=20m
```

固定 1/32/128/256 MiB、1/4/8/16/32 个客户端，每批同时发起请求，等待该批结束后再发起。
操作数至少 32 才能覆盖最大并发；限速组以 2ms 节拍读取最多 1 MiB 的块。
仅包含数据库快照导出和文件读取，不包含 HTTP、模型语义或几何执行。
报告成功数量/吞吐/延迟、过载数量、连接池等待及 Go RSS；拒绝请求不算成功吞吐。
小样本延迟分位数仅作描述，不是尾延迟 SLO；各组复用同一测试进程，RSS 可受前组影响。
PG 与临时磁盘必须独立采样，不能把 Go RSS 或累计分配当作整个系统内存。

版本历史成本可用以下基准独立测量：

```bash
go test ./internal/editing -run '^$' -bench '^BenchmarkSaveHistory$' \
  -benchtime=1x -count=3 -benchmem -timeout=10m
```

它通过真实保存入口准备 1/100/1000 个版本，使用极小确定性几何结果隔离历史开销。
准备过程不计入测量；`initial-bundle-bytes` 是测量开始前的仓库大小。
快照保存不会自动构建 Git bundle，因此该值可以为零；按需 Git 查询不在该保存基准内。
每次测量会追加版本，因此应使用固定操作数，并与相同历史长度、格式和环境对比。

小模型的吞吐不能证明大模型的内存效率。按产品大小、历史长度、热点/多模型和并发
分别测量，加入慢下载与构建/比较混合负载，并记录成功构建数量与冲突/过载比例。
同时观察 Go heap、Python/Node RSS、PostgreSQL、磁盘及临时目录；
采样进程树 RSS 不包含数据库，也不等同于系统唯一物理内存。

性能回归测试应使用匹配的环境与工作负载，故障注入另行运行。
运行前先完成[正确性验证](/operations/testing)，避免把错误或快速拒绝当成性能收益。

## PSS、句柄与匿名临时文件

`SnapshotConcurrency` 和 `NativeHTTP` 另以 250ms 间隔采样 Linux `/proc`。
可将 `SOURCEAEC_BENCH_PG_PID` 设为专用测试数据库的 postmaster PID，以纳入其进程树；
未设置时 `observation-pg-included=0`，不能将结果当作包含数据库的系统测量。
每个进程只计一次，子进程从全部线程发现。报告 PSS、FD，以及已删除但仍打开的普通文件
逻辑字节和分配字节；同一 device/inode 的多个描述符不重复计算文件空间。

`sampled-pss-bytes` 是比例分摊共享页后的观察值，不是内存硬上限。
各项峰值独立取最大，不保证同时发生。`observation-samples`、`sampled-pss-read` 和
`observation-read-errors` 一并检查；进程退出、描述符关闭或权限不足会造成读取失败。
短峰可能漏采，采样自身也有 CPU/I/O 成本；命名临时目录和 PG 持久/TOAST 空间仍须另外测量。

快照矩阵可选择 SHA256 序列构成的不可压缩合成字节，并检查实际 PG 存储长度：

```bash
SOURCEAEC_BENCH_ENTROPY=sha256 go test ./internal/editing -run '^$' \
  -bench '^BenchmarkSnapshotConcurrency$' -benchtime=512x -count=3 -timeout=30m
```

默认仍使用可压缩内容；该开关不代表真实几何。每组使用独立临时根命名空间，结束后删除自身
测试 schema。成功 p95/p99 与过载计数分别报告；固定批次仍有批间屏障，不等同于持续到达负载。
