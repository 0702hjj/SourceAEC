# Performance and capacity

Measure matched workloads and report successful operations separately from conflicts, overload, timeout and other failures.
Fast rejection is not geometry throughput. Stage metrics are cumulative counts/times, not request percentiles.
All Go benchmarks below run from `server` with an explicit dedicated test database.

## Native and HTTP workloads

```bash
go test ./internal/editing -run '^$' -bench '^BenchmarkMixedNativeEditing$' \
  -benchtime=2000x -count=5 -cpu=1,4,8,16,32
SOURCEAEC_BENCH_GEOMETRY=1 go test ./internal/api -run '^$' \
  -bench '^BenchmarkNativeHTTP$' -benchtime=32x -count=5 -cpu=1,4,8,16,32
SOURCEAEC_BENCH_GEOMETRY=1 go test ./internal/api -run '^$' \
  -bench '^BenchmarkGeometryEnvironment$' -benchtime=1x -count=5 -cpu=1
```

MixedNativeEditing rotates reads, optimistic staging, execution and derived writes; its default executor is deterministic.
SOURCEAEC_BENCH_GEOMETRY=1 selects warmed real geometry with SOURCEAEC_PYTHON. Native tests exclude HTTP/Node/cold dependency setup.
NativeHTTP includes authenticated requests, staging, saves, comparisons, downloads and completed Node conversion.
Environment cold groups create fresh application environments but may reuse uv interpreter/wheel caches.

Report `http-ok/s`, `http-build-ok/s`, successful counts/latency, and rejection counts. `http-p99-ns` includes rejected requests;
`http-ok-p99-ns` does not. A logical operation can issue multiple HTTP requests. Do not infer a production SLO from short bursts.

## Large objects and history

```bash
go test ./internal/artifact -run '^$' -bench '^BenchmarkArtifactRead$' -benchtime=1x -count=5 -benchmem
go test ./internal/editing -run '^$' -bench '^BenchmarkSnapshotConcurrency$' \
  -benchtime=32x -count=3 -benchmem -timeout=20m
go test ./internal/editing -run '^$' -bench '^BenchmarkSaveHistory$' \
  -benchtime=1x -count=3 -benchmem -timeout=10m
```

ArtifactRead uses compressible synthetic 1/32/128/256 MiB contents. SnapshotConcurrency covers 1/4/8/16/32 clients,
fast/paced reads and at least 32 operations. It excludes HTTP and geometry. SaveHistory prepares 1/100/1000 versions with tiny
synthetic products to isolate history cost; preparation is untimed and Git projection is outside normal save measurements.

B/op is cumulative allocation, not peak memory. Process-tree RSS may double-count shared pages, miss short-lived children and exclude PG.
Observe Go heap, Python/Node native memory, PG/TOAST/materialization/temp files, persistent disk, file descriptors and post-load recovery.
Use realistic incompressible geometry, slow clients, mixed builds/comparisons, hot-model changes followed by producer shutdown,
and sustained samples for p95/p99. Fault injection is a separate workload.
Profiles and benchmark binaries belong in temporary directories, never committed runtime data.

## PSS, descriptors and unlinked temporary files

`SnapshotConcurrency` and `NativeHTTP` also sample Linux `/proc` every 250 ms.
Set `SOURCEAEC_BENCH_PG_PID` to the dedicated test database's postmaster PID to include its process tree.
Otherwise `observation-pg-included=0`: the sample excludes database memory. Processes are deduplicated and
children are discovered from every thread. Reports include PSS, descriptors and logical/allocated bytes of
unlinked regular files. Multiple descriptors for the same device/inode count file space once.

`sampled-pss-bytes` proportionally attributes shared pages; it is not a hard limit. Independent maxima need not
occur together. Check `observation-samples`, `sampled-pss-read` and `observation-read-errors`; exited processes,
closed descriptors and permissions can prevent reads. Short peaks may be missed and observation has CPU/I/O cost.
Named temporary directories and PostgreSQL persistent/TOAST space still require separate measurement.

Use incompressible synthetic SHA256 byte sequences for snapshot reads:

```bash
SOURCEAEC_BENCH_ENTROPY=sha256 go test ./internal/editing -run '^$' \
  -bench '^BenchmarkSnapshotConcurrency$' -benchtime=512x -count=3 -timeout=30m
```

The default remains compressible contents; this option is not real geometry. Stored size is checked outside timed reads.
Each case removes only its own temporary-root test schema. Successful p95/p99 and overload counts are separate.
Fixed bursts have inter-batch barriers and do not represent a continuous arrival process.
