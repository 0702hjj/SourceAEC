# Persistence and versions

PostgreSQL is authoritative. Disk directories are recoverable projections for the sandbox, Git and converter; do not edit `data/` manually.
A save commits source, products, immutable history references, version number and staging reset in one transaction without running Git
or rewriting the full bundle. Failed execution preserves current state. Patches are instructions; full snapshots are durable facts.

## Ownership of state

| Data | Durable source | Filesystem role |
| --- | --- | --- |
| Source, products, inputs, staging, versions | PostgreSQL artifacts | Recoverable workspace projection |
| Source history and counters | versionstore immutable references/head | On-demand source projection |
| Git DAG and branches | Stored bundles and projection records | On-demand Git workspace |
| Preview generation | Database manifest plus immutable files | Rebuildable display cache |
| Workflow definitions, blobs, tickets, attempts, leases, phases | PostgreSQL | Explicit whole-script recovery |
| Running processes and queue notifications | In-process only | Not durable execution state |

Native typed artifact operations avoid repeated JSON/base64 business transport for large products.
Model `.py` source is runtime data, not maintained application code. External environments stay outside version control.
Configure `VIEWER_PG_DSN`; `VIEWER_STORAGE_NAMESPACE` separates business namespaces. Back up both database and runtime directory.

## History and upgrades

First access migrates legacy product archives into the database catalog before recording completion.
Subsequent directory changes do not change version numbering. Recoverable legacy timestamps are retained; unavailable ones remain absent.
Journaled uncommitted paths are reconciled rather than imported as versions.

Git operations project up to 32 records per transaction; cancellation preserves committed progress.
Preparation is bounded to 30 seconds or 64 batches, returning 429 if more work remains. Retry continues progress.
Saves expose `historyRevision` (`r_...`), not a Git SHA; `/git` materializes and returns actual commits.
Checkout/merge preserve DAG/conflict rules and still require identity validation before publication.
Legacy bundle import may require Git; new-model normal saves do not.


Git compatibility has at most three jobs per process and a combined 1 GiB bundle-source high-water budget.
Read the length and reserve before loading the large column; SQL checks the actual length again to prevent
uncharged growth for nontransactional callers. Each job charges the larger input or generated bundle size.
Grow the reservation before writing output; exhaustion returns 429, cancels the Git process group and rolls
back the current transaction. Previously committed projection batches remain resumable. Captured snapshots
and private workspaces share ownership, releasing only after workspace cleanup and snapshot closure;
failed cleanup retains the reservation. This is not a Go/Git/PG RSS or total scratch-disk hard quota and is
separate from download/comparison budgets. New-model normal saves still avoid Git; legacy migration,
queries and branch operations use this admission.

Stop old writers before upgrading storage/recovery protocols. Initial startup may hash existing artifacts and rebuild previews lacking manifests.
Keep existing product archives and bundles in backups. Legacy scripts importing retired in-repository SDKs need source adaptation or an
administrator-provided compatible external runtime. SQLite migration is offline (`cmd/migrate-sqlite`), requires CGO and is not in static releases.

## Workflow storage

Content-addressed workflow blobs remain in PostgreSQL and are not bulk-projected on restart.
Model deletion atomically removes definitions, runs, attempts, tickets and blobs. The state store does not execute geometry;
editing workers invoke the shared sandbox/publication adapters. Old/canceled attempts cannot publish.

Pending admission limits count tickets/runs and logical input bytes; completed history/blobs currently remain until model deletion.
The 64-run/2-GiB limit does not cap accumulated storage, PG temp usage or RSS.
See [execution](/en/architecture/execution), [previews](/en/architecture/previews) and [workflows](/en/reference/workflows).

Optional notebook caches use separate PostgreSQL generation/head tables, scoped by namespace/model. A model transaction replaces the reference and collects the prior unreferenced generation. They are disposable accelerators, not version-history payloads; build records retain diagnostic generation IDs rather than promises to retain old cache packages. Model deletion removes these rows atomically.
