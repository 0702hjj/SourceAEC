# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Stage this synthetic script through the IFC service.

Each wall is built as a cached standalone IFC fragment. Changing a_height only
rebuilds wall:a; the final complete downloadable IFC is assembled every time.
sourceaec_tasks is injected by the sandbox; helper modules use its PYTHONPATH.
"""
import sys
import ifcopenshell
from sourceaec_tasks import Task, run_tasks
import script_lib
from script_lib import api, create_skeleton, attach_design_key, write_and_validate
from editable_elements import create_wall

PARAMS = {"a_height": 3.0, "b_height": 3.0}


def wall_a(params, dependencies):
    model = ifcopenshell.file(schema="IFC4")
    body, storeys = create_skeleton(model, "Fragment", {"Ground": 0})
    create_wall(model, body, storeys["Ground"], key="wall:a", height=params["a_height"], length=5.0)
    return {"ifc": model.to_string(), "map": {"wall:a": script_lib._CALLSITES["wall:a"]}}


def wall_b(params, dependencies):
    model = ifcopenshell.file(schema="IFC4")
    body, storeys = create_skeleton(model, "Fragment", {"Ground": 0})
    create_wall(model, body, storeys["Ground"], key="wall:b", height=params["b_height"], length=5.0, y=4.0)
    return {"ifc": model.to_string(), "map": {"wall:b": script_lib._CALLSITES["wall:b"]}}


def build(params, out_path):
    fragments = run_tasks(params, [Task("wall:a", wall_a, ("a_height",)), Task("wall:b", wall_b, ("b_height",))])
    model = ifcopenshell.file(schema="IFC4")
    body, storeys = create_skeleton(model, "Incremental walls", {"Ground": 0})
    material = api("material.add_material", model, name="Concrete", category="concrete")
    for key, result in fragments.items():
        fragment = ifcopenshell.file.from_string(result["ifc"])
        wall = model.add(fragment.by_type("IfcWall")[0])
        api("spatial.assign_container", model, products=[wall], relating_structure=storeys["Ground"])
        attach_design_key(model, wall, key)
        api("material.assign_material", model, products=[wall], type="IfcMaterial", material=material)
        script_lib._CALLSITES.update(result["map"])
    if not write_and_validate(model, out_path):
        raise ValueError("IFC validation failed")


if __name__ == "__main__":
    build(PARAMS, sys.argv[1])
