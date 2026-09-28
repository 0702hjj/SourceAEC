# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""契约：diff 三件套——script/diff（大版本文本 diff）、script/staging/diff
（小版本步间 diff）、POST /diff（语义 diff）。

语义 diff 的版本快照由 profile 手写（ifc：墙改名；cad：同 key LINE 终点变
化），断言只到「added/removed/changed 是列表、changed 非空且为对象」的形状
级；字段名（guid/key）与计数细节留在两侧各自套件。
"""

from __future__ import annotations

import json
from pathlib import Path

from fastapi.testclient import TestClient

from conftest import (
    MODEL_ID,
    UNKNOWN_MODEL_ID,
    TargetProfile,
    staging_script,
)


def _versions_dir(data_dir: Path) -> Path:
    return data_dir / "models" / MODEL_ID / "versions"


def _put_and_save(client: TestClient, marker: str) -> str:
    resp = client.put(f"/models/{MODEL_ID}/script", json={"script": staging_script(marker)})
    assert resp.status_code == 200, resp.text
    resp = client.post(f"/models/{MODEL_ID}/script/save")
    assert resp.status_code == 200, resp.text
    return resp.json()["version"]


class TestScriptDiff:
    def test_diff_between_big_versions(self, client: TestClient):
        assert _put_and_save(client, "a") == "v1"
        assert _put_and_save(client, "b") == "v2"
        resp = client.post(
            f"/models/{MODEL_ID}/script/diff", json={"base": "v1", "target": "v2"}
        )
        assert resp.status_code == 200, resp.text
        body = resp.json()
        assert body["base"] == "v1" and body["target"] == "v2"
        assert body["engine"] == "script"
        assert isinstance(body["text_diff"], str)
        assert isinstance(body["params_changes"], list)
        actions = {c["key"]: c["action"] for c in body["params_changes"]}
        assert actions == {"marker": "modified"}
        stats = body["stats"]
        assert set(stats) == {"added", "removed"}
        assert all(isinstance(v, int) for v in stats.values())

    def test_diff_unknown_version_404(self, client: TestClient):
        _put_and_save(client, "a")
        resp = client.post(
            f"/models/{MODEL_ID}/script/diff", json={"base": "v1", "target": "v9"}
        )
        assert resp.status_code == 404


class TestStagingDiff:
    def test_last_two_steps(self, client: TestClient):
        client.put(f"/models/{MODEL_ID}/script", json={"script": staging_script("a")})
        client.put(f"/models/{MODEL_ID}/script", json={"script": staging_script("b")})
        resp = client.get(f"/models/{MODEL_ID}/script/staging/diff")
        assert resp.status_code == 200, resp.text
        body = resp.json()
        assert body["from"] == 0 and body["to"] == 1
        assert isinstance(body["text_diff"], str)
        actions = {c["key"]: c["action"] for c in body["params_changes"]}
        assert actions == {"marker": "modified"}

    def test_explicit_range(self, client: TestClient):
        client.put(f"/models/{MODEL_ID}/script", json={"script": staging_script("a")})
        client.put(f"/models/{MODEL_ID}/script", json={"script": staging_script("b")})
        resp = client.get(
            f"/models/{MODEL_ID}/script/staging/diff", params={"from": 0, "to": 1}
        )
        assert resp.status_code == 200
        assert resp.json()["from"] == 0 and resp.json()["to"] == 1

    def test_out_of_range_422(self, client: TestClient):
        client.put(f"/models/{MODEL_ID}/script", json={"script": staging_script("a")})
        client.put(f"/models/{MODEL_ID}/script", json={"script": staging_script("b")})
        beyond = client.get(
            f"/models/{MODEL_ID}/script/staging/diff", params={"from": 1, "to": 5}
        )
        empty = client.get(
            f"/models/{MODEL_ID}/script/staging/diff", params={"from": 1, "to": 1}
        )
        assert beyond.status_code == 422
        assert empty.status_code == 422

    def test_fewer_than_two_steps_409(self, client: TestClient):
        client.put(f"/models/{MODEL_ID}/script", json={"script": staging_script("a")})
        resp = client.get(f"/models/{MODEL_ID}/script/staging/diff")
        assert resp.status_code == 409


class TestSemanticDiff:
    def test_diff_two_versions(self, client: TestClient, data_dir: Path, profile):
        profile.write_diff_versions(data_dir)
        resp = client.post(
            f"/models/{MODEL_ID}/diff", json={"base": "v1", "target": "v2"}
        )
        assert resp.status_code == 200, resp.text
        payload = resp.json()
        assert payload["base"] == "v1" and payload["target"] == "v2"
        assert isinstance(payload["added"], list)
        assert isinstance(payload["removed"], list)
        assert isinstance(payload["changed"], list)
        assert payload["changed"], "构造的版本对必须产生 changed 条目"
        assert all(isinstance(c, dict) for c in payload["changed"])

    def test_diff_target_current_no_cache(
        self, client: TestClient, data_dir: Path, profile: TargetProfile
    ):
        profile.write_diff_versions(data_dir)
        resp = client.post(
            f"/models/{MODEL_ID}/diff", json={"base": "v1", "target": "current"}
        )
        assert resp.status_code == 200, resp.text
        payload = resp.json()
        assert payload["target"] == "current"
        assert isinstance(payload["changed"], list)
        # target=current 永不缓存（uploads 可变，无稳定缓存键）
        assert not (_versions_dir(data_dir) / "diff-v1-current.json").exists()

    def test_diff_result_cached(self, client: TestClient, data_dir: Path, profile):
        profile.write_diff_versions(data_dir)
        first = client.post(
            f"/models/{MODEL_ID}/diff", json={"base": "v1", "target": "v2"}
        ).json()
        cache_file = _versions_dir(data_dir) / "diff-v1-v2.json"
        assert cache_file.is_file()
        assert json.loads(cache_file.read_text(encoding="utf-8")) == first
        second = client.post(
            f"/models/{MODEL_ID}/diff", json={"base": "v1", "target": "v2"}
        ).json()
        assert second == first

    def test_diff_unknown_version_404(self, client: TestClient, data_dir, profile):
        profile.write_diff_versions(data_dir)
        for body in (
            {"base": "v9", "target": "v2"},
            {"base": "v1", "target": "v9"},
        ):
            resp = client.post(f"/models/{MODEL_ID}/diff", json=body)
            assert resp.status_code == 404, body

    def test_diff_without_any_version_404(self, client: TestClient):
        resp = client.post(
            f"/models/{MODEL_ID}/diff", json={"base": "v1", "target": "current"}
        )
        assert resp.status_code == 404

    def test_diff_missing_params_422(self, client: TestClient):
        assert client.post(f"/models/{MODEL_ID}/diff", json={}).status_code == 422
        assert (
            client.post(f"/models/{MODEL_ID}/diff", json={"base": "v1"}).status_code
            == 422
        )

    def test_diff_bad_model_id_422(self, client: TestClient):
        resp = client.post(
            "/models/not_a_valid_id/diff", json={"base": "v1", "target": "v2"}
        )
        assert resp.status_code == 422

    def test_diff_missing_model_404(self, client: TestClient):
        resp = client.post(
            f"/models/{UNKNOWN_MODEL_ID}/diff", json={"base": "v1", "target": "v2"}
        )
        assert resp.status_code == 404


class TestDiffBodyShape:
    """W-0038：DiffBody 声明层形状校验（pydantic Field pattern）。

    base 只接受版本名（``v\\d+``）；target 额外放行字面量 ``current``。非法形状
    （路径分隔符、空串、非版本名）在 handler 前被 422 拦截——缓存路径
    ``versions/diff-{base}-{target}.json`` 不在未校验输入上构造，versions/
    下不得出现以非法输入命名的残留文件。
    """

    def test_illegal_base_or_target_422(self, client: TestClient):
        for body in (
            # 路径分隔符：注入可越出预期缓存目录
            {"base": "../evil", "target": "v2"},
            {"base": "v1/../../etc", "target": "v2"},
            {"base": "v1", "target": "current/../evil"},
            # 空串
            {"base": "", "target": "v2"},
            {"base": "v1", "target": ""},
            # 非版本名（含大小写、数字、杂缀）
            {"base": "version", "target": "v2"},
            {"base": "V1", "target": "v2"},
            {"base": "v1x", "target": "v2"},
            {"base": "1", "target": "v2"},
            {"base": "v-1", "target": "v2"},
            {"base": "v1", "target": "V2"},
            {"base": "v1", "target": "now"},
        ):
            resp = client.post(f"/models/{MODEL_ID}/diff", json=body)
            assert resp.status_code == 422, body

    def test_illegal_shape_no_cache_path_residue(
        self, client: TestClient, data_dir: Path
    ):
        for body in (
            {"base": "../evil", "target": "v2"},
            {"base": "v1", "target": "current/../evil"},
        ):
            assert client.post(f"/models/{MODEL_ID}/diff", json=body).status_code == 422
        # 422 在缓存路径构造前拦截：versions/ 无任何以非法输入构造的 diff-*.json
        versions = _versions_dir(data_dir)
        assert not versions.exists() or not any(
            "evil" in p.name for p in versions.iterdir()
        )

    def test_legal_shapes_behavior_unchanged(
        self, client: TestClient, data_dir: Path, profile: TargetProfile
    ):
        """合法值域（版本名 / target=current）不受形状门影响。"""
        profile.write_diff_versions(data_dir)
        assert (
            client.post(
                f"/models/{MODEL_ID}/diff", json={"base": "v1", "target": "v2"}
            ).status_code
            == 200
        )
        resp = client.post(
            f"/models/{MODEL_ID}/diff", json={"base": "v2", "target": "current"}
        )
        assert resp.status_code == 200
        assert resp.json()["target"] == "current"
