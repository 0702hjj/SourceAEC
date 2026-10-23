# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Sandbox-owned cache setup and bounded publication; never mount data roots."""
from __future__ import annotations

import ast
import hashlib

from . import task_runtime, build_inputs


def code_fingerprint(script: str) -> str:
    tree = ast.parse(script)
    tree.body = [node for node in tree.body if not (
        isinstance(node, (ast.Assign, ast.AnnAssign)) and any(
            isinstance(target, ast.Name) and target.id == "PARAMS"
            for target in (node.targets if isinstance(node, ast.Assign) else [node.target]))) ]
    return hashlib.sha256(ast.dump(tree, include_attributes=True).encode()).hexdigest()


def prepare(spec, script: str, out_path: str, workdir: str, env) -> dict:
    from .go_runtime import call
    return call("build.cache.prepare", args={
        "codeFingerprint": code_fingerprint(script), "product": out_path,
        "workdir": workdir, "envDir": str(env.env_dir),
        "helpers": [str(spec.flows_dir), *map(str, spec.extra_pythonpath)],
        "inputs": build_inputs.current(), "taskRuntime": task_runtime.__file__})


def publish(out_path: str, workdir: str) -> dict | None:
    from .go_runtime import call
    return call("build.cache.publish", args={"product": out_path, "workdir": workdir})
