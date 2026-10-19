# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Replay versioned inputs and exact dependency requirements for old sources."""
from contextlib import contextmanager
import json
from pathlib import Path
from aibim_sandbox import build_inputs, deps


@contextmanager
def replay(directory: str, version: str):
    base = Path(directory)
    def read(suffix):
        path = base / f"{version}.{suffix}.json"
        return json.loads(path.read_bytes()) if path.exists() else {}
    inputs = read("inputs")
    manifest = read("build")
    packages = manifest.get("identity", {}).get("packages", {})
    requirements = tuple(f"{name}=={version}" for name, version in sorted(packages.items())) or None
    with build_inputs.use(inputs), deps.use_dependencies(requirements, manifest.get("identity", {}).get("environment")):
        yield
