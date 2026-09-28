# cad fixture 库 —— W-0052 cad→ifc 消化管线实验矩阵（6 形态）

> aiifc skill 的**持久化上游产物输入库**：每形态目录 = `building.json`（aidxf v2 交付
> 契约形态）+ `bim_supplement.json`（aiplan v1 形态）+ 各 zone DXF（`dxfkit.draw` 产出，
> 与 aidxf 主 agent 画图同一 API——含沿墙门窗、弧墙、archdxf.stairs 楼梯符号）。
> 消费方：`skills/aiifc/tests/test_digestion_matrix.py`（每形态 ≥1 条全链契约测试）。
> 实验结论（保真/丢失/需人工补）：`docs/internal/w0052-digestion-matrix.md`。

## 形态清单

| 目录 | 形态 | building.json zones | DXF | 特征 |
|---|---|---|---|---|
| `01-single-zone` | a. 单 zone 标准层（baseline） | tower 1→1（residence） | tower.dxf（10×8m + 1 窗 + 1 门） | 最小可用输入 |
| `02-multi-zone` | b. 多 zone | podium 1→2（retail）+ tower 3→5（office） | podium.dxf（12×10m）+ tower.dxf（8×8m） | 非重叠 floors_from/to 映射 + hip 屋顶 |
| `03-multi-storey` | c. 多楼层 | std 1→3（residence） | std.dxf（10×8 + 内隔墙 + 隔墙门 + 入户门 + 2 窗） | 同 zone 多层复制同一 DXF |
| `04-arc-wall` | d. 曲线墙 | curve 1→1（residence） | curve.dxf（3 直墙 + r=5m 半圆弧墙 + 1 窗） | 弧墙 arc 形态 |
| `05-openings` | e. 门窗定位 | tower 1→1（office） | tower.dxf（12×8 + 内隔墙 + 3 门 3 窗，沿墙 s 各不同） | 沿墙定位链路 |
| `06-stair-roof` | f. 楼梯 + 斜屋顶 | std 1→2（residence） | std.dxf（10×8 + 双跑楼梯符号 + 2 窗） | archdxf.stairs 产出形态 + gable 屋顶 |

## 再生成（确定性）

DXF 经 `dxfkit.new_doc()`（`_AsciiDrawing`，字节级确定）产出，重跑逐字节一致：

```bash
skills/.venv/bin/python skills/aiifc/tests/fixtures/cad/regen.py
```

- `skills/.venv` 由 `bash tools/install_skill_venv.sh` 建（含 ezdxf + dxfkit/archdxf
  editable）；脚本自带路径 bootstrap（dist 优先，无 dist 用 `skills/aidxf` 源），任何
  装了 ezdxf 的 python 都能跑。
- JSON（building/bim_supplement）由 regen.py 直接写出，与 DXF 同源再生成。
- 门窗画法口径：`draw.window/door` 的 `at_or_along` 是**沿墙起点距离**（非中心），
  与 `wall_run(cuts=[(s, w)])` 同一 s 对齐（aidxf 组合规律，见
  `skills/aidxf/references/draw_composition.md`）。

## 约束

- 楼梯形态注记：aidxf v3 DXF 是 2D 平面图——**斜屋顶几何不在 DXF**（语义在
  `bim_supplement.roof`），楼梯以 STAIR 图层平面符号产出（archdxf.stairs 形态）。
  这是当前上游真实口径，非 fixture 简化。
- fixture DXF/JSON 不进行手工编辑——改形态一律改 `regen.py` 再生成（防漂移）。
