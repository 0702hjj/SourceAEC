# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""契约：locate（命中/未命中/stale 降级）+ edit-call（标量改写/失败零副作用/
stale map 409 fail-closed）。

入参差异进 profile：ifc ``?guid=``（guid → designKey hop），cad ``?key=``
（直查）；响应键字段 designKey / key；edit-call body 字段同名差异。断言做
形状级（found/origin/line/col/snippet/params_keys 的存在性与类型）与经
profile 参数化的语义级（``read_entity_label`` 读产物标签验证改写生效）。

W-0060 T2 收编两侧镜像 test_script_edit.py 的 HTTP 面：params-origin 可改写、
非法参数名/NaN 422、undo 后 409 迁入；原有用例吸收 per-service 更强断言
（origin 预检、产物标签校验）。
"""

from __future__ import annotations

import json
from pathlib import Path

from fastapi.testclient import TestClient

from conftest import (
    MODEL_ID,
    UNKNOWN_MODEL_ID,
    TargetProfile,
    _traced_variant,
    staging_script,
)


def _map_path(data_dir: Path) -> Path:
    return data_dir / "models" / MODEL_ID / "current.map.json"


def _upload(data_dir: Path, profile: TargetProfile) -> Path:
    return data_dir / "uploads" / f"{MODEL_ID}.{profile.ext}"


def _save_key_script(client: TestClient, profile: TargetProfile) -> str:
    script = profile.key_script(profile.default_key)
    resp = client.put(f"/models/{MODEL_ID}/script", json={"script": script})
    assert resp.status_code == 200, resp.text
    resp = client.post(f"/models/{MODEL_ID}/script/save", json={})
    assert resp.status_code == 200, resp.text
    return script


def _locate(client: TestClient, profile: TargetProfile, data_dir: Path):
    query = profile.locate_key(data_dir, profile.default_key)
    return client.get(
        f"/models/{MODEL_ID}/script/locate", params={profile.locate_query: query}
    )


def _locate_unknown_model(client: TestClient, profile: TargetProfile):
    return client.get(
        f"/models/{UNKNOWN_MODEL_ID}/script/locate",
        params={profile.locate_query: profile.default_key},
    )


def _edit_call(
    client: TestClient,
    profile: TargetProfile,
    data_dir: Path,
    *,
    argument: str,
    value: object,
    key: str | None = None,
):
    # body 键值是构件 design key（ifc 的 locate 入参 guid 仅用于 locate 查询）。
    body = {
        profile.edit_body_field: key or profile.default_key,
        "argument": argument,
        "value": value,
    }
    return client.post(f"/models/{MODEL_ID}/script/edit-call", json=body)


class TestLocateHit:
    def test_locate_hit_after_save(self, client, data_dir, profile):
        _save_key_script(client, profile)
        resp = _locate(client, profile, data_dir)
        assert resp.status_code == 200, resp.text
        body = resp.json()
        assert body["found"] is True
        assert body[profile.locate_resp_field] == profile.default_key
        assert body["origin"] == "literal"
        assert isinstance(body["line"], int) and body["line"] > 0
        assert isinstance(body["col"], int) and body["col"] >= 0
        assert isinstance(body["snippet"], str) and body["snippet"]
        assert body["params_keys"] == []

    def test_locate_hit_after_run_without_save(self, client, data_dir, profile):
        script = profile.key_script(profile.default_key)
        resp = client.put(f"/models/{MODEL_ID}/script", json={"script": script})
        assert resp.status_code == 200, resp.text
        resp = client.post(f"/models/{MODEL_ID}/script/run")
        assert resp.status_code == 200, resp.text
        body = _locate(client, profile, data_dir).json()
        assert body["found"] is True


class TestLocateMiss:
    def test_locate_unknown_model_404(self, client, profile):
        assert _locate_unknown_model(client, profile).status_code == 404

    def test_locate_map_missing_found_false_no_stale(self, client, data_dir, profile):
        _save_key_script(client, profile)
        assert _map_path(data_dir).is_file()
        _map_path(data_dir).unlink()
        resp = _locate(client, profile, data_dir)
        assert resp.status_code == 200
        body = resp.json()
        assert body["found"] is False
        assert body[profile.locate_resp_field] == profile.default_key
        assert "stale" not in body
        assert "line" not in body

    def test_locate_key_not_in_map_found_false(self, client, data_dir, profile):
        from app.script_runner import script_hash

        script = _save_key_script(client, profile)
        _map_path(data_dir).write_text(
            json.dumps({"scriptHash": script_hash(script), "map": {}}),
            encoding="utf-8",
        )
        resp = _locate(client, profile, data_dir)
        assert resp.status_code == 200
        body = resp.json()
        assert body == {"found": False, profile.locate_resp_field: profile.default_key}


class TestLocateStale:
    """staging 与 map 分叉 → 200 降级 found=false + stale=true，绝不带旧行号。"""

    def test_stale_after_unrun_staged_edit(self, client, data_dir, profile):
        script = _save_key_script(client, profile)
        resp = client.put(
            f"/models/{MODEL_ID}/script",
            json={"script": "# line-shifting edit\n" + script},
        )
        assert resp.status_code == 200, resp.text
        body = _locate(client, profile, data_dir).json()
        assert body["found"] is False
        assert body["stale"] is True
        assert body[profile.locate_resp_field] == profile.default_key
        assert "line" not in body and "col" not in body and "snippet" not in body

    def test_stale_after_undo(self, client, data_dir, profile):
        script = _save_key_script(client, profile)
        arg = profile.entity_arg
        renamed = script.replace(f'{arg}="W1"', f'{arg}="W2"')
        assert renamed != script
        client.put(f"/models/{MODEL_ID}/script", json={"script": renamed})
        assert client.post(f"/models/{MODEL_ID}/script/run").status_code == 200
        assert client.post(f"/models/{MODEL_ID}/script/undo").status_code == 200
        body = _locate(client, profile, data_dir).json()
        assert body["found"] is False and body["stale"] is True

    def test_rerun_clears_stale(self, client, data_dir, profile):
        script = _save_key_script(client, profile)
        client.put(
            f"/models/{MODEL_ID}/script", json={"script": "# edit\n" + script}
        )
        assert client.post(f"/models/{MODEL_ID}/script/run").status_code == 200
        body = _locate(client, profile, data_dir).json()
        assert body["found"] is True


class TestEditCallSuccess:
    def test_edit_call_rewrites_reruns_and_stages(
        self, client, data_dir, profile
    ):
        _save_key_script(client, profile)
        before = _upload(data_dir, profile).read_bytes()
        assert profile.read_entity_label(data_dir, MODEL_ID) == "W1"

        arg = profile.entity_arg
        resp = _edit_call(client, profile, data_dir, argument=arg, value="W2")
        assert resp.status_code == 200, resp.text
        body = resp.json()
        assert body["modelId"] == MODEL_ID
        assert body["staged"] == 1
        assert f'{arg}="W2"' in body["script"]
        assert f'{arg}="W1"' not in body["script"]

        # 沙箱重跑已生效（uploads 变化 + 语义级标签）且进入暂存
        assert _upload(data_dir, profile).read_bytes() != before
        assert profile.read_entity_label(data_dir, MODEL_ID) == "W2"
        got = client.get(f"/models/{MODEL_ID}/script").json()
        assert got["staged"] == 1
        assert got["canUndo"] is True

    def test_params_origin_editable_for_other_argument(
        self, client, data_dir, profile
    ):
        """origin=params 的调用点可改写：key 走 params 引用，但其他实参仍是字面量。"""
        script = profile.params_key_script(profile.default_key)
        resp = client.put(f"/models/{MODEL_ID}/script", json={"script": script})
        assert resp.status_code == 200, resp.text
        assert client.post(f"/models/{MODEL_ID}/script/save", json={}).status_code == 200
        entry = json.loads(_map_path(data_dir).read_text(encoding="utf-8"))["map"][
            profile.default_key
        ]
        assert entry["origin"] == "params"

        resp = _edit_call(
            client, profile, data_dir, argument=profile.entity_arg, value="WP"
        )
        assert resp.status_code == 200, resp.text
        assert profile.read_entity_label(data_dir, MODEL_ID) == "WP"


class TestEditCallFailures:
    def test_unknown_key_404(self, client, data_dir, profile):
        _save_key_script(client, profile)
        resp = _edit_call(
            client, profile, data_dir,
            argument=profile.entity_arg, value="X", key="editapi:nope:1",
        )
        assert resp.status_code == 404

    def test_missing_map_404(self, client, data_dir, profile):
        _save_key_script(client, profile)
        _map_path(data_dir).unlink()
        resp = _edit_call(
            client, profile, data_dir, argument=profile.entity_arg, value="X"
        )
        assert resp.status_code == 404

    def test_unknown_model_404(self, client, data_dir, profile):
        body = {
            profile.edit_body_field: profile.default_key,
            "argument": profile.entity_arg,
            "value": "X",
        }
        resp = client.post(
            f"/models/{UNKNOWN_MODEL_ID}/script/edit-call", json=body
        )
        assert resp.status_code == 404

    def test_traced_origin_422_upload_unchanged(self, client, data_dir, profile):
        script = _traced_variant(
            profile.key_script(profile.default_key), profile.default_key
        )
        resp = client.put(f"/models/{MODEL_ID}/script", json={"script": script})
        assert resp.status_code == 200, resp.text
        assert client.post(f"/models/{MODEL_ID}/script/save", json={}).status_code == 200
        # origin 预检（W-0060 T2 自 per-service 版上移）：确认前置形态确实是 traced
        entry = json.loads(_map_path(data_dir).read_text(encoding="utf-8"))["map"][
            profile.default_key
        ]
        assert entry["origin"] == "traced"
        before = _upload(data_dir, profile).read_bytes()

        resp = _edit_call(
            client, profile, data_dir, argument=profile.entity_arg, value="X"
        )
        assert resp.status_code == 422
        assert _upload(data_dir, profile).read_bytes() == before

    def test_non_scalar_value_422_upload_unchanged(self, client, data_dir, profile):
        _save_key_script(client, profile)
        before = _upload(data_dir, profile).read_bytes()
        resp = _edit_call(
            client, profile, data_dir, argument=profile.entity_arg, value={"x": 1}
        )
        assert resp.status_code == 422
        assert _upload(data_dir, profile).read_bytes() == before

    def test_non_identifier_argument_422_upload_unchanged(
        self, client, data_dir, profile
    ):
        """非法参数名（libcst CSTValidationError 路径）→ 422 而非 500。"""
        _save_key_script(client, profile)
        before = _upload(data_dir, profile).read_bytes()
        resp = _edit_call(client, profile, data_dir, argument="na me", value="X")
        assert resp.status_code == 422
        assert _upload(data_dir, profile).read_bytes() == before

    def test_non_finite_float_422_upload_unchanged(
        self, client, data_dir, profile
    ):
        """NaN 值（json.loads 接受 NaN 字面量）→ 422 而非 500。"""
        _save_key_script(client, profile)
        before = _upload(data_dir, profile).read_bytes()
        body = (
            b'{"' + profile.edit_body_field.encode() + b'": "'
            + profile.default_key.encode() + b'", "argument": "'
            + profile.entity_arg.encode() + b'", "value": NaN}'
        )
        resp = client.post(
            f"/models/{MODEL_ID}/script/edit-call",
            content=body,
            headers={"Content-Type": "application/json"},
        )
        assert resp.status_code == 422
        assert _upload(data_dir, profile).read_bytes() == before

    def test_build_failure_422_zero_side_effects(self, client, data_dir, profile):
        """重写合法但沙箱 run 失败（未知 kwargs → TypeError）→ 422，staging/
        uploads/map 全部不变。"""
        _save_key_script(client, profile)
        before_upload = _upload(data_dir, profile).read_bytes()
        before_map = _map_path(data_dir).read_bytes()

        resp = _edit_call(
            client, profile, data_dir, argument="editapi_bogus_kwarg", value=1
        )
        assert resp.status_code == 422, resp.text
        assert client.get(f"/models/{MODEL_ID}/script").json()["staged"] == 0
        assert _upload(data_dir, profile).read_bytes() == before_upload
        assert _map_path(data_dir).read_bytes() == before_map


class TestEditCallStaleMap:
    """staging 与 map 分叉 → 409 fail-closed、零副作用（防旧行号改错调用）。"""

    def test_unrun_staged_edit_409_zero_side_effects(self, client, data_dir, profile):
        script = _save_key_script(client, profile)
        before_upload = _upload(data_dir, profile).read_bytes()
        before_map = _map_path(data_dir).read_bytes()
        shifted = "# line-shifting edit\n" + script
        resp = client.put(f"/models/{MODEL_ID}/script", json={"script": shifted})
        assert resp.status_code == 200, resp.text

        resp = _edit_call(
            client, profile, data_dir, argument=profile.entity_arg, value="W9"
        )
        assert resp.status_code == 409, resp.text
        assert _upload(data_dir, profile).read_bytes() == before_upload
        assert _map_path(data_dir).read_bytes() == before_map
        got = client.get(f"/models/{MODEL_ID}/script").json()
        assert got["script"] == shifted
        assert got["staged"] == 1

    def test_legacy_bare_map_409(self, client, data_dir, profile):
        _save_key_script(client, profile)
        envelope = json.loads(_map_path(data_dir).read_text(encoding="utf-8"))
        _map_path(data_dir).write_text(json.dumps(envelope["map"]), encoding="utf-8")
        resp = _edit_call(
            client, profile, data_dir, argument=profile.entity_arg, value="W9"
        )
        assert resp.status_code == 409, resp.text
        # 产物未被触碰（W-0060 T2 自 per-service 版上移的语义级校验）
        assert profile.read_entity_label(data_dir, MODEL_ID) == "W1"

    def test_undo_after_run_409_label_keeps_post_run(
        self, client, data_dir, profile
    ):
        """run 之后 undo：map 描述的是 undo 前的脚本 → 409。"""
        script = _save_key_script(client, profile)
        arg = profile.entity_arg
        renamed = script.replace(f'{arg}="W1"', f'{arg}="W2"')
        assert renamed != script
        assert client.put(
            f"/models/{MODEL_ID}/script", json={"script": renamed}
        ).status_code == 200
        assert client.post(f"/models/{MODEL_ID}/script/run").status_code == 200
        assert client.post(f"/models/{MODEL_ID}/script/undo").status_code == 200

        resp = _edit_call(client, profile, data_dir, argument=arg, value="W9")
        assert resp.status_code == 409, resp.text
        assert profile.read_entity_label(data_dir, MODEL_ID) == "W2"  # 保持 run 后状态
