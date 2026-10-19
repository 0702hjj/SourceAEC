# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Parametric factories with explicit source mappings for browser editing."""
import math
import numpy as np
from script_lib import api, create_entity, _record_callsite


def create_wall(model, context, container, *, key: str, name: str = "Wall",
                length: float = 5.0, height: float = 3.0, thickness: float = 0.2,
                x: float = 0.0, y: float = 0.0):
    """Create an axis-aligned wall; dimensions and position are SI metres.

    Use a stable key and pass tunables directly as PARAMS references. The map
    refers to the caller's factory invocation, including its geometry arguments.
    """
    dimensions = (length, height, thickness)
    if any(not math.isfinite(v) or v <= 0 for v in dimensions):
        raise ValueError("wall dimensions must be positive and finite")
    if not all(math.isfinite(v) for v in (x, y)):
        raise ValueError("wall position must be finite")
    wall = create_entity(model, "IfcWall", key=key, name=name)
    representation = api("geometry.add_wall_representation", model, context=context,
                         length=length, height=height, thickness=thickness)
    api("geometry.assign_representation", model, product=wall, representation=representation)
    matrix = np.eye(4)
    matrix[0, 3], matrix[1, 3] = x, y
    api("geometry.edit_object_placement", model, product=wall, matrix=matrix, is_si=True)
    api("spatial.assign_container", model, products=[wall], relating_structure=container)
    pset = api("pset.add_pset", model, product=wall, name="Pset_SourceAECGeometry")
    api("pset.edit_pset", model, pset=pset,
        properties={"Length": length, "Height": height, "Thickness": thickness})
    material = api("material.add_material", model, name="Concrete", category="concrete")
    api("material.assign_material", model, products=[wall], type="IfcMaterial", material=material)
    _record_callsite(key)
    return wall
