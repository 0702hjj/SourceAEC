# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""SQLite business-artifact store shared with the gateway.

Workspace files are projections for tools requiring filenames. Derived viewer
files and historical product caches are intentionally not durable artifacts.
"""
from __future__ import annotations

from contextlib import contextmanager
import os
from pathlib import Path
import sqlite3
import threading
from typing import Iterable

_ROOTS: set[Path] = set()
_LOCK = threading.RLock()


@contextmanager
def connect(root: str | Path):
    db = sqlite3.connect(str(Path(root) / "sourceaec.sqlite3"), timeout=30)
    db.execute("PRAGMA journal_mode=WAL")
    db.execute("PRAGMA foreign_keys=ON")
    db.execute("CREATE TABLE IF NOT EXISTS artifacts "
               "(path TEXT PRIMARY KEY, content BLOB NOT NULL, mode INTEGER NOT NULL DEFAULT 420)")
    try:
        with db:
            yield db
    finally:
        db.close()


def durable(key: str) -> bool:
    parts = Path(key).parts
    return (not key.endswith(".tmp") and ".git" not in parts
            and not any(p.endswith("_cache") for p in parts)
            and "versions" not in parts
            and Path(key).name not in {"model.xkt", "metadata.json", "render.json"})


def initialize(root: str) -> None:
    base = Path(root).resolve()
    with _LOCK:
        if base in _ROOTS:
            return
        base.mkdir(parents=True, exist_ok=True)
        with connect(base) as db:
            db.execute("CREATE TABLE IF NOT EXISTS migrations (name TEXT PRIMARY KEY)")
            imported = db.execute("SELECT 1 FROM migrations WHERE name='files-v1'").fetchone()
            for name in (() if imported else ("models", "uploads", "projects", "plans", "chat", "skill-work")):
                for path in (base / name).rglob("*"):
                    key = path.relative_to(base).as_posix()
                    if path.is_file() and not path.is_symlink() and durable(key):
                        db.execute("INSERT OR IGNORE INTO artifacts(path,content) VALUES(?,?)",
                                   (key, path.read_bytes()))
            db.execute("INSERT OR IGNORE INTO migrations(name) VALUES('files-v1')")
            for key, content, mode in db.execute("SELECT path,content,mode FROM artifacts"):
                path = (base / key).resolve()
                if not path.is_relative_to(base):
                    raise ValueError("invalid artifact path")
                if not path.exists() or path.read_bytes() != content:
                    path.parent.mkdir(parents=True, exist_ok=True)
                    path.write_bytes(content)
                    path.chmod(mode)
        _ROOTS.add(base)


def owner(path: str | Path) -> tuple[Path, str] | None:
    target = Path(path).resolve()
    for root in tuple(_ROOTS):
        if target.is_relative_to(root):
            key = target.relative_to(root).as_posix()
            return (root, key) if durable(key) else None
    return None


def publish(files: Iterable[tuple[str | Path, bytes]]) -> None:
    """Commit a set of artifacts together before exposing their projections."""
    groups: dict[Path, list[tuple[str, bytes]]] = {}
    for path, content in files:
        match = owner(path)
        if match:
            root, key = match
            groups.setdefault(root, []).append((key, content))
    for root, entries in groups.items():
        with connect(root) as db:
            db.executemany("INSERT INTO artifacts(path,content) VALUES(?,?) "
                           "ON CONFLICT(path) DO UPDATE SET content=excluded.content", entries)


def capture(path: str | Path) -> None:
    publish([(path, Path(path).read_bytes())])


def capture_tree(path: str | Path) -> None:
    base = Path(path)
    publish((p, p.read_bytes()) for p in base.rglob("*")
            if p.is_file() and not p.is_symlink() and owner(p))


def remove(path: str | Path) -> None:
    match = owner(path)
    if match:
        root, key = match
        with connect(root) as db:
            db.execute("DELETE FROM artifacts WHERE path=?", (key,))


def read(path: str | Path) -> bytes:
    match = owner(path)
    if match:
        root, key = match
        with connect(root) as db:
            row = db.execute("SELECT content FROM artifacts WHERE path=?", (key,)).fetchone()
        if row:
            return row[0]
    return Path(path).read_bytes()


def replace(source: str, destination: str) -> None:
    capture_data = Path(source).read_bytes()
    publish([(destination, capture_data)])
    os.replace(source, destination)
