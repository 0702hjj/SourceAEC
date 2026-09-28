# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""script_versions 领域套件（W-0060 T1 收编两侧镜像 test_script_versions.py）。

模块级函数直测（save/list/load/script_path），经 ``from app import
script_versions`` 走目标服务的 ext 绑定 shim（连带钉住绑定本身）。差异面
（产物种子/扩展名）经 profile（seed_product/ext），断言两侧同构。

用例并集 6 例 = cad 侧 6（其中 5 例与 ifc 侧同构，prune 为 cad 独有——
ifc 侧等价覆盖原在 test_ifc_lazy_materialize 套件，保留不动）；共享后
prune 在 ifc 环境首次获得该层覆盖（全链路执行数 +1）。
"""

from __future__ import annotations

import json
from pathlib import Path

import pytest

from app import script_versions, versions

SCRIPT_A = 'PARAMS = {"a": 1}\n'


class TestScriptVersions:
    def test_save_writes_script_and_meta(
        self, tmp_path: Path, product_path: Path, model_id: str, profile
    ):
        data_dir = str(tmp_path)
        version = script_versions.save(
            data_dir, model_id, SCRIPT_A, str(product_path), note="n1"
        )
        assert version == "v1"
        base = tmp_path / "models" / model_id
        assert (base / "scripts" / "v1.py").read_text() == SCRIPT_A
        assert (
            base / "versions" / f"v1.{profile.ext}"
        ).read_bytes() == product_path.read_bytes()
        meta = json.loads((base / "scripts" / "v1.meta.json").read_text())
        assert meta["note"] == "n1" and meta["version"] == "v1"

    def test_save_increments_and_lists_oldest_first(
        self, tmp_path: Path, product_path: Path, model_id: str
    ):
        data_dir = str(tmp_path)
        assert script_versions.save(
            data_dir, model_id, SCRIPT_A, str(product_path)
        ) == "v1"
        assert script_versions.save(
            data_dir, model_id, SCRIPT_A, str(product_path), note="x"
        ) == "v2"
        listed = script_versions.list_scripts(data_dir, model_id)
        assert [s["version"] for s in listed] == ["v1", "v2"]
        assert listed[0]["note"] == "" and listed[1]["note"] == "x"
        assert all("createdAt" in s for s in listed)

    def test_lockstep_with_existing_snapshots(
        self, tmp_path: Path, product_path: Path, model_id: str, profile
    ):
        """versions/ 已占 v1/v2 时，脚本大版本取 max(两侧 next) 保证成对不冲突。"""
        data_dir = str(tmp_path)
        versions.snapshot(data_dir, model_id, str(product_path))
        versions.snapshot(data_dir, model_id, str(product_path))
        version = script_versions.save(
            data_dir, model_id, SCRIPT_A, str(product_path)
        )
        assert version == "v3"
        base = tmp_path / "models" / model_id
        assert (base / "scripts" / "v3.py").is_file()
        assert (base / "versions" / f"v3.{profile.ext}").is_file()
        assert (base / "versions" / f"v1.{profile.ext}").is_file()

    def test_prune_rebuildable_snapshots(
        self, tmp_path: Path, product_path: Path, model_id: str, profile
    ):
        """只留最新物化：有脚本的旧 versions/v{m} 被裁剪；无脚本的快照保留。"""
        data_dir = str(tmp_path)
        script_versions.save(data_dir, model_id, SCRIPT_A, str(product_path))
        versions.snapshot(data_dir, model_id, str(product_path))  # v2, no script
        script_versions.save(data_dir, model_id, SCRIPT_A, str(product_path))
        base = tmp_path / "models" / model_id / "versions"
        assert not (base / f"v1.{profile.ext}").exists()  # rebuildable -> pruned
        assert (base / f"v2.{profile.ext}").is_file()  # no script -> preserved
        assert (base / f"v3.{profile.ext}").is_file()  # latest stays materialized

    def test_load_script(self, tmp_path: Path, product_path: Path, model_id: str):
        data_dir = str(tmp_path)
        script_versions.save(data_dir, model_id, SCRIPT_A, str(product_path))
        assert script_versions.load_script(data_dir, model_id, "v1") == SCRIPT_A
        with pytest.raises(KeyError):
            script_versions.load_script(data_dir, model_id, "v9")
        with pytest.raises(KeyError):
            script_versions.load_script(data_dir, model_id, "bogus")

    def test_script_path_validation(self, tmp_path: Path, model_id: str):
        assert script_versions.script_path(str(tmp_path), model_id, "../etc") is None
        assert script_versions.script_path(str(tmp_path), model_id, "v1") is None
