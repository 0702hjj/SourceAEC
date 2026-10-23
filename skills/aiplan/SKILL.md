---
name: aiplan
description: Refine an architectural brief and record declarative planning inputs for SourceAEC projects.
license: MIT
---

# Planning inputs

Record the project's brief, assumptions, units, site constraints and spatial
requirements as declarative data. Use the project's plan APIs to read and
version `plan.json`, `bim_supplement.json` and `building.json`; their `project`
identifier must match the target project.

The Go delivery endpoint validates JSON and project identity, then versions the
plan and BIM supplement together. It does not run a planning solver or certify
regulatory compliance. Separate measured or checked facts from unverified
assumptions. CAD/IFC authoring consumes these inputs through the model script
API and external geometry runtimes.

There is no bundled planning CLI, Python package, hook or automatic design gate.
Do not claim analysis or validation that has not been performed.
