# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""test_sandbox_units.py — 纯函数/常量/环境构造契约（ifc/cad 双实现参数化）。

不起子进程真跑脚本（除 _limits 的 rlimit 探针），钉死 T1 合一前两侧必须
一致的共享件：常量值、script_hash、_int_env、_tail、_OutputGuard、
_sandbox_env、_sandbox_cmd、verify_sandbox_backend、_nproc_budget/_limits。
"""

from __future__ import annotations

import hashlib
import io
import json
import subprocess
import sys

import pytest
from fastapi import HTTPException


class TestConstants:
    """共享常量值两侧一致（T1 单一源的取值基线）。"""

    @pytest.mark.parametrize(
        "name,expected",
        [
            ("RUN_TIMEOUT_S", 60),
            ("MEM_LIMIT_BYTES", 1 << 30),
            ("MAX_PROCS", 256),
            ("STDERR_TAIL_BYTES", 2048),
            ("FSIZE_LIMIT_BYTES", 256 << 20),
            ("OUTPUT_LIMIT_BYTES", 1 << 20),
            ("PRODUCT_LIMIT_BYTES", 256 << 20),
            ("RUN_CONCURRENCY", 3),
        ],
    )
    def test_constant_values(self, adapter, name, expected):
        assert getattr(adapter.runner, name) == expected


class TestScriptHash:
    def test_matches_sha256_hexdigest(self, adapter):
        text = "PARAMS = {}\n"
        assert adapter.runner.script_hash(text) == hashlib.sha256(
            text.encode("utf-8")
        ).hexdigest()

    def test_stable_and_sensitive(self, adapter):
        assert adapter.runner.script_hash("abc") == adapter.runner.script_hash("abc")
        assert adapter.runner.script_hash("abc") != adapter.runner.script_hash("abc ")
        assert adapter.runner.script_hash("中文") != adapter.runner.script_hash("abc")


class TestIntEnv:
    @pytest.mark.parametrize(
        "raw,expected",
        [(None, 42), ("", 42), ("abc", 42), ("0", 42), ("-5", 42), ("7", 7)],
    )
    def test_int_env_fallback(self, adapter, monkeypatch, raw, expected):
        if raw is None:
            monkeypatch.delenv("X_CONTRACT_INT", raising=False)
        else:
            monkeypatch.setenv("X_CONTRACT_INT", raw)
        assert adapter.runner._int_env("X_CONTRACT_INT", 42) == expected


class TestTail:
    def test_short_data_unchanged(self, adapter):
        assert adapter.runner._tail(b"hello") == "hello"

    def test_keeps_last_limit_bytes(self, adapter):
        data = b"a" * 100 + b"b" * 2048
        assert adapter.runner._tail(data) == "b" * 2048

    def test_invalid_utf8_replaced(self, adapter):
        out = adapter.runner._tail(b"\xff\xfe ok")
        assert out.endswith(" ok")
        assert "�" in out


class TestOutputGuard:
    """分块泵：累计计数、超 cap 置 exceeded、stderr 只留尾。"""

    def _pump(self, guard, data: bytes, *, is_stderr: bool) -> None:
        guard.pump(io.BytesIO(data), is_stderr=is_stderr)

    def test_total_accumulates(self, adapter):
        guard = adapter.runner._OutputGuard(1 << 20)
        self._pump(guard, b"x" * 100, is_stderr=False)
        self._pump(guard, b"y" * 50, is_stderr=True)
        assert guard.total == 150
        assert not guard.exceeded.is_set()

    def test_exceeded_when_over_cap(self, adapter):
        guard = adapter.runner._OutputGuard(100)
        self._pump(guard, b"x" * 100, is_stderr=False)
        assert not guard.exceeded.is_set()
        self._pump(guard, b"x", is_stderr=False)
        assert guard.exceeded.is_set()

    def test_stderr_tail_bounded(self, adapter):
        guard = adapter.runner._OutputGuard(1 << 20)
        self._pump(guard, b"e" * 10000, is_stderr=True)
        assert len(guard.stderr_tail) == adapter.runner.STDERR_TAIL_BYTES

    def test_stdout_not_kept(self, adapter):
        guard = adapter.runner._OutputGuard(1 << 20)
        self._pump(guard, b"o" * 5000, is_stderr=False)
        assert bytes(guard.stderr_tail) == b""


class TestLimits:
    """preexec rlimits：RLIMIT_NPROC（预算值）与 RLIMIT_FSIZE（env 值）生效。"""

    def test_nproc_budget_at_least_max_procs(self, adapter):
        assert adapter.runner._nproc_budget() >= adapter.runner.MAX_PROCS

    def test_limits_include_nproc_and_fsize(self, adapter):
        runner = adapter.runner
        budget = runner._nproc_budget()
        code = (
            "import json, resource\n"
            "print(json.dumps([resource.getrlimit(resource.RLIMIT_NPROC),"
            " resource.getrlimit(resource.RLIMIT_FSIZE)]))"
        )
        proc = subprocess.run(
            [sys.executable, "-c", code],
            preexec_fn=lambda: runner._limits(budget, 12345),
            capture_output=True,
            timeout=10,
        )
        assert proc.returncode == 0, proc.stderr
        (nproc, fsize) = json.loads(proc.stdout)
        assert nproc == [budget, budget]
        assert fsize == [12345, 12345]


class TestSandboxEnv:
    """沙箱环境变量：最小键集与取值；PYTHONPATH 以 flows_dir 开头。

    cad 额外在 PYTHONPATH 追加 drawlib_dir（适配器声明的差异，此处只钉公共
    前缀与公共键）。
    """

    def test_common_keys(self, adapter, settings, tmp_path):
        env = adapter.runner._sandbox_env(settings, str(tmp_path))
        assert env["PYTHONDONTWRITEBYTECODE"] == "1"
        assert env["PYTHONUTF8"] == "1"
        for var in ("OPENBLAS_NUM_THREADS", "OMP_NUM_THREADS",
                    "MKL_NUM_THREADS", "NUMEXPR_NUM_THREADS"):
            assert env[var] == "1"
        assert env["HOME"] == str(tmp_path)
        assert env["TMPDIR"] == str(tmp_path)
        assert "PATH" in env

    def test_pythonpath_starts_with_flows_dir(self, adapter, settings, tmp_path):
        env = adapter.runner._sandbox_env(settings, str(tmp_path))
        assert env["PYTHONPATH"].split(":")[0] == settings.flows_dir


class TestSandboxCmd:
    """bwrap 命令包装：rlimit 后端为空前缀；bwrap 后端含隔离旗标且不整根挂载。

    T2 起后端选择走 settings.sandbox_backend 显式值（``with_backend`` 副本），
    不再 monkeypatch detect_backend。
    """

    def test_rlimit_backend_empty_prefix(self, adapter, settings, tmp_path):
        rs = adapter.with_backend(settings, "rlimit")
        assert adapter.runner._sandbox_cmd(rs, str(tmp_path)) == []

    def test_bwrap_prefix_flags(self, adapter, settings, tmp_path):
        rs = adapter.with_backend(settings, "bwrap")
        cmd = adapter.runner._sandbox_cmd(rs, str(tmp_path))
        cmdstr = " ".join(cmd)
        assert "--unshare-net" in cmd
        assert "--die-with-parent" in cmd
        assert "--tmpfs /tmp" in cmdstr
        assert f"--ro-bind {settings.flows_dir} {settings.flows_dir}" in cmdstr
        assert f"--bind {tmp_path} {tmp_path}" in cmdstr
        assert f"--chdir {tmp_path}" in cmdstr
        # W-0047 挂载收窄：不整根挂载，/etc 与 /data 不进沙箱
        assert "--ro-bind / /" not in cmdstr
        assert "--ro-bind /etc /etc" not in cmdstr
        assert "--ro-bind /data /data" not in cmdstr
        assert cmd[-1] == "--"


class TestProbeFidelity:
    """W-0048 裁决点②：后端探测命令与真跑同一挂载形态（含 --tmpfs /tmp）。

    T0 实测旧探针无 tmpfs：venv/flows 落在 /tmp 下时探针通过、真跑 execvp
    失败（tmpfs 遮蔽），后端误判 bwrap 可用。探针必须带 tmpfs/proc/unshare-net。
    """

    def test_probe_cmd_mirrors_sandbox_mounts(self, adapter):
        import shutil

        if shutil.which("bwrap") is None:
            pytest.skip("bwrap 不可用（探针命令为空）")
        cmd = adapter.runner.sandbox_probe_cmd()
        cmdstr = " ".join(cmd)
        assert "--tmpfs /tmp" in cmdstr
        assert "--proc /proc" in cmdstr
        assert "--unshare-net" in cmd
        assert cmd[0].endswith("bwrap")


class TestVerifySandboxBackend:
    """fail-closed 新语义（W-0048 T2）：生产只有 bwrap 一条路径。

    显式 rlimit（测试夹具）放行；auto/bwrap 探测不到 bwrap 即 503；旧
    ALLOW_RLIMIT_FALLBACK env 不再放行。无 bwrap 环境用 PATH 遮蔽模拟。
    """

    def test_rlimit_explicit_passes(self, adapter, settings, tmp_path, monkeypatch):
        """显式 rlimit（仅测试）：即使环境无 bwrap 也放行。"""
        monkeypatch.setenv("PATH", str(tmp_path))  # 遮蔽 bwrap
        adapter.reset_probe_cache()
        rs = adapter.with_backend(settings, "rlimit")
        adapter.runner.verify_sandbox_backend(rs)

    def test_auto_with_bwrap_passes(self, adapter, settings):
        if adapter.runner.detect_backend() != "bwrap":
            pytest.skip("bwrap 不可用（auto 落 503 由 PATH 遮蔽用例覆盖）")
        rs = adapter.with_backend(settings, "auto")
        adapter.runner.verify_sandbox_backend(rs)

    def test_auto_without_bwrap_503(self, adapter, settings, tmp_path, monkeypatch):
        monkeypatch.setenv("PATH", str(tmp_path))  # 遮蔽 bwrap
        adapter.reset_probe_cache()
        rs = adapter.with_backend(settings, "auto")
        with pytest.raises(HTTPException) as exc:
            adapter.runner.verify_sandbox_backend(rs)
        assert exc.value.status_code == 503
        assert "SANDBOX_BACKEND" in str(exc.value.detail)

    def test_legacy_env_flag_no_longer_bypasses(
        self, adapter, settings, tmp_path, monkeypatch
    ):
        """生产 env ALLOW_RLIMIT_FALLBACK=1 不再放行 auto 降级（语义已删除）。"""
        monkeypatch.setenv("PATH", str(tmp_path))
        monkeypatch.setenv("ALLOW_RLIMIT_FALLBACK", "1")
        adapter.reset_probe_cache()
        rs = adapter.with_backend(settings, "auto")
        with pytest.raises(HTTPException) as exc:
            adapter.runner.verify_sandbox_backend(rs)
        assert exc.value.status_code == 503
        assert "ALLOW_RLIMIT_FALLBACK" not in str(exc.value.detail)

    def test_bwrap_explicit_unavailable_503(
        self, adapter, settings, tmp_path, monkeypatch
    ):
        """显式 bwrap 但环境无 bwrap → 503（显式选择不静默降级）。"""
        monkeypatch.setenv("PATH", str(tmp_path))
        adapter.reset_probe_cache()
        rs = adapter.with_backend(settings, "bwrap")
        with pytest.raises(HTTPException) as exc:
            adapter.runner.verify_sandbox_backend(rs)
        assert exc.value.status_code == 503
