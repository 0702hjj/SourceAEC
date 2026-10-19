# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Real IFC fragments: partial builds must match a clean full rebuild."""
import json
from pathlib import Path
import shutil
import ifcopenshell
import ifcopenshell.geom
from app import script_runner
from app.config import load_settings
from aibim_sandbox import script_params

EXAMPLE = Path(__file__).resolve().parents[3] / "examples" / "incremental_walls.py"


def _geometry(path):
    model = ifcopenshell.open(str(path))
    settings = ifcopenshell.geom.settings()
    settings.set("use-world-coords", True)
    result = {}
    for wall in model.by_type("IfcWall"):
        # Keep the owning C++ shape alive while reading its buffers. Compare
        # triangles independent of tessellator vertex/index ordering.
        shape = ifcopenshell.geom.create_shape(settings, wall)
        vertices = shape.geometry.verts
        faces = shape.geometry.faces
        points = [tuple(round(value, 8) for value in vertices[i:i + 3])
                  for i in range(0, len(vertices), 3)]
        triangles = [tuple(sorted(points[index] for index in faces[i:i + 3]))
                     for i in range(0, len(faces), 3)]
        result[wall.GlobalId] = tuple(sorted(triangles))
    return result


def test_incremental_wall_matches_full_rebuild(tmp_path):
    settings = load_settings()
    source = EXAMPLE.read_text()
    product = tmp_path / "model.ifc"
    script_runner.run_script(settings, source, str(product))
    before = _geometry(product)
    changed = script_params.replace_params(source, {"a_height": 4.0, "b_height": 3.0})
    script_runner.run_script(settings, changed, str(product))
    manifest = json.loads(Path(str(product) + ".build.json").read_text())
    assert manifest["tasks"] == {"executed": ["wall:a"], "reused": ["wall:b"]}
    incremental = _geometry(product)
    assert len(incremental) == 2
    assert sum(before[key] != incremental[key] for key in before) == 1
    shutil.rmtree(str(product) + ".tasks_cache")
    script_runner.run_script(settings, changed, str(product))
    assert _geometry(product) == incremental


def test_selected_wall_height_edits_source_and_reuses_other_fragment(client, data_dir):
    from conftest import MODEL_ID
    source = EXAMPLE.read_text()
    assert client.put(f"/models/{MODEL_ID}/script", json={"script": source}).status_code == 200
    assert client.post(f"/models/{MODEL_ID}/script/run").status_code == 200
    from script_lib import deterministic_guid
    located = client.get(f"/models/{MODEL_ID}/script/locate", params={
        "guid": deterministic_guid("wall:a")}).json()
    assert located["found"]
    assert next(a for a in located["arguments"] if a["argument"] == "height")["paramPath"] == ["a_height"]
    response = client.post(f"/models/{MODEL_ID}/script/edit-call", json={
        "designKey": "wall:a", "argument": "height", "value": 4.0,
        "mode": "parameter", "baseRevision": located["revision"],
    })
    assert response.status_code == 200, response.text
    script = client.get(f"/models/{MODEL_ID}/script").json()["script"]
    assert script_params.extract_params(script)["a_height"] == 4.0
    assert 'height=params["a_height"]' in script
    record = json.loads((data_dir / "uploads" / f"{MODEL_ID}.ifc.build.json").read_text())
    assert record["tasks"] == {"executed": ["wall:a"], "reused": ["wall:b"]}
