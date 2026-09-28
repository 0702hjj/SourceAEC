"""aiifc CLI --project-id 测试（中间产物规范落盘 skill-work/{projectID}）。

规范：中间产物（design.json/features.json/演示 IFC——辅助信息，不进版本）落
skill-work/{projectID}/；build 脚本 + IFC 版本化走 models/{modelId}/（script-as-source，不经此）。
"""

import json
import os
import subprocess
import sys
from pathlib import Path

import pytest

from aiifc.cli import resolve_skill_workdir

CLI = [sys.executable, "-m", "aiifc.cli"]


@pytest.fixture()
def data_root(tmp_path, monkeypatch):
    root = tmp_path / "data"
    monkeypatch.setenv("VIEWER_DATA_DIR", str(root))
    return root


def test_resolve_skill_workdir(data_root):
    """projectId → {VIEWER_DATA_DIR}/skill-work/{pid}/。"""
    assert resolve_skill_workdir("p_x") == str(data_root / "skill-work" / "p_x")


def test_resolve_requires_data_dir(monkeypatch):
    """无 VIEWER_DATA_DIR → SystemExit（提示独立使用显式 -o）。"""
    monkeypatch.delenv("VIEWER_DATA_DIR", raising=False)
    with pytest.raises(SystemExit):
        resolve_skill_workdir("p_x")


def test_consume_upstream_project_id_lands_in_workdir(data_root, tmp_path):
    """consume-upstream --project-id → design.json 自动落 skill-work/{pid}/（中间产物规范落盘）。"""
    building = tmp_path / "b.json"
    building.write_text('{"version":2,"project":"t","zones":[{"zone":"z","floors_from":1,"floors_to":1,"modelId":"m_1"}]}')
    bim = tmp_path / "bim.json"
    bim.write_text('{}')
    r = subprocess.run(CLI + ["consume-upstream", "--building", str(building),
                              "--bim", str(bim), "--dxf-dir", str(tmp_path),
                              "--project-id", "p_up"],
                       capture_output=True, text=True, env={**os.environ})
    assert r.returncode == 0, r.stderr
    design_path = data_root / "skill-work" / "p_up" / "design.json"
    assert design_path.is_file(), f"design.json 应落 skill-work/p_up/，got stdout={r.stdout}"
    design = json.loads(design_path.read_text(encoding="utf-8"))
    assert design["frame"]["storeys"] == {"1F": 0.0}


def test_explicit_out_overrides_project_id(data_root, tmp_path):
    """显式 -o 优先于 --project-id（中间产物落显式路径）。"""
    building = tmp_path / "b.json"
    building.write_text('{"version":2,"project":"t","zones":[{"zone":"z","floors_from":1,"floors_to":1,"modelId":"m_1"}]}')
    bim = tmp_path / "bim.json"
    bim.write_text('{}')
    explicit = tmp_path / "custom_design.json"
    r = subprocess.run(CLI + ["consume-upstream", "--building", str(building),
                              "--bim", str(bim), "--dxf-dir", str(tmp_path),
                              "--project-id", "p_up", "-o", str(explicit)],
                       capture_output=True, text=True, env={**os.environ})
    assert r.returncode == 0, r.stderr
    assert explicit.is_file(), "显式 -o 应优先（落显式路径）"
    assert not (data_root / "skill-work" / "p_up" / "design.json").exists(), \
        "显式 -o 时不应落 skill-work 缺省"


def test_design_build_project_id_lands_in_workdir(data_root, tmp_path):
    """design-build --project-id → features.json 落 skill-work/{pid}/。"""
    design = tmp_path / "d.json"
    design.write_text(json.dumps({
        "meta": {"units": "m", "name": "t"},
        "frame": {"footprint": [[0, 0], [10, 0], [10, 8], [0, 8]], "storeys": {"1F": 0.0}},
        "floors": {"1F": {"walls": [{"axis": [[0, 0], [10, 0]], "t": 0.2, "kind": "ext"}],
                          "openings": [], "slabs": [], "stairs": []}},
    }))
    env = dict(os.environ)
    env["AIIFC_FLOWS_DIR"] = str(Path(__file__).resolve().parents[1] / "references" / "docs" / "flows")
    r = subprocess.run(CLI + ["design-build", str(design), "--project-id", "p_db"],
                       capture_output=True, text=True, env=env)
    assert r.returncode == 0, r.stderr
    assert (data_root / "skill-work" / "p_db" / "features.json").is_file(), \
        "features.json 应落 skill-work/p_db/"


def _build_skeleton_ifc(tmp_path, monkeypatch, data_root):
    """造骨架 IFC（design-build → build-script），返回 model.ifc 路径。"""
    design = tmp_path / "d.json"
    design.write_text(json.dumps({
        "meta": {"units": "m", "modulus": 0.1, "name": "t"},
        "frame": {"footprint": [[0, 0], [10, 0], [10, 8], [0, 8]], "storeys": {"1F": 0.0}},
        "floors": {"1F": {"walls": [{"axis": [[0, 0], [10, 0]], "t": 0.2, "kind": "ext"}],
                          "openings": [], "slabs": [], "stairs": []}},
    }))
    env = dict(os.environ)
    env["AIIFC_FLOWS_DIR"] = str(Path(__file__).resolve().parents[1] / "references" / "docs" / "flows")
    env["VIEWER_DATA_DIR"] = str(data_root)
    r = subprocess.run(CLI + ["design-build", str(design), "--project-id", "p_rev"],
                       capture_output=True, text=True, env=env)
    assert r.returncode == 0, r.stderr
    features = data_root / "skill-work" / "p_rev" / "features.json"
    assert features.is_file()
    r = subprocess.run(CLI + ["build-script", str(features), "--project-id", "p_rev"],
                       capture_output=True, text=True, env=env)
    assert r.returncode == 0, r.stderr
    return data_root / "skill-work" / "p_rev" / "model.ifc"


def test_design_review_command(tmp_path, monkeypatch):
    """aiifc design-review：SKILL #21 强制检查——跑黑盒质量报告，成功返回报告文本。"""
    data_root = tmp_path / "data"
    model = _build_skeleton_ifc(tmp_path, monkeypatch, data_root)
    env = dict(os.environ)
    env["AIIFC_FLOWS_DIR"] = str(Path(__file__).resolve().parents[1] / "references" / "docs" / "flows")
    r = subprocess.run(CLI + ["design-review", str(model), "office"],
                       capture_output=True, text=True, env=env, timeout=120)
    assert r.returncode == 0, f"design-review 失败: {r.stderr[:300]}"
    assert "DESIGN REVIEW" in r.stdout, f"应输出审查报告: {r.stdout[:200]}"


def test_ifc_inspect_command(tmp_path, monkeypatch):
    """aiifc ifc-inspect：SKILL #24 强制验证——跑结构检查，成功返回检查 JSON。"""
    data_root = tmp_path / "data"
    model = _build_skeleton_ifc(tmp_path, monkeypatch, data_root)
    env = dict(os.environ)
    env["AIIFC_FLOWS_DIR"] = str(Path(__file__).resolve().parents[1] / "references" / "docs" / "flows")
    r = subprocess.run(CLI + ["ifc-inspect", str(model), "--no-psets"],
                       capture_output=True, text=True, env=env, timeout=60)
    assert r.returncode == 0, f"ifc-inspect 失败: {r.stderr[:300]}"
    assert "ifc_inspect" in r.stdout, f"应输出检查摘要: {r.stdout[:200]}"


def test_review_reports_land_in_skill_work(tmp_path, monkeypatch):
    """design-review / ifc-inspect 带 --project-id → 报告 JSON 落 skill-work/{pid}/reports/
    （中间产物规范落盘，与 design-build/build-script 统一，不进 analysis_results/）。"""
    data_root = tmp_path / "data"
    model = _build_skeleton_ifc(tmp_path, monkeypatch, data_root)
    env = dict(os.environ)
    env["AIIFC_FLOWS_DIR"] = str(Path(__file__).resolve().parents[1] / "references" / "docs" / "flows")
    env["VIEWER_DATA_DIR"] = str(data_root)
    r = subprocess.run(CLI + ["design-review", str(model), "office", "--project-id", "p_rev"],
                       capture_output=True, text=True, env=env, timeout=120)
    assert r.returncode == 0, r.stderr[:300]
    reports = data_root / "skill-work" / "p_rev" / "reports"
    files = list(reports.glob("*.json")) if reports.is_dir() else []
    assert any("analysis" in f.name for f in files), f"审查报告应落 skill-work/reports: {files}"
    r2 = subprocess.run(CLI + ["ifc-inspect", str(model), "--no-psets", "--project-id", "p_rev"],
                        capture_output=True, text=True, env=env, timeout=60)
    assert r2.returncode == 0, r2.stderr[:300]
    files2 = list(reports.glob("*.json")) if reports.is_dir() else []
    assert any("structure" in f.name for f in files2), f"检查报告应落 skill-work/reports: {files2}"
