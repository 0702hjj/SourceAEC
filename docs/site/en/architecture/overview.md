# Architecture overview

The Go gateway is the only business entry point. Browsers, the built-in agent and the external MCP bridge share editing contracts.
PostgreSQL stores authoritative state; external Python executes geometry and Node converts IFC display artifacts.

```mermaid
flowchart TB
    WEB[Web / IFC and DXF viewers] --> API[Go gateway / auth / REST / SSE]
    MCP[External Go MCP bridge] --> API
    API --> EDIT[editing / synchronous edits and publication]
    API --> WF[editing workers + workflow / durable phases]
    EDIT --> CAPTURE[Short snapshot / frozen source, inputs and identity]
    WF --> CAPTURE
    CAPTURE --> SANDBOX[bubblewrap / external Python geometry]
    SANDBOX --> CHECK[Go format, source-map and budget checks]
    CHECK --> PUBLISH[Shared model admission / lock / identity recheck]
    PUBLISH --> PG[(PostgreSQL / products, versions and workflows)]
    WF <--> PG
    PUBLISH --> QUEUE[In-process conversion queue]
    QUEUE --> NODE[Node IFC converter]
    NODE --> PREVIEW[Identity check / immutable preview generation]
    PREVIEW --> PG
    API --> READ[Committed snapshots / Range reads]
    PG --> READ
    PREVIEW --> READ
    PG --> FILES[Journal / recoverable workspace projections]
```

## Responsibility map

| Layer | Implementation | Read next |
| --- | --- | --- |
| Entry | `api`, `agent`, `mcpbridge` | [Gateway](/en/architecture/gateway), [Web](/en/architecture/web), [integration](/en/integrations/overview) |
| Model domain | `editing`, `editruntime` | [Editing](/en/architecture/editing), [data flow](/en/architecture/data-flow) |
| Durable work | `workflow` and editing workers | [Execution](/en/architecture/execution), [workflow API](/en/reference/workflows) |
| Authoritative state | `artifact`, `versionstore`, `sourcepatch`, `store` | [Storage](/en/architecture/storage) |
| External execution | bubblewrap, Python, Node | [Sandbox](/en/architecture/sandbox), [formats](/en/architecture/formats), [previews](/en/architecture/previews) |

## Deployment boundary

The supported contract is one gateway, a shared workspace and one database namespace. Process-local queues/budgets are not global
replica quotas. Environment preparation, geometry execution and slow downloads do not retain database connections for their duration.
Publication rechecks source/input/branch/model identity; durable publication also validates the complete ownership fence.

Synchronous requests do not automatically resume. Explicit workflows persist definitions and completed phases; unfinished geometry
may execute again, while committed versions do not advance twice. No Python stack or notebook cell recovery is provided.
Use documentation matching your deployed source revision; upstream SDK capabilities are not automatically SourceAEC capabilities.
