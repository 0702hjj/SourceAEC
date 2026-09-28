---
name: aiifc
description: Thin IfcOpenShell reference skill for IFC model authoring via ifcopenshell.api, including spatial structure, geometry, openings, materials, psets and design-review workflows.
version: 0.1.1
license: Apache-2.0
compatibility: Documentation/reference bundle for ifcopenshell API surfaces.
metadata:
  project: aiifc
  package-name: ifcopenshell
---

# AI_IFC SDK Skill

## Philosophy

- Thin reference skill: docs only, no SDK source code bundled.
- Agent reads these docs, then writes ifcopenshell.api code directly (not MCP tool calls).
- Python runtime: needs `ifcopenshell` + `ifcquery` + `numpy` (see `requirements.txt`; PyPI releases, no local source). Use whichever Python has them installed — system `python3` often lacks ifcopenshell.

## MUST Requirements

**Reading:**
1. Read `references/SDK_OVERVIEW.md` and `references/docs/api/README.md` before choosing usecases.
2. Read the exact usecase Markdown page (`docs/api/<pkg>.<usecase>.md`) for every usecase you call.
3. Read `references/MODELING_WORKFLOWS.md` before building your first model.
4. **Read `references/SPATIAL_QUALITY.md` BEFORE framing any design JSON** — footprint articulation (CP-05), door swing clearance (GI-08), stair continuity (GI-09) must be internalized up front so the design JSON is born compliant. Also read `references/DESIGN_PATTERNS.md` before choosing massing/facade/spatial organization.
5. **Component recipes — consult before building any element**: check `references/docs/design/` for a matching recipe. The index is in `docs/design/README.md`.

> **Two IFC authoring paths (2026-08-21)** — this skill declares two paths; **path selection is dictated by the calling agent (orchestrator) per pipeline** (this skill does NOT choose):
> - **`workflows/CONSUME_UPSTREAM.md`** (cad->ifc, consume upstream): consume upstream artifacts (`bim_supplement.json` + `building.json` + per-zone DXF model IDs) → parse → IFC build script. Does NOT produce design.json.
> - **`workflows/PLAN_IFC.md`** (ifc standalone, design.json first): optional design.json draft to frame design intent for confirmation → script → IFC.
> Both paths share the **script-as-source main chain** (build script contract MUST #25-31 + script_lib + sandbox build → IFC) — the pre-authoring intake branches, the main chain is identical. Simple direct builds (`simple wall/slab` MUST #19) still skip the workflow entirely.

**API usage:**
6. All API calls use **keyword arguments**; the first positional argument is always `model`.
7. Use `ifcopenshell.api.run("<package>.<usecase>", model, **kwargs)`.
8. Follow the documented API signatures exactly — do not invent parameters.

**Skeleton:**
9. **Skeleton first**: build Project→Site→Building→Storey spatial tree before any element or geometry.
10. **Every element needs a container**: always `spatial.assign_container`. No orphan elements.

**Geometry:**
11. **If a product has geometry, it needs placement**: `geometry.edit_object_placement` for every product with a body representation.
12. **`create_2pt_wall` returns the representation but does NOT assign it** — always `geometry.assign_representation` separately.

**Opening:**
13. **Set world coordinates for openings before `feature.add_feature`** (it converts world → relative automatically). Never set "wall-local" coordinates manually.
14. **Opening thickness must exceed wall thickness** (≥ 1.5×) to fully penetrate.

**Normativity:**
15. **Pset applicability**: before `pset.add_pset`, verify via `PsetQto.get_applicable_names()` (see PSET_REFERENCE.md).
16. **PredefinedType enum**: verify the value is in the entity's allowed enum list (see ENTITY_SPECS.md).
17. **Pset + material coverage**: every product class must get ≥1 pset AND ≥1 material; spot-check with `util.element.get_psets(e)` / `get_material(e)`.

**Generation (script is the single source of truth):**
18. **Complex buildings**: MAY be framed with a design JSON **draft** first (auxiliary planning info, NOT a complete representation, NOT the deliverable) — LLM outputs parametric intent, never coordinate math. See `references/DESIGN_JSON_SCHEMA.md`. The deliverable is always a **build script** conforming to the script contract below (#25-31); `flows/design_builder.py` → `features.json` → `flows/build_script_template.py` is one way to get there.
19. **Simple single wall/slab**: may still be coded directly (Pipeline Stages).

**Validation (three layers):**
20. **During**: `tracker.snapshot()` after each step changing geometry or placement.
21. **After**: run `flows/design_review.py` once per build as a **fixed black-box check** — read its text report. If errors, fix the model against `references/SPATIAL_QUALITY.md`.
22. **Export**: `model.write()` is followed by `ifcopenshell.validate`. No model is complete without passing schema validation.

**平台 CLI 调用（host execute 环境）**：本 skill 的 flows 在**平台沙箱/execute 下经 `aiifc` 命令调用**（禁止直接跑 `python xxx.py`——execute 命令白名单只放行 skill CLI）。对应关系：
- `flows/design_builder.py` → `aiifc design-build <design.json> --project-id <pid>`
- `flows/build_script_template.py` → `aiifc build-script <features.json> --project-id <pid>`
- `flows/design_review.py`（#21 强制检查）→ `aiifc design-review <model.ifc> [building_type]`
- `flows/ifc_inspect.py`（#24 强制验证）→ `aiifc ifc-inspect <model.ifc> [--storey] [--cls] [--ids]`
- `aiifc consume-upstream --building <b.json> --bim <bim.json> --dxf-dir <dir> --project-id <pid>`（cad→ifc 消费上游）

`--project-id` 让中间产物（design.json/features.json/演示 IFC）结构性落 `skill-work/{pid}/`（不版本化）；构建脚本 + IFC 版本化走平台 script-as-source（stage/run/save → `models/{modelId}/`），不经此 CLI。

**Output:**
23. **Output paths (generic)**: generated IFC → working directory as **`model.ifc`** (write via staging copy, self-check, then atomic replace). If your host provides a specific output contract, follow that instead.
24. **No visualization rendering**: do NOT call `ifc_plot` / `ifc_render` / any image output. All verification is **text-only**: `ifcopenshell.validate` + `design_review.py` + `ifc_inspect.py`.

**Script contract (script-as-source — the build script is the single source of truth for the IFC):**
25. **PARAMS block**: every build script MUST declare a top-level `PARAMS = {...}` **literal dict (JSON-compatible)** holding all tunable parameters. Host UIs render the parameter form from PARAMS, so nothing tunable may live outside it.
26. **Deterministic identity**: every element's GlobalId MUST come from `script_lib.deterministic_guid(key)` with a stable, unique key `{storey}:{kind}:{n}`. `script_lib.create_entity` derives the GlobalId and writes `Pset_AIIFC.designKey` automatically — only entities created via raw `root.create_entity` need an explicit `script_lib.attach_design_key`, and that explicit key MUST equal the creation key (mismatch = designKey/GlobalId drift, breaks the locate chain). Same script + same PARAMS → same GlobalIds across runs and versions.
27. **Entry point**: the script MUST expose `build(params, out_path)`; the `__main__` guard calls `build(PARAMS, out)`. Verify compliance statically with `script_lib.validate_script_contract(path)`.
28. **Incremental edits, never rewrites**: modifying an existing model = **incrementally editing its existing script** (PARAMS first, then the minimal geometry logic). NEVER regenerate/rewrite the script from scratch — keep the script diff readable, it is the AI's context for the next edit.
29. **Validate exit**: script output MUST go through `script_lib.write_and_validate(model, out_path)` (model.write + ifcopenshell.validate). No script run is complete without passing schema validation.
30. **C-locate（创建点可定位）**：审查可见构件必须经 `script_lib.create_entity(...)` 创建；禁止绕过工厂直接 `root.create_entity`——工厂记录调用点供 web 端「选中构件→定位脚本」。
31. **C-scalar（web 可编辑参数为标量）**：需要 web 端编辑的参数必须是标量字面量或 `params` 引用；表达式参数只能手改脚本（edit-call 拒绝）。

## Entity Mental Model

Every element needs four components:

| Component | Usecase | What it does |
|---|---|---|
| **Entity** | `root.create_entity` | Creates the IFC entity |
| **Placement** | `geometry.edit_object_placement` | Position/rotation |
| **Representation** | `geometry.add_*_representation` + `assign_representation` | Body geometry |
| **Container** | `spatial.assign_container` | Places into a storey |

Optional: **Type** (`type.assign_type`), **Material** (`material.assign_material`), **Style/color** (`style_color.py`), **Pset/Qto** (`pset.add_pset`/`add_qto`), **Opening** (`feature.add_feature`), **Filling** (`feature.add_filling`).

## References (Reading Map)

| Doc | Role |
|---|---|
| `references/SDK_OVERVIEW.md` | Package map, entity-relationship model, typical patterns |
| `references/MODELING_WORKFLOWS.md` | Pipeline stages, placement/opening/grounding discipline, **Design JSON 框定**, ifc_inspect |
| `references/DESIGN_JSON_SCHEMA.md` | Design JSON format contract (wall forms, normalization rules, full example) |
| `references/DESIGN_PATTERNS.md` | Design patterns: massing, facade, structure, circulation, spatial |
| `references/ENTITY_SPECS.md` | Entity attribute tables (REQ/OPT/type/source) |
| `references/PSET_REFERENCE.md` | Applicable Pset/Qto per element type |
| `references/SPATIAL_QUALITY.md` | Design review rules (SS/GI/PR/RH/MC/CP/FD/SQ) |
| `references/docs/api/README.md` + `<pkg>.<usecase>.md` | API index (14 categories, 103 usecases) + exact signatures |
| `references/docs/flows/README.md` + `*.py` | Runnable per-stage code and tools (tracker / design_review / ifc_inspect / **script_lib**) |
| `references/docs/design/README.md` + recipes | Component building recipes (stairs, roof, windows, parapet, balcony) |
| `templates/build_skeleton.py` | Minimal complete model (skeleton + wall + slab) — copy and edit |
| `workflows/PLAN_IFC.md` | **工作流编排**：plan（可选草稿）→ script（事实源）→ IFC 的阶段顺序与跳步规则（辅助设计师） |
