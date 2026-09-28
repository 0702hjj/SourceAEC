# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""共享画法层（archdxf + dxfkit）单一事实源约束——防版本漂移。"""

from __future__ import annotations

import dataclasses
import os
import sys
from pathlib import Path

from aibim_sandbox import backend as sandbox_backend

from app import script_runner


class TestDrawlibSingleSource:
    """共享画法层（archdxf + dxfkit）单一事实源——防版本漂移。

    约束（用户硬要求）：依赖的 archdxf 库不能有版本差异——
    skill（editable install）与 services/cad 沙箱（PYTHONPATH + ro-bind）必须引用
    同一份**源**源码（skills/aidxf/scripts/packages/*/src）。dist 是打包产物（gitignored），
    config 缺省从源推导（CI 干净克隆无 dist 也能跑）；test_dist_archdxf_in_sync_with_source
    仍强制 dist 与源同步（防两份漂移）。
    """

    def test_drawlib_dir_defaults_to_source_packages(self, settings):
        """沙箱 drawlib_dir 默认指向**源** skill 的 archdxf + dxfkit src（单一事实源）。

        2026-08-21：缺省从 skills/aidxf/（git 跟踪）推导——skills/dist/ 是打包产物（gitignored，
        CI 干净克隆无 dist）。单一事实源语义不变：skill editable install 与沙箱引用同一份源码。
        """
        parts = [p for p in settings.drawlib_dir.split(":") if p]
        assert len(parts) == 2, f"drawlib 应含 archdxf + dxfkit 两路径，got {parts}"
        for p in parts:
            assert "skills/aidxf/scripts/packages" in p, (
                f"drawlib 路径应指向源 skill 单一事实源: {p}"
            )
            assert os.path.isdir(p), f"drawlib 路径不存在: {p}"
        assert any("archdxf/src" in p for p in parts)
        assert any("dxfkit/src" in p for p in parts)

    def test_sandbox_env_pythonpath_includes_drawlib(self, settings, tmp_path):
        """沙箱 PYTHONPATH = flows_dir + drawlib_dir（同时支持 cad_script_lib + dxfkit.draw）。"""
        env = sandbox_backend.sandbox_env(script_runner._spec(settings), str(tmp_path))
        pp = env["PYTHONPATH"]
        assert settings.flows_dir in pp
        for p in settings.drawlib_dir.split(":"):
            if p:
                assert p in pp, f"PYTHONPATH 缺 drawlib 路径: {p}"

    def test_sandbox_cmd_robinds_drawlib_when_bwrap(self, settings, tmp_path):
        """bwrap 沙箱把 drawlib 各路径只读挂载（与 flows_dir 同构）。"""
        rs = dataclasses.replace(settings, sandbox_backend="bwrap")
        cmd = sandbox_backend.sandbox_cmd(script_runner._spec(rs), str(tmp_path))
        cmdstr = " ".join(cmd)
        for p in settings.drawlib_dir.split(":"):
            if p and os.path.isdir(p):
                assert f"--ro-bind {p} {p}" in cmdstr, f"bwrap 缺 drawlib ro-bind: {p}"

    def test_draw_chain_importable_via_sandbox_pythonpath(self, settings):
        """draw 链（archdxf + dxfkit.draw）经沙箱 PYTHONPATH 可 import，且 archdxf 是源单一事实源。"""
        for p in [settings.flows_dir] + [x for x in settings.drawlib_dir.split(":") if x]:
            if p not in sys.path:
                sys.path.insert(0, p)
        import archdxf
        from dxfkit import draw  # noqa: F401
        import cad_script_lib  # noqa: F401
        assert "skills/aidxf" in archdxf.__file__, (
            f"archdxf 应来自源 skill 单一事实源，got {archdxf.__file__}"
        )

    def test_dist_archdxf_in_sync_with_source(self, settings):
        """dist（正式）与开发源（skills/aidxf）的 archdxf/dxfkit 代码同步——防两份漂移。

        dist 是 gitignored 打包产物：CI 干净克隆没有 dist——显式 skip（W-0053：
        静默通过看不出「检查没跑」与「同步」的区别）；本机重打包后必须真比对。
        venv 侧（skills/.venv editable 钉在 dist 副本）的一致性检查在
        tests/skill/test_drawlib_single_source.py（W-0053 新增）。
        """
        import filecmp

        import pytest
        repo = Path(settings.flows_dir).resolve().parent.parent.parent
        for pkg in ("archdxf", "dxfkit"):
            dist = repo / "skills/dist/aidxf/scripts/packages" / pkg / "src" / pkg
            src = repo / "skills/aidxf/scripts/packages" / pkg / "src" / pkg
            if not (dist.is_dir() and src.is_dir()):
                pytest.skip(
                    f"dist 未构建（gitignored，pkg={pkg}）——同步检查仅在 dist 存在时生效"
                )
            diff = filecmp.dircmp(str(dist), str(src))
            problems = (
                diff.diff_files + diff.left_only + diff.right_only
            )
            # 递归子目录
            def _collect(d):
                out = list(d.diff_files) + list(d.left_only) + list(d.right_only)
                for sub in d.subdirs.values():
                    out += _collect(sub)
                return [x for x in out if x != "__pycache__"]
            problems = _collect(diff)
            assert not problems, (
                f"{pkg} 的 dist 与开发源不同步（版本漂移风险）: {problems}"
            )
