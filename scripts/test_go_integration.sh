#!/usr/bin/env bash
set -euo pipefail

# A dedicated test database is mandatory; never infer a deployment DSN.
: "${VIEWER_TEST_PG_DSN:?set VIEWER_TEST_PG_DSN to a dedicated test database}"
export VIEWER_PG_DSN="$VIEWER_TEST_PG_DSN"
export SOURCEAEC_TEST_GEOMETRY=1
for executable in go git bwrap prlimit uv; do
  command -v "$executable" >/dev/null || {
    echo "missing integration prerequisite: $executable" >&2
    exit 1
  }
done

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT/server"
go test -count=1 ./internal/editing ./internal/api ./internal/mcpbridge
go test -race -count=5 ./internal/editing -run '^Test(ConcurrentInstancesRejectLostUpdate|CanceledModelLockWaitReleasesConnection)$'
