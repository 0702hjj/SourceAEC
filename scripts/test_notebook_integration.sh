#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail
: "${VIEWER_TEST_PG_DSN:?set a dedicated test database}"
: "${SOURCEAEC_TEST_NOTEBOOK_PYTHON:?provide an external managed SimpleCADAPI interpreter}"
export VIEWER_PG_DSN="$VIEWER_TEST_PG_DSN"
unset VIEWER_STORAGE_NAMESPACE
for executable in go bwrap prlimit; do
  command -v "$executable" >/dev/null || { echo "missing notebook integration prerequisite: $executable" >&2; exit 1; }
done
"$SOURCEAEC_TEST_NOTEBOOK_PYTHON" -I -B -c 'import importlib.metadata; assert importlib.metadata.version("simplecadapi") == "2.2.0b1"'
repo_dir=$(cd "$(dirname "$0")/.." && pwd)
cd "$repo_dir/server"
go test -count=1 ./internal/notebookcache ./internal/editing -run '^Test(Archive|Generation|Notebook)'
go test -race -count=5 ./internal/notebookcache ./internal/editing -run '^Test(Archive|Generation|NotebookPolicy|NotebookReport|NotebookAddressSpace|NotebookWorkflow)'
