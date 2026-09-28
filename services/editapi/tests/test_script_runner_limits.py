# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""script_runner 资源限额与并发闸（W-0047 沙箱加固）：

进程组击杀（killpg）、bwrap 挂载收窄、RLIMIT_FSIZE/NPROC、输出洪泛分块读、
产物/map 大小校验、进程级并发闸（满即 429）。

W-0057 T3 收编自两侧 test_script_runner_limits.py（94% 同构）：monkeypatch
seam 都在 aibim_sandbox（backend/runner），判定与原两侧逐条等价；GOOD
改写锚点经 ``GOOD_WRITE_STMT``、产物扩展名经 profile 折算。
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
import threading
import time
from pathlib import Path

import pytest
from fastapi import HTTPException

from aibim_sandbox import backend as sandbox_backend
from aibim_sandbox import runner as sandbox_runner

from app import script_runner

from conftest import (
    BIG_MAP_SCRIPT,
    GOOD_SCRIPT,
    GOOD_WRITE_STMT,
    MODERATE_STDOUT_SCRIPT,
    STDOUT_FLOOD_SCRIPT,
    TargetProfile,
)
from script_runner_scripts import (
    BIG_WRITE_SCRIPT,
    FORK_LOOP_SCRIPT,
    LEAK_PROBE_SCRIPT,
    RUNTIME_ERROR_SCRIPT,
)


def _pid_gone(pid: int) -> bool:
    """进程已消失或已僵死（待 reap，等同已死）。"""
    try:
        os.kill(pid, 0)
    except ProcessLookupError:
        return True
    try:
        with open(f"/proc/{pid}/stat", "r", encoding="ascii") as fh:
            return fh.read().rsplit(") ", 1)[1].split()[0] == "Z"
    except OSError:
        return True


class TestProcessGroupKill:
    """超时必须杀整个进程组（start_new_session + killpg）：脚本 fork 出的
    孙进程不得成孤儿继续跑。"""

    def test_timeout_kills_forked_children(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        out = tmp_path / f"fork.{profile.ext}"
        with pytest.raises(HTTPException) as exc:
            script_runner.run_script(settings, FORK_LOOP_SCRIPT, str(out), timeout=2)
        assert exc.value.status_code == 422
        assert "超时" in str(exc.value.detail)
        assert not out.exists()
        m = re.search(r"CHILD:(\d+)", str(exc.value.detail))
        assert m, f"fork 出的子进程 pid 应随 stderr 截尾带出: {exc.value.detail}"
        child_pid = int(m.group(1))
        # 条件等待 killpg 生效（禁止固定 sleep）
        deadline = time.monotonic() + 5
        while time.monotonic() < deadline:
            if _pid_gone(child_pid):
                break
            time.sleep(0.05)
        else:
            pytest.fail(f"forked child {child_pid} survived process-group kill")

    def test_limits_include_nproc(self):
        """preexec 的 rlimits 含 RLIMIT_NPROC（现有 task 数 + MAX_PROCS 余量，防 fork 炸弹）。"""
        budget = sandbox_backend._nproc_budget(script_runner.MAX_PROCS)
        assert budget >= script_runner.MAX_PROCS
        code = (
            "import json, resource, sys\n"
            "sys.stdout.write(json.dumps(resource.getrlimit(resource.RLIMIT_NPROC)))\n"
        )
        proc = subprocess.run(
            [sys.executable, "-c", code],
            preexec_fn=lambda: sandbox_backend._limits(script_runner.CONFIG,
                budget, script_runner.FSIZE_LIMIT_BYTES
            ),
            capture_output=True,
            timeout=10,
        )
        assert proc.returncode == 0, proc.stderr
        assert json.loads(proc.stdout) == [budget, budget]

    @pytest.mark.skipif(os.geteuid() == 0, reason="root 可能绕过 RLIMIT_NPROC")
    def test_nproc_budget_blocks_fork_bomb(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        """超出余量的 fork 被 RLIMIT_NPROC 拦截 → 422 且不产出文件。"""
        script = GOOD_SCRIPT.replace(
            GOOD_WRITE_STMT,
            "import os, time\n"
            f"        for _ in range({script_runner.MAX_PROCS} + 50):\n"
            "            if os.fork() == 0:\n"
            "                time.sleep(5)\n"
            "                os._exit(0)\n"
            "        fh.write('FORKED')",
        )
        out = tmp_path / f"bomb.{profile.ext}"
        with pytest.raises(HTTPException) as exc:
            script_runner.run_script(settings, script, str(out), timeout=30)
        assert exc.value.status_code == 422
        assert not out.exists()


class TestMountIsolation:
    """W-0047：bwrap 按需挂载（不再 --ro-bind / /）——/data 与 /etc 不进沙箱。

    整根只读挂载会把其他租户的模型（/data）挂给脚本只读，脚本可把内容写进
    自己产物经下载接口带出（跨租户读洞）。
    """

    @pytest.mark.skipif(
        sandbox_backend.detect_backend() != "bwrap",
        reason="挂载收窄只在 bwrap 后端生效（rlimit 降级不隔离 FS 读）",
    )
    def test_data_and_etc_not_readable(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        out = tmp_path / f"probe.{profile.ext}"
        script_runner.run_script(settings, LEAK_PROBE_SCRIPT, str(out))
        content = out.read_text(encoding="utf-8")
        assert content.startswith("BLOCKED"), f"沙箱内读到了宿主路径: {content}"
        assert "TMP:ok" in content  # --tmpfs /tmp 仍需可用


class TestFileSizeLimit:
    """W-0047：RLIMIT_FSIZE 限单文件写（防写满 /data 卷），env 可配。"""

    def test_limits_include_fsize(self):
        code = (
            "import json, resource, sys\n"
            "sys.stdout.write(json.dumps(resource.getrlimit(resource.RLIMIT_FSIZE)))\n"
        )
        proc = subprocess.run(
            [sys.executable, "-c", code],
            preexec_fn=lambda: sandbox_backend._limits(script_runner.CONFIG,
                sandbox_backend._nproc_budget(script_runner.MAX_PROCS), 12345
            ),
            capture_output=True,
            timeout=10,
        )
        assert proc.returncode == 0, proc.stderr
        assert json.loads(proc.stdout) == [12345, 12345]

    def test_oversize_write_killed(
        self, settings, tmp_path: Path, profile: TargetProfile, monkeypatch
    ):
        monkeypatch.setenv("SCRIPT_MAX_FSIZE_BYTES", str(1 << 20))
        out = tmp_path / f"big.{profile.ext}"
        with pytest.raises(HTTPException) as exc:
            script_runner.run_script(settings, BIG_WRITE_SCRIPT, str(out), timeout=30)
        assert exc.value.status_code == 422
        assert not out.exists()


class TestOutputFlood:
    """W-0047：stdout/stderr 分块读，累计超上限杀进程组（不再全量读入内存）。"""

    def test_stdout_flood_killed(
        self, settings, tmp_path: Path, profile: TargetProfile, monkeypatch
    ):
        monkeypatch.setenv("SCRIPT_MAX_OUTPUT_BYTES", str(64 << 10))
        out = tmp_path / f"flood.{profile.ext}"
        with pytest.raises(HTTPException) as exc:
            script_runner.run_script(settings, STDOUT_FLOOD_SCRIPT, str(out), timeout=30)
        assert exc.value.status_code == 422
        assert "输出" in str(exc.value.detail)
        assert not out.exists()

    def test_moderate_output_still_runs(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        out = tmp_path / f"ok.{profile.ext}"
        script_runner.run_script(settings, MODERATE_STDOUT_SCRIPT, str(out))
        assert out.is_file()


class TestProductLimit:
    """W-0047：产物与 map sidecar 发布前大小校验（env SCRIPT_MAX_PRODUCT_BYTES）。"""

    def test_oversize_product_rejected(
        self, settings, tmp_path: Path, profile: TargetProfile, monkeypatch
    ):
        monkeypatch.setenv("SCRIPT_MAX_PRODUCT_BYTES", "4096")
        out = tmp_path / f"big.{profile.ext}"
        with pytest.raises(HTTPException) as exc:
            script_runner.run_script(settings, BIG_WRITE_SCRIPT, str(out))
        assert exc.value.status_code == 422
        assert "上限" in str(exc.value.detail)
        assert not out.exists()

    def test_oversize_map_rejected_before_publish(
        self, settings, tmp_path: Path, profile: TargetProfile, monkeypatch
    ):
        """map 超限与产物一样在发布前拒绝：产物也不落盘（不留错位产物）。"""
        monkeypatch.setenv("SCRIPT_MAX_PRODUCT_BYTES", "4096")
        out = tmp_path / f"model.{profile.ext}"
        with pytest.raises(HTTPException) as exc:
            script_runner.run_script(settings, BIG_MAP_SCRIPT, str(out))
        assert exc.value.status_code == 422
        assert "map" in str(exc.value.detail)
        assert not out.exists()
        assert not Path(str(out) + ".map.json").exists()


class TestConcurrencyGate:
    """W-0047：进程级并发闸（SCRIPT_RUN_CONCURRENCY，默认 3），满即 429。"""

    def test_gate_full_rejects_429(
        self, settings, tmp_path: Path, profile: TargetProfile, monkeypatch
    ):
        monkeypatch.setattr(sandbox_runner, "_RUN_GATE", threading.Semaphore(0))
        out = tmp_path / f"out.{profile.ext}"
        with pytest.raises(HTTPException) as exc:
            script_runner.run_script(settings, GOOD_SCRIPT, str(out))
        assert exc.value.status_code == 429
        assert not out.exists()

    def test_gate_released_after_run(
        self, settings, tmp_path: Path, profile: TargetProfile, monkeypatch
    ):
        """闸在 run 结束（含失败）后释放：连续两次 run 不互锁。"""
        monkeypatch.setattr(sandbox_runner, "_RUN_GATE", threading.Semaphore(1))
        out1, out2 = tmp_path / f"a.{profile.ext}", tmp_path / f"b.{profile.ext}"
        script_runner.run_script(settings, GOOD_SCRIPT, str(out1))
        script_runner.run_script(settings, GOOD_SCRIPT, str(out2))
        assert out1.is_file() and out2.is_file()

    def test_gate_released_after_failure(
        self, settings, tmp_path: Path, profile: TargetProfile, monkeypatch
    ):
        monkeypatch.setattr(sandbox_runner, "_RUN_GATE", threading.Semaphore(1))
        with pytest.raises(HTTPException):
            script_runner.run_script(
                settings, RUNTIME_ERROR_SCRIPT, str(tmp_path / f"x.{profile.ext}")
            )
        out = tmp_path / f"ok.{profile.ext}"
        script_runner.run_script(settings, GOOD_SCRIPT, str(out))
        assert out.is_file()
