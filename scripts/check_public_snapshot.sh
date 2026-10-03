#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
cd "$ROOT"

forbidden_paths=(
  docs/internal
  docs/work
  docs/superpowers
  docs/.internal
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

if rg -n -i --pcre2 \
  '(AI_BIM|SimpleCADAPI-archive|cyvol0521|gaiahub|(?<!Simple)CADapi|IFC_front|wanda|万达|前排观景楼王|真实项目|真实图纸|gaia 商用|github\.com/0702hjj/AI_IFC|0702hjj\.github\.io/AI_IFC)' \
  --hidden --glob '!.git' --glob '!**/.git/**' --glob '!scripts/check_public_snapshot.sh' \
  --glob '!**/node_modules/**' --glob '!**/.vitepress/dist/**' \
  --glob '!**/*.lock' --glob '!**/*.ifc' --glob '!**/*.dxf' .; then
  echo "private marker found" >&2
  exit 1
fi

mapfile -t public_docs < <(git ls-files '*.md' '.github/**' 'docs/site/**')
if rg -n -i \
  '(docs/(internal|work|superpowers)|aibim-orchestrator|aiblueprint-mcp|W-[0-9]{4}|私有归档仓)' \
  "${public_docs[@]}"; then
  echo "internal documentation marker found" >&2
  exit 1
fi

if rg -n \
  '(/home/[A-Za-z0-9._-]+|/Users/[A-Za-z0-9._-]+|[A-Za-z]:\\Users\\[A-Za-z0-9._-]+)' \
  --hidden --glob '!.git' --glob '!**/.git/**' \
  --glob '!**/node_modules/**' --glob '!**/.vitepress/dist/**' \
  --glob '!**/*.lock' --glob '!**/*.ifc' --glob '!**/*.dxf' .; then
  echo "developer absolute path found" >&2
  exit 1
fi

if rg -n \
  '(sk-[A-Za-z0-9_-]{16,}|gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|AKIA[0-9A-Z]{16}|-----BEGIN (RSA |EC |OPENSSH |DSA )?PRIVATE KEY-----)' \
  --hidden --glob '!.git' --glob '!**/.git/**' \
  --glob '!**/node_modules/**' --glob '!**/.vitepress/dist/**' \
  --glob '!**/*.lock' --glob '!**/*.ifc' --glob '!**/*.dxf' .; then
  echo "credential-like value found" >&2
  exit 1
fi

echo "public snapshot audit passed"
