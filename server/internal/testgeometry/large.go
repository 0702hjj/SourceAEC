// SPDX-License-Identifier: Apache-2.0
// Package testgeometry supplies synthetic model sources for integration tests.
package testgeometry

import "fmt"

// These are synthetic user geometry scripts, not application business logic.
// Coordinates vary deterministically; no padding/comments inflate product size.
func LargeScript(kind string, size int) string {
	prefix := fmt.Sprintf("PARAMS = {'target_bytes': %d}\n", size)
	if kind == "dxf" {
		return prefix + `import random, sys
from ezdxf.addons.r12writer import r12writer
rng = random.Random(702)
with open(sys.argv[1], "w", encoding="ascii") as stream:
    with r12writer(stream) as drawing:
        while stream.tell() < PARAMS['target_bytes'] - 4096:
            start = tuple(rng.random() * 1000.0 for _ in range(3))
            end = (start[0] + 0.5, start[1] + 0.25, start[2] + 0.125)
            drawing.add_line(start, end)
`
	}
	return prefix + `import random, sys
import ifcopenshell
import ifcopenshell.api as api
model = ifcopenshell.file(schema="IFC4")
project = api.run("root.create_entity", model, ifc_class="IfcProject", name="Synthetic mesh")
api.run("unit.assign_unit", model)
context = api.run("context.add_context", model, context_type="Model")
body = api.run("context.add_context", model, context_type="Model", context_identifier="Body", target_view="MODEL_VIEW", parent=context)
site = api.run("root.create_entity", model, ifc_class="IfcSite", name="Synthetic site")
building = api.run("root.create_entity", model, ifc_class="IfcBuilding", name="Synthetic building")
storey = api.run("root.create_entity", model, ifc_class="IfcBuildingStorey", name="Synthetic storey")
api.run("aggregate.assign_object", model, products=[site], relating_object=project)
api.run("aggregate.assign_object", model, products=[building], relating_object=site)
api.run("aggregate.assign_object", model, products=[storey], relating_object=building)
rng = random.Random(702)
size = len(model.to_string().encode("utf-8"))
chunk = 0
while size < PARAMS['target_bytes'] - 65536:
    vertices, faces = [], []
    for triangle in range(128):
        x, y, z = (rng.random() * 1000.0 for _ in range(3))
        vertices.extend(((x, y, z), (x + 0.5, y, z), (x, y + 0.5, z + 0.125)))
        offset = triangle * 3 + 1
        faces.append((offset, offset + 1, offset + 2))
    points = model.create_entity("IfcCartesianPointList3D", CoordList=vertices)
    mesh = model.create_entity("IfcTriangulatedFaceSet", Coordinates=points, Closed=False, CoordIndex=faces)
    shape = model.create_entity("IfcShapeRepresentation", ContextOfItems=body, RepresentationIdentifier="Body", RepresentationType="Tessellation", Items=[mesh])
    definition = model.create_entity("IfcProductDefinitionShape", Representations=[shape])
    product = model.create_entity("IfcBuildingElementProxy", GlobalId=ifcopenshell.guid.compress(format(chunk + 1, "032x")), Name="Synthetic triangles", Representation=definition)
    relation = model.create_entity("IfcRelContainedInSpatialStructure", GlobalId=ifcopenshell.guid.compress(format(2**64 + chunk, "032x")), RelatedElements=[product], RelatingStructure=storey)
    size += sum(len(str(entity).encode("utf-8")) + 2 for entity in (points, mesh, shape, definition, product, relation))
    chunk += 1
model.write(sys.argv[1])
`
}
