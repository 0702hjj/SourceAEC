# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Explicit, scalar edit bindings for a unique source call site."""
from __future__ import annotations
import ast
from . import script_params


def _call(script: str, line: int) -> ast.Call | None:
    calls = [node for node in ast.walk(ast.parse(script))
             if isinstance(node, ast.Call) and node.lineno == line]
    return calls[0] if len(calls) == 1 else None


def _param_path(node) -> list[str] | None:
    parts = []
    while isinstance(node, ast.Subscript):
        if not isinstance(node.slice, ast.Constant) or not isinstance(node.slice.value, str):
            return None
        parts.insert(0, node.slice.value)
        node = node.value
    if isinstance(node, ast.Name) and node.id in {"params", "PARAMS"} and parts:
        return parts
    return None


def bindings(script: str, line: int) -> list[dict]:
    call = _call(script, line)
    if call is None:
        return []
    params = script_params.extract_params(script)
    result = []
    for kw in call.keywords:
        if kw.arg is None or kw.arg in {"key", "GlobalId", "ifc_class"}:
            continue
        path = _param_path(kw.value)
        try:
            value = params
            if path:
                for part in path:
                    value = value[part]
            else:
                value = ast.literal_eval(kw.value)
        except (ValueError, TypeError, KeyError):
            continue
        if isinstance(value, (str, int, float, bool)):
            result.append({"argument": kw.arg, "value": value, "paramPath": path})
    return result


def update_parameter(script: str, line: int, argument: str, value) -> str:
    binding = next((b for b in bindings(script, line) if b["argument"] == argument), None)
    if not binding or not binding["paramPath"]:
        raise ValueError("argument has no direct PARAMS binding")
    if not isinstance(value, (str, int, float, bool)):
        raise ValueError("value must be a scalar")
    params = script_params.extract_params(script)
    node = params
    path = binding["paramPath"]
    for part in path[:-1]:
        node = node[part]
    node[path[-1]] = value
    return script_params.replace_params(script, params)
