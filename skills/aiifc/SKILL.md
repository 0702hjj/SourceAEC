---
name: aiifc
description: Author and edit IFC models through SourceAEC's script API using the external IfcOpenShell runtime.
license: Apache-2.0
---

# IFC model authoring

Use the project's existing model and the same stage → run → save API for human
and AI edits. Fetch current source and its revision before modifying a model;
supply `baseRevision` when staging to avoid overwriting concurrent edits.

Model source is a runtime artifact, not an installed SourceAEC Python package.
Expose tunable values in a top-level literal `PARAMS` dictionary. The runtime
executes the source with the output filename as its first argument. Use the
external `ifcopenshell` API directly; there is no local `script_lib`, hook,
SourceAEC Python SDK, or helper CLI to import. If dependencies are declared in a
PEP 723 block, use exact registry versions supported by the deployment.

Keep IFC units, spatial containment, placements and stable GlobalIds explicit.
Validate generated geometry and relevant model properties before saving.
A successful process exit alone does not establish design or code compliance.
Use verified library documentation for unfamiliar calls rather than inventing
API names. For simple geometry, prefer the model's existing construction style.

Go owns staging, validation, sandbox orchestration, versions, Git and publication.
Do not write into server persistence directories or bypass its API.
