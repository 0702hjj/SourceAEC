# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Content-addressed successful builds and their reproducibility manifest."""
from __future__ import annotations
import hashlib
import json
from pathlib import Path
import sys

from . import artifacts, deps, build_inputs


def digest(path: str) -> str | None:
    try:
        return hashlib.sha256(Path(path).read_bytes()).hexdigest()
    except OSError:
        return None


def identity(spec, script: str, env) -> dict:
    packages = {}
    for path in sorted(Path(env.env_dir).glob("lib/python*/site-packages/*.dist-info/METADATA")):
        name = version = None
        for line in path.read_text().splitlines():
            if line.startswith("Name: "): name = line[6:]
            if line.startswith("Version: "): version = line[9:]
            if name and version: break
        if name and version: packages[name] = version
    helpers = {}
    for index, root in enumerate((spec.flows_dir, *spec.extra_pythonpath)):
        for path in sorted(Path(root).rglob("*.py")):
            helpers[f"{index}/{path.relative_to(root).as_posix()}"] = digest(str(path))
    return {"scriptHash": hashlib.sha256(script.encode()).hexdigest(),
            "python": sys.version.split()[0], "packages": packages,
            "helpers": helpers, "product": spec.config.product_name,
            "environment": Path(env.env_dir).name, "dependencies": list(env.deps), "inputsHash": build_inputs.fingerprint()}


def reusable(spec, script: str, product: str, map_path: str) -> bool:
    try:
        record = json.loads(Path(product + ".build.json").read_bytes())
    except (OSError, ValueError):
        return False
    # Dependency environments are immutable; ensure the declared environment
    # still exists before comparing its installed package versions.
    env = deps.ensure_env(spec, script)
    return (record.get("identity") == identity(spec, script, env)
            and record.get("productHash") == digest(product)
            and record.get("mapHash") == digest(map_path))


def publish(spec, script: str, product: str, map_path: str, env, task_stats=None) -> None:
    record = {"identity": identity(spec, script, env),
              "productHash": digest(product), "mapHash": digest(map_path),
              "tasks": task_stats}
    record["revision"] = hashlib.sha256(json.dumps(record, sort_keys=True).encode()).hexdigest()
    path = product + ".build.json"
    content = json.dumps(record, sort_keys=True).encode()
    artifacts.publish([(path, content)])
    temporary = Path(path + ".tmp")
    temporary.write_bytes(content)
    temporary.replace(path)
