# SourceAEC Contributor Guide

SourceAEC is a self-hosted, agent-friendly interface for IFC operations. It
supports IFC authoring, editing, inspection, and versioning through Python
build scripts, REST APIs, and Agent Skills. Planning and CAD/DXF workflows are
optional extensions. This file applies to the whole repository.

## Components

| Component | Directory | Verification | Development command |
| --- | --- | --- | --- |
| Web client | `web` | `npm test`; `npm run lint`; `npm run build` | `npm run dev` |
| Go gateway and business runtime | `server` | `go test ./...`; `go vet ./...` | `go run ./cmd/server` |
| IFC converter | `converter` | `npm test` | invoked by the gateway |
| MCP bridge | `server/cmd/sourceaec-mcp` | included in Go tests | `go run ./cmd/sourceaec-mcp` |
| Documentation | `docs` | `npm run docs:build` | `npm run docs:dev` |

All business code, including editing, staging, validation, sandbox orchestration,
versioning, persistence, planning delivery and MCP integration, is Go.
Do not add maintained Python source, Python tests, hooks, SDKs or application
manifests. Python is only an external geometry runtime: user/agent-authored
model source is materialized in a disposable sandbox and produces model files.
Do not embed Python business logic in Go strings to evade this boundary.
Skills contain declarations and references only; external geometry libraries
(such as IfcOpenShell, ezdxf or SimpleCADAPI) are not vendored into this repository.

PostgreSQL is required. Set `VIEWER_PG_DSN`; tests require a dedicated database,
and native review-store tests also use `VIEWER_TEST_PG_DSN`.
The gateway runs editing in-process, serves `web/dist` in production, and starts
no Python HTTP services. `SOURCEAEC_PYTHON` may select an administrator-provided
geometry interpreter; otherwise Go prepares pinned environments using uv.

## Change Discipline

- Add focused tests for every behavior change. For bug fixes, reproduce the
  failure first, then make the test pass.
- Keep tests beside their implementation (`*_test.go`, `*.test.ts(x)`, or
  shell integration checks). Wait for asynchronous writes with a condition and timeout,
  never a fixed sleep.
- Run the suites for every component you change. Before a pull request, also
  run `scripts/check_file_size.sh`, `scripts/check_public_snapshot.sh` and
  `scripts/check_go_business.sh`.
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
- Keep IFC and CAD behavior aligned through `server/internal/editing`;
  format-specific differences belong in focused adapters.

## Security

- Production script execution is fail-closed and requires bubblewrap. The
  rlimit backend is test-only and must not be used as production isolation.
- Expose clients through the Go gateway and configure its token and CORS
  allowlist for production. External MCP clients use the authenticated gateway.
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
