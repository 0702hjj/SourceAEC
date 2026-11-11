#!/usr/bin/env bash
# SPDX-License-Identifier: Apache-2.0
set -euo pipefail
root=$(git -C "$(dirname "$0")/.." rev-parse --show-toplevel)
gate_root=$(mktemp -d "${TMPDIR:-/tmp}/sourceaec-size-gate.XXXXXX")
trap 'rm -rf -- "$gate_root"' EXIT
mkdir -p "$gate_root/scripts" "$gate_root/docs/openapi/go-server/paths" "$gate_root/docs/site/public"
cp "$root/scripts/check_file_size.sh" "$root/scripts/file_size_whitelist.txt" "$gate_root/scripts/"
# Keep this fixture independent of extra developer tools such as ripgrep.
mkdir "$gate_root/bin"
for executable in git bash dirname mktemp mkdir cp awk mv rm wc grep; do
  ln -s "$(command -v "$executable")" "$gate_root/bin/$executable"
done
export PATH="$gate_root/bin"
git -C "$gate_root" init --quiet
# Valid JSON with 501 lines, first as a generated output with its bounded source elsewhere.
awk 'BEGIN { for (i=0;i<500;i++) print ""; print "{}" }' > "$gate_root/docs/site/public/go-server.openapi.json"
git -C "$gate_root" add scripts docs
run_gate() { (cd "$gate_root" && bash scripts/check_file_size.sh); }
run_gate > "$gate_root/output" 2>&1
cp "$gate_root/docs/site/public/go-server.openapi.json" "$gate_root/docs/openapi/go-server/paths/probe.json"
git -C "$gate_root" add docs/openapi
if run_gate > "$gate_root/output" 2>&1; then
  echo 'oversized maintained contract source incorrectly passed' >&2
  exit 1
fi
grep -qF 'docs/openapi/go-server/paths/probe.json' "$gate_root/output"
printf '{}\n' > "$gate_root/docs/openapi/go-server/paths/probe.json"
grep -vFx 'docs/site/public/go-server.openapi.json' "$gate_root/scripts/file_size_whitelist.txt" > "$gate_root/whitelist"
mv "$gate_root/whitelist" "$gate_root/scripts/file_size_whitelist.txt"
if run_gate > "$gate_root/output" 2>&1; then
  echo 'unregistered oversized generated contract incorrectly passed' >&2
  exit 1
fi
grep -qF 'docs/site/public/go-server.openapi.json' "$gate_root/output"
echo 'file-size gate regression checks passed'
