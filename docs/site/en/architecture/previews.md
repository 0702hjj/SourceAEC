# Conversion, previews and reads

IFC conversion captures committed source/build identity. Publication rechecks it and publishes geometry/metadata as one immutable generation.
Stale success or failure cannot replace newer state. The current manifest is retained; unreferenced generations older than one hour are
collected at startup/after conversion. One XKT/metadata generation is limited to 1 GiB combined.

Reads open file handles under the model lock, then release the connection; collection/deletion does not interrupt an open Range read.
Duplicate conversions do not replace complete previews or overwrite ready with late failure.
Deletion uses the common admission/model lock and removes source, versions, bundles, manifests and associated state.
Late geometry conflicts and stale conversions are discarded; old projections cannot resurrect a deleted model.

## Downloads and budgets

Downloads export a committed repeatable-read snapshot in 1 MiB chunks into a private file, unlink it after opening,
and release the database connection before serving Range responses. They never read a mutable source projection.
Each process allows 8 snapshots and 4 GiB total snapshot files. Slow clients hold file capacity until reads finish.

A separate 1 GiB source-byte budget covers concurrent SQL exports. Reserve using the size from the same snapshot before copying;
return it after transaction/connection cleanup. Export overload returns 429 without holding a connection while waiting.
This limits source bytes, not PG RSS/temp/TOAST materialization. File snapshots trade Go heap retention for export/disk costs.
No cross-process or global disk quota is promised.

## Conversion lifecycle

The bounded queue does not hold its mutex while waiting for model admission. It rechecks shutdown/capacity before enqueue.
Workers can retry publication of completed conversion results rather than rerunning the converter.
Node execution is limited to two minutes, excluding database reads/final publication waits; timeout cancels the child and publishes failure
only if the original identity still matches. Preview cleanup is retryable; `previewGCFailed` records failures without rolling back committed versions.

Durable conversion phases recover lost notifications; ordinary queue memory remains disposable.
See [storage](/en/architecture/storage) and [data flow](/en/architecture/data-flow).
