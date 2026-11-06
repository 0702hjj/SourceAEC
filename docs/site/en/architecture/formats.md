# IFC / DXF adapters

Both formats share the Go editing lifecycle. IFC scripts can use external IfcOpenShell; DXF scripts can use ezdxf or another configured
runtime that exports ASCII DXF. Binary DXF is unsupported.

Go traverses IFC STEP records and normalizes properties/references, matching GlobalId. This is bounded structural/property inspection,
not a complete IFC compliance or geometry kernel. DXF compares AIDXF/SOURCEAEC XDATA keys; unkeyed entities match by content,
so movement may appear as deletion/addition.

Go emits `render.json` schemaVersion 2 for LINE, CIRCLE, ARC, TEXT, MTEXT, LWPOLYLINE and one level of positively uniform-scaled INSERT.
Other entity types appear in `unsupported`. Preview support does not imply full CAD validation.
Blocks/polyline expansion reserve work before allocating output; oversized candidates fail rather than return partial success.

Input, record, field, nesting, reference and output limits are listed under [execution budgets](/en/architecture/execution#structural-budgets).
See [model/render schema](/en/reference/api-model#render-json-schema) and [engine boundaries](/en/architecture/boundaries).
