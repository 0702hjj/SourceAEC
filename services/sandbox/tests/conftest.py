# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""conftest — 双实现参数化夹具。

T2 起 rlimit 退役为测试夹具（ALLOW_RLIMIT_FALLBACK env 放行语义已删除）：
无 bwrap 环境由 autouse 夹具注入 ``SANDBOX_BACKEND=rlimit``（显式测试选择），
有 bwrap 时不注入——auto 默认走 bwrap 真路径；fail-closed 两态由
TestFailClosed/TestVerifySandboxBackend 用 settings 级 auto + PATH 遮蔽覆盖。
"""

from __future__ import annotations

import sys
from pathlib import Path

import pytest

sys.path.insert(0, str(Path(__file__).resolve().parent))

from aibim_sandbox import backend  # noqa: E402
from adapters import ADAPTER_NAMES, get_adapter  # noqa: E402


@pytest.fixture(autouse=True)
def _rlimit_backend_when_no_bwrap(monkeypatch):
    """无 bwrap 环境：测试显式选择 rlimit 后端（rlimit 仅测试可设）。"""
    if backend.detect_backend() != "bwrap":
        monkeypatch.setenv("SANDBOX_BACKEND", "rlimit")


@pytest.fixture(autouse=True)
def _probe_cache_isolated():
    """探针缓存（环境事实）用例间隔离：PATH 遮蔽用例不污染后续真实探测。"""
    yield
    backend._PROBE_OK = None


@pytest.fixture(params=ADAPTER_NAMES, ids=ADAPTER_NAMES)
def adapter(request):
    """同一组契约测试在 ifc/cad 两套实现上各跑一遍。"""
    return get_adapter(request.param)


@pytest.fixture()
def settings(adapter):
    return adapter.make_settings()
