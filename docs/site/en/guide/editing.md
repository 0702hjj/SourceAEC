# Editing and versions

Scripts and captured inputs are the model source. Every save executes again; unchanged code does not imply deterministic output.
A script-backed model supports source navigation, parameter editing, staging and immutable versions. An uploaded plain model
can be reviewed until a script is authored for it.

## Stage, run, save

Read the script and revision. Submit source, literal PARAMS or an exact UTF-8 byte patch against that revision.
Invalid edits leave staging unchanged. Staging retains up to ten steps with undo/redo; discard removes the staged chain.
`run` executes and publishes the current product without creating a version. `save` executes again and creates a version.
All viewers share the same editing lifecycle.

Source maps locate IFC GlobalId or DXF keys. Safely mapped scalar literals/PARAMS support targeted edits;
computed values can be located but require source editing. Stale maps require a fresh run.

## Versions and comparison

Versions retain source, source maps, inputs and products. Rollback re-executes saved source with archived inputs.
Text/PARAMS differences and entity-property differences serve separate purposes. Property diffs match GlobalId or stable DXF keys;
they do not detect every geometry change and are not an incremental geometry plan.

Saved product comparisons use archives; legacy versions without products may replay source.
Opening branch management materializes Git history on demand. Checkout/merge require clean staging and preserve the original
branch/product when conflicts or candidate builds fail.

## Uploaded models and recovery

Staging the first script preserves an uploaded original as `bootstrap.ifc`. A successful v1 save makes the model script-backed;
it does not prove equivalence to the upload. Compare explicitly.

Ordinary synchronous requests do not resume execution after restart. Explicit [durable workflows](/en/reference/workflows)
persist a whole-script definition and phase checkpoints. No Python stack or notebook cell recovery is provided.
External SDK caches do not change these guarantees.

See [editing API](/en/reference/edit-api), [storage](/en/architecture/storage) and [data flow](/en/architecture/data-flow).
