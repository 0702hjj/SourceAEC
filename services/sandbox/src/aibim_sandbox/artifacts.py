# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Filesystem compatibility adapter. PostgreSQL and transactions belong to Go."""
from __future__ import annotations
import base64
from contextlib import contextmanager
import os
from pathlib import Path
from typing import Iterable
from .go_runtime import call, _TRANSACTIONS

_ROOTS: set[Path] = set()


def durable(key: str) -> bool:
    return call("durable", args=key)


def initialize(root: str) -> None:
    base = Path(root).resolve()
    if base not in _ROOTS:
        call("initialize", str(base))
        _ROOTS.add(base)


def owner(path: str | Path) -> tuple[Path, str] | None:
    target = Path(path).absolute()
    for root in sorted(tuple(_ROOTS), key=lambda p: len(str(p)), reverse=True):
        if target.is_relative_to(root):
            key = target.relative_to(root).as_posix()
            return (root, key) if durable(key) else None
    return None


def publish(files: Iterable[tuple[str | Path, bytes]]) -> None:
    groups = {}
    for path, content in files:
        match = owner(path)
        if match:
            root, key = match
            groups.setdefault(root, []).append({"path": key, "content": base64.b64encode(content).decode()})
    for root, entries in groups.items():
        call("publish", str(root), entries)


def capture(path: str | Path) -> None:
    publish([(path, Path(path).read_bytes())])


def capture_tree(path: str | Path) -> None:
    match = owner(path)
    if match:
        root, key = match
        call("capture_tree", str(root), key)


def remove(path: str | Path) -> None:
    track_projection(str(path))
    match = owner(path)
    if match:
        call("remove", str(match[0]), match[1])


def read(path: str | Path) -> bytes:
    match = owner(path)
    if match:
        return base64.b64decode(call("read", str(match[0]), match[1]))
    return Path(path).read_bytes()


def replace(source: str, destination: str) -> None:
    publish([(destination, Path(source).read_bytes())])
    os.replace(source, destination)


@contextmanager
def transaction(root: str):
    root = str(Path(root).resolve())
    if root in _TRANSACTIONS.get():
        yield
        return
    transaction_id = call("begin", root)
    token = _TRANSACTIONS.set({**_TRANSACTIONS.get(), root: transaction_id})
    try:
        yield
    except BaseException:
        call("rollback", root)
        raise
    else:
        call("commit", root)
    finally:
        _TRANSACTIONS.reset(token)


def track_projection(path: str) -> None:
    target = Path(path).absolute()
    for root in _TRANSACTIONS.get():
        if target.is_relative_to(Path(root)):
            call("track", root, target.relative_to(root).as_posix())
            return
