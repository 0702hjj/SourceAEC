# Data flow and transactions

The diagram separates capture, external execution, atomic publication and derived conversion. Participant IDs match the
[architecture overview](/en/architecture/overview).

```mermaid
sequenceDiagram
    participant CLIENT as Web / Agent / MCP
    participant API as Go gateway
    participant CAPTURE as Frozen capture
    participant SANDBOX as Geometry sandbox
    participant PUBLISH as Go publication
    participant PG as PostgreSQL
    participant QUEUE as IFC conversion
    CLIENT->>API: Stage source with baseRevision
    API->>PG: Validate revision / commit staging
    alt Explicit durable run/save
        CLIENT->>API: key / action / baseRevision
        API->>PG: Reserve capture capacity
        API->>CAPTURE: Freeze source, inputs and environment seal
        CAPTURE->>PG: Commit definition, blobs and run together
        API-->>CLIENT: 202 / workflow ID
        PG->>CAPTURE: Worker claim / lease and attempt
    else Synchronous run/save
        CLIENT->>API: run or save
        API->>CAPTURE: Short read-only snapshot
    end
    CAPTURE->>SANDBOX: Close snapshot / pass frozen inputs
    Note over SANDBOX,PG: No SQL connection or model lock during preparation/execution
    SANDBOX-->>PUBLISH: Complete product and source map
    PUBLISH->>PUBLISH: Check format, mapping and structural budgets
    opt Durable geometry completion
        PUBLISH->>PG: Candidate blobs + geometry checkpoint in one transaction
    end
    PUBLISH->>PG: Shared admission / model lock / identity and fence recheck
    PUBLISH->>PG: Products, saved version and publication marker in one transaction
    Note over PUBLISH,PG: Journal protects projections, receipt resolves lost COMMIT acknowledgements
    opt IFC
        PUBLISH->>QUEUE: Committed source and build identity
        QUEUE->>QUEUE: Node conversion in a private directory
        QUEUE->>PG: Recheck identity / publish complete preview generation
    end
    CLIENT->>API: Synchronous result / poll workflow / read preview
```

## Completion points

`run` publishes a product without a version; `save` executes and saves an immutable version.
A durable 202 confirms the frozen definition was accepted, not that geometry/publication/conversion completed.
IFC publication and preview conversion are separate. Conversion recovery does not save another version;
DXF render data is generated directly by Go.

## Read path

```mermaid
flowchart LR
    PG[(Committed PostgreSQL snapshot)] --> EXPORT[Reserve source bytes using snapshot size]
    EXPORT --> SNAP[Private file / unlink after opening]
    SNAP --> RELEASE[Return export budget and database connection]
    RELEASE --> CLIENT[Range / slow client]
    MANIFEST[Preview manifest matching source identity] --> PREVIEW[Open immutable file handles under lock]
    PREVIEW --> CLIENT
```

Export byte reservations end with export; file reservations remain until client reads finish.
Neither is a hard limit on PG memory/temp space or total system RSS.
See [previews](/en/architecture/previews) and [capacity verification](/en/operations/performance).
