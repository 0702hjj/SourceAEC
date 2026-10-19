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
    return {wall.GlobalId: tuple(ifcopenshell.geom.create_shape(settings, wall).geometry.verts)
            for wall in model.by_type("IfcWall")}


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
