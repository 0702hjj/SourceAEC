# Editing domain

`server/internal/editing` implements IFC/DXF editing in-process. Shared behavior includes literal PARAMS,
exact source patches, ten-step staging/undo/redo, revision checks, run/save/rollback, comparisons, Git branches and publication.
Format-specific behavior belongs in focused Go adapters.

Read source and revision, then stage → run → save. A mismatched baseRevision conflicts; failed execution does not publish products
or create versions. Capture closes its short snapshot before environment preparation/geometry. Publication takes shared model admission
and a PostgreSQL lock, rechecks identity and consumes the completed candidate without executing again.

Normal saves commit immutable source-history references without rebuilding Git. Branch operations project real commits on demand.
Explicit [workflows](/en/reference/workflows) reuse capture/build/publication and persist phase checkpoints;
ordinary requests only recover committed state/projections after restart.

Python model source is user/agent runtime input, materialized in a disposable directory. Go owns environment policy,
validation, resource limits, persistence and publication. No embedded drawing SDK or Python AST service is shipped.
See [API](/en/reference/edit-api), [sandbox](/en/architecture/sandbox) and [execution](/en/architecture/execution).
