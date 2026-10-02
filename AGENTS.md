# SourceAEC Contributor Guide

SourceAEC is a self-hosted AI-editable AEC platform. It supports IFC and
CAD/DXF workflows through Python build scripts, a Go gateway, a React web
client, and two peer editing services. This file applies to the whole
repository.

## Components

| Component | Directory | Verification | Development command |
| --- | --- | --- | --- |
| Web client | `web` | `npm test`; `npm run lint`; `npm run build` | `npm run dev` |
| Go gateway | `server` | `go test ./...`; `go vet ./...` | `go run ./cmd/server` |
| IFC converter | `converter` | `npm test` | invoked by the gateway |
| IFC edit service | `services/ifc` | `uv run --group dev pytest` | `uv run uvicorn app.main:app --port 8100` |
| CAD edit service | `services/cad` | `uv run --group dev pytest` | `uv run uvicorn app.main:app --port 8200` |
| Shared sandbox | `services/sandbox` | `uv run --group dev pytest` | imported by both edit services |
| Shared edit API | `services/editapi` | run `../editapi/tests` from both service environments | imported by both edit services |
| MCP bridge | `mcp` | `uv run --group dev pytest` | `uv run python -m app.server` |
| Documentation | `docs` | `npm run docs:build` | `npm run docs:dev` |

Use the same absolute `VIEWER_DATA_DIR` for the gateway and both edit
services. The production gateway serves `web/dist`; the Vite server is for
development only.

## Change Discipline

- Add focused tests for every behavior change. For bug fixes, reproduce the
  failure first, then make the test pass.
- Keep tests beside their implementation (`*_test.go`, `*.test.ts(x)`, or
  `test_*.py`). Wait for asynchronous writes with a condition and timeout,
  never a fixed sleep.
- Run the suites for every component you change. Before a pull request, also
  run `scripts/check_file_size.sh` and `scripts/check_public_snapshot.sh`.
- Source and documentation files must stay below 500 lines unless listed in
  `scripts/file_size_whitelist.txt`. Split by responsibility rather than
  compressing formatting.
- Keep changes scoped. Do not commit runtime data, build output, dependency
  directories, caches, or generated files outside their documented source.

## API And Domain Rules

- The Go gateway is the public entry point. API responses use the envelope
  `{code, message, data}`, with `code=0` for success.
- Model IDs match `^m_[0-9a-f]{16}$`.
- Editing is script-as-source: stage a script, run it in the sandbox, then
  save an immutable version. Human and AI edits use the same API and differ
  only in provenance.
- Put business validation in focused `verify*` or `validate*` functions.
  Handlers decode, call validation/domain logic, and translate errors.
- Keep IFC and CAD behavior aligned through the shared sandbox and edit API
  packages; service-specific differences belong in their profiles.

## Security

- Production script execution is fail-closed and requires bubblewrap. The
  rlimit backend is test-only and must not be used as production isolation.
- Bind the unauthenticated Python edit services to loopback. Expose clients
  through the Go gateway and configure its token and CORS allowlist for
  production.
- Never commit credentials, customer names, real-project drawings or derived
  data, private repository references, developer-machine paths, internal
  plans, or incident notes. Use synthetic fixtures only.
- Treat `data/` as runtime state. Do not edit or commit it manually.

## Documentation

Public documentation lives in `docs/site` and is published at
<https://0702hjj.github.io/SourceAEC/>. Use root-relative VitePress links;
the configured site base is `/SourceAEC/`. Repository, Issue, license, and
contribution links must target <https://github.com/0702hjj/SourceAEC>.

Build locally before publishing:

```bash
cd docs
npm install
npm run docs:build
```

Do not add internal planning or project-management material to the public
documentation tree.

## Licensing

The repository is primarily Apache-2.0. The self-contained `skills/aiplan`
and `skills/aidxf` directories are MIT-licensed by default; an individual
file's SPDX identifier overrides the directory default. Preserve attribution
and license files when changing or packaging them. The web client includes
AGPL-3.0 xeokit and MPL-2.0 web-ifc assets; review `NOTICE` before distributing
frontend builds.
