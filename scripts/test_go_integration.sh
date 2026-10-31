#!/usr/bin/env bash
set -euo pipefail

# A dedicated test database is mandatory; never infer a deployment DSN.
: "${VIEWER_TEST_PG_DSN:?set VIEWER_TEST_PG_DSN to a dedicated test database}"
export VIEWER_PG_DSN="$VIEWER_TEST_PG_DSN"
# Each temporary test root needs its own schema, regardless of shell settings.
unset VIEWER_STORAGE_NAMESPACE
export SOURCEAEC_TEST_GEOMETRY=1
export SOURCEAEC_TEST_CONVERTER=1
for executable in go git bwrap prlimit uv node; do
  command -v "$executable" >/dev/null || {
    echo "missing integration prerequisite: $executable" >&2
    exit 1
  }
done

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT/server"
go test -count=1 ./internal/artifact ./internal/store ./internal/convert ./internal/editruntime ./internal/editing ./internal/api ./internal/mcpbridge
go test -race -count=5 ./internal/artifact ./internal/store ./internal/convert ./internal/editruntime ./internal/editing ./internal/api ./internal/agent -run '^Test(ProductComparison|SeparateConverter|AgentNative|NativeMCP|Disconnected|FailedProjection|Interrupted|GitWorkspace|GitPreserves|DownloadUses|RenderUses|Preview|Viewer|DuplicateConversion|ConcurrentQueues|Deletion|DeletedModel|ModelList|ConcurrentInstances|ConcurrentSaves|CanceledExecution|Execution|Detached|Commit|CanceledModel|IndependentNamespaces|BlockedModel|HotModel|ModelAdmission|SharedModelAdmission|DerivedWaiters|ModelRecovery|NativeArtifacts|Version|LegacyProduct|Conversion|Enqueue|FullQueue|Shutdown|Recovery|Projection|ScriptRead|ScriptSnapshot|EditingProcessCrash|EditingUsesInput|BuildCollection|BuildInputValidation|CollectBuildInputs|Sandbox)'
