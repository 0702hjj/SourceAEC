"""fixture 再生成脚本——W-0052 cad→ifc 消化管线实验矩阵（6 形态）输入库。

每形态目录 = building.json（aidxf v2 交付契约形态）+ bim_supplement.json（aiplan v1）
+ 各 zone DXF（dxfkit.draw 产出——与 aidxf 主 agent 画图同一 API，含沿墙门窗/弧墙/
archdxf.stairs 楼梯符号）。DXF 为确定性产物（_AsciiDrawing 字节级确定），重跑逐字节一致。

再生成（cwd 任意）：
    skills/.venv/bin/python skills/aiifc/tests/fixtures/cad/regen.py
（skills/.venv 由 tools/install_skill_venv.sh 建；无该 venv 时可用任何装了
  ezdxf 的 python——脚本自带 dxfkit/archdxf 路径 bootstrap。）
"""

from __future__ import annotations

import json
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parents[4]  # skills/aiifc/tests/fixtures/cad → repo root

# dxfkit/archdxf bootstrap：优先 dist（单一事实源），缺 dist 用 skills/aidxf 源
for pkg in ("archdxf", "dxfkit"):
    for base in (REPO / "skills" / "dist" / "aidxf" / "scripts" / "packages",
                 REPO / "skills" / "aidxf" / "scripts" / "packages"):
        p = base / pkg / "src"
        if p.is_dir():
            sys.path.insert(0, str(p))
            break

from dxfkit import draw  # noqa: E402 —— bootstrap 后导入


def _building(project: str, zones: list[dict]) -> dict:
    """building.json（aidxf v2 形态：site/standards 原样透传 + zones 记 modelId）。"""
    return {
        "version": 2,
        "project": project,
        "site": {"anchor": [0, 0], "setback_m": 3.0},
        "standards": {"code": "GB50016"},
        "zones": zones,
    }


def _bim(project: str, roof: dict | None = None) -> dict:
    """bim_supplement.json（aiplan v1 形态：version/project/source_plan_sha256 必填）。"""
    out = {"version": 1, "project": project,
           "source_plan_sha256": "0" * 64}
    if roof:
        out["roof"] = roof
    return out


def _zone(zid: str, f_from: int, f_to: int, model: str, typology: str) -> dict:
    return {"zone": zid, "floors_from": f_from, "floors_to": f_to,
            "modelId": model, "typology": typology}


def _new_doc():
    draw.reset_keys()
    return draw.new_doc()


def _rect_walls(msp, w: float, h: float, t: float, cuts_s=(), cuts_e=()):
    """四边形外墙（逆时针：南→东→北→西），南/东墙可带开洞 cuts=[(s,wmm)]。"""
    ks = draw.wall_run(msp, (0, 0), (w, 0), t, cuts=list(cuts_s))          # 南
    ke = draw.wall_run(msp, (w, 0), (w, h), t, cuts=list(cuts_e))          # 东
    draw.wall_run(msp, (w, h), (0, h), t, cuts=[])                         # 北
    draw.wall_run(msp, (0, h), (0, 0), t, cuts=[])                         # 西
    return ks, ke


# ---------------------------------------------------------------- 每形态

def gen_01_single_zone(d: Path) -> None:
    """a. 单 zone 标准层（baseline）：10×8m 矩形 + 1 窗（南 s=4100 起）+ 1 门（东 s=3550 起）。

    注：draw.window/door 的 at_or_along 是**沿墙起点距离**（非中心）——与 wall_run
    cuts=[(s, w)] 同一 s 对齐（aidxf 组合规律）。
    """
    msp = _new_doc().modelspace()
    win_s, win_w = 4100, 1800
    dr_s, dr_w = 3550, 900
    ks, ke = _rect_walls(msp, 10000, 8000, 200,
                         cuts_s=[(win_s, win_w)],
                         cuts_e=[(dr_s, dr_w)])
    draw.window(msp, ks, win_s, win_w)
    ok = draw.opening(msp, ke, dr_s, dr_w)
    draw.door(msp, ke, ok, dr_s, dr_w, "in-left")
    _save(msp, d / "tower.dxf")
    _json(d, _building("single_tower", [_zone("tower", 1, 1, "m_0000000000000001", "residence")]),
          _bim("single_tower"))


def gen_02_multi_zone(d: Path) -> None:
    """b. 多 zone：podium（零售 1-2F，12×10m）+ tower（办公 3-5F，8×8m），hip 屋顶。"""
    msp = _new_doc().modelspace()
    ks, _ = _rect_walls(msp, 12000, 10000, 200, cuts_s=[(3250, 1500)])
    draw.window(msp, ks, 3250, 1500)
    _save(msp, d / "podium.dxf")

    msp = _new_doc().modelspace()
    ks, _ = _rect_walls(msp, 8000, 8000, 200, cuts_s=[(3250, 1500)])
    draw.window(msp, ks, 3250, 1500)
    _save(msp, d / "tower.dxf")

    _json(d, _building("multi_zone", [
        _zone("podium", 1, 2, "m_0000000000000002", "retail"),
        _zone("tower", 3, 5, "m_0000000000000003", "office"),
    ]), _bim("multi_zone", {"type": "hip", "slope_deg": 30, "ridge_h_m": 2.6}))


def gen_03_multi_storey(d: Path) -> None:
    """c. 多楼层：单 zone 1-3F 同层平面（10×8 + 内隔墙 + 隔墙门 + 入户门 + 2 窗）。"""
    msp = _new_doc().modelspace()
    ks, ke = _rect_walls(msp, 10000, 8000, 200,
                         cuts_s=[(3200, 1600), (6800, 1600)],
                         cuts_e=[(3550, 900)])
    draw.window(msp, ks, 3200, 1600)
    draw.window(msp, ks, 6800, 1600)
    ok = draw.opening(msp, ke, 3550, 900)
    draw.door(msp, ke, ok, 3550, 900, "in-left")
    kp = draw.wall_run(msp, (0, 5000), (6000, 5000), 100, cuts=[(3050, 900)])
    okp = draw.opening(msp, kp, 3050, 900)
    draw.door(msp, kp, okp, 3050, 900, "in-right")
    _save(msp, d / "std.dxf")
    _json(d, _building("multi_storey", [_zone("std", 1, 3, "m_0000000000000004", "residence")]),
          _bim("multi_storey"))


def gen_04_arc_wall(d: Path) -> None:
    """d. 曲线墙：南/东/西直墙 + 北侧 r=5m 半圆弧墙收口，南墙 1 窗。"""
    doc = _new_doc()
    msp = doc.modelspace()
    ks = draw.wall_run(msp, (0, 0), (10000, 0), 200, cuts=[(4100, 1800)])
    draw.window(msp, ks, 4100, 1800)
    draw.wall_run(msp, (10000, 0), (10000, 8000), 200, cuts=[])
    draw.wall_run(msp, (0, 8000), (0, 0), 200, cuts=[])
    msp.add_arc(center=(5000, 8000), radius=5000, start_angle=0, end_angle=180,
                dxfattribs={"layer": "WALL"})
    _save(msp, d / "curve.dxf")
    _json(d, _building("arc_tower", [_zone("curve", 1, 1, "m_0000000000000005", "residence")]),
          _bim("arc_tower"))


def gen_05_openings(d: Path) -> None:
    """e. 门窗定位：12×8 矩形 + 内隔墙——入户门/隔墙门/3 窗（沿墙 s 各不同）。"""
    msp = _new_doc().modelspace()
    ks, ke = _rect_walls(msp, 12000, 8000, 200,
                         cuts_s=[(1500, 1000), (5100, 1800)],
                         cuts_e=[(3550, 900)])
    ok = draw.opening(msp, ks, 1500, 1000)
    draw.door(msp, ks, ok, 1500, 1000, "in-left")           # 入户门（南 s=1500）
    draw.window(msp, ks, 5100, 1800)                        # 南窗 s=5100
    oke = draw.opening(msp, ke, 3550, 900)
    draw.door(msp, ke, oke, 3550, 900, "in-right")          # 东门 s=3550
    kn = draw.wall_run(msp, (12000, 8000), (0, 8000), 200, cuts=[(3250, 1500), (8250, 1500)])
    draw.window(msp, kn, 3250, 1500)                        # 北窗 s=3250（从东端起算）
    draw.window(msp, kn, 8250, 1500)                        # 北窗 s=8250
    kp = draw.wall_run(msp, (0, 4000), (7000, 4000), 100, cuts=[(3050, 900)])
    okp = draw.opening(msp, kp, 3050, 900)
    draw.door(msp, kp, okp, 3050, 900, "in-right")          # 隔墙门 s=3050
    _save(msp, d / "tower.dxf")
    _json(d, _building("openings", [_zone("tower", 1, 1, "m_0000000000000006", "office")]),
          _bim("openings"))


def gen_06_stair_roof(d: Path) -> None:
    """f. 楼梯 + 斜屋顶：双跑楼梯（archdxf.stairs 符号：上行段/平台/下行段）+ gable 屋顶。

    注：aidxf v3 DXF 为 2D 平面图——斜屋顶几何不在 DXF（语义在 bim_supplement.roof）；
    楼梯以 STAIR 图层平面符号产出（archdxf.stairs 的产出形态）。
    """
    doc = _new_doc()
    msp = doc.modelspace()
    ks, _ = _rect_walls(msp, 10000, 8000, 200, cuts_s=[(3200, 1500), (6300, 1500)])
    draw.window(msp, ks, 3200, 1500)
    draw.window(msp, ks, 6300, 1500)
    # 双跑楼梯：西行段 up（y 1100..4100）+ 平台 + 东行段 dn
    draw.draw_stair(msp, (2000, 2600), (1200, 3000), 1200)   # at=梯段中心
    draw.draw_landing(msp, (2600, 4400), width=2400, depth=600)
    draw.draw_stair(msp, (3200, 2600), (1200, 3000), 1200)
    _save(msp, d / "std.dxf")
    _json(d, _building("stair_roof", [_zone("std", 1, 2, "m_0000000000000007", "residence")]),
          _bim("stair_roof", {"type": "gable", "slope_deg": 30, "ridge_h_m": 2.8,
                              "overhang_m": 0.5}))


# ---------------------------------------------------------------- 落盘

def _save(msp, path: Path) -> None:
    msp.doc.saveas(str(path))
    print(f"  DXF {path.name} ({path.stat().st_size} B)")


def _json(d: Path, building: dict, bim: dict) -> None:
    (d / "building.json").write_text(json.dumps(building, indent=1) + "\n", encoding="utf-8")
    (d / "bim_supplement.json").write_text(json.dumps(bim, indent=1) + "\n", encoding="utf-8")
    print(f"  JSON building/bim_supplement（zones={len(building['zones'])}）")


GENERATORS = {
    "01-single-zone": gen_01_single_zone,
    "02-multi-zone": gen_02_multi_zone,
    "03-multi-storey": gen_03_multi_storey,
    "04-arc-wall": gen_04_arc_wall,
    "05-openings": gen_05_openings,
    "06-stair-roof": gen_06_stair_roof,
}


def main() -> int:
    for name, gen in GENERATORS.items():
        d = HERE / name
        d.mkdir(parents=True, exist_ok=True)
        print(f"[{name}]")
        gen(d)
    print(f"OK: {len(GENERATORS)} 形态再生成 → {HERE}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
