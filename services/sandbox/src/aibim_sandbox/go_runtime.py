# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Private stdio transport to the Go business runtime; never mounted in a sandbox."""
from __future__ import annotations
import atexit
import json
import os
from pathlib import Path
import select
import subprocess
import threading
from contextvars import ContextVar

_TRANSACTIONS = ContextVar("sourceaec_go_transactions", default={})
_LOCK = threading.RLock()
_PROCESS = None


def call(op: str, root: str = "", args=None, *, tx: str | None = None):
    global _PROCESS
    root = str(Path(root).resolve()) if root else ""
    with _LOCK:
        if _PROCESS is None:
            binary = os.environ.get("SOURCEAEC_RUNTIME_BIN", "sourceaec-runtime")
            _PROCESS = subprocess.Popen([binary], stdin=subprocess.PIPE,
                                        stdout=subprocess.PIPE, text=True)
        if _PROCESS.poll() is not None:
            raise RuntimeError("Go business runtime exited; restart the edit service")
        request = {"op": op, "root": root, "args": args,
                   "tx": tx if tx is not None else _TRANSACTIONS.get().get(root, "")}
        _PROCESS.stdin.write(json.dumps(request, allow_nan=False) + "\n")
        _PROCESS.stdin.flush()
        if not select.select([_PROCESS.stdout], [], [], 180)[0]:
            _PROCESS.kill()
            raise RuntimeError("Go business runtime timed out; restart the edit service")
        line = _PROCESS.stdout.readline()
        if not line:
            raise RuntimeError("Go business runtime disconnected")
        result = json.loads(line)
        if result.get("error"):
            raise ValueError(result["error"])
        return result.get("data")


def _close():
    if _PROCESS is not None and _PROCESS.poll() is None:
        _PROCESS.stdin.close()
        try:
            _PROCESS.wait(timeout=5)
        except subprocess.TimeoutExpired:
            _PROCESS.kill()
            _PROCESS.wait()


atexit.register(_close)
