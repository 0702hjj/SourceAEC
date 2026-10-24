---
name: sourceaec
description: Connect an agent to SourceAEC project and model APIs for inspecting, staging, running and versioning IFC or DXF changes. Use for SourceAEC integration, not general CAD modeling instruction.
license: Apache-2.0
---

# SourceAEC integration

Use the provided Go domain tools, or the authenticated Go gateway REST API.
Operate on the project's existing modelId. Read current source and revision,
submit an incremental edit with baseRevision, run it, inspect the generated
product, then save the accepted version. A run updates the working product;
a save creates an immutable version. Resolve revision conflicts by rereading
current source. Do not write directly into persistence directories.

Model source is runtime data. It declares exactly one top-level literal PARAMS
dictionary and receives the output filename as its first process argument.
The supported model products are IFC and ASCII DXF. Go owns validation,
execution isolation, state, versions and publication; the skill has no hooks,
SDK, helper CLI or executable code.

Read only the relevant format reference:

- [IFC](references/ifc.md): units, spatial ownership, stable identity and mapping.
- [DXF](references/dxf.md): drawing units, layers, stable keys and preview limits.

Planning is optional declarative input. Use project APIs to read or version
plan.json, bim_supplement.json and building.json when the task needs them;
their project identity must match the target project. Delivery stores related
inputs; it does not run a design solver or establish regulatory compliance.

Use the installed external SDK's own documentation or skill for modeling
methods and exact APIs, including SimpleCADAPI when configured. Do not copy its
SDK or manuals into SourceAEC. An SDK's capability does not imply that
SourceAEC supports all its formats or workflows; unsupported products need an
explicit integration. This skill does not prescribe an Agent role hierarchy.
