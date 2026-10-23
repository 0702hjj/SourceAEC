#!/usr/bin/env bash
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
mkdir -p "$ROOT/bin"
cd "$ROOT/server"
CGO_ENABLED=0 go build -o "$ROOT/bin/sourceaec-runtime" ./cmd/edit-runtime
