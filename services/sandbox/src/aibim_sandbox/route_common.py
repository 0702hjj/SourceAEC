# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""Cross-route request helpers: single-point definitions.

AGENTS.md「校验与业务隔离」§3: request-shape helpers shared by route
modules are defined here exactly once — route files import, never redefine.

W-0048 T1：原 services/ifc 与 services/cad 的 route_common.py 合一——
扩展名参数化为 ``ext``；``model_lock`` 签名统一为
``(request, model_id, ext)``（cad 旧签名只吃 model_id），锁来源统一为
``request.app.state.model_locks``（ifc 挂 ModelRegistry——与
registry.save/unload 同一锁域；cad 挂本模块 ``ModelLocks`` LRU 实例），
服务侧 app/route_common.py 薄适配用 functools.partial 绑定 ext。
"""

from __future__ import annotations

import os
import threading
from collections import OrderedDict

from fastapi import HTTPException, Request

MODEL_ID_PATTERN = r"^m_[0-9a-f]{16}$"

LOCKS_MAX = 1024


class ModelLocks:
    """LRU-bounded per-key reentrant lock map.

    Hits move the entry to the end, inserts past the cap evict the oldest —
    bounded memory for unbounded keys. Hands out locks under a guard lock.
    """

    def __init__(self, max_locks: int = LOCKS_MAX) -> None:
        self._max = max_locks
        self._locks: "OrderedDict[str, threading.RLock]" = OrderedDict()
        self._guard = threading.Lock()

    def lock(self, key: str) -> threading.RLock:
        """Return the per-key reentrant lock (same lock for the same key)."""
        with self._guard:
            lock = self._locks.get(key)
            if lock is None:
                lock = threading.RLock()
                self._locks[key] = lock
            else:
                self._locks.move_to_end(key)
            while len(self._locks) > self._max:
                self._locks.popitem(last=False)
            return lock


def _upload_path(data_dir: str, model_id: str, ext: str) -> str:
    return os.path.join(data_dir, "uploads", f"{model_id}.{ext}")


def model_upload_path(request: Request, model_id: str, ext: str) -> str:
    """uploads/{id}.<ext> path; 404 when the model upload does not exist."""
    path = _upload_path(request.app.state.settings.data_dir, model_id, ext)
    if not os.path.isfile(path):
        raise HTTPException(status_code=404, detail="model not found")
    return path


def model_lock(request: Request, model_id: str, ext: str) -> threading.RLock:
    """Per-model lock keyed by the uploads path (shared across edit surfaces).

    The lock provider is ``request.app.state.model_locks`` — ifc installs its
    ModelRegistry (same lock domain as registry.save/unload), cad installs a
    ``ModelLocks`` instance.
    """
    path = _upload_path(request.app.state.settings.data_dir, model_id, ext)
    return request.app.state.model_locks.lock(os.path.abspath(path))
