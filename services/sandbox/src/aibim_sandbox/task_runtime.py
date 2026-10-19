# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Opt-in dependency-driven tasks, injected into each sandbox as sourceaec_tasks.

Tasks return JSON-compatible intermediate results (geometry recipes, parameters,
entity descriptions). Assembly always runs, producing a complete IFC/DXF. A task
only sees its declared parameters and dependency results. Scripts using arbitrary
state keep the existing full-build path.
"""
from __future__ import annotations

from dataclasses import dataclass
import hashlib
import json
import os
from pathlib import Path
import re
from typing import Callable


@dataclass(frozen=True)
class Task:
    key: str
    run: Callable[[dict, dict], object]
    params: tuple[str, ...] = ()
    dependencies: tuple[str, ...] = ()


def _json(value) -> bytes:
    return json.dumps(value, sort_keys=True, separators=(",", ":"),
                      ensure_ascii=False, allow_nan=False).encode()


def run_tasks(params: dict, tasks: list[Task]) -> dict:
    """Evaluate a DAG, reusing successful results with identical declared inputs.

    Code/environment fingerprints are supplied by the trusted sandbox parent.
    Corrupt/missing caches recompute; no task writes directly into a shared cache.
    """
    incoming = Path(os.environ.get("SOURCEAEC_TASK_INPUT", ".task-input"))
    outgoing = Path(os.environ.get("SOURCEAEC_TASK_OUTPUT", ".task-output"))
    outgoing.mkdir(exist_ok=True)
    fingerprint = os.environ.get("SOURCEAEC_BUILD_FINGERPRINT", "uncached")
    by_key = {task.key: task for task in tasks}
    if len(by_key) != len(tasks):
        raise ValueError("duplicate task key")
    results, hashes, visiting = {}, {}, set()
    stats = {"executed": [], "reused": []}

    def evaluate(key):
        if key in results:
            return results[key]
        if key in visiting:
            raise ValueError(f"cyclic task dependency: {key}")
        task = by_key[key]
        if not re.fullmatch(r"[A-Za-z0-9_:/.-]{1,160}", key):
            raise ValueError("invalid task key")
        visiting.add(key)
        dependencies = {dep: evaluate(dep) for dep in task.dependencies}
        selected = {name: params[name] for name in task.params}
        signature = hashlib.sha256(_json([fingerprint, key, selected,
                                 {dep: hashes[dep] for dep in task.dependencies}])).hexdigest()
        cache_name = hashlib.sha256(key.encode()).hexdigest() + ".json"
        cached = None
        try:
            cached = json.loads((incoming / cache_name).read_bytes())
        except (OSError, ValueError):
            pass
        if (fingerprint != "uncached" and isinstance(cached, dict)
                and cached.get("signature") == signature and "result" in cached
                and cached.get("digest") == hashlib.sha256(_json(cached["result"])).hexdigest()):
            result = cached["result"]
            stats["reused"].append(key)
        else:
            # Copy to prevent one task mutating another task's cached value.
            result = task.run(json.loads(_json(selected)), json.loads(_json(dependencies)))
            result = json.loads(_json(result))
            stats["executed"].append(key)
        digest = hashlib.sha256(_json(result)).hexdigest()
        (outgoing / cache_name).write_bytes(_json({"signature": signature,
                                                  "result": result, "digest": digest}))
        results[key], hashes[key] = result, digest
        visiting.remove(key)
        return result

    for key in by_key:
        evaluate(key)
    (outgoing / "stats.json").write_bytes(_json(stats))
    return results
