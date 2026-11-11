# Optional external notebook cache

Use only when a task explicitly relies on an installed SimpleCADAPI notebook runtime.
SourceAEC still accepts IFC or ASCII DXF products from user/agent-authored wrapper scripts.
It does not export every upstream format or preserve Python stacks/per-cell crash progress.

Declare `tool.sourceaec.notebook` in PEP 723 metadata with adapter `simplecadapi-v1`,
up to 16 `skill-work/.../*.py` input paths and `reuse="declared"` only when reuse is safe.
The default `never` does not restore/persist state. Unobservable dynamic dependencies and
side effects require declared inputs or disabled reuse; do not infer determinism.

Read captured notebook inputs beneath `/inputs/`. Only server-owned, model/namespace-scoped
completed cache generations are restored into per-attempt writable `__marimo__` directories.
Never upload pickle/cache state or use the SDK cell key as a stable SourceAEC business ID.

After the external SDK fully returns, emit version-1 completion JSON to
`SOURCEAEC_NOTEBOOK_REPORT`: each notebook has path, captured-source SHA-256,
SDK definition.content_hash (`sha256:<64 hex>`) and cells with key/status ran/cached/skipped.
Continue writing the normal IFC/DXF product to the first process argument.

Detailed limits and the complete report contract are in the platform documentation:
https://0702hjj.github.io/SourceAEC/en/architecture/notebooks
Upstream geometry APIs remain the responsibility of the installed SDK documentation.
