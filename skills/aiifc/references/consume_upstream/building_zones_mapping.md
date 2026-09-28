# building_zones_mapping.md —— building.json zones → design frame（consume-upstream 阶段 ②）

> 契约事实源：`aiifc/consume_upstream.py`（`_meta` / `_frame` / `_typical` / `_footprint_from_dxf`）+ 测试
> （`test_storeys_from_zones` / `test_typical_from_typology` / `test_meta_from_project` / `test_footprint_from_dxf`）。
> 本文件是 `workflows/CONSUME_UPSTREAM.md` 阶段 ② 的解析规则之一：**building.json（plan 形态整栋楼）→
> design.json 的 meta + frame{storeys, typical, footprint}**（楼层/标准层/首层轮廓）。

## 输入：building.json（aidxf `deliver_building` 产物）

| 字段 | 类型 | 说明 |
|---|---|---|
| `project` | string | 项目名（→ `design.meta.name`） |
| `site` | object | 场地（当前 consume_upstream 不消费） |
| `standards` | object | 设计标准（当前不消费） |
| `zones[]` | array | 分区（**核心输入**） |
| `zones[].zone` | string | 分区名（→ DXF 定位 `dxf_dir/<zone>.dxf`） |
| `zones[].floors_from` | int | 起始层号（1 起） |
| `zones[].floors_to` | int | 结束层号（默认 = floors_from） |
| `zones[].modelId` | string | 该 zone 的 DXF 平台模型 modelId（→ 经平台模型定位 DXF） |
| `zones[].typology` | string | 户型/功能类型（→ 标准层分组） |
| `zones[].note/area` | — | 设计说明/面积（当前不消费） |

## 映射规则（代码实现）

### meta（`_meta`）
```
building.project → design.meta.name（缺省 "building"）
design.meta = { units: "m", modulus: 0.1, name: <project> }
```
单位恒为米（`units: "m"`），模数 0.1m（design_builder 吸附网格）。

### frame.storeys（`_frame` 逐 zone floors_from/to）
```
对每个 zone，f ∈ [floors_from, floors_to]：
  name = "{f}F"
  storeys[name] = (f - 1) * 3.0   # 层高缺省 3.0m（从 0 起递增）
```
- 层高**固定 3.0m 缺省**（当前无层高输入来源；bim_supplement/standards 若有层高字段为 P2 消费）。
- 楼层名 `{n}F`（1F/2F/...）——与 DESIGN_JSON_SCHEMA 的 `storeys` 键一致。
- zones 空 → 兜底 `{"1F": 0.0}`。

### frame.typical（`_typical`）
```
对每个 zone：typ = zones[].typology（缺省 zones[].zone，再缺省 "STD"）
  若 floors_to > floors_from（多层）：typical[typ.upper()] += ["{f}F" for f in [from,to]]
```
- **标准层** = 多层 zone（floors_to > floors_from）→ 归到 `frame.typical.<TYPOLOGY>`（大写键）。
- 单层 zone 不产生 typical（典型 = 需复制的多层楼）。

### frame.footprint（`_footprint_from_dxf` 首层 outline）
```
首层 zone = floors_from 最小的 zone
DXF 定位：dxf_dir/<zone>.dxf（缺省 dxf_dir/floor.dxf）
readback(dxf).outline_mm（mm 多边形）→ [ [x/1000, y/1000] ... ]（m，round 3 位）
```
- **精确几何直用**：DXF outline（mm）→ footprint（m），**不降级近似**。
- 首层 zone 的 DXF 缺失 / readback 失败 → footprint 空（design_builder 会报「footprint 缺失」——P2 经 `zones[].modelId` 精确对 DXF 后必达）。

## 输出：design.json frame

```json
{
  "meta":   {"units": "m", "modulus": 0.1, "name": "office"},
  "frame":  {
    "storeys":  {"1F": 0.0, "2F": 3.0, "3F": 6.0},
    "typical":  {"OFFICE": ["2F", "3F"]},
    "footprint": [[0, 0], [12, 0], [12, 8], [0, 8]]
  },
  "floors": { ... }
}
```

## 边界与纪律

1. **层高固定 3.0m**（当前契约）：consume-upstream 无层高输入；若需要真实层高，上游需在 building/bim 提供（P2）。
2. **楼层名 `{n}F` 统一**：与 design.json 协议一致，禁止其它命名（如 Level 1）。
3. **zone → DXF 定位**：当前 `dxf_dir/<zone>.dxf`（或 floor.dxf）；**P2 经 `zones[].modelId` → 平台模型 DXF**（`stage_upstream_to_workdir` 已按 modelId 复制到工作区，见该工具）。
4. **只消费 zones**：site/standards/area/note 当前不映射（P2 按需补）。
5. **测试锚点**：`test_storeys_from_zones`（floors_from/to → 层表+层高）、`test_typical_from_typology`（多层→typical）、`test_meta_from_project`（project→name）、`test_footprint_from_dxf`（首层 outline→footprint m）。
