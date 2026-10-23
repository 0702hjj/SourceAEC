#!/usr/bin/env bash
# Python source belongs to disposable runtime workspaces, never this repository.
set -euo pipefail
cd "$(dirname "$0")/.."
tracked=$(git ls-files '*.py' '*.pyi' '*.pyc' '*.pyo')
if [[ -n "$tracked" ]]; then
  printf 'Maintained Python source is forbidden:\n%s\n' "$tracked" >&2
  exit 1
fi
if git ls-files | rg '(^|/)(pyproject\.toml|requirements[^/]*\.txt|uv\.lock)$'; then
  echo 'Python application manifests are forbidden; configure external runtimes instead.' >&2
  exit 1
fi
echo 'Go business boundary passed'
