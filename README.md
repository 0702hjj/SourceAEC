# SourceAEC

[中文](README.zh-CN.md)

An open-source, self-hosted, agent-friendly interface for creating and editing
IFC and CAD/DXF models. SourceAEC gives people and AI agents the same
script-as-source APIs, sandboxed execution workflow, and semantic version
history, so model operations are explicit, reviewable, and portable.

> Documentation: <https://0702hjj.github.io/SourceAEC/>

## Highlights

- IFC authoring and editing with IfcOpenShell.
- CAD/DXF editing with ezdxf and a browser canvas viewer.
- Python build scripts are the model source of truth.
- Sandboxed trial runs and immutable version snapshots.
- Attribute-level semantic diffs keyed by IFC GlobalId.
- React frontend, Go gateway, and peer Python edit services.
- In-process AI agent with deterministic offline fallback.

## Architecture

```text
Browser (React, xeokit/web-ifc, Fabric)
        |
        v
Go gateway :8090 ----> IFC service :8100 ----> IfcOpenShell
        |              CAD service :8200 ----> ezdxf
        |                       |
        +----> converter        +----> shared sandbox/edit API packages
        +----> in-process AI agent
```

## Screenshots

| Model library | IFC properties |
|---|---|
| ![Model library](assets/screenshots/library.png) | ![IFC properties](assets/screenshots/properties.png) |

| Version diff | AI chat |
|---|---|
| ![Version diff](assets/screenshots/diff.png) | ![AI chat](assets/screenshots/chat.png) |

## Quick Start

Requirements include Go 1.26, Node.js 22, Python 3.10, `uv`, and bubblewrap
on Linux. Production script execution fails closed when bubblewrap is absent.

```bash
cd converter && npm install
cd ../web && npm install
cd ../services/ifc && uv sync
cd ../cad && uv sync
```

Run the two Python services, the Go gateway, and the Vite development server:

```bash
# terminal 1
cd services/ifc
VIEWER_DATA_DIR="$(realpath -m ../../data)" uv run uvicorn app.main:app --port 8100

# terminal 2
cd services/cad
VIEWER_DATA_DIR="$(realpath -m ../../data)" uv run uvicorn app.main:app --port 8200

# terminal 3
cd server && go run ./cmd/server

# terminal 4
cd web && npm run dev
```

Both Python services and the Go gateway must use the same absolute data
directory. An empty `VIEWER_LLM_API_KEY` selects the deterministic offline
agent model.

## Repository Layout

```text
web/               React frontend
server/            Go REST gateway and in-process agent
converter/         IFC-to-XKT converter
services/ifc/      IFC edit service
services/cad/      CAD edit service
services/sandbox/  shared sandbox runtime
services/editapi/  shared script editing API
mcp/               optional MCP bridge
skills/             AI authoring and planning tools
tools/              packaging and agent debugging tools
examples/           synthetic examples
```

## Development

Run the suite for every component you change. The primary commands are:

```bash
cd web && npm test && npm run lint && npm run build
cd server && go test ./... && go vet ./...
cd converter && npm test
cd services/ifc && uv run --group dev pytest
cd services/cad && uv run --group dev pytest
cd services/sandbox && uv run --group dev pytest
cd mcp && uv run --group dev pytest
scripts/check_file_size.sh
scripts/check_public_snapshot.sh
```

See [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request.

Documentation is maintained with the source:

```bash
cd docs
npm install
npm run docs:dev
npm run docs:build
```

## Development Provenance

SourceAEC is an independent implementation based on public technical
standards, documented open-source interfaces, public research, and functional
and design specifications independently prepared by the maintainer. Its
implementation was produced iteratively by AI coding agents through reviewed
pull requests. No private or confidential third-party repository, source
code, internal document, customer data, drawing, or other non-public technical
material was supplied to those agents, used as an implementation input, or
copied into this project. The repository history and pull requests preserve
the development record.

Contributions must have a documented, lawful provenance. Contributors certify
their right to submit each contribution, disclose incorporated third-party
material and its license, and must not submit confidential information or
trade secrets. See [CONTRIBUTING.md](CONTRIBUTING.md) for the DCO sign-off and
provenance requirements. If you believe material in this repository infringes
your rights, please open an Issue with the affected path and the basis of your
claim so it can be investigated promptly. Do not disclose confidential
materials or personal data in a public Issue; use a private GitHub security
report or private maintainer contact when available.

## Use And Warranty Boundary

SourceAEC is a developer tool and reference implementation, not a safety-
critical, regulatory, surveying, or construction approval system. Generated
IFC/DXF content, AI suggestions, conversions, and sandbox results require
independent human review before they are used in design, procurement,
construction, operation, or compliance decisions. The project does not make
any promise that an output is accurate, fit for a particular purpose,
interoperable with every downstream tool, compliant with a law or standard,
or free of third-party rights claims. The Apache-2.0 license and the notices
for bundled dependencies govern the applicable warranty and liability terms.

## License

SourceAEC is licensed under Apache-2.0, with nested MIT-licensed skills and
third-party runtime components described in [NOTICE](NOTICE). The frontend
depends on AGPL-3.0 xeokit, and the tracked web-ifc WASM files remain subject
to MPL-2.0.
