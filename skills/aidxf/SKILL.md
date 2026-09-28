---
name: aidxf
description: Convert plan.json inputs into layered DXF drawings with declarative skeleton and room DSLs plus deterministic validation and rendering.
version: 3.0.0
license: MIT
compatibility: Self-contained Python packages with ezdxf and shapely runtime dependencies.
metadata:
  project: aidxf
---

# aidxf

Use this skill to turn a validated building plan into per-zone DXF drawings.
The agent declares intent in JSON; the CLI performs coordinate derivation,
schema validation, geometry checks, rendering, and readback.

## Workflow

1. Run `aidxfv3 preprocess --plan <plan.json> --out <derived>`.
2. Create and validate `skeleton.json` against `references/schemas/skeleton.schema.json`.
3. Normalize and check the skeleton with `aidxfv3 normalize` and `aidxfv3 check`.
4. Create `rooms.json`, render the DXF, then run readback and reconciliation.
5. Deliver the generated build script and model through the platform script API.

## Public References

- `references/schemas/`: plan, skeleton, room, and building contracts.
- `references/design/`: output and drawing conventions.
- `references/orchestrator/`: generic execution and review flow.
- `references/draw_api.md`: supported drawing calls.
- `references/machine_contract.md`: CLI behavior and exit codes.
- `references/examples/`: synthetic examples.

The public distribution intentionally contains no real-project reference
library. `goldlib` supports optional user-owned reference collections, but
the core preprocessing, validation, drawing, and delivery path does not
require one.

## Commands

```text
aidxfv3 preprocess --plan <plan.json> --out <derived>
aidxfv3 validate --dsl <skeleton-or-rooms.json>
aidxfv3 normalize --dsl <skeleton.json>
aidxfv3 check --dsl <skeleton.json> --plan <plan.json>
aidxfv3 readback --dxf <floor.dxf>
aidxfv3 reconcile --decl <rooms.json> --graph <readback.json>
aidxfv3 state sync --project <dir>
aidxfv3 state reconcile --project <dir>
```

Do not modify the input `plan.json`. Treat generated JSON and command exit
codes as workflow state, and stop for user review before committing a layout.

