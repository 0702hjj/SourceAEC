# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
from aibim_sandbox.script_bindings import bindings, update_parameter

SCRIPT = '''PARAMS = {"wall": {"height": 3}}
def build(params, out_path):
    wall(key="wall", height=params["wall"]["height"], name="Wall")
'''


def test_parameter_binding_preserves_shared_expression():
    assert bindings(SCRIPT, 3) == [
        {"argument": "height", "value": 3, "paramPath": ["wall", "height"]},
        {"argument": "name", "value": "Wall", "paramPath": None},
    ]
    edited = update_parameter(SCRIPT, 3, "height", 4)
    assert 'height=params["wall"]["height"]' in edited
    assert bindings(edited, 3)[0]["value"] == 4


def test_ambiguous_call_site_is_not_editable():
    assert bindings(SCRIPT.replace('name="Wall"', 'name=lookup("Wall")'), 3) == []


def test_scalar_override_supports_negative_coordinates():
    from aibim_sandbox.script_edit import rewrite_call_argument
    assert rewrite_call_argument("wall(x=1.0)\n", 1, "x", -2.5) == "wall(x=-2.5)\n"
