# dxf_to_ifc_geometry.md —— per-zone DXF → design floors（consume-upstream 阶段 ②）

> 契约事实源：`aiifc/consume_upstream.py`（`_fill_from_dxf` / `_nearest_wall` / `_point_to_segment` / `_mm_to_m`）
> + 测试（`test_dxf_walls_and_openings` / `test_dxf_arc_wall` / `test_openings_along_wall_positioning`）。
> 本文件是 `workflows/CONSUME_UPSTREAM.md` 阶段 ② 最核心的解析规则：**per-zone DXF 几何 → design.json
> floors 的 walls / openings / slabs**（下游 build 脚本再映射 IfcWall/IfcWindow/IfcDoor/IfcSlab）。

## 输入：zone DXF（aidxf S4-b 平台模型，`dxf_dir/<zone>.dxf`）

经 `dxfkit.readback`（editable 装进 skills/.venv）解析为结构化几何：

| readback 字段 | 类型 | 说明 |
|---|---|---|
| `wall_segments` | list[((x1,y1),(x2,y2))] | 直墙段（**mm** 坐标） |
| `wall_arcs` | list[{center, radius, start_angle, end_angle}] | 曲线墙（center mm / radius mm / 角度 °） |
| `windows` | list[{at, width_mm}] | 窗（at = 窗位置 mm） |
| `doors` | list[{at, width_mm}] | 门（at = 门位置 mm） |
| `outline_mm` | list[(x,y)] | 轮廓多边形（mm，首层用 footprint） |

## 映射规则（代码实现 `_fill_from_dxf`）

### 直墙段 → floors.walls（axis 折线形态）
```
for i, (x1,y1),(x2,y2) in wall_segments：
  walls.append({
    "axis": [[x1/1000, y1/1000], [x2/1000, y2/1000]],  # mm→m，round 3
    "t": 0.2, "kind": "int", "key": f"wall:{i}",
  })
```
- 墙厚 `t: 0.2m` 固定（当前无墙厚输入）；`kind: "int"` 固定（DXF 层名未区分内外墙——P2 按层名精确）。
- `key` 稳定唯一（`wall:{i}`）——design.json → features → build 脚本 GlobalId 的地基。

### 曲线墙 → floors.walls（arc 形态）
```
for j, arc in wall_arcs：
  walls.append({
    "arc": {"center": [cx/1000, cy/1000], "r": r/1000, "a0": start_angle, "a1": end_angle},
    "t": 0.2, "kind": "int", "key": f"wall_arc:{j}",
  })
```
- 角度原样（°）；design_builder 以 ~12° 弦段近似弧。

### 门窗 → floors.openings（沿墙精确定位）
```
门窗 at（mm）→ 最近墙段（_nearest_wall）：
  d, along = _point_to_segment(at, seg[0], seg[1])   # 距离 + 投影距 seg[0] 长度（mm）
  取距离最小的墙段 i
openings.append({
  "wall": 墙段索引 i, "along": along/1000（m）,
  "w": width_mm/1000（m）,
  "h": 1.5（窗）/ 2.1（门）, "sill": 0.9（窗）/ 0.0（门）,
  "type": "window" | "door", "key": f"opening:win:{k}" | f"opening:door:{k}",
})
```
- **沿墙定位**：`along` = 门窗中心在墙段上的投影距离（m），`wall` = 所在墙段索引（对应 `walls` 顺序）——build 脚本据此在墙 axis 上开洞放门窗。
- 门窗高/窗台**固定**：窗 h=1.5 / sill=0.9；门 h=2.1 / sill=0.0（当前无输入来源，P2 从 DXF 属性/XData 取）。

### 轮廓 → floors.slabs.profile
```
outline_mm → floor.slabs = [{ "profile": [[x/1000, y/1000]...], "t": 0.15, "key": "slab:0" }]
```
- 楼板厚 `t: 0.15m` 固定；`key: "slab:0"`（每层单板）。

## 输出：design.json floors.<storey>

```json
{
  "1F": {
    "walls": [
      {"axis": [[0, 0], [12, 0]], "t": 0.2, "kind": "int", "key": "wall:0"},
      {"arc": {"center": [6, 4], "r": 2.0, "a0": 0, "a1": 180}, "t": 0.2, "kind": "int", "key": "wall_arc:0"}
    ],
    "openings": [
      {"wall": 0, "along": 3.0, "w": 1.5, "h": 1.5, "sill": 0.9, "type": "window", "key": "opening:win:0"},
      {"wall": 1, "along": 1.2, "w": 0.9, "h": 2.1, "sill": 0.0, "type": "door", "key": "opening:door:0"}
    ],
    "slabs": [{"profile": [[0,0],[12,0],[12,8],[0,8]], "t": 0.15, "key": "slab:0"}],
    "stairs": []
  }
}
```

## 边界与纪律

1. **精确几何直用**：DXF 是坐标级精确几何，直接映射 design.json axis/footprint（mm→m），**不降级为近似语义**——这是用户定的几何处理原则。
2. **墙厚/门窗高/窗台固定缺省**：当前无输入来源（0.2m / 1.5m / 0.9m / 2.1m / 0.0m）——P2 从 DXF 层名/XData/属性精确化。
3. **内外墙 `kind` 统一 "int"**：DXF 层名未区分 → 全部内墙；P2 按层名（WALL_EXT/WALL_INT 等）精确。
4. **门窗沿墙锚定**：`wall` 索引 + `along` 投影距离是**确定性定位**（同一 DXF 多次解析结果一致）——跨版本 diff / 编辑回写的地基。
5. **DXF 缺失**（zone 无对应文件）→ 该层 walls/openings 空（下游 design_builder 报错提示，不静默）。
6. **测试锚点**：`test_dxf_walls_and_openings`（直墙/门窗映射）、`test_dxf_arc_wall`（弧墙）、`test_openings_along_wall_positioning`（门窗沿墙定位）。
