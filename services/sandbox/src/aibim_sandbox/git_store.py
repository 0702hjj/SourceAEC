# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""Real Git commits/branches/merges; complete bundles are durable SQLite blobs.

Each operation uses a private temporary worktree. No user-controlled ref is
passed as an option and merge conflicts never mutate the stored repository.
"""
from __future__ import annotations

from contextlib import contextmanager
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
from typing import Iterator

from .artifacts import connect

_BRANCH = re.compile(r"^[a-zA-Z0-9][a-zA-Z0-9_/-]{0,79}$")


def validate_branch(branch: str) -> None:
    if not _BRANCH.fullmatch(branch) or "//" in branch or branch.endswith("/"):
        raise ValueError("invalid branch name")


def _git(repo: Path, *args: str) -> str:
    proc = subprocess.run(["git", "-C", str(repo), "-c", "user.name=SourceAEC",
                           "-c", "user.email=sourceaec@localhost",
                           "-c", "core.hooksPath=/dev/null", *args],
                          capture_output=True, text=True, timeout=30,
                          env={"PATH": os.environ.get("PATH", ""), "LANG": "C.UTF-8",
                               "GIT_CONFIG_GLOBAL": os.devnull, "GIT_CONFIG_NOSYSTEM": "1"})
    if proc.returncode:
        raise ValueError(proc.stderr.strip() or proc.stdout.strip() or "Git operation failed")
    return proc.stdout.strip()


def _schema(db) -> None:
    db.execute("CREATE TABLE IF NOT EXISTS repositories "
               "(model_id TEXT PRIMARY KEY, bundle BLOB NOT NULL, branch TEXT NOT NULL)")


@contextmanager
def workspace(root: str, model_id: str) -> Iterator[tuple[Path, str]]:
    with connect(root) as db:
        _schema(db)
        row = db.execute("SELECT bundle,branch FROM repositories WHERE model_id=?",
                         (model_id,)).fetchone()
    with tempfile.TemporaryDirectory(prefix="sourceaec-git-") as directory:
        repo = Path(directory) / "repo"
        repo.mkdir()
        _git(repo, "init", "-b", "main")
        branch = "main"
        if row:
            bundle = Path(directory) / "history.bundle"
            bundle.write_bytes(row[0])
            _git(repo, "fetch", str(bundle), "+refs/heads/*:refs/heads/*",
                 "+refs/tags/*:refs/tags/*", "+refs/notes/*:refs/notes/*", "--update-head-ok")
            branch = row[1]
            _git(repo, "checkout", branch)
        yield repo, branch


def persist(root: str, model_id: str, repo: Path, branch: str) -> None:
    bundle = repo.parent / "saved.bundle"
    _git(repo, "bundle", "create", str(bundle), "--all")
    with connect(root) as db:
        _schema(db)
        db.execute("INSERT INTO repositories(model_id,bundle,branch) VALUES(?,?,?) "
                   "ON CONFLICT(model_id) DO UPDATE SET bundle=excluded.bundle,branch=excluded.branch",
                   (model_id, bundle.read_bytes(), branch))


def _commit(repo: Path, script: str, note: str, source: str,
            inputs: dict | None = None) -> str:
    (repo / "build.py").write_text(script, encoding="utf-8")
    if inputs is not None:
        identity = inputs.get("current.build.json", {}).get("identity", {})
        # Derived hashes and source maps change on every build. Store them as
        # commit notes, so they do not create spurious source-merge conflicts.
        environment = {name: identity[name] for name in
                       ("python", "packages", "environment", "helpers", "engine") if name in identity}
        tracked = {"current.inputs.json": inputs.get("current.inputs.json", {}),
                   "current.build.json": {"identity": environment}}
        (repo / "inputs.json").write_text(json.dumps(tracked, sort_keys=True, indent=2) + "\n", encoding="utf-8")
    _git(repo, "add", "--", "build.py", "inputs.json")
    _git(repo, "commit", "--allow-empty", "-m", note or "Update model",
         "-m", f"SourceAEC-Source: {source}")
    revision = _git(repo, "rev-parse", "HEAD")
    if inputs:
        _git(repo, "notes", "--ref=build", "add", "-f", "-m", json.dumps(inputs, sort_keys=True), revision)
    return revision


def commit(root: str, model_id: str, script: str, note: str = "",
           source: str = "human", inputs: dict | None = None,
           version: str | None = None) -> dict:
    with workspace(root, model_id) as (repo, branch):
        revision = _commit(repo, script, note, source, inputs or {})
        if version:
            _git(repo, "tag", version)
        persist(root, model_id, repo, branch)
        return {"commit": revision, "branch": branch}


def state(root: str, model_id: str) -> dict:
    with workspace(root, model_id) as (repo, branch):
        refs = _git(repo, "for-each-ref", "--format=%(refname:short)", "refs/heads").splitlines()
        if not refs:
            return {"branch": branch, "branches": [], "commits": []}
        raw = _git(repo, "log", "--branches", "--tags", "--topo-order", "-100", "--format=%H%x09%P%x09%s")
        commits = []
        for line in raw.splitlines():
            revision, parents, note = line.split("\t", 2)
            commits.append({"commit": revision, "parents": parents.split(), "note": note})
        return {"branch": branch, "branches": refs, "commits": commits,
                "commit": _git(repo, "rev-parse", "HEAD")}


def prepare(root: str, model_id: str, action: str, branch: str):
    """Yield a candidate repository; callers build its source before publishing."""
    validate_branch(branch)
    return _candidate(root, model_id, action, branch)


@contextmanager
def _candidate(root: str, model_id: str, action: str, branch: str):
    with workspace(root, model_id) as (repo, current):
        if action == "create":
            _git(repo, "checkout", "-b", branch)
            current = branch
        elif action == "checkout":
            _git(repo, "checkout", branch)
            current = branch
        elif action == "merge":
            _git(repo, "merge", "--no-ff", "--no-edit", branch)
        else:
            raise ValueError("unknown version action")
        yield repo, current, (repo / "build.py").read_text(encoding="utf-8")


def import_history(root: str, model_id: str) -> None:
    with connect(root) as db:
        _schema(db)
        if db.execute("SELECT 1 FROM repositories WHERE model_id=?", (model_id,)).fetchone():
            return
    directory = Path(root) / "models" / model_id / "scripts"
    paths = sorted((p for p in directory.glob("v*.py") if re.fullmatch(r"v[0-9]+", p.stem)),
                   key=lambda p: int(p.stem[1:]))
    for path in paths:
        commit(root, model_id, path.read_text(), f"Import {path.stem}",
               source="migration", version=path.stem)
