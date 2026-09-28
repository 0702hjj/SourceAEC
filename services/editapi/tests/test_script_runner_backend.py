# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""script_runner 后端选择与默认依赖（W-0048 T2/T4）：

Settings.sandbox_backend 显式化（auto/bwrap/rlimit），生产 fail-closed；
无 PEP 723 声明的存量脚本注入服务默认依赖集。

W-0057 T3 收编自两侧 test_script_runner_backend.py（94% 同构）：默认依赖
期望值走 profile.default_deps，产物扩展名经 profile 折算，其余判定与原
两侧逐条等价。
"""

from __future__ import annotations

import dataclasses
from pathlib import Path

import pytest
from fastapi import HTTPException

from aibim_sandbox import backend as sandbox_backend

from app import script_runner
from app.config import load_settings

from conftest import GOOD_SCRIPT, TargetProfile


class TestDefaultDeps:
    """W-0048 T4：无 PEP 723 声明的存量脚本注入服务默认依赖集（白嫖清单基线）。"""

    def test_default_deps(self, profile: TargetProfile):
        assert script_runner.CONFIG.default_deps == profile.default_deps


class TestSandboxBackendSelection:
    """W-0048 T2：后端选择显式化（Settings.sandbox_backend），rlimit 退役为测试夹具。

    生产 fail-closed：auto 且无 bwrap 一律 503，旧 ALLOW_RLIMIT_FALLBACK env
    不再放行；rlimit 仅测试可经 settings 显式选择（无 bwrap 环境由 conftest
    autouse 夹具注入 SANDBOX_BACKEND=rlimit）。
    """

    def test_settings_default_auto(self, monkeypatch):
        monkeypatch.delenv("SANDBOX_BACKEND", raising=False)
        assert load_settings().sandbox_backend == "auto"

    def test_settings_reads_sandbox_backend_env(self, monkeypatch):
        monkeypatch.setenv("SANDBOX_BACKEND", "rlimit")
        assert load_settings().sandbox_backend == "rlimit"

    def test_settings_invalid_backend_rejected(self, monkeypatch):
        monkeypatch.setenv("SANDBOX_BACKEND", "bogus")
        with pytest.raises(ValueError, match="SANDBOX_BACKEND"):
            load_settings()

    def test_auto_without_bwrap_503(
        self, settings, tmp_path: Path, profile: TargetProfile, monkeypatch
    ):
        """auto + 无 bwrap → 503；旧放行 env 设了也不放行（生产 env 不再影响）。"""
        monkeypatch.setenv("PATH", str(tmp_path))  # 遮蔽 bwrap，模拟无 bwrap 环境
        monkeypatch.setenv("ALLOW_RLIMIT_FALLBACK", "1")
        sandbox_backend._PROBE_OK = None
        rs = dataclasses.replace(settings, sandbox_backend="auto")
        out = tmp_path / f"out.{profile.ext}"
        with pytest.raises(HTTPException) as exc:
            script_runner.run_script(rs, GOOD_SCRIPT, str(out))
        assert exc.value.status_code == 503
        assert "ALLOW_RLIMIT_FALLBACK" not in str(exc.value.detail)
        assert not out.exists()

    def test_explicit_rlimit_backend_runs(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        """rlimit 仅测试可显式选：settings 级 rlimit 直接执行（无 env 放行）。"""
        rs = dataclasses.replace(settings, sandbox_backend="rlimit")
        out = tmp_path / f"out.{profile.ext}"
        script_runner.run_script(rs, GOOD_SCRIPT, str(out))
        assert out.is_file()
