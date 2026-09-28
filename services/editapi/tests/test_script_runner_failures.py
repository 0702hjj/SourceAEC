# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""script_runner 失败语义与恶意用例：422 细节透传 + 三恶意拦截
（死循环超时 / 超内存 rlimit / 越界写 bwrap）+ 网络隔离。

W-0057 T3 收编自两侧 test_script_runner_failures.py（90% 同构）：判定与
原两侧逐条等价；GOOD 改写锚点（ifc ISO 头 / cad SECTION 头）经
``GOOD_WRITE_STMT`` 单点物化，产物扩展名经 profile 折算。
"""

from __future__ import annotations

from pathlib import Path

import pytest
from fastapi import HTTPException

from aibim_sandbox import backend as sandbox_backend

from app import script_runner

from conftest import (
    ESCAPE_WRITE_SCRIPT,
    GOOD_SCRIPT,
    GOOD_WRITE_STMT,
    NO_OUTPUT_SCRIPT,
    TargetProfile,
)
from script_runner_scripts import (
    INFINITE_LOOP_SCRIPT,
    MEMORY_BOMB_SCRIPT,
    NOISY_FAILURE_SCRIPT,
    RUNTIME_ERROR_SCRIPT,
)


class TestFailureSemantics:
    def test_runtime_error_422_with_stderr(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        out = tmp_path / f"out.{profile.ext}"
        with pytest.raises(HTTPException) as exc:
            script_runner.run_script(settings, RUNTIME_ERROR_SCRIPT, str(out))
        assert exc.value.status_code == 422
        assert "boom-marker" in str(exc.value.detail)
        assert not out.exists()

    def test_stderr_tail_truncated(self, settings, tmp_path: Path, profile: TargetProfile):
        with pytest.raises(HTTPException) as exc:
            script_runner.run_script(
                settings, NOISY_FAILURE_SCRIPT, str(tmp_path / f"out.{profile.ext}")
            )
        detail = str(exc.value.detail)
        assert len(detail) < 10000
        assert "E" * 100 in detail  # tail kept

    def test_missing_output_422(self, settings, tmp_path: Path, profile: TargetProfile):
        with pytest.raises(HTTPException) as exc:
            script_runner.run_script(
                settings, NO_OUTPUT_SCRIPT, str(tmp_path / f"o.{profile.ext}")
            )
        assert exc.value.status_code == 422
        assert "产出" in str(exc.value.detail) or "output" in str(exc.value.detail).lower()


class TestMalicious:
    """死循环 / 超内存 / 越界写，全部被拦截且不产出文件。"""

    def test_infinite_loop_killed_by_timeout(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        out = tmp_path / f"loop.{profile.ext}"
        with pytest.raises(HTTPException) as exc:
            script_runner.run_script(settings, INFINITE_LOOP_SCRIPT, str(out), timeout=2)
        assert exc.value.status_code == 422
        assert "超时" in str(exc.value.detail) or "timeout" in str(exc.value.detail).lower()
        assert not out.exists()

    def test_memory_bomb_killed_by_rlimit(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        out = tmp_path / f"bomb.{profile.ext}"
        with pytest.raises(HTTPException) as exc:
            script_runner.run_script(settings, MEMORY_BOMB_SCRIPT, str(out), timeout=30)
        assert exc.value.status_code == 422
        assert not out.exists()

    @pytest.mark.skipif(
        sandbox_backend.detect_backend() != "bwrap",
        reason="越界写硬拦截需要 bwrap（rlimit 降级模式不拦截 FS 写）",
    )
    def test_write_outside_sandbox_blocked(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        target = tmp_path / "evil.txt"
        script = ESCAPE_WRITE_SCRIPT.replace(
            'params["target"]', repr(str(target))
        )
        out = tmp_path / f"out.{profile.ext}"
        with pytest.raises(HTTPException) as exc:
            script_runner.run_script(settings, script, str(out))
        assert exc.value.status_code == 422
        assert not target.exists()
        assert not out.exists()

    def test_no_network_in_sandbox(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        if sandbox_backend.detect_backend() != "bwrap":
            pytest.skip("网络隔离由 bwrap --unshare-net 提供")
        script = GOOD_SCRIPT.replace(
            GOOD_WRITE_STMT,
            "import socket\n"
            "        try:\n"
            "            socket.create_connection(('8.8.8.8', 53), timeout=3)\n"
            "            fh.write('NET-OPEN')\n"
            "        except OSError:\n"
            "            fh.write('NET-BLOCKED')",
        )
        out = tmp_path / f"net.{profile.ext}"
        script_runner.run_script(settings, script, str(out))
        assert out.read_text(encoding="utf-8") == "NET-BLOCKED"
