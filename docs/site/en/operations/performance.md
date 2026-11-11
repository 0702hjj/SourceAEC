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
PostgreSQL persistent/TOAST space still requires separate measurement.

The same observer records Go heap allocation/in-use, peak goroutines and initial/final values, excluding its own
goroutine. Host `/proc/meminfo` supplies minimum `MemAvailable` and maximum `Shmem`; these machine-wide values
can reflect unrelated workloads. With an explicit postmaster, verify its PID and data directory and count named
files under default `base/pgsql_tmp`, reporting logical and allocated bytes with inode deduplication. Custom
tablespaces, access failures or traversal limits make coverage incomplete. This excludes WAL, persistent tables,
other temporary directories and unlinked files. `observation-host-samples` and `observation-pg-temp-samples`
count valid observations; omit their value metrics when coverage is zero instead of reporting missing data as zero.
Initial/final sampling is synchronous, but brief peaks can still be missed. Go heap excludes Python/Node native
heaps; host, PSS and file-byte observations cannot simply be added together.

Use incompressible synthetic SHA256 byte sequences for snapshot reads:

```bash
SOURCEAEC_BENCH_ENTROPY=sha256 go test ./internal/editing -run '^$' \
  -bench '^BenchmarkSnapshotConcurrency$' -benchtime=512x -count=3 -timeout=30m
```

The default remains compressible contents; this option is not real geometry. Stored size is checked outside timed reads.
Each case removes only its own temporary-root test schema. Successful p95/p99 and overload counts are separate.
Fixed bursts have inter-batch barriers and do not represent a continuous arrival process.


Temporary directories may reside on tmpfs: allocated file pages then consume memory and unmapped pages may not
appear in process PSS. Record the temporary filesystem type and host memory as well; adding PSS and file bytes
does not establish unique physical memory use.

## Stable builds after producers stop

Set `SOURCEAEC_BENCH_RECOVERY=1` for `BenchmarkNativeHTTP` to enable a separate recovery phase.
After all concurrent producers join and the mixed timer stops, build each model sequentially,
require an unchanged source revision, drain real Node conversion and validate the current IFC preview.
Failure fails the benchmark. `recovery-*` metrics report successful counts, per-format latency, CPU and PSS/FD
separately; these successes are not added to mixed throughput. No further mutation is produced during recovery.
Percentiles from small recovery cohorts are descriptive and do not replace sustained mixed-load or large-geometry validation.

The single hot model defaults to IFC. Set `SOURCEAEC_BENCH_HOT_FORMAT=dxf` for the same DXF hot-model scenario;
the 32-model group continues to alternate both formats.


Native HTTP benchmarks require an unset `VIEWER_STORAGE_NAMESPACE` and isolate each temporary root. After HTTP
and conversion workers shut down, remove only that case namespace to avoid accumulating models across repeats.

## On-demand Git projection of long histories

`BenchmarkSaveHistory` excludes Git export. Measure first projection and subsequent queries separately:

```bash
go test ./internal/editing -run '^$' -bench '^BenchmarkGitHistoryProjection$' \
  -benchtime=1x -count=3 -timeout=30m
```

Prepare 1/100/1000 versions; `entropyBytes=65536` adds 64 KiB of changing hexadecimal comments per version.
Hex text remains partly compressible, unlike the binary download fixture. Tiny deterministic geometry isolates
history cost and preparation is untimed. `cold-*` reports cumulative initial time, CPU, sampled resources and
429 retries that retain committed progress, with a five-minute overall deadline. Standard ns/op and `warm-*`
measure queries after projection completes. CPU includes Go and reaped Git children, excluding independent PG CPU;
PG inclusion in PSS requires an explicit postmaster PID and is reported separately. Verify version count and Git
references, record actual bundle size, and remove only the case's test namespace. These results are not real
geometry throughput; sampling can miss brief Git processes and temporary files.

`pg-tables-before-bytes` and `pg-tables-after-cold-bytes` include the case namespace tables, indexes, TOAST
and unreclaimed dead pages. They exclude WAL and named temporary files; these two observations are not disk peaks.

## CPU accounting for isolated descendants

Default `cpu-reaped-only=1` counts Go and reaped children and may omit bubblewrap descendants; it is not full
geometry CPU. For a complete workload counter, use a dedicated cgroup v2 unit and explicitly opt in:

```bash
systemd-run --user --scope --unit=sourceaec-bench-native \
  env SOURCEAEC_BENCH_CPU_CGROUP=1 SOURCEAEC_BENCH_GEOMETRY=1 \
  SOURCEAEC_BENCH_RECOVERY=1 go test ./internal/api -run '^$' \
  -bench '^BenchmarkNativeHTTP$' -benchtime=30s -count=3 -cpu=4 -timeout=30m
```

Export dedicated database DSNs and the external interpreter first; run this from `server`. The reader requires
the current `sourceaec-bench-*` scope/service and its readable `cpu.stat`, rejecting shared terminal groups.
`cpu-cgroup=1` includes exited and live descendants in that unit. Reserve it for the workload. Independent PG CPU
is excluded; including PG in PSS does not change the CPU boundary. Phase totals do not attribute each rejection's
individual cost. Run `TestCgroupCPUIncludesIsolatedGeometry` inside the same kind of dedicated unit with the
geometry interpreter and `SOURCEAEC_TEST_GEOMETRY=1` to verify the accounting path.
