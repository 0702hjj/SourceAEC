# Durable workflows

Opt into this API to continue whole-script work after gateway restart. Existing browser run/save endpoints remain synchronous.
All endpoints use gateway authentication and `{code,message,data}` (`code=0` on success).
Bearer tokens belong in headers; workflow queries reject `?token=` authentication.

## Submit and query

Stage source, then read its current revision from `GET /api/v1/models/{id}/script`:

```http
POST /api/v1/models/{id}/workflows
Authorization: Bearer <token>
Content-Type: application/json

{"idempotencyKey":"save-001","action":"save","baseRevision":"<current revision>","source":"ai","note":"Update wall"}
```

Action is `run` or `save`; source is `human` (default) or `ai`. Keys contain 1–128 letters/digits/dots/underscores/colons/hyphens.
Requests are limited to 1 MiB and reject unknown fields. Capture capacity is reserved before freezing source, project inputs and environment.
Only after immutable definition, blobs and run commit does the API return **202** with `data.id` (`wf_` plus 32 hex characters).
If disconnected, retry the same key/request: the original may or may not have committed.

Same model/key/intent returns the original run (**200**) without recapture. Reusing a key with changed action/baseRevision/source/note
returns **409**. Concurrent capture for that key returns retryable **429**. Resume also checks pending capacity.
Other 429 responses indicate capture/build overload; retry unchanged requests later. Capture identity changes return 409;
reread and use a new key. Uploaded models without source cannot submit script workflows.

| Method/path (relative to `/api/v1/models/{id}`) | Result |
| --- | --- |
| GET `/workflows?limit=20` | `data.workflows` summaries; limit 1–100; no large blobs/results |
| GET `/workflows/{workflowId}` | Phases, state, attempts and committed result |
| POST `/workflows/{workflowId}/cancel` | Persist cancellation of unfinished work |
| POST `/workflows/{workflowId}/resume` | Resume failed/canceled work with the frozen definition/checkpoints |

Phases: `geometry`, `publication`, `conversion`. States: `pending`, `running`, `succeeded`, `failed`, `canceled`, `superseded`.
IFC requires preview conversion after publication; DXF finishes after publication.
Detail result is the committed editing response, e.g. `{"modelId":"...","version":"v1","staged":0}`.
Summaries omit result. Public errors omit internal leases/environment paths/storage exceptions; use server logs for diagnostics.

## Recovery and cancellation

Geometry is at least once: process loss before checkpoint commit may execute it again. Durable candidates go directly to publication.
Publication verifies ownership fence/frozen identity under the shared model lock; versions, history and phase markers commit together.
Stale results become superseded. A damaged unpublished candidate can regenerate once; missing frozen inputs/environment drift fail
rather than substitute current inputs.

Workers scan PostgreSQL independently of notifications. Normal leases last 30 seconds with one-second heartbeats.
Checkpoint/publication transactions holding the run lock extend their lease only to the three-minute phase deadline;
separate heartbeats skip busy rows and claimers skip locked rows. Crashes roll back extensions.
Each phase allows three automatic attempts with bounded transient retry; deterministic script/environment errors fail.
Explicit resume resets phase attempts without changing the definition. Succeeded/superseded runs cannot resume.

Cancellation removes publication ownership; the sandbox stops after failed renewal and late output cannot publish.
**Cancel does not undo committed versions**, nor instantly stop an already queued derived conversion.
A failed/canceled IFC conversion retains the committed version/result; resume only recovers conversion from its source/build identity.

## Deployment and budgets

Supported: one gateway, shared workspace and PostgreSQL namespace. Process budgets are not global replica quotas.
Default uv environments explicitly use managed CPython and versioned cache policy. A custom venv linked to system `/usr` Python is insufficient.
Seals cover interpreter/venv/base runtime contents before/after execution, not OS state, time or randomness.
Publishing an already verified candidate does not rerun geometry.

A namespace permits 64 pending runs/tickets and 2 GiB logical input. Capture conservatively reserves 259 MiB for five minutes,
then replaces it with actual input/manifest accounting. Small requests near capacity may be rejected early.
Raw inputs: 64 MiB/file, 256 MiB total, 2048 files; script/manifest each at most 1 MiB.
Two scanning workers share three complete build slots with synchronous execution; candidate retention remains charged.
Blobs stay in PostgreSQL without bulk filesystem projection. Model deletion cleans workflow state;
completed runs/blobs currently remain until deletion. Pending budgets do not cap history, PG temp or total RSS.

See [execution](/en/architecture/execution), [sandbox](/en/architecture/sandbox), [MCP](/en/integrations/agents)
and [OpenAPI](/go-server.openapi.json).
