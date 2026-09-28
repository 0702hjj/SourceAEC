# SPDX-License-Identifier: Apache-2.0
# Copyright (C) 2026 0702hjj

"""render.json payload v2 tests: entity-keyed geometry + unsupported surfacing.

Covers build_render_payload (pure function) and the GET /models/{id}/render.json
endpoint + the run/save publish hook. Key contract: render 实体的 key 集合
（滤掉块展开产生的 key=None 子实体）必须等于 current.map.json 的 key 集合。
"""

from __future__ import annotations

import json
import math
from pathlib import Path

import ezdxf
import pytest

import cad_script_lib

from app.render import build_render_payload

from tests.conftest import MODEL_ID

GOOD_SCRIPT = '''\
import sys

import ezdxf

from cad_script_lib import add_entity, write_and_validate

PARAMS = {"length": 10}

def build(params, out_path):
    doc = ezdxf.new("R2010")
    msp = doc.modelspace()
    add_entity(msp, "LINE", start=(0, 0), end=(params["length"], 0))
    write_and_validate(doc, out_path)

if __name__ == "__main__":
    build(PARAMS, sys.argv[1])
'''

BASE = f"/models/{MODEL_ID}"


def _build(path: Path, *ops) -> Path:
    cad_script_lib.reset_state()
    doc = ezdxf.new("R2010")
    msp = doc.modelspace()
    for op in ops:
        op(msp)
    assert cad_script_lib.write_and_validate(doc, str(path))
    return path


def _line(key, start=(0, 0), end=(10, 0), **attribs):
    def op(msp):
        cad_script_lib.add_entity(
            msp, "LINE", key=key, start=start, end=end, dxfattribs=attribs or None
        )
    return op


def _entity(payload, predicate):
    return next(e for e in payload["entities"] if predicate(e))


def _render_path(data_dir: Path) -> Path:
    return data_dir / "models" / MODEL_ID / "render.json"


class TestEntityGeometry:
    def test_line_fields(self, tmp_path):
        path = _build(
            tmp_path / "m.dxf",
            _line("0:line:1", layer="WALL", color=1, linetype="DASHED"))
        payload = build_render_payload(str(path))
        assert payload["schemaVersion"] == 2
        entry = _entity(payload, lambda e: e["type"] == "LINE")
        assert entry["key"] == "0:line:1"
        assert entry["layer"] == "WALL"
        assert entry["color"] == 1
        assert entry["linetype"] == "DASHED"
        assert entry["start"] == [0.0, 0.0]
        assert entry["end"] == [10.0, 0.0]

    def test_circle_fields(self, tmp_path):
        def op(msp):
            cad_script_lib.add_entity(
                msp, "CIRCLE", key="0:circle:1", center=(5, 5), radius=2)
        payload = build_render_payload(str(_build(tmp_path / "m.dxf", op)))
        entry = _entity(payload, lambda e: e["type"] == "CIRCLE")
        assert entry["key"] == "0:circle:1"
        assert entry["center"] == [5.0, 5.0]
        assert entry["radius"] == 2.0

    def test_arc_fields(self, tmp_path):
        def op(msp):
            cad_script_lib.add_entity(
                msp, "ARC", key="0:arc:1", center=(0, 0), radius=4,
                start_angle=15, end_angle=120)
        payload = build_render_payload(str(_build(tmp_path / "m.dxf", op)))
        entry = _entity(payload, lambda e: e["type"] == "ARC")
        assert entry["key"] == "0:arc:1"
        assert entry["center"] == [0.0, 0.0]
        assert entry["radius"] == 4.0
        assert entry["start_angle"] == 15.0
        assert entry["end_angle"] == 120.0

    def test_text_fields(self, tmp_path):
        def op(msp):
            cad_script_lib.add_entity(
                msp, "TEXT", key="0:text:1", text="标注", insert=(1, 2), height=3.5)
        payload = build_render_payload(str(_build(tmp_path / "m.dxf", op)))
        entry = _entity(payload, lambda e: e["type"] == "TEXT")
        assert entry["key"] == "0:text:1"
        assert entry["text"] == "标注"
        assert entry["insert"] == [1.0, 2.0]
        assert entry["height"] == 3.5

    def test_mtext_fields(self, tmp_path):
        def op(msp):
            cad_script_lib.add_entity(
                msp, "MTEXT", key="0:mtext:1", text="多行", insert=(2, 3))
        payload = build_render_payload(str(_build(tmp_path / "m.dxf", op)))
        entry = _entity(payload, lambda e: e["type"] == "MTEXT")
        assert entry["key"] == "0:mtext:1"
        assert entry["text"] == "多行"
        assert entry["insert"] == [2.0, 3.0]

    def test_lwpolyline_explodes_to_line_and_arc_segments(self, tmp_path):
        """LWPOLYLINE 炸开：直线段 → LINE 条目；bulge 段 → ARC 条目（同 key）。"""
        def op(msp):
            cad_script_lib.add_entity(
                msp, "LWPOLYLINE", key="0:lwpolyline:1", format="xyb",
                points=[(0, 0, 0.0), (10, 0, 0.5), (10, 10, 0.0)])
        payload = build_render_payload(str(_build(tmp_path / "m.dxf", op)))
        segments = [
            e for e in payload["entities"] if e["key"] == "0:lwpolyline:1"]
        assert [s["type"] for s in segments] == ["LINE", "ARC"]
        line, arc = segments
        assert line["start"] == [0.0, 0.0]
        assert line["end"] == [10.0, 0.0]
        # bulge=0.5，弦 (10,0)→(10,10)：r=6.25，center=(6.25,5)
        assert arc["center"] == [6.25, 5.0]
        assert arc["radius"] == 6.25
        assert arc["start_angle"] == pytest.approx(306.869898, abs=1e-6)
        assert arc["end_angle"] == pytest.approx(413.130103, abs=1e-6)

    def test_closed_lwpolyline_adds_closing_segment(self, tmp_path):
        def op(msp):
            cad_script_lib.add_entity(
                msp, "LWPOLYLINE", key="0:lwpolyline:1", format="xyb",
                points=[(0, 0, 0.0), (10, 0, 0.0), (10, 10, 0.0)], closed=True)
        payload = build_render_payload(str(_build(tmp_path / "m.dxf", op)))
        segments = [
            e for e in payload["entities"] if e["key"] == "0:lwpolyline:1"]
        assert [s["type"] for s in segments] == ["LINE", "LINE", "LINE"]
        closing = segments[-1]
        assert closing["start"] == [10.0, 10.0]
        assert closing["end"] == [0.0, 0.0]

    def test_coordinates_keep_original_dxf_frame(self, tmp_path):
        """v2 不做 screen 归一化：原始 DXF 坐标保留，数字 round 6。"""
        path = _build(
            tmp_path / "m.dxf",
            _line("0:line:1", start=(1000.1234567891, -2000), end=(1001, 2001)))
        payload = build_render_payload(str(path))
        entry = _entity(payload, lambda e: e["type"] == "LINE")
        assert entry["start"] == [1000.123457, -2000.0]
        assert entry["end"] == [1001.0, 2001.0]


class TestInsertExpansion:
    def test_insert_entry_and_block_expansion(self, tmp_path):
        """INSERT 本体入 entities（name/insert/rotation/scale）；块内实体展开一层，
        应用 translate+rotate+uniform scale，子实体 key=None 且标 block 来源。"""
        def op(msp):
            blk = msp.doc.blocks.new("BLK")
            blk.add_line((0, 0), (1, 1))
            blk.add_circle((0, 0), 1)
            cad_script_lib.add_entity(
                msp, "INSERT", key="0:insert:1", name="BLK", insert=(10, 5),
                dxfattribs={"rotation": 90, "xscale": 2, "yscale": 2})
        payload = build_render_payload(str(_build(tmp_path / "m.dxf", op)))
        insert = _entity(payload, lambda e: e["type"] == "INSERT")
        assert insert["key"] == "0:insert:1"
        assert insert["name"] == "BLK"
        assert insert["insert"] == [10.0, 5.0]
        assert insert["rotation"] == 90.0
        assert insert["scale"] == 2.0
        children = [e for e in payload["entities"] if e.get("block") == "BLK"]
        assert {c["type"] for c in children} == {"LINE", "CIRCLE"}
        assert all(c["key"] is None for c in children)
        line = next(c for c in children if c["type"] == "LINE")
        # scale 2 + rotate 90°: (1,1) -> (-2,2); + insert (10,5)
        assert line["start"] == [10.0, 5.0]
        assert line["end"] == [8.0, 7.0]
        circle = next(c for c in children if c["type"] == "CIRCLE")
        assert circle["center"] == [10.0, 5.0]
        assert circle["radius"] == 2.0

    def test_insert_nonuniform_scale_goes_unsupported(self, tmp_path):
        """非等比 scale 不展开：INSERT 本体仍在 entities，unsupported 记一条。"""
        def op(msp):
            blk = msp.doc.blocks.new("BLK")
            blk.add_line((0, 0), (1, 1))
            cad_script_lib.add_entity(
                msp, "INSERT", key="0:insert:1", name="BLK", insert=(1, 1),
                dxfattribs={"xscale": 2, "yscale": 1})
        payload = build_render_payload(str(_build(tmp_path / "m.dxf", op)))
        insert = _entity(payload, lambda e: e["type"] == "INSERT")
        assert insert["key"] == "0:insert:1"
        assert not [e for e in payload["entities"] if e.get("block") == "BLK"]
        entry = _entity(
            {"entities": payload["unsupported"]},
            lambda e: e["type"] == "INSERT")
        assert entry["handle"]
        assert entry["coords"] == [1.0, 1.0]

    def test_nested_insert_goes_unsupported(self, tmp_path):
        """块内 INSERT（嵌套第二层）不展开，进 unsupported。"""
        def op(msp):
            inner = msp.doc.blocks.new("INNER")
            inner.add_line((0, 0), (1, 0))
            outer = msp.doc.blocks.new("OUTER")
            outer.add_blockref("INNER", (0, 0))
            cad_script_lib.add_entity(
                msp, "INSERT", key="0:insert:1", name="OUTER", insert=(0, 0))
        payload = build_render_payload(str(_build(tmp_path / "m.dxf", op)))
        nested = [u for u in payload["unsupported"] if u["type"] == "INSERT"]
        assert len(nested) == 1
        assert nested[0]["handle"]


class TestArcAngleNormalization:
    """W-0042：bulge 段 end 越界编码 + INSERT 旋转角度归一 + 跨零 ARC bounds。

    角度契约：start ∈ [0,360)；bulge 段「end ∉ [0,360) ⟺ 有向 bulge」完备
    判据（落回 [0,360) 时越界编码）；原生 ARC 恒 CCW，end ∈ [0,360) 且
    end < start 表跨零；INSERT 旋转后 end − start = 真值有向 sweep。
    """

    @staticmethod
    def _polar_arc_segment(radius, start_deg, sweep_deg):
        """圆心原点圆弧段 → LWPOLYLINE xyb 两点 + 对应 bulge。"""
        bulge = math.tan(math.radians(sweep_deg) / 4.0)
        p1 = (radius * math.cos(math.radians(start_deg)),
              radius * math.sin(math.radians(start_deg)))
        p2 = (radius * math.cos(math.radians(start_deg + sweep_deg)),
              radius * math.sin(math.radians(start_deg + sweep_deg)))
        return [(p1[0], p1[1], bulge), (p2[0], p2[1], 0.0)]

    def test_bulge_cw_fall_back_end_encoded_out_of_range(self, tmp_path):
        """bulge CW 落回：start=100, sweep=−60 → end 编码 −320（非 40）。

        未修复时 end=40 落回 [0,360)，与原生跨零 ARC 不可区分（原生恒
        CCW、end<start 表跨零），CW 真值被前端按 CCW 画反。
        """
        def op(msp):
            cad_script_lib.add_entity(
                msp, "LWPOLYLINE", key="0:lwpolyline:1", format="xyb",
                points=self._polar_arc_segment(10.0, 100.0, -60.0))
        payload = build_render_payload(str(_build(tmp_path / "m.dxf", op)))
        arc = _entity(payload, lambda e: e["type"] == "ARC")
        assert arc["start_angle"] == pytest.approx(100.0, abs=1e-6)
        assert arc["end_angle"] == pytest.approx(-320.0, abs=1e-6)

    def test_bulge_ccw_fall_back_end_encoded_out_of_range(self, tmp_path):
        """bulge CCW 落回（判据对称）：start=100, sweep=+60 → end 编码 520（非 160）。"""
        def op(msp):
            cad_script_lib.add_entity(
                msp, "LWPOLYLINE", key="0:lwpolyline:1", format="xyb",
                points=self._polar_arc_segment(10.0, 100.0, 60.0))
        payload = build_render_payload(str(_build(tmp_path / "m.dxf", op)))
        arc = _entity(payload, lambda e: e["type"] == "ARC")
        assert arc["start_angle"] == pytest.approx(100.0, abs=1e-6)
        assert arc["end_angle"] == pytest.approx(520.0, abs=1e-6)

    def test_insert_rotation_normalizes_native_arc_angles(self, tmp_path):
        """原生 ARC {10,50} + rotation −60 → {310,350}。

        start ∈ [0,360) 且 end − start = 40 = CCW 真值 sweep（未修复时
        payload 为 {−50,−10}，start 出界）。
        """
        def op(msp):
            blk = msp.doc.blocks.new("BLK")
            blk.add_arc((0, 0), 10, 10, 50)
            cad_script_lib.add_entity(
                msp, "INSERT", key="0:insert:1", name="BLK", insert=(0, 0),
                dxfattribs={"rotation": -60})
        payload = build_render_payload(str(_build(tmp_path / "m.dxf", op)))
        arc = _entity(payload, lambda e: e["type"] == "ARC")
        assert arc["start_angle"] == 310.0
        assert arc["end_angle"] == 350.0

    def test_insert_rotation_normalizes_bulge_cw_angles(self, tmp_path):
        """bulge CW {start:300, sweep:−120} + rotation +90 → {30,−90}。

        end − start = −120 = 真值有向 sweep（未修复时 payload 为 {390,270}，
        start 出界且靠前端 k 平移补救）。
        """
        def op(msp):
            blk = msp.doc.blocks.new("BLK")
            blk.add_lwpolyline(
                self._polar_arc_segment(10.0, 300.0, -120.0), format="xyb")
            cad_script_lib.add_entity(
                msp, "INSERT", key="0:insert:1", name="BLK", insert=(0, 0),
                dxfattribs={"rotation": 90})
        payload = build_render_payload(str(_build(tmp_path / "m.dxf", op)))
        arc = _entity(payload, lambda e: e["type"] == "ARC")
        assert arc["start_angle"] == pytest.approx(30.0, abs=1e-6)
        assert arc["end_angle"] == pytest.approx(-90.0, abs=1e-6)

    def test_cross_zero_native_arc_bounds_include_zero_degree_extreme(
            self, tmp_path):
        """跨零原生 ARC 270→45（CCW +135）：bounds 必须含 0° 极值点 (cx+r, cy)。

        未修复时 sweep = 45−270 = −225 按 CW 解读，0° 不采样，max.x 漏成
        cos45·r ≈ 7.071068。
        """
        def op(msp):
            cad_script_lib.add_entity(
                msp, "ARC", key="0:arc:1", center=(0, 0), radius=10,
                start_angle=270, end_angle=45)
        payload = build_render_payload(str(_build(tmp_path / "m.dxf", op)))
        assert payload["bounds"] == {"min": [0.0, -10.0],
                                     "max": [10.0, 7.071068]}


class TestUnsupported:
    def test_unknown_entity_surfaced_not_dropped(self, tmp_path):
        def op(msp):
            msp.add_point((1, 2))
        payload = build_render_payload(str(_build(tmp_path / "m.dxf", op)))
        assert payload["entities"] == []
        assert len(payload["unsupported"]) == 1
        entry = payload["unsupported"][0]
        assert entry["type"] == "POINT"
        assert entry["handle"]
        assert entry["coords"] == [1.0, 2.0]


class TestPayloadShape:
    def test_bounds_cover_geometry(self, tmp_path):
        def op(msp):
            cad_script_lib.add_entity(
                msp, "LINE", key="0:line:1", start=(0, 0), end=(10, 0))
            cad_script_lib.add_entity(
                msp, "CIRCLE", key="0:circle:1", center=(5, 5), radius=2)
        payload = build_render_payload(str(_build(tmp_path / "m.dxf", op)))
        assert payload["bounds"] == {"min": [0.0, 0.0], "max": [10.0, 7.0]}

    def test_layers_listing(self, tmp_path):
        path = _build(tmp_path / "m.dxf", _line("0:line:1", layer="WALL"))
        payload = build_render_payload(str(path))
        names = {layer["name"] for layer in payload["layers"]}
        assert "WALL" in names
        wall = next(l for l in payload["layers"] if l["name"] == "WALL")
        assert isinstance(wall["color"], int)
        assert isinstance(wall["linetype"], str)

    def test_payload_json_serializable(self, tmp_path):
        path = _build(tmp_path / "m.dxf", _line("0:line:1"))
        json.dumps(build_render_payload(str(path)), ensure_ascii=False)


class TestKeyContract:
    def test_render_keys_match_current_map_keys(self, client, data_dir):
        """契约：render 实体 key 集合（滤 None）== current.map.json key 集合。"""
        client.put(f"{BASE}/script", json={"script": GOOD_SCRIPT})
        resp = client.post(f"{BASE}/script/run")
        assert resp.status_code == 200
        payload = client.get(f"{BASE}/render.json").json()
        with open(data_dir / "models" / MODEL_ID / "current.map.json",
                  encoding="utf-8") as fh:
            map_keys = set(json.load(fh)["map"])
        render_keys = {e["key"] for e in payload["entities"] if e["key"]}
        assert render_keys == map_keys == {"0:line:1"}


class TestEndpoint:
    def test_get_render_on_demand_for_upload_only_model(self, client):
        """无 render.json 时按 uploads dxf 即时生成（fixture 含 LINE+CIRCLE）。"""
        resp = client.get(f"{BASE}/render.json")
        assert resp.status_code == 200
        payload = resp.json()
        assert payload["schemaVersion"] == 2
        keys = {e["key"] for e in payload["entities"] if e["key"]}
        assert keys == {"0:line:1", "0:circle:1"}

    def test_get_render_404_unknown_model(self, client):
        resp = client.get("/models/m_ffffffffffffffff/render.json")
        assert resp.status_code == 404


class TestRunSaveHook:
    def test_publish_layering_single_source(self):
        """W-0059 分层钉住：几何在 render、IO 发布在 render_publish。

        render 不得回流 publish 面（保持纯函数域）；best-effort 删除用
        aibim_editapi 共享 remove_quiet，不得再持有私有拷贝。若有人把
        publish 复制回 render.py 或恢复 _remove_quiet，该用例变红。
        """
        from app import render, render_publish, routes_script_run
        from aibim_editapi.routes_script_run import remove_quiet

        assert not hasattr(render, "publish_render_json")
        assert not hasattr(render_publish, "_remove_quiet")
        assert render_publish.remove_quiet is remove_quiet
        assert routes_script_run.AFTER_RUN is render_publish.publish_render_json

    def test_run_publishes_render_json(self, client, data_dir):
        client.put(f"{BASE}/script", json={"script": GOOD_SCRIPT})
        resp = client.post(f"{BASE}/script/run")
        assert resp.status_code == 200
        path = _render_path(data_dir)
        assert path.is_file()
        payload = json.loads(path.read_text(encoding="utf-8"))
        line = _entity(payload, lambda e: e["type"] == "LINE")
        assert line["end"] == [10.0, 0.0]
        # GET 走已发布文件
        assert client.get(f"{BASE}/render.json").json() == payload

    def test_save_updates_render_json(self, client, data_dir):
        client.put(f"{BASE}/script", json={"script": GOOD_SCRIPT})
        assert client.post(f"{BASE}/script/save").status_code == 200
        client.put(f"{BASE}/script", json={"params": {"length": 42}})
        resp = client.post(f"{BASE}/script/save")
        assert resp.status_code == 200
        payload = json.loads(_render_path(data_dir).read_text(encoding="utf-8"))
        line = _entity(payload, lambda e: e["type"] == "LINE")
        assert line["end"] == [42.0, 0.0]

    def test_generation_failure_keeps_run_ok_and_deletes_stale(
            self, client, data_dir, monkeypatch):
        """render 生成失败不阻断 run 主流程，且删除旧 render.json 防错位。"""
        client.put(f"{BASE}/script", json={"script": GOOD_SCRIPT})
        assert client.post(f"{BASE}/script/run").status_code == 200
        assert _render_path(data_dir).is_file()

        import app.render as render_module

        def boom(_path):
            raise RuntimeError("render-boom")

        monkeypatch.setattr(render_module, "build_render_payload", boom)
        client.put(f"{BASE}/script", json={"params": {"length": 20}})
        resp = client.post(f"{BASE}/script/run")
        assert resp.status_code == 200
        assert not _render_path(data_dir).exists()

    def test_write_failure_keeps_run_ok_and_removes_stale(
            self, client, data_dir, monkeypatch):
        """render.json 写盘失败（os.replace OSError）不阻断 run，且删旧文件防错位。"""
        client.put(f"{BASE}/script", json={"script": GOOD_SCRIPT})
        assert client.post(f"{BASE}/script/run").status_code == 200
        assert _render_path(data_dir).is_file()

        import os as os_module

        import app.routes_scripts as routes_module

        real_replace = os_module.replace

        def boom(src, dst, *args, **kwargs):
            if str(dst).endswith("render.json"):
                raise OSError("disk-full")
            return real_replace(src, dst, *args, **kwargs)

        monkeypatch.setattr(routes_module.os, "replace", boom)
        client.put(f"{BASE}/script", json={"params": {"length": 20}})
        resp = client.post(f"{BASE}/script/run")
        assert resp.status_code == 200
        assert not _render_path(data_dir).exists()


if __name__ == "__main__":
    raise SystemExit(pytest.main([__file__, "-q"]))
