# Concurrency and recovery

Editing, model deletion, conversion publication and status writes share process-local admission before acquiring a database connection:
one active write and up to 64 waiters per namespace/model. Cancellation returns capacity; overload is retryable.
PostgreSQL model locks provide cross-process exclusion. The supported deployment is one gateway with a shared workspace/namespace;
independent projections and global replica budgets have no deployment contract.

Read-only script/parameter/version requests use committed snapshots without waiting for geometry.
Capture freezes source, inputs and identity in a short repeatable-read snapshot. Build runs without a database connection/model lock.
Publication reacquires admission/lock and checks staging, product, version, branch, ownership and input identity.
Changed state returns 409; the caller must reread. Captured data and executor input maps are independent.

## Resource lifecycles

Three complete build lifecycles include candidates waiting for publication. Three stored-product comparisons separately reserve up to
512 MiB combined raw input before reading. Legacy source replay also reserves build capacity and up to 256 MiB per product.
These budgets exclude parsing structures and external native heaps; they are not RSS quotas.
Inputs are captured from authoritative project ownership/plans/skill-work in the same snapshot: up to 64 MiB per file, 256 MiB total.
Projection absence or uncommitted external writes cannot change captured bytes.

A journal records projection paths before mutation. Restart reconciles files against committed database contents under the recovery lock.
Write transactions also commit short-lived receipts. If COMMIT acknowledgement is lost, query the receipt under the model lock;
when confirmation is impossible, report an unknown result and retain the journal rather than blindly rerun.
Receipts remain at least ten minutes. Stage metrics are cumulative counts/nanoseconds, not latency percentiles.

## Structural budgets

| Object | Budget |
| --- | --- |
| Source-map raw/normalized JSON | 16 MiB each, 262144 tokens including keys/delimiters, 32 container levels |
| Format input | 256 MiB |
| IFC STEP | 1000000 records, 2000000 direct fields, 128 aggregate levels, 32-byte record identifiers including `#` |
| IFC roots/inverse relations | 100000 roots, 2000000 visits including missing references |
| IFC normalization | 100000 calls, 256 MiB scanned, 64 MiB cumulative builder writes including repeated parent copies/keys |
| DXF parsing | 100000 records, 2000000 group codes |
| DXF expansion | 100000 entities/visits, 2000000 traversed group codes; repeated blocks/parent queries count again |
| DXF output reservation | 256 MiB comparison signatures, 128 MiB render output, conservative string-escaping estimates |

Checks occur before reading/expanding/writing where applicable. Oversized candidates fail with 422 without partial publication;
previously uploaded oversized documents may require splitting before comparison/preview.
These protective limits do not establish production capacity or complete geometric correctness.

## Durable recovery

Synchronous requests do not automatically resume. Explicit workflows persist frozen definitions, source/inputs/environment seals and phase
checkpoints. Two workers scan SQL facts; notifications are optional accelerators. Leases use database time and full fencing.
Candidate blobs/checkpoints commit together; model versions/history/publication markers commit together using the same receipt.
The conversion phase rebuilds lost enqueue notifications from published source/build identity without another save.
No Python stack is persisted. See [workflow API](/en/reference/workflows), [previews](/en/architecture/previews) and
[capacity verification](/en/operations/performance).
