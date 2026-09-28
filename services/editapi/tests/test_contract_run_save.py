# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""契约：run/save/rollback + 大版本落盘布局 + versions 列表。

版本目录布局断言（两侧同构）：``models/{id}/scripts/v{n}.py``（全留）+
``v{n}.meta.json`` sidecar + ``models/{id}/versions/v{n}.{ifc|dxf}``（旧产物
裁剪、只留最新）。marker 脚本不调 flows → 无 map sidecar；flows 脚本 →
``scripts/v{n}.map.json`` 随版本 lockstep。
"""

from __future__ import annotations

import json
from pathlib import Path

from fastapi.testclient import TestClient

from conftest import (
    MODEL_ID,
    TargetProfile,
    FAILING_SCRIPT,
    staging_script,
)


def _scripts_dir(data_dir: Path) -> Path:
    return data_dir / "models" / MODEL_ID / "scripts"


def _versions_dir(data_dir: Path) -> Path:
    return data_dir / "models" / MODEL_ID / "versions"


def _upload(data_dir: Path, profile: TargetProfile) -> Path:
    return data_dir / "uploads" / f"{MODEL_ID}.{profile.ext}"


def _put(client: TestClient, script: str) -> None:
    resp = client.put(f"/models/{MODEL_ID}/script", json={"script": script})
    assert resp.status_code == 200, resp.text


def _save(client: TestClient, note: str = "") -> str:
    resp = client.post(f"/models/{MODEL_ID}/script/save", json={"note": note})
    assert resp.status_code == 200, resp.text
    return resp.json()["version"]


class TestRun:
    def test_run_without_script_409(self, client: TestClient):
        assert client.post(f"/models/{MODEL_ID}/script/run").status_code == 409

    def test_run_replaces_upload_and_returns_shape(
        self, client: TestClient, data_dir: Path, profile: TargetProfile
    ):
        _put(client, staging_script("run1"))
        resp = client.post(f"/models/{MODEL_ID}/script/run")
        assert resp.status_code == 200, resp.text
        body = resp.json()
        assert body["modelId"] == MODEL_ID
        assert body["ok"] is True
        # semanticDiff：dict(三计数) 或 None（diff 不可用时降级），形状二选一
        diff = body["semanticDiff"]
        assert diff is None or (
            set(diff) == {"added", "removed", "changed"}
            and all(isinstance(v, int) for v in diff.values())
        )
        # run 用同一份 marker 脚本：uploads 产物内容两侧一致可断
        assert _upload(data_dir, profile).read_text() == "EDITAPI:run1"

    def test_run_failing_script_422_upload_untouched(
        self, client: TestClient, data_dir: Path, profile: TargetProfile
    ):
        before = _upload(data_dir, profile).read_bytes()
        _put(client, FAILING_SCRIPT)
        resp = client.post(f"/models/{MODEL_ID}/script/run")
        assert resp.status_code == 422
        assert "editapi-contract-fail-marker" in resp.json()["detail"]
        assert _upload(data_dir, profile).read_bytes() == before


class TestSave:
    def test_save_creates_lockstep_layout(
        self, client: TestClient, data_dir: Path, profile: TargetProfile
    ):
        script = staging_script("s1")
        _put(client, script)
        resp = client.post(f"/models/{MODEL_ID}/script/save", json={"note": "first"})
        assert resp.status_code == 200, resp.text
        body = resp.json()
        assert body["modelId"] == MODEL_ID
        assert body["version"] == "v1"
        assert body["staged"] == 0

        assert (_scripts_dir(data_dir) / "v1.py").read_text(encoding="utf-8") == script
        assert (_scripts_dir(data_dir) / "v1.meta.json").is_file()
        assert (_versions_dir(data_dir) / f"v1.{profile.ext}").is_file()
        # uploads 同步为重跑产物；marker 脚本不产 map sidecar
        assert _upload(data_dir, profile).read_text() == "EDITAPI:s1"
        assert not (data_dir / "models" / MODEL_ID / "current.map.json").exists()

        # staging 清空但基线仍可读
        got = client.get(f"/models/{MODEL_ID}/script").json()
        assert got["staged"] == 0
        assert got["script"] == script

    def test_save_failure_422_no_version(
        self, client: TestClient, data_dir: Path
    ):
        _put(client, FAILING_SCRIPT)
        resp = client.post(f"/models/{MODEL_ID}/script/save")
        assert resp.status_code == 422
        assert not list(_scripts_dir(data_dir).glob("v*.py"))
        assert not _versions_dir(data_dir).exists()
        # staging 保留，可修好再 save
        assert client.get(f"/models/{MODEL_ID}/script").json()["staged"] == 1

    def test_save_without_script_409(self, client: TestClient):
        assert client.post(f"/models/{MODEL_ID}/script/save").status_code == 409

    def test_second_save_prunes_older_product_keeps_scripts(
        self, client: TestClient, data_dir: Path, profile: TargetProfile
    ):
        _put(client, staging_script("one"))
        assert _save(client) == "v1"
        _put(client, staging_script("two"))
        assert _save(client) == "v2"

        scripts = sorted(p.name for p in _scripts_dir(data_dir).glob("v*.py"))
        assert scripts == ["v1.py", "v2.py"]
        products = sorted(
            p.name for p in _versions_dir(data_dir).glob(f"v*.{profile.ext}")
        )
        assert products == [f"v2.{profile.ext}"]

    def test_save_publishes_map_sidecar_for_flows_script(
        self, client: TestClient, data_dir: Path, profile: TargetProfile
    ):
        """调 flows（script_lib/cad_script_lib）的脚本 → map sidecar 随版本落盘。"""
        _put(client, profile.key_script(profile.default_key))
        assert _save(client) == "v1"
        map_sidecar = _scripts_dir(data_dir) / "v1.map.json"
        assert map_sidecar.is_file()
        envelope = json.loads(map_sidecar.read_text(encoding="utf-8"))
        assert isinstance(envelope["scriptHash"], str)
        assert isinstance(envelope["map"], dict)
        assert profile.default_key in envelope["map"]

    def test_list_scripts_after_save(self, client: TestClient):
        _put(client, staging_script("n1"))
        _save(client, note="note-1")
        resp = client.get(f"/models/{MODEL_ID}/scripts")
        assert resp.status_code == 200
        body = resp.json()
        assert [s["version"] for s in body["scripts"]] == ["v1"]
        assert body["scripts"][0]["note"] == "note-1"
        assert all(s["createdAt"] for s in body["scripts"])
        assert [v["version"] for v in body["versions"]] == ["v1"]

    def test_get_versions_endpoint(self, client: TestClient):
        assert client.get(f"/models/{MODEL_ID}/versions").json() == {
            "versions": [],
            "current": None,
        }
        _put(client, staging_script("v"))
        _save(client)
        body = client.get(f"/models/{MODEL_ID}/versions").json()
        assert [v["version"] for v in body["versions"]] == ["v1"]
        assert body["current"] == "v1"


class TestRollback:
    def test_rollback_restores_script_and_reruns(
        self, client: TestClient, data_dir: Path, profile: TargetProfile
    ):
        _put(client, staging_script("one"))
        _save(client)
        _put(client, staging_script("two"))
        _save(client)

        resp = client.post(f"/models/{MODEL_ID}/script/rollback", json={"version": "v1"})
        assert resp.status_code == 200, resp.text
        body = resp.json()
        assert body["modelId"] == MODEL_ID
        assert body["version"] == "v1"
        assert body["script"] == staging_script("one")
        # staging 重置到 v1 基线，uploads 重跑为 v1 产物
        got = client.get(f"/models/{MODEL_ID}/script").json()
        assert got["staged"] == 0
        assert got["script"] == staging_script("one")
        assert _upload(data_dir, profile).read_text() == "EDITAPI:one"

    def test_rollback_unknown_version_404(self, client: TestClient):
        resp = client.post(
            f"/models/{MODEL_ID}/script/rollback", json={"version": "v9"}
        )
        assert resp.status_code == 404
