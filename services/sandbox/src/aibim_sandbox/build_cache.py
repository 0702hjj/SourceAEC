# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Sandbox-owned cache setup and bounded publication; never mount data roots."""
from __future__ import annotations

import ast
import hashlib
import json
import os
from pathlib import Path
import shutil

from . import task_runtime, build_inputs


def code_fingerprint(script: str) -> str:
    tree = ast.parse(script)
    tree.body = [node for node in tree.body if not (
        isinstance(node, (ast.Assign, ast.AnnAssign)) and any(
            isinstance(target, ast.Name) and target.id == "PARAMS"
            for target in (node.targets if isinstance(node, ast.Assign) else [node.target]))) ]
    return hashlib.sha256(ast.dump(tree, include_attributes=True).encode()).hexdigest()


def prepare(spec, script: str, out_path: str, workdir: str, env) -> dict:
    work = Path(workdir)
    shutil.copyfile(task_runtime.__file__, work / "sourceaec_tasks.py")
    incoming = work / ".task-input"
    outgoing = work / ".task-output"
    incoming.mkdir()
    cache = Path(out_path + ".tasks_cache")
    if cache.is_dir():
        for path in cache.glob("*.json"):
            if path.is_file() and not path.is_symlink() and path.stat().st_size <= 16 << 20:
                shutil.copyfile(path, incoming / path.name)
    # Installed packages + helper sources are part of the environment identity.
    digest = hashlib.sha256(code_fingerprint(script).encode())
    digest.update(Path(task_runtime.__file__).read_bytes())
    digest.update(build_inputs.fingerprint().encode())
    for root in (spec.flows_dir, *spec.extra_pythonpath):
        for path in sorted(Path(root).rglob("*.py")):
            digest.update(str(path.relative_to(root)).encode())
            digest.update(path.read_bytes())
    digest.update(str(env.env_dir).encode())
    for path in sorted(Path(env.env_dir).glob("lib/python*/site-packages/*.dist-info/METADATA")):
        digest.update(path.read_bytes())
    return {"SOURCEAEC_TASK_INPUT": str(incoming),
            "SOURCEAEC_TASK_OUTPUT": str(outgoing),
            "SOURCEAEC_BUILD_FINGERPRINT": digest.hexdigest()}


def publish(out_path: str, workdir: str) -> dict | None:
    source = Path(workdir) / ".task-output"
    if not source.is_dir():
        return None
    paths = list(source.glob("*.json"))
    if any(p.is_symlink() or p.stat().st_size > 16 << 20 for p in paths):
        return None
    if sum(p.stat().st_size for p in paths) > 64 << 20:
        return None
    target = Path(out_path + ".tasks_cache")
    temporary = Path(out_path + ".tasks_cache.tmp")
    shutil.rmtree(temporary, ignore_errors=True)
    temporary.mkdir(parents=True)
    for path in paths:
        shutil.copyfile(path, temporary / path.name)
    shutil.rmtree(target, ignore_errors=True)
    os.replace(temporary, target)
    try:
        return json.loads((target / "stats.json").read_bytes())
    except (OSError, ValueError):
        return None
