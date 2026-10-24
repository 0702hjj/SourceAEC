<!-- SPDX-License-Identifier: Apache-2.0 -->
# IFC integration reference

The default IFC geometry runtime uses external IfcOpenShell. Additional SDKs
must be available in the selected runtime environment; use the installed
version's upstream documentation for modeling calls.

Keep project units, spatial containment and product placements explicit. Preserve
GlobalIds for unchanged objects so versions can match them. A generated IFC file
alone does not prove correct geometry or design compliance.

SourceAEC compares IFC STEP attributes and related property definitions by
GlobalId. This is structural comparison, not a complete geometric equivalence
or compliance engine. Inspect actual products when the task depends on shape.

Source maps are optional. To enable source selection, write a JSON envelope to
SOURCEAEC_MAP_OUT: {"map":{"<GlobalId>":{"line":12,"origin":"declared"}}}.
Use the object's GlobalId as the key and a unique source callsite line. Go binds
the map to the executed source revision. Stale maps cannot be used for edits.
Scalar edits can update a direct PARAMS binding or override a call keyword;
ambiguous callsites require editing the source directly.
