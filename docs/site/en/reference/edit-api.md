# Script editing API

All paths below are relative to `/api/v1/models/{id}` and use the gateway envelope/authentication.

| Method | Path | Behavior |
| --- | --- | --- |
| GET / PUT | `/script` | Read source / replace source, PARAMS or exact patch |
| GET | `/script/params` | Read literal PARAMS |
| POST | `/script/undo`, `/script/redo`, `/script/discard` | Ten-step staging history |
| POST | `/script/run` | Execute/publish current product without a version |
| POST | `/script/save` | Execute/save immutable version and history reference |
| POST | `/script/rollback` | Restore model using version source/inputs |
| GET | `/scripts`, `/edit/versions` | Source/product version catalogs |
| POST | `/script/diff`, `/edit/diff` | Source/PARAMS or product comparisons |
| GET | `/script/staging/diff` | Staging comparison using from/to |
| GET | `/script/locate` | Locate by IFC guid or DXF key |
| POST | `/script/edit-call` | Scalar binding/override edit |
| GET | `/git` | Branch/commit status |
| POST | `/git/branches`, `/git/checkout`, `/git/merge` | Branch operations |
| POST | `/edit/diff-upload` | Multipart file comparison with USER provenance |

PUT accepts exactly one of script, params or patch. Source/parameter changes may carry baseRevision; patches require it
and must match the exact current source SHA-256. Stale bases return 409.
A patch contains 1–128 ordered, nonoverlapping `{start,end,text}` edits using UTF-8 byte positions `[start,end)`;
boundaries cannot split characters. Resulting source is limited to 1 MiB. Failed patches leave staging unchanged;
success shares ordinary undo/redo. No fuzzy application is performed.

For `PARAMS = {}
`, the braces occupy bytes `[9,11)`:

```json
{"baseRevision":"<current source SHA-256>","patch":[{"start":9,"end":11,"text":"{'width': 80}"}]}
```

Save accepts note and source (`human`/`ai`); rollback accepts version. Save returns version and historyRevision (`r_...`, not Git SHA).
Git queries materialize real commits on demand; failed projection does not invalidate the saved version.
Product diff accepts base/target, with `current` allowed as target.

Scripts require a top-level literal PARAMS and write to their first process argument. Optional SOURCEAEC_MAP_OUT JSON is
`{"map":{"entity-key":{"line":12,"origin":"declared"}}}`; Go binds the source digest.
Keys are IFC GlobalId or DXF XDATA keys. No platform Python SDK is required.
IFC property/reference inspection is not full geometry/compliance analysis.
See [sandbox](/en/architecture/sandbox), [storage](/en/architecture/storage) and [workflows](/en/reference/workflows).


Branch creation, checkout and merge may include `baseRevision`, the SHA-256 of current staged source.
When provided, check it during consistent capture and before application; stale requests return 409 without
applying the branch action or replacing newer source. Legacy calls may omit it; read the current revision
and include it for optimistic protection. Git job/source-byte exhaustion returns 429; retry after capacity
is released. Already committed history projection batches retain progress.
