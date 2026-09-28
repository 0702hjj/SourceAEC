# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""契约：script 暂存面（staging CRUD + params 模式 + undo/redo/discard）。

提炼自两侧 test_script_staging.py / test_routes_scripts.py 的共同骨架；
断言形状级：状态码 + 响应字段存在性与类型，不碰服务特有细节。
"""

from __future__ import annotations

from pathlib import Path

from fastapi.testclient import TestClient

from conftest import (
    CONTRACT_VIOLATION_SCRIPT,
    UNKNOWN_MODEL_ID,
    staging_script,
)


class TestStagingBasics:
    def test_get_script_404_without_script(self, client: TestClient, model_id: str):
        assert client.get(f"/models/{model_id}/script").status_code == 404
        assert client.get(f"/models/{model_id}/script/params").status_code == 404

    def test_get_script_404_unknown_model(self, client: TestClient):
        assert client.get(f"/models/{UNKNOWN_MODEL_ID}/script").status_code == 404

    def test_stage_full_script_and_read_back(
        self, client: TestClient, model_id: str
    ):
        resp = client.put(
            f"/models/{model_id}/script", json={"script": staging_script("a")}
        )
        assert resp.status_code == 200
        body = resp.json()
        assert body["modelId"] == model_id
        assert body["staged"] == 1
        assert body["canUndo"] is True and body["canRedo"] is False

        got = client.get(f"/models/{model_id}/script").json()
        assert got["script"] == staging_script("a")
        assert got["staged"] == 1
        assert got["maxSteps"] == 10
        assert got["canUndo"] is True and got["canRedo"] is False

    def test_stage_contract_violation_422_not_staged(
        self, client: TestClient, model_id: str
    ):
        resp = client.put(
            f"/models/{model_id}/script", json={"script": CONTRACT_VIOLATION_SCRIPT}
        )
        assert resp.status_code == 422
        assert isinstance(resp.json()["detail"], str)
        assert client.get(f"/models/{model_id}/script").status_code == 404

    def test_stage_requires_exactly_one_mode(self, client: TestClient, model_id: str):
        neither = client.put(f"/models/{model_id}/script", json={"note": "x"})
        both = client.put(
            f"/models/{model_id}/script",
            json={"script": staging_script("a"), "params": {"marker": "b"}},
        )
        assert neither.status_code == 422
        assert both.status_code == 422

    def test_stage_unknown_model_404(self, client: TestClient):
        resp = client.put(
            f"/models/{UNKNOWN_MODEL_ID}/script", json={"script": staging_script("a")}
        )
        assert resp.status_code == 404

    def test_scripts_list_empty_before_any_save(
        self, client: TestClient, model_id: str
    ):
        resp = client.get(f"/models/{model_id}/scripts")
        assert resp.status_code == 200
        assert resp.json()["scripts"] == []
        assert resp.json()["versions"] == []


class TestParamsMode:
    def test_params_only_rewrites_params_block(
        self, client: TestClient, model_id: str
    ):
        client.put(f"/models/{model_id}/script", json={"script": staging_script("a")})
        resp = client.put(
            f"/models/{model_id}/script", json={"params": {"marker": "patched"}}
        )
        assert resp.status_code == 200
        assert resp.json()["staged"] == 2
        params = client.get(f"/models/{model_id}/script/params").json()["params"]
        assert params == {"marker": "patched"}

    def test_params_without_script_409(self, client: TestClient, model_id: str):
        resp = client.put(
            f"/models/{model_id}/script", json={"params": {"marker": "x"}}
        )
        assert resp.status_code == 409

    def test_get_params_roundtrip(self, client: TestClient, model_id: str):
        client.put(f"/models/{model_id}/script", json={"script": staging_script("a")})
        resp = client.get(f"/models/{model_id}/script/params")
        assert resp.status_code == 200
        assert resp.json() == {"modelId": model_id, "params": {"marker": "a"}}


class TestStagingNavigation:
    def test_undo_redo_navigation(self, client: TestClient, model_id: str):
        client.put(f"/models/{model_id}/script", json={"script": staging_script("a")})
        client.put(f"/models/{model_id}/script", json={"script": staging_script("b")})
        assert client.get(f"/models/{model_id}/script").json()["staged"] == 2

        resp = client.post(f"/models/{model_id}/script/undo")
        assert resp.status_code == 200
        assert resp.json()["script"] == staging_script("a")
        assert resp.json()["canRedo"] is True

        resp = client.post(f"/models/{model_id}/script/redo")
        assert resp.status_code == 200
        assert resp.json()["script"] == staging_script("b")
        assert resp.json()["canUndo"] is True

    def test_undo_nothing_409(self, client: TestClient, model_id: str):
        assert client.post(f"/models/{model_id}/script/undo").status_code == 409

    def test_redo_nothing_409(self, client: TestClient, model_id: str):
        assert client.post(f"/models/{model_id}/script/redo").status_code == 409

    def test_discard_drops_staged_back_to_base(
        self, client: TestClient, model_id: str
    ):
        client.put(f"/models/{model_id}/script", json={"script": staging_script("a")})
        client.put(f"/models/{model_id}/script", json={"script": staging_script("b")})
        resp = client.post(f"/models/{model_id}/script/discard")
        assert resp.status_code == 200
        body = resp.json()
        assert body["discarded"] == 2
        assert body["script"] is None
        # 无保存基线（从未 save）→ 回到无脚本态
        assert client.get(f"/models/{model_id}/script").status_code == 404


class TestSeedFromBigVersion:
    """归档形态（只有 scripts/v{n}.py、无 staging 缓冲）→ GET /script 以最新
    大版本 seed base（两侧共有的 M5/C1 语义）。"""

    def test_get_script_seeds_from_latest_big_version(
        self, client: TestClient, data_dir: Path, model_id: str
    ):
        scripts = data_dir / "models" / model_id / "scripts"
        scripts.mkdir(parents=True)
        (scripts / "v1.py").write_text(staging_script("chat-v1"), encoding="utf-8")
        (scripts / "v2.py").write_text(staging_script("chat-v2"), encoding="utf-8")

        resp = client.get(f"/models/{model_id}/script")
        assert resp.status_code == 200
        body = resp.json()
        assert body["script"] == staging_script("chat-v2")
        assert body["staged"] == 0
        assert body["canUndo"] is False and body["canRedo"] is False
        # 幂等：再 GET 不变
        assert client.get(f"/models/{model_id}/script").json() == body

    def test_get_params_seeded(
        self, client: TestClient, data_dir: Path, model_id: str
    ):
        scripts = data_dir / "models" / model_id / "scripts"
        scripts.mkdir(parents=True)
        (scripts / "v1.py").write_text(staging_script("chat-v1"), encoding="utf-8")
        resp = client.get(f"/models/{model_id}/script/params")
        assert resp.status_code == 200
        assert resp.json()["params"] == {"marker": "chat-v1"}
