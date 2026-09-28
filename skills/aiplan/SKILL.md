---
name: aiplan
description: Normalize user requirements and site constraints into plan.json and bim_supplement.json for downstream CAD and IFC workflows.
version: 0.1.0
license: MIT
compatibility: Self-contained Python CLI using jsonschema and shapely.
metadata:
  project: aiplan
---

# aiplan

Use this skill for the planning stage. It captures requirements, validates
geometry and area constraints, and produces two versionable outputs:

- `plan.json`: the implementation brief consumed by CAD tooling.
- `bim_supplement.json`: roof, structure, and property information consumed
  by BIM tooling.

It does not draw DXF or author IFC directly.

## Workflow

1. Collect the site, setbacks, program, storeys, and unresolved decisions.
2. Confirm the design direction with the user.
3. Write `design_intent.json` using the bundled schema.
4. Run derive, normalize, geometry checks, and the design gate.
5. Validate and land the plan and BIM supplement as a pair.

## Public References

- `references/schemas/`: output contracts.
- `references/examples/`: synthetic valid examples.
- `references/predicate_vocabulary.md`: requirement vocabulary.
- `references/bim_param_defaults.md`: documented defaults.
- `references/question_templates.json`: review prompts.

The public distribution intentionally contains no real-project examples or
case-derived design library. Design decisions must come from the user's
requirements, applicable codes, and explicitly supplied reference material.

## Commands

```text
aiplan validate <plan|bim|intent> <file>
aiplan derive --lot <json-or-path> [--setbacks <json-or-path>]
aiplan normalize --intent <file> --lot <json-or-path>
aiplan geom check --zones <json-or-path>
aiplan gate <plan.json>
aiplan land <plan.json> <bim_supplement.json> --outdir <dir>
aiplan canon <file>
aiplan route <workspace>
```

All JSON arguments accept either an inline value or a file path. Persist
normalized output before validation so failures can be reviewed and retried.

