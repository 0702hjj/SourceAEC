# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""script_runner happy path：契约脚本写产物到 argv[1]，原子发布、沙箱临时
目录清理、双后端显式路径，以及 ScriptMap 信封（{"scriptHash", "map"}）的
原子发布/清理。

W-0057 T3 收编自两侧 test_script_runner_happy.py：共同用例直接共享；
REAL 构建（ifcopenshell 骨架 vs ezdxf+XDATA）与产物校验走 profile 工厂
（real_script / verify_real_product / real_map_key）；cad 特有的双后端
显式与 MapEnvelope 用例两侧同构共享（信封结构由共享 runner 发布）。
"""

from __future__ import annotations

import dataclasses
import json
import tempfile
from pathlib import Path

import pytest

from aibim_sandbox import backend as sandbox_backend

from app import script_runner

from conftest import GOOD_SCRIPT, TargetProfile


class TestHappyPath:
    def test_run_writes_out_file_atomically(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        out = tmp_path / f"model.{profile.ext}"
        script_runner.run_script(settings, GOOD_SCRIPT, str(out))
        assert out.is_file()
        assert "/* t */" in out.read_text(encoding="utf-8")
        assert not Path(str(out) + ".tmp").exists()

    def test_run_overwrites_existing_out(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        out = tmp_path / f"model.{profile.ext}"
        out.write_text("stale", encoding="utf-8")
        script_runner.run_script(settings, GOOD_SCRIPT, str(out))
        assert "stale" not in out.read_text(encoding="utf-8")

    def test_run_real_build_with_flows_lib(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        """契约脚本经 PYTHONPATH import flows 库构建出真实产物（两侧各自校验）。"""
        out = tmp_path / f"real.{profile.ext}"
        script_runner.run_script(settings, profile.real_script, str(out))
        assert out.stat().st_size > 0
        profile.verify_real_product(out)

    def test_sandbox_temp_dir_cleaned_up(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        out = tmp_path / f"model.{profile.ext}"
        script_runner.run_script(settings, GOOD_SCRIPT, str(out))
        leftovers = [
            p
            for p in Path(tempfile.gettempdir()).glob(
                script_runner.CONFIG.temp_prefix + "*"
            )
        ]
        assert leftovers == []

    def test_rlimit_backend_explicit(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        """rlimit 后端的显式路径（settings 级选择，不再 monkeypatch detect_backend）。"""
        rs = dataclasses.replace(settings, sandbox_backend="rlimit")
        out = tmp_path / f"model.{profile.ext}"
        script_runner.run_script(rs, profile.real_script, str(out))
        assert out.stat().st_size > 0

    def test_bwrap_backend_explicit(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        """bwrap 后端的显式路径（settings 级选择，与自动探测解耦）。"""
        if sandbox_backend.detect_backend() != "bwrap":
            pytest.skip("bwrap 不可用（probe 失败）")
        rs = dataclasses.replace(settings, sandbox_backend="bwrap")
        out = tmp_path / f"model.{profile.ext}"
        script_runner.run_script(rs, GOOD_SCRIPT, str(out))
        assert out.is_file()


class TestMapEnvelope:
    """ScriptMap sidecar → {"scriptHash", "map"} 信封的原子发布/清理。"""

    def test_map_envelope_published_with_script_hash(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        out = tmp_path / f"model.{profile.ext}"
        map_dest = tmp_path / "current.map.json"
        script_runner.run_script(
            settings, profile.real_script, str(out), map_out=str(map_dest)
        )
        envelope = json.loads(map_dest.read_text(encoding="utf-8"))
        assert envelope["scriptHash"] == script_runner.script_hash(profile.real_script)
        entries = envelope["map"]
        # REAL 构建的确定性 key 两侧各自进 profile；调用点指向用户脚本行
        assert profile.real_map_key in entries
        assert entries[profile.real_map_key]["origin"] == "traced"
        assert entries[profile.real_map_key]["line"] > 0

    def test_missing_sidecar_deletes_stale_map(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        out = tmp_path / f"model.{profile.ext}"
        map_dest = tmp_path / "current.map.json"
        script_runner.run_script(
            settings, profile.real_script, str(out), map_out=str(map_dest)
        )
        assert map_dest.is_file()
        script_runner.run_script(
            settings, GOOD_SCRIPT, str(out), map_out=str(map_dest)
        )
        assert not map_dest.exists()

    def test_default_map_dest_next_to_out(
        self, settings, tmp_path: Path, profile: TargetProfile
    ):
        out = tmp_path / f"model.{profile.ext}"
        script_runner.run_script(settings, profile.real_script, str(out))
        envelope = json.loads(
            Path(str(out) + ".map.json").read_text(encoding="utf-8")
        )
        assert envelope["scriptHash"] == script_runner.script_hash(profile.real_script)
