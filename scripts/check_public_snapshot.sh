#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

forbidden_paths=(
  AGENTS.md
  docs
  research
  img
  skills/aibim-orchestrator
  skills/aidxf/references/golden
  skills/aiplan/references/golden
)

for path in "${forbidden_paths[@]}"; do
  if [[ -e "$path" ]]; then
    echo "forbidden public path: $path" >&2
    exit 1
  fi
done

if rg -n -i \
  '(AI_BIM|SimpleCADAPI-archive|cyvol0521|gaiahub|CADapi|IFC_front|wanda|万达|前排观景楼王|真实项目|真实图纸|gaia 商用)' \
  --glob '!scripts/check_public_snapshot.sh' \
  --glob '!**/*.lock' --glob '!**/*.ifc' --glob '!**/*.dxf' .; then
  echo "private marker found" >&2
  exit 1
fi

if rg -n \
  '(/home/[A-Za-z0-9._-]+|/Users/[A-Za-z0-9._-]+|[A-Za-z]:\\Users\\[A-Za-z0-9._-]+)' \
  --glob '!**/*.lock' --glob '!**/*.ifc' --glob '!**/*.dxf' .; then
  echo "developer absolute path found" >&2
  exit 1
fi

if rg -n \
  '(sk-[A-Za-z0-9_-]{16,}|gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|AKIA[0-9A-Z]{16}|-----BEGIN (RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----)' \
  --glob '!**/*.lock' --glob '!**/*.ifc' --glob '!**/*.dxf' .; then
  echo "credential-like value found" >&2
  exit 1
fi

echo "public snapshot audit passed"
