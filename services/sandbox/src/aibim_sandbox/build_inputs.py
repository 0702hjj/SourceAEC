# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Versioned read-only build inputs, projected inside each private sandbox."""
from __future__ import annotations
from contextlib import contextmanager
from contextvars import ContextVar
import base64
import hashlib
import json
from pathlib import Path

_CURRENT = ContextVar("sourceaec_build_inputs", default=None)


def collect(root: str, model_id: str) -> dict[str, str]:
    base = Path(root)
    try:
        model = json.loads((base / "models" / model_id / "model.json").read_bytes())
        project = model.get("projectId", "")
    except (OSError, ValueError):
        return {}
    if not project.startswith("p_") or not project[2:].isalnum() or "/" in project:
        return {}
    result = {}
    total = 0
    for name in ("plans", "skill-work"):
        directory = base / name / project
        for path in directory.rglob("*"):
            if (path.is_file() and not path.is_symlink()
                    and path.suffix in {".json", ".py", ".dxf"}
                    and "plan_history" not in path.parts):
                size = path.stat().st_size
                total += size
                if total > 256 << 20:
                    raise ValueError("build inputs exceed 256 MiB")
                if size > 64 << 20:
                    raise ValueError("build input exceeds 64 MiB")
                content = path.read_bytes()
                result[path.relative_to(base).as_posix()] = base64.b64encode(content).decode()
    return result


@contextmanager
def use(inputs: dict[str, str]):
    token = _CURRENT.set(inputs)
    try:
        yield
    finally:
        _CURRENT.reset(token)


def current() -> dict[str, str]:
    return _CURRENT.get() or {}


def fingerprint() -> str:
    return hashlib.sha256(json.dumps(current(), sort_keys=True).encode()).hexdigest()


def project(workdir: str) -> str:
    target = Path(workdir) / ".build-inputs"
    target.mkdir()
    for key, encoded in current().items():
        path = target / key
        if ".." in Path(key).parts or Path(key).is_absolute():
            raise ValueError("invalid build input path")
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(base64.b64decode(encoded, validate=True))
        path.chmod(0o444)
    return str(target)


def active() -> bool:
    return _CURRENT.get() is not None
