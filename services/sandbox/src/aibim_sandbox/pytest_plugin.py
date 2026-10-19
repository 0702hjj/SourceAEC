# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj
"""Release per-test PostgreSQL pools after all service fixtures have drained."""
import pytest


@pytest.fixture(autouse=True)
def _release_go_workspace_pools():
    from aibim_sandbox import artifacts
    from aibim_sandbox.go_runtime import call
    before = set(artifacts._ROOTS)
    yield
    for root in set(artifacts._ROOTS) - before:
        call("release", str(root))
        artifacts._ROOTS.discard(root)
