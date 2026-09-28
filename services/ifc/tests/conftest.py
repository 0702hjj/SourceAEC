# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""Shared fixtures: copy the sample IFC into tmp_path so tests never touch the repo fixture."""

from __future__ import annotations

import shutil
from pathlib import Path

import pytest
from fastapi.testclient import TestClient

from aibim_sandbox import backend as sandbox_backend

from app.config import load_settings
from app.main import create_app

FIXTURE_IFC = (
    Path(__file__).resolve().parents[3]
    / "converter"
    / "test"
    / "fixtures"
    / "wall-with-opening-and-window.ifc"
)

MODEL_ID = "m_0123456789abcdef"


@pytest.fixture(autouse=True)
def _rlimit_backend_when_no_bwrap(monkeypatch):
    """W-0048 T2：rlimit 退役为测试夹具（ALLOW_RLIMIT_FALLBACK env 语义已删除）。

    无 bwrap 环境显式选择 rlimit 后端；有 bwrap 时不注入——auto 默认走 bwrap
    真路径。fail-closed 用例用 settings 级 auto + PATH 遮蔽覆盖两态。
    """
    if sandbox_backend.detect_backend() != "bwrap":
        monkeypatch.setenv("SANDBOX_BACKEND", "rlimit")


@pytest.fixture(autouse=True)
def _probe_cache_isolated():
    """探针缓存（环境事实）用例间隔离：PATH 遮蔽用例不污染后续真实探测。"""
    yield
    sandbox_backend._PROBE_OK = None


@pytest.fixture()
def settings():
    return load_settings()


@pytest.fixture()
def ifc_path(tmp_path: Path) -> Path:
    """Copy the sample IFC fixture into a tmp dir and return its path."""
    dst = tmp_path / "model.ifc"
    shutil.copy(FIXTURE_IFC, dst)
    return dst


@pytest.fixture()
def data_dir(tmp_path: Path) -> Path:
    """Viewer data dir with the fixture IFC registered under MODEL_ID."""
    uploads = tmp_path / "uploads"
    uploads.mkdir()
    dst = uploads / f"{MODEL_ID}.ifc"
    dst.write_bytes(FIXTURE_IFC.read_bytes())
    return tmp_path


@pytest.fixture()
def client(data_dir: Path, monkeypatch: pytest.MonkeyPatch) -> TestClient:
    monkeypatch.setenv("VIEWER_DATA_DIR", str(data_dir))
    return TestClient(create_app())
