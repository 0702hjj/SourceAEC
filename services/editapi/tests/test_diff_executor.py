# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Timeouts must cancel queued work without pretending to stop running threads."""
from concurrent.futures import ThreadPoolExecutor
import threading
from types import SimpleNamespace

from fastapi import HTTPException
import pytest
from aibim_editapi import routes_diff


def test_timed_out_queued_diff_never_starts(monkeypatch):
    release = threading.Event()
    occupied = threading.Event()
    computed = threading.Event()
    def busy():
        occupied.set()
        assert release.wait(5)
    with ThreadPoolExecutor(max_workers=1) as executor:
        monkeypatch.setattr(routes_diff, "_DIFF_EXECUTOR", executor)
        blocker = executor.submit(busy)
        try:
            assert occupied.wait(2)
            with pytest.raises(HTTPException) as error:
                routes_diff.verify_diff_within_timeout(
                    SimpleNamespace(diff_timeout_s=0.05), lambda: computed.set() or {})
            assert error.value.status_code == 504
        finally:
            release.set()
        blocker.result(timeout=2)
        # A sentinel completes after the timed-out queue entry has been drained.
        executor.submit(lambda: None).result(timeout=2)
        assert not computed.is_set()
