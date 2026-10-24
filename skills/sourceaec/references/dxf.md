<!-- SPDX-License-Identifier: Apache-2.0 -->
# DXF integration reference

The default DXF geometry runtime uses external ezdxf. Specify drawing units,
layers and dimensions in the model source. Export ASCII DXF; binary DXF is not
supported by the current Go adapter.

For stable entity selection and comparison, attach a key as group 1000 XDATA
under APPID AIDXF or SOURCEAEC. Entities without keys are compared by content;
a changed keyless entity may appear as a deletion and an addition.

Go creates the browser render payload for LINE, CIRCLE, ARC, TEXT, MTEXT,
LWPOLYLINE and one level of uniformly, positively scaled INSERT. Check its
unsupported list when preview fidelity matters. Preview support does not
establish complete CAD or engineering-drawing validation.

Optional source maps use the XDATA key in the envelope written to
SOURCEAEC_MAP_OUT: {"map":{"entity-key":{"line":12,"origin":"declared"}}}.
Go binds maps to the executed revision; a key needs a unique source callsite.

For SimpleCADAPI modeling methods and exports, use its own installed upstream
skill/documentation. SourceAEC does not bundle that SDK, automatically convert
STEP/assembly products into DXF, or claim support for every upstream workflow.
