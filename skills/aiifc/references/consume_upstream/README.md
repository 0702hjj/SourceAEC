# consume_upstream/ —— cad->ifc consume-upstream parse references

> 本目录支撑 `workflows/CONSUME_UPSTREAM.md` 阶段 ②（parse & consume）——把
> bim_supplement / building.json / DXF 解析为 design.json（DESIGN_JSON_SCHEMA 协议）的
> 解析规则与映射表。**契约事实源 = `aiifc/consume_upstream.py` + 测试**，本文档从实现提炼。

## Mapping 文档（已补全，2026-08-23）

| 文件 | 内容 | 事实源 |
|---|---|---|
| `bim_supplement_mapping.md` | bim_supplement.json（屋顶/特殊结构/PSET）→ design.json roof（透传） | `_roof_from_bim` + `test_roof_from_bim` |
| `building_zones_mapping.md` | building.json zones/storeys/spatial → design frame（storeys/typical/footprint） | `_meta`/`_frame`/`_typical`/`_footprint_from_dxf` + 4 测试 |
| `dxf_to_ifc_geometry.md` | per-zone DXF 几何 → floors walls/openings/slabs（精确几何直用 mm→m） | `_fill_from_dxf`/`_nearest_wall` + 4 测试 |

## 输入锚点（来自 workflows/CONSUME_UPSTREAM.md）

- `bim_supplement.json`（aiplan `deliver_plan`）：屋顶/特殊结构/PSET
- `building.json`（aidxf `deliver_building`）：site/standards/zones[]（floors_from/to + modelId + typology）
- per-zone DXF（aidxf S4-b `init_model` 平台模型）：outline/core/walls/rooms/openings

## 边界

- 本目录只放 **parse references**（规则/映射/纪律）——解析由 `consume_upstream` 代码执行。
- script-as-source 契约（MUST #25-31）与 script_lib 在 `SKILL.md` + `references/docs/flows/`。

## 已实现 vs P2

- ✅ 已实现：roof 透传、storeys/typical/footprint、DXF 直墙/弧墙/门窗沿墙/轮廓板
- 🔧 P2：内外墙 kind 按层名、墙厚/门窗高/窗台从 DXF 属性取、zone→DXF 经 `zones[].modelId` 精确对、
  特殊结构/PSET 映射、层高输入来源
