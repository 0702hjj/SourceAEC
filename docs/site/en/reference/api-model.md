# Models and review API

Responses use the [REST envelope/authentication](/en/reference/rest-api). Model IDs are `m_` plus 16 lowercase hex characters;
issue IDs are `i_` plus 12 lowercase hex characters. Use [OpenAPI](/go-server.openapi.json) for complete request/response schemas.

## Models

| Method/path | Contract |
| --- | --- |
| POST `/api/v1/models` | Multipart `file`, IFC/DXF, up to 200 MB; IFC queues conversion, DXF starts ready |
| GET `/api/v1/models` | Model list, states converting/ready/failed |
| GET `/api/v1/models/{id}` | Model detail, including previewGeneration after successful conversion |
| POST `/api/v1/models/{id}/retry` | Retry failed conversion; model admission overload returns HTTP 429 / code 42900 |
| DELETE `/api/v1/models/{id}` | Delete model/artifacts/review data and detach project membership |
| GET `/api/v1/models/{id}/download` | Download committed original IFC/DXF with Content-Disposition and Range support |

The browser hides upload; the endpoint remains available. Source changes invalidate old preview identity until matching conversion completes.

## Review issues

GET/POST `/api/v1/models/{id}/issues` lists/creates issues. POST is multipart: required `issue` JSON with title,
optional entityId/entityName/entityType/comment/author/provenance/camera, and optional PNG `screenshot` up to 5 MB.
Default author is local-user and source is UI. PATCH `/issues/{issueId}` updates title/comment/status;
DELETE removes the issue/screenshot. Status values are open/checking/resolved; lists sort newest first.

## Overrides and changes

GET `/overrides` returns `{entityId:{field:value}}`. PUT `/entities/{entityId}/properties` accepts:

```json
{"entityName":"Wall","fields":{"FireRating":"F60","Comments":"Review note"}}
```

Only Name, Description, Classification, FireRating and Comments are allowed. Empty strings clear overrides.
Each field update creates a change entry; GET `/changes` lists newest first. Overrides affect metadata, not the IFC source.
Provenance UI/AI/USER is caller-declared and enum-validated, not proof of authorship.

## Static assets

Raw files do not use JSON envelopes:

| GET path | Content |
| --- | --- |
| `/v1/models/{id}/model.xkt` | Converted geometry; Range supported |
| `/v1/models/{id}/metadata.json` | IFC tree/property sets |
| `/v1/models/{id}/render.json` | DXF render schema |
| `/v1/models/{id}/issues/{file}` | Screenshot; filename must match i_[0-9a-f]{12}\.png |

XKT/metadata can use `?generation=<previewGeneration>` from the same model-detail response.
Stale or nonmatching generations return 404; reread model state and load a complete matching preview.

## metadata.json schema

```json
{"projectId":"project","metaObjects":[{"id":"wall-guid","type":"IfcWall","name":"Wall","parent":"storey-guid","propertySetIds":["pset-wall"]}],"propertySets":[{"id":"pset-wall","name":"Pset_WallCommon","type":"Pset","properties":[{"name":"FireRating","value":"120min","type":"IfcLabel"}]}]}
```

metaObjects IDs match IFC GlobalId/XKT IDs. Hierarchy runs from Site/Building/Storey to elements;
objects without property sets omit propertySetIds.

## render.json schema

```json
{"schemaVersion":2,"bounds":{"min":[0,0],"max":[100,80]},"layers":[{"name":"WALL","color":7,"linetype":"CONTINUOUS"}],"entities":[{"key":"e_wall","type":"LINE","layer":"WALL","start":[0,0],"end":[10,0]}],"unsupported":[{"type":"HATCH","handle":"1F","coords":[5,5]}]}
```

Go derives render data from committed products. Coordinates retain DXF space; bounds may be null.
Keys are XDATA identities. LWPOLYLINE expands to LINE/ARC; INSERT expands one level with null child keys.
Unsupported types are explicitly listed. See [formats](/en/architecture/formats).
