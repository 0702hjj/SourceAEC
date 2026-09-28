"""消化矩阵契约测试（W-0052）——6 形态 fixture → consume-upstream → design-build →
build-script 全链，每形态 ≥1 条（参数化）+ 语义保真专项断言。

fixture 库：tests/fixtures/cad/（再生成见同目录 regen.py + README.md）。
保真度结论（保真/丢失/需人工补）记录在 docs/internal/w0052-digestion-matrix.md——
本文件把「当前口径」固化为契约：含诚实记录的丢失项（楼梯 STAIR 图层被 readback
忽略 → design stairs=[]；屋顶 metadata 进 features 但 IFC 不构建）。
"""

import json
import os
import subprocess
import sys
from pathlib import Path

import pytest

FIXTURES = Path(__file__).resolve().parent / "fixtures" / "cad"
CLI = [sys.executable, "-m", "aiifc.cli"]
FLOWS = Path(__file__).resolve().parents[1] / "references" / "docs" / "flows"
REPO = Path(__file__).resolve().parents[3]


def _chain_env() -> dict:
    """CLI 子进程环境：AIIFC_FLOWS_DIR 指**源** flows；PYTHONPATH 前置源 aiifc 包 +
    dxfkit/archdxf（dist 优先，同 conftest）——不依赖 editable 安装，防测到旧 dist 副本。"""
    env = dict(os.environ)
    env["AIIFC_FLOWS_DIR"] = str(FLOWS)
    src = str(Path(__file__).resolve().parents[1] / "scripts" / "aiifc_cli")
    paths = [src]
    for pkg in ("archdxf", "dxfkit"):
        for base in (REPO / "skills" / "dist" / "aidxf" / "scripts" / "packages",
                     REPO / "skills" / "aidxf" / "scripts" / "packages"):
            p = base / pkg / "src"
            if p.is_dir():
                paths.append(str(p))
                break
    env["PYTHONPATH"] = os.pathsep.join(paths) + os.pathsep + env.get("PYTHONPATH", "")
    return env


def _run_chain(form: str, tmp_path: Path) -> tuple[dict, dict, Path]:
    """fixture → 三步全链。每步 rc=0（build-script 的 rc 含 ifcopenshell.validate——
    validate ERR 即退出非零，W-0052 曾因多层 key 重复 → GlobalId 冲突在此拦截）。"""
    fix = FIXTURES / form
    design_p = tmp_path / "design.json"
    r = subprocess.run(
        CLI + ["consume-upstream", "--building", str(fix / "building.json"),
               "--bim", str(fix / "bim_supplement.json"),
               "--dxf-dir", str(fix), "-o", str(design_p)],
        capture_output=True, text=True, env=_chain_env())
    assert r.returncode == 0, f"[{form}] consume-upstream: {r.stderr or r.stdout}"
    features_p = tmp_path / "features.json"
    r = subprocess.run(CLI + ["design-build", str(design_p), "-o", str(features_p)],
                       capture_output=True, text=True, env=_chain_env())
    assert r.returncode == 0, f"[{form}] design-build: {r.stderr or r.stdout}"
    ifc_p = tmp_path / "model.ifc"
    r = subprocess.run(CLI + ["build-script", str(features_p), "-o", str(ifc_p)],
                       capture_output=True, text=True, env=_chain_env())
    assert r.returncode == 0, f"[{form}] build-script: {r.stderr or r.stdout}"
    for p in (design_p, features_p, ifc_p):
        assert p.is_file(), f"[{form}] 产物缺失: {p}"
    return (json.loads(design_p.read_text(encoding="utf-8")),
            json.loads(features_p.read_text(encoding="utf-8")), ifc_p)


def _ifc_counts(path: Path) -> dict:
    """IFC 打开 + 构件计数（ifcopenshell；打开失败即测试失败）。"""
    import ifcopenshell
    m = ifcopenshell.open(str(path))
    return {cls: len(m.by_type(cls)) for cls in
            ("IfcBuildingStorey", "IfcWall", "IfcWindow", "IfcDoor",
             "IfcSlab", "IfcStair", "IfcRoof", "IfcOpeningElement")}


def _walls_max_x(design: dict, floor: str) -> float:
    pts = [p for w in design["floors"][floor]["walls"]
           for p in (w.get("axis") or [])] + \
          [w["arc"]["center"] for w in design["floors"][floor]["walls"] if "arc" in w]
    return max(x for x, _ in pts)


# 每形态语义断言（输入构造口径见 fixtures/cad/regen.py docstring）
FORMS = {
    "01-single-zone": {
        "storeys": ["1F"],
        "ifc": {"IfcBuildingStorey": 1, "IfcWindow": 1, "IfcDoor": 1, "IfcSlab": 1},
    },
    "02-multi-zone": {
        "storeys": ["1F", "2F", "3F", "4F", "5F"],
        "ifc": {"IfcBuildingStorey": 5, "IfcWindow": 5, "IfcSlab": 5},
    },
    "03-multi-storey": {
        "storeys": ["1F", "2F", "3F"],
        "ifc": {"IfcBuildingStorey": 3, "IfcWindow": 6, "IfcDoor": 6, "IfcSlab": 3},
    },
    "04-arc-wall": {
        "storeys": ["1F"],
        "ifc": {"IfcBuildingStorey": 1, "IfcWindow": 1},
    },
    "05-openings": {
        "storeys": ["1F"],
        "ifc": {"IfcBuildingStorey": 1, "IfcWindow": 3, "IfcDoor": 3},
    },
    "06-stair-roof": {
        "storeys": ["1F", "2F"],
        "ifc": {"IfcBuildingStorey": 2, "IfcWindow": 4, "IfcSlab": 2},
    },
}


@pytest.mark.parametrize("form,expect", sorted(FORMS.items()),
                         ids=lambda f: f if isinstance(f, str) else "")
def test_full_chain_per_form(form, expect, tmp_path):
    """每形态 1 条全链契约：三步 rc=0 + 产物存在 + design schema 关键字段 + IFC 打开。

    build-script rc=0 内含 ifcopenshell.validate 通过（write_and_validate 出口）。
    """
    design, features, ifc_p = _run_chain(form, tmp_path)
    # design.json：storeys/floors 结构合法（DESIGN_JSON_SCHEMA 关键字段）
    assert list(design["frame"]["storeys"]) == expect["storeys"]
    assert set(design["floors"]) == set(expect["storeys"])
    for fn, fd in design["floors"].items():
        assert fd["walls"], f"{form} {fn} 应有墙"
        assert "openings" in fd and "slabs" in fd and "stairs" in fd
    # features.json：walls 全量进 features（design→features 构件数一致）
    assert len(features["walls"]) == sum(len(fd["walls"]) for fd in design["floors"].values())
    assert features["storeys"] == design["frame"]["storeys"]
    # IFC：可打开 + 构件计数（楼梯/屋顶为 0 是当前口径的诚实契约，见专项测试）
    counts = _ifc_counts(ifc_p)
    assert counts["IfcWall"] >= 1
    for cls, n in expect["ifc"].items():
        assert counts[cls] == n, f"{form} {cls}: {counts[cls]} != {n}"


def test_02_multi_zone_floor_mapping(tmp_path):
    """多 zone：非重叠 floors_from/to 映射保真——1F/2F 几何来自 podium（12m 宽）、
    3F-5F 来自 tower（8m 宽）；typical 按 typology 归组；footprint 取首层 zone。"""
    design, features, ifc_p = _run_chain("02-multi-zone", tmp_path)
    assert _walls_max_x(design, "1F") > 11.0, "1F 应为 podium（12m 宽）几何"
    assert _walls_max_x(design, "3F") < 9.0, "3F 应为 tower（8m 宽）几何"
    typical = design["frame"]["typical"]
    assert typical["RETAIL"] == ["1F", "2F"] and typical["OFFICE"] == ["3F", "4F", "5F"]
    fp = design["frame"]["footprint"]
    assert max(x for x, _ in fp) > 11.0, "footprint 应来自首层 zone（podium）"


def test_02_roof_only_top_floor_metadata(tmp_path):
    """屋顶链路：bim roof 语义（type/slope）保真到 design **顶层** + features.roof；
    IFC 不构建 IfcRoof（build_script_template 未消费 features.roof——记录在案的
    「需人工补」项，roof_pitched 配方在 flows docs）。"""
    design, features, ifc_p = _run_chain("02-multi-zone", tmp_path)
    assert "roof" not in design["floors"]["1F"]
    roof = design["floors"]["5F"]["roof"]
    assert roof["type"] == "hip" and roof["slope_deg"] == 30
    assert features["roof"]["type"] == "hip"
    assert _ifc_counts(ifc_p)["IfcRoof"] == 0, "当前口径：IFC 无屋顶几何（诚实契约）"


def test_03_floors_identical_and_keys_unique(tmp_path):
    """多楼层：同 zone 多层复制同一 DXF——每层几何一致（walls 数相同）且跨层 key
    唯一（确定性 GlobalId = uuid5(NS, key)，key 撞 → IFC UR1 失败）。"""
    design, features, ifc_p = _run_chain("03-multi-storey", tmp_path)
    n1 = len(design["floors"]["1F"]["walls"])
    assert n1 == len(design["floors"]["2F"]["walls"]) == len(design["floors"]["3F"]["walls"])
    keys = [w["key"] for fd in design["floors"].values() for w in fd["walls"]]
    assert len(keys) == len(set(keys)), "跨层 key 不得重复（GlobalId 冲突）"
    # typical：同 typology 多层归组（RESIDENCE: 1F..3F）
    assert design["frame"]["typical"]["RESIDENCE"] == ["1F", "2F", "3F"]


def test_04_arc_wall_semantics(tmp_path):
    """弧墙：design 保真 arc 参数（center/r/a0/a1，mm→m）；design_builder 展开为
    分段折线（~12°/段）→ IFC 每段一个 IfcWall（分段近似，非 IfcTrimmedCurve）。"""
    design, features, ifc_p = _run_chain("04-arc-wall", tmp_path)
    arcs = [w for w in design["floors"]["1F"]["walls"] if "arc" in w]
    assert len(arcs) == 1, "应保真 1 道弧墙（arc 形态）"
    arc = arcs[0]["arc"]
    assert arc["r"] == pytest.approx(5.0, abs=0.01)
    assert arc["center"] == pytest.approx([5.0, 8.0], abs=0.05)
    straight = len(design["floors"]["1F"]["walls"]) - 1
    # IFC 墙数 > design 直墙数（弧段展开）且展开段数 ≥ 180°/12°=15
    n_ifc = _ifc_counts(ifc_p)["IfcWall"]
    assert n_ifc > straight and n_ifc - straight >= 15 - 8, (
        f"弧墙应展开为 ≥15 段折线（短段吸附损耗后仍应显著多于直墙数）: {n_ifc} vs {straight}")


def test_05_openings_positioning(tmp_path):
    """门窗定位链路：宽 w 保真（mm→m）；along 落在**长墙面段**（≥1m，防门扇封口线
    碎片段误挂）且在真实 station ±0.6m 内（宿主为墙面栅格边界而非墙轴，含面起点
    偏移 + 50mm 量化——分米级误差是当前口径）。"""
    design, features, ifc_p = _run_chain("05-openings", tmp_path)
    fd = design["floors"]["1F"]
    wins = sorted(o["along"] for o in fd["openings"] if o["type"] == "window")
    doors = sorted(o["along"] for o in fd["openings"] if o["type"] == "door")
    # 画图 station（中心 = s + w/2）：北窗 3.0/8.0、南窗 6.0；入户门 2.0、东门 4.0、隔墙门 3.5
    for got, want in zip(wins, (3.0, 6.0, 8.0)):
        assert got == pytest.approx(want, abs=0.6), f"窗 along 失真: {got} vs {want}"
    for got, want in zip(doors, (2.0, 3.5, 4.0)):
        assert got == pytest.approx(want, abs=0.6), f"门 along 失真: {got} vs {want}"
    widths = sorted(o["w"] for o in fd["openings"])  # 洞宽保真（mm→m，readback 精确值）
    assert widths == [0.9, 0.9, 1.0, 1.5, 1.5, 1.8]


def test_06_stair_symbols_currently_dropped(tmp_path):
    """楼梯（诚实契约）：DXF 的 STAIR 图层符号（archdxf.stairs 形态）被 readback
    IGNORE_LAYERS 全忽略 → design/features stairs=[]、IFC IfcStair=0。design schema
    本身支持 stairs（at+size），缺在上游喂入与 build 构建——记录为「丢失」不粉饰。"""
    design, features, ifc_p = _run_chain("06-stair-roof", tmp_path)
    for fn, fd in design["floors"].items():
        assert fd["stairs"] == [], "当前口径：STAIR 图层不进 design（丢失项契约）"
    assert features["stairs"] == []
    assert _ifc_counts(ifc_p)["IfcStair"] == 0
    # 同 fixture 的屋顶链路：design 顶层 roof = gable、IFC 无 IfcRoof
    assert design["floors"]["2F"]["roof"]["type"] == "gable"
    assert "roof" not in design["floors"]["1F"]
    assert _ifc_counts(ifc_p)["IfcRoof"] == 0
