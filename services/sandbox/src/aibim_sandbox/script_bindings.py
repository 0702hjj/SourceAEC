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


def _facts(script: str, line: int) -> list[dict]:
    call = _call(script, line)
    if call is None:
        return []
    facts = []
    for kw in call.keywords:
        fact = {"argument": kw.arg or "", "paramPath": _param_path(kw.value)}
        try:
            value = ast.literal_eval(kw.value)
            fact["literal"] = isinstance(value, (str, int, float, bool))
            if fact["literal"]:
                fact["value"] = value
        except (ValueError, TypeError):
            fact["literal"] = False
        facts.append(fact)
    return facts


def bindings(script: str, line: int) -> list[dict]:
    from .go_runtime import call
    facts = _facts(script, line)
    if not facts:
        return []
    return call("edit.bindings", args={"params": script_params.extract_params(script), "facts": facts})


def update_parameter(script: str, line: int, argument: str, value) -> str:
    from .go_runtime import call
    params = call("edit.parameter", args={"params": script_params.extract_params(script),
                  "facts": _facts(script, line), "argument": argument, "value": value})
    return script_params.replace_params(script, params)
