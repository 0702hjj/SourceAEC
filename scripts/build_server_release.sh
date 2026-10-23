#!/usr/bin/env bash
# Build the server-only Linux distribution; no frontend or Python environment.
set -euo pipefail
ROOT=$(cd "$(dirname "$0")/.." && pwd)
VERSION=${1:?usage: build_server_release.sh VERSION}
if [[ ! "$VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo 'VERSION must be MAJOR.MINOR.PATCH' >&2
  exit 1
fi
OUT="$ROOT/dist/server-v$VERSION"
mkdir -p "$OUT"
STAGE=$(mktemp -d)
trap 'rm -rf "$STAGE"' EXIT
cd "$ROOT/server"
for arch in amd64 arm64; do
  name="sourceaec_${VERSION}_linux_${arch}"
  package="$STAGE/$name"
  mkdir -p "$package/bin" "$package/server"
  for command in server edit-runtime sourceaec-mcp; do
    binary=sourceaec-server
    if [[ "$command" == edit-runtime ]]; then binary=sourceaec-runtime; fi
    if [[ "$command" == sourceaec-mcp ]]; then binary=sourceaec-mcp; fi
    CGO_ENABLED=0 GOOS=linux GOARCH="$arch" go build -trimpath \
      -ldflags='-s -w' -o "$package/bin/$binary" "./cmd/$command"
    go version -m "$package/bin/$binary" > "$package/bin/$binary.buildinfo.txt"
  done
  cp "$ROOT/server/server_config.example.json" "$package/server/"
  cp "$ROOT/LICENSE" "$ROOT/NOTICE" "$package/"
  cp "$ROOT/scripts/server_release_README.md" "$package/README.md"
  tar -C "$STAGE" -czf "$OUT/$name.tar.gz" "$name"
done
cd "$OUT"
sha256sum sourceaec_"$VERSION"_linux_*.tar.gz > SHA256SUMS
printf 'Server release artifacts: %s\n' "$OUT"
