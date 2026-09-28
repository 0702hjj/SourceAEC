# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""test_sandbox_contract.py — run_script 行为契约（T1 起：双服务配置参数化）。

T0 时同一组用例钉两侧**实现**的公共行为；W-0048 T1 合一后两侧同为
aibim_sandbox 实现，参数化改为「双服务配置」——验证共享 runner 正确兑现
每个服务的配置差异（临时目录前缀/产物名/flows 模块名由适配器声明，
不在此断言一致）。新增 TestSingleImplementation 钉单实现事实。
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import tempfile
import threading
import time
from pathlib import Path

import pytest
from fastapi import HTTPException

import contract_scripts as cs
from adapters import require_bwrap


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


class TestContractGate:
    """静态契约门：PARAMS/build/__main__ 缺失或 PARAMS 非字面量 → 422，不执行。"""

    @pytest.mark.parametrize(
        "script,marker",
        [
            (cs.NO_PARAMS_SCRIPT, "PARAMS"),
            (cs.NO_BUILD_SCRIPT, "build"),
            (cs.NO_MAIN_SCRIPT, "__main__"),
            (cs.NON_LITERAL_PARAMS_SCRIPT, "字面量"),
            (cs.SYNTAX_ERROR_SCRIPT, "语法错误"),
        ],
    )
    def test_contract_violations_422(self, adapter, settings, tmp_path, script, marker):
        out = tmp_path / adapter.product_name
        with pytest.raises(HTTPException) as exc:
            adapter.runner.run_script(settings, script, str(out))
        assert exc.value.status_code == 422
        assert marker in str(exc.value.detail)
        assert not out.exists()

    def test_validate_script_text_returns_error_list(self, adapter, settings):
        assert adapter.runner.validate_script_text(settings, cs.GOOD_SCRIPT) == []
        errors = adapter.runner.validate_script_text(settings, cs.NO_PARAMS_SCRIPT)
        assert any("PARAMS" in e for e in errors)


class TestHappyPath:
    def test_writes_product_atomically(self, adapter, settings, tmp_path):
        out = tmp_path / adapter.product_name
        adapter.runner.run_script(settings, cs.GOOD_SCRIPT, str(out))
        assert out.is_file()
        assert "/* t */" in out.read_text(encoding="utf-8")
        assert not Path(str(out) + ".tmp").exists()

    def test_overwrites_stale_product(self, adapter, settings, tmp_path):
        out = tmp_path / adapter.product_name
        out.write_text("stale", encoding="utf-8")
        adapter.runner.run_script(settings, cs.GOOD_SCRIPT, str(out))
        assert "stale" not in out.read_text(encoding="utf-8")

    def test_temp_dir_cleaned_up(self, adapter, settings, tmp_path):
        adapter.runner.run_script(settings, cs.GOOD_SCRIPT, str(tmp_path / "o"))
        leftovers = list(Path(tempfile.gettempdir()).glob(adapter.temp_prefix + "*"))
        assert leftovers == []

    def test_flows_module_importable_in_sandbox(self, adapter, settings, tmp_path):
        """沙箱子进程 PYTHONPATH/ro-bind 暴露 flows helper 模块。"""
        script = cs.FLOWS_IMPORT_SCRIPT.format(flows_module=adapter.flows_module)
        out = tmp_path / adapter.product_name
        adapter.runner.run_script(settings, script, str(out))
        assert out.read_text(encoding="utf-8") == "FLOWS-OK"


class TestFailureSemantics:
    def test_runtime_error_422_with_stderr_tail(self, adapter, settings, tmp_path):
        out = tmp_path / adapter.product_name
        with pytest.raises(HTTPException) as exc:
            adapter.runner.run_script(settings, cs.RUNTIME_ERROR_SCRIPT, str(out))
        assert exc.value.status_code == 422
        assert "boom-marker" in str(exc.value.detail)
        assert not out.exists()

    def test_stderr_tail_truncated(self, adapter, settings, tmp_path):
        with pytest.raises(HTTPException) as exc:
            adapter.runner.run_script(
                settings, cs.NOISY_FAILURE_SCRIPT, str(tmp_path / "o")
            )
        detail = str(exc.value.detail)
        assert len(detail) < 10000
        assert "E" * 100 in detail  # 尾部保留

    def test_missing_product_422(self, adapter, settings, tmp_path):
        with pytest.raises(HTTPException) as exc:
            adapter.runner.run_script(settings, cs.NO_OUTPUT_SCRIPT, str(tmp_path / "o"))
        assert exc.value.status_code == 422
        assert "产出" in str(exc.value.detail)


class TestTimeout:
    def test_infinite_loop_422_timeout(self, adapter, settings, tmp_path):
        out = tmp_path / adapter.product_name
        with pytest.raises(HTTPException) as exc:
            adapter.runner.run_script(settings, cs.INFINITE_LOOP_SCRIPT, str(out), timeout=2)
        assert exc.value.status_code == 422
        assert "超时" in str(exc.value.detail)
        assert not out.exists()

    def test_timeout_kills_process_group(self, adapter, settings, tmp_path):
        """超时杀整个进程组：fork 出的孙进程不得成孤儿继续跑。"""
        out = tmp_path / adapter.product_name
        with pytest.raises(HTTPException) as exc:
            adapter.runner.run_script(settings, cs.FORK_LOOP_SCRIPT, str(out), timeout=2)
        assert exc.value.status_code == 422
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

    def test_memory_bomb_killed_by_rlimit(self, adapter, settings, tmp_path):
        out = tmp_path / adapter.product_name
        with pytest.raises(HTTPException) as exc:
            adapter.runner.run_script(settings, cs.MEMORY_BOMB_SCRIPT, str(out), timeout=30)
        assert exc.value.status_code == 422
        assert not out.exists()


class TestOutputFlood:
    """stdout/stderr 分块泵：累计超 SCRIPT_MAX_OUTPUT_BYTES 杀进程组 → 422。"""

    def test_stdout_flood_killed(self, adapter, settings, tmp_path, monkeypatch):
        monkeypatch.setenv("SCRIPT_MAX_OUTPUT_BYTES", str(64 << 10))
        out = tmp_path / adapter.product_name
        with pytest.raises(HTTPException) as exc:
            adapter.runner.run_script(settings, cs.STDOUT_FLOOD_SCRIPT, str(out), timeout=30)
        assert exc.value.status_code == 422
        assert "输出" in str(exc.value.detail)
        assert not out.exists()

    def test_moderate_output_still_runs(self, adapter, settings, tmp_path):
        out = tmp_path / adapter.product_name
        adapter.runner.run_script(settings, cs.MODERATE_STDOUT_SCRIPT, str(out))
        assert out.is_file()


class TestProductLimit:
    """产物与 map sidecar 发布前大小校验（SCRIPT_MAX_PRODUCT_BYTES）。"""

    def test_oversize_product_rejected(self, adapter, settings, tmp_path, monkeypatch):
        monkeypatch.setenv("SCRIPT_MAX_PRODUCT_BYTES", "4096")
        out = tmp_path / adapter.product_name
        with pytest.raises(HTTPException) as exc:
            adapter.runner.run_script(settings, cs.BIG_WRITE_SCRIPT, str(out))
        assert exc.value.status_code == 422
        assert "上限" in str(exc.value.detail)
        assert not out.exists()

    def test_oversize_map_rejected_before_publish(
        self, adapter, settings, tmp_path, monkeypatch
    ):
        """map 超限与产物一样在发布前拒绝：产物也不落盘（不留错位产物）。"""
        monkeypatch.setenv("SCRIPT_MAX_PRODUCT_BYTES", "4096")
        out = tmp_path / adapter.product_name
        with pytest.raises(HTTPException) as exc:
            adapter.runner.run_script(settings, cs.BIG_MAP_SCRIPT, str(out))
        assert exc.value.status_code == 422
        assert "map" in str(exc.value.detail)
        assert not out.exists()
        assert not Path(str(out) + ".map.json").exists()


class TestMapEnvelope:
    """ScriptMap sidecar → {"scriptHash", "map"} 信封的原子发布/清理/校验。"""

    def test_envelope_published_with_script_hash(self, adapter, settings, tmp_path):
        out = tmp_path / adapter.product_name
        map_dest = tmp_path / "current.map.json"
        adapter.runner.run_script(settings, cs.MAP_SCRIPT, str(out), map_out=str(map_dest))
        envelope = json.loads(map_dest.read_text(encoding="utf-8"))
        assert envelope["scriptHash"] == adapter.runner.script_hash(cs.MAP_SCRIPT)
        assert envelope["scriptHash"] == hashlib.sha256(
            cs.MAP_SCRIPT.encode("utf-8")
        ).hexdigest()
        assert envelope["map"] == {"0:wall:1": {"line": 7, "origin": "traced"}}

    def test_default_map_dest_next_to_out(self, adapter, settings, tmp_path):
        out = tmp_path / adapter.product_name
        adapter.runner.run_script(settings, cs.MAP_SCRIPT, str(out))
        envelope = json.loads(Path(str(out) + ".map.json").read_text(encoding="utf-8"))
        assert envelope["scriptHash"] == adapter.runner.script_hash(cs.MAP_SCRIPT)

    def test_missing_sidecar_deletes_stale_map(self, adapter, settings, tmp_path):
        out = tmp_path / adapter.product_name
        map_dest = tmp_path / "current.map.json"
        adapter.runner.run_script(settings, cs.MAP_SCRIPT, str(out), map_out=str(map_dest))
        assert map_dest.is_file()
        adapter.runner.run_script(settings, cs.GOOD_SCRIPT, str(out), map_out=str(map_dest))
        assert not map_dest.exists()

    @pytest.mark.parametrize(
        "script", [cs.BAD_JSON_MAP_SCRIPT, cs.NON_DICT_MAP_SCRIPT]
    )
    def test_invalid_sidecar_422(self, adapter, settings, tmp_path, script):
        """非法 map sidecar 发布前 422（W-0048 裁决点①：先校验后发布）。

        T0 实测旧实现产物已落盘后才 422（失败原子性与超限 map 不一致）；
        T1 统一为发布前校验——产物与 map 都不落盘。
        """
        out = tmp_path / adapter.product_name
        map_dest = tmp_path / "current.map.json"
        with pytest.raises(HTTPException) as exc:
            adapter.runner.run_script(settings, script, str(out), map_out=str(map_dest))
        assert exc.value.status_code == 422
        assert "map" in str(exc.value.detail)
        assert not out.exists()
        assert not map_dest.exists()


class TestConcurrencyGate:
    """进程级并发闸（aibim_sandbox.runner._RUN_GATE，大小 SCRIPT_RUN_CONCURRENCY）：

    满即 429，结束释放。T1 起闸住共享 runner 模块，patch 打 adapter.impl。
    """

    def test_gate_full_rejects_429(self, adapter, settings, tmp_path, monkeypatch):
        monkeypatch.setattr(adapter.impl, "_RUN_GATE", threading.Semaphore(0))
        out = tmp_path / adapter.product_name
        with pytest.raises(HTTPException) as exc:
            adapter.runner.run_script(settings, cs.GOOD_SCRIPT, str(out))
        assert exc.value.status_code == 429
        assert not out.exists()

    def test_gate_released_after_run(self, adapter, settings, tmp_path, monkeypatch):
        monkeypatch.setattr(adapter.impl, "_RUN_GATE", threading.Semaphore(1))
        out1, out2 = tmp_path / "a", tmp_path / "b"
        adapter.runner.run_script(settings, cs.GOOD_SCRIPT, str(out1))
        adapter.runner.run_script(settings, cs.GOOD_SCRIPT, str(out2))
        assert out1.is_file() and out2.is_file()

    def test_gate_released_after_failure(self, adapter, settings, tmp_path, monkeypatch):
        monkeypatch.setattr(adapter.impl, "_RUN_GATE", threading.Semaphore(1))
        with pytest.raises(HTTPException):
            adapter.runner.run_script(settings, cs.RUNTIME_ERROR_SCRIPT, str(tmp_path / "x"))
        out = tmp_path / "ok"
        adapter.runner.run_script(settings, cs.GOOD_SCRIPT, str(out))
        assert out.is_file()


class TestSingleImplementation:
    """W-0048 T1：双服务配置跑的是同一共享实现（aibim_sandbox.runner）。"""

    def test_both_adapters_share_one_impl(self, adapter):
        from adapters import get_adapter

        assert adapter.impl is get_adapter("ifc").impl is get_adapter("cad").impl
        assert adapter.impl.__name__ == "aibim_sandbox.runner"


class TestFailClosed:
    """T2 fail-closed 新语义：auto 无 bwrap 一律 503（ALLOW_RLIMIT_FALLBACK
    env 不再放行）；rlimit 退役为测试夹具，仅 settings 级显式选择可执行。"""

    def test_auto_without_bwrap_503(self, adapter, settings, tmp_path, monkeypatch):
        monkeypatch.setenv("PATH", str(tmp_path))  # 遮蔽 bwrap，模拟无 bwrap 环境
        adapter.reset_probe_cache()
        rs = adapter.with_backend(settings, "auto")
        out = tmp_path / adapter.product_name
        with pytest.raises(HTTPException) as exc:
            adapter.runner.run_script(rs, cs.GOOD_SCRIPT, str(out))
        assert exc.value.status_code == 503
        assert not out.exists()

    def test_legacy_env_flag_no_longer_bypasses(
        self, adapter, settings, tmp_path, monkeypatch
    ):
        """ALLOW_RLIMIT_FALLBACK=1（旧放行 env）不再影响 fail-closed 判定。"""
        monkeypatch.setenv("PATH", str(tmp_path))
        monkeypatch.setenv("ALLOW_RLIMIT_FALLBACK", "1")
        adapter.reset_probe_cache()
        rs = adapter.with_backend(settings, "auto")
        out = tmp_path / adapter.product_name
        with pytest.raises(HTTPException) as exc:
            adapter.runner.run_script(rs, cs.GOOD_SCRIPT, str(out))
        assert exc.value.status_code == 503
        assert "ALLOW_RLIMIT_FALLBACK" not in str(exc.value.detail)
        assert not out.exists()

    def test_explicit_rlimit_backend_runs(self, adapter, settings, tmp_path):
        """rlimit 仅测试可显式选：settings 级 rlimit 直接执行（无任何 env 放行）。"""
        rs = adapter.with_backend(settings, "rlimit")
        out = tmp_path / adapter.product_name
        adapter.runner.run_script(rs, cs.GOOD_SCRIPT, str(out))
        assert out.is_file()


class TestBackendProbe:
    """环境探测（detect_backend）：返回值域 {bwrap, rlimit}；探针结果进程内缓存。

    T2 起探测只报环境事实，不做后端选择（选择走 Settings.sandbox_backend）。
    """

    def test_detect_returns_known_backend(self, adapter):
        adapter.reset_probe_cache()
        assert adapter.runner.detect_backend() in ("bwrap", "rlimit")

    def test_probe_result_cached(self, adapter):
        adapter.reset_probe_cache()
        first = adapter.runner.detect_backend()
        assert adapter.runner.detect_backend() == first
        assert adapter.backend_module._PROBE_OK is not None

    def test_detect_bwrap_when_available(self, adapter):
        """断言前提是「bwrap 真可用」（探针过），不是「二进制存在」——GH
        Actions 默认 AppArmor 拦非特权 userns，二进制在但探针挂（PR #82
        CI 红）；探针失败的环境 skip 而非红。"""
        adapter.reset_probe_cache()
        if adapter.runner.detect_backend() != "bwrap":
            pytest.skip("bwrap 探针失败（无二进制或 userns 被 AppArmor 拦）")
        assert adapter.runner.detect_backend() == "bwrap"


class TestBwrapIsolation:
    """仅 bwrap 生效的硬隔离契约（CI 无 bwrap 时 skip）：越界写/断网/挂载收窄。"""

    def test_write_outside_sandbox_blocked(self, adapter, settings, tmp_path):
        require_bwrap(adapter)
        target = tmp_path / "evil.txt"
        script = cs.ESCAPE_WRITE_SCRIPT.format(target=str(target))
        out = tmp_path / adapter.product_name
        with pytest.raises(HTTPException) as exc:
            adapter.runner.run_script(settings, script, str(out))
        assert exc.value.status_code == 422
        assert not target.exists()
        assert not out.exists()

    def test_no_network_in_sandbox(self, adapter, settings, tmp_path):
        require_bwrap(adapter)
        out = tmp_path / adapter.product_name
        adapter.runner.run_script(settings, cs.NET_PROBE_SCRIPT, str(out))
        assert out.read_text(encoding="utf-8") == "NET-BLOCKED"

    def test_data_and_etc_not_readable(self, adapter, settings, tmp_path):
        """W-0047 挂载收窄：/data 与 /etc 不进沙箱；--tmpfs /tmp 仍可用。"""
        require_bwrap(adapter)
        out = tmp_path / adapter.product_name
        adapter.runner.run_script(settings, cs.LEAK_PROBE_SCRIPT, str(out))
        content = out.read_text(encoding="utf-8")
        assert content.startswith("BLOCKED"), f"沙箱内读到了宿主路径: {content}"
        assert "TMP:ok" in content


class TestNprocBudget:
    """RLIMIT_NPROC 预算：现有 task 数 + MAX_PROCS 余量，超出余量的 fork 被拦。"""

    @pytest.mark.skipif(os.geteuid() == 0, reason="root 可能绕过 RLIMIT_NPROC")
    def test_fork_bomb_blocked(self, adapter, settings, tmp_path):
        script = cs.FORK_BOMB_SCRIPT.format(forks=adapter.runner.MAX_PROCS + 50)
        out = tmp_path / adapter.product_name
        with pytest.raises(HTTPException) as exc:
            adapter.runner.run_script(settings, script, str(out), timeout=30)
        assert exc.value.status_code == 422
        assert not out.exists()
