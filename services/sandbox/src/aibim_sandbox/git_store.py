# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Compatibility adapter for Go-owned Git candidates and PostgreSQL bundles."""
from contextlib import contextmanager
from pathlib import Path
from .go_runtime import call

_CANDIDATES = {}


def validate_branch(branch: str) -> None:
    # Validation is shared with the Go operation, without opening storage.
    call("git.validate", args={"branch": branch})


def commit(root, model_id, script, note="", source="human", inputs=None, version=None):
    return call("git.commit", root, {"model": model_id, "script": script,
                "note": note, "source": source, "inputs": inputs or {}, "version": version or ""})


def state(root, model_id):
    return call("git.state", root, {"model": model_id})


@contextmanager
def prepare(root, model_id, action, branch):
    result = call("git.prepare", root, {"model": model_id, "action": action, "branch": branch})
    repo = Path(result["repo"])
    _CANDIDATES[str(repo)] = result["candidate"]
    try:
        yield repo, result["branch"], result["script"]
    finally:
        _CANDIDATES.pop(str(repo), None)
        call("git.discard", root, {"model": model_id, "candidate": result["candidate"]})


def persist(root, model_id, repo, branch):
    call("git.persist", root, {"model": model_id, "candidate": _CANDIDATES[str(repo)]})


def import_history(root, model_id):
    call("git.import", root, {"model": model_id})


def candidate_inputs(root, model_id, repo):
    return call("git.inputs", root, {"model": model_id, "candidate": _CANDIDATES[str(repo)]})
