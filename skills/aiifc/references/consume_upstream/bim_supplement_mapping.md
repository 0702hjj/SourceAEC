# bim_supplement_mapping.md —— bim_supplement.json → IFC 建模映射（consume-upstream 阶段 ②）

> 契约事实源：`aiifc/consume_upstream.py`（`_roof_from_bim` / `consume_upstream`）+ `test_consume_upstream.py::test_roof_from_bim`。
> 本文件是 `workflows/CONSUME_UPSTREAM.md` 阶段 ② 的解析规则之一：**bim_supplement.json（屋顶/特殊结构/PSET）→ design.json 的 roof 字段**（下游 build 脚本再映射 IfcRoof/特殊构件/Psets）。

## 输入：bim_supplement.json（aiplan `deliver_plan` 产物）

schema 事实源：`skills/aiplan/references/schemas/bim_supplement.schema.json`。

| 字段 | 类型 | 说明 |
|---|---|---|
| `roof` | object | 屋顶定义（**consume-upstream 当前唯一消费的字段**） |
| `roof.type` | enum | `gable` \| `hip` \| `shed` \| `flat` \| `freeform` |
| `roof.slope_deg` | number | 坡度（25~45°） |
| `roof.ridge_h_m` | number | 屋脊高（m，1.5~4.0） |
| `roof.overhang_m` | number | 挑檐（m，0.3~0.6） |
| `roof.dormer_count` | integer | 老虎窗数 |
| （其它） | — | 特殊结构 / PSET 定义（当前 `consume_upstream` 不消费；P2 细化） |

## 映射规则（代码实现 `_roof_from_bim`）

```
bim_supplement.roof（若存在）→ design.json floors.<顶层>.roof（**原样透传，不转换**）
```

- 仅**顶层**（`_floor_from_zone` 对每个楼层调用，roof 只在 bim.roof 存在时注入——实现上对每层都注入，调用方 `_floors` 逐 zone 逐层；**语义 = 整楼屋顶挂在各层，实际 build 脚本取顶层**，见「边界」）。
- roof **非空**（`isinstance(bim, dict)` 且 `bim.get("roof")` 有值）→ 注入；否则无 roof 字段。
- **无转换/无验证**：`consume_upstream` 直接把 bim.roof dict 透传进 design.json。schema 校验由 aiplan 侧保证（`aiplan validate` / `aiplan gate`）。

## 输出：design.json floors.<storey>.roof

```json
{
  "1F": {
    "walls": [...],
    "roof": {
      "type": "gable",
      "slope_deg": 30,
      "ridge_h_m": 2.5,
      "overhang_m": 0.4
    }
  }
}
```

下游 `aiifc design-build`（`design_builder.py`）把 roof 规范化进 features.json → `build-script` 产 IfcRoof/斜屋面。

## 边界与纪律

1. **不消费 plan.json**：bim_supplement 是唯一 BIM 补充来源；plan.json 只喂 cad（CONSUME_UPSTREAM.md 阶段 ① MUST）。
2. **roof 只认 `bim.roof` 键**：当前实现只映射屋顶；特殊结构 / PSET 的映射是 P2（`_roof_from_bim` 注释「P2 细化映射」）。
3. **透传而非重造**：LLM 不得自行「设计」屋顶——bim_supplement 已含完整设计意图（CAD 覆盖不了的 BIM 补充），consume-upstream 原样消费，不做设计决策。
4. **顶层语义**：若楼层 >1，roof 应只出现在最高层（`build_script_template` 侧按 storeys 最高层取）；当前 `consume_upstream` 对每层都注入是简化为「整楼一种屋顶」，P2 按 floors_from/to 精确化。
5. **测试锚点**：`test_roof_from_bim`——构造 bim.roof → 断言注入 design.json 顶层 roof 透传。
