#!/usr/bin/env bash
# Native acceptance checks. Use a dedicated PostgreSQL database, never production.
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
: "${VIEWER_PG_DSN:?set VIEWER_PG_DSN to a dedicated test database}"
cd "$ROOT/server"
go test ./internal/editing ./internal/mcpbridge ./internal/releasepackage -count=1
