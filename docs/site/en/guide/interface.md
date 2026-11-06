# Interface and review

The library lists projects and models; the viewer opens IFC or DXF models. Project deletion also removes its sessions,
planning artifacts and models. Model states are `converting`, `ready` and `failed`; failed conversion can be retried.
The browser hides direct upload; the REST upload contract remains available.

## IFC viewers

The xeokit path loads converted XKT and supports the model tree, properties, sections, measurement, review pins and diff colors.
The web-ifc path reads the original IFC in the browser and offers basic geometry, selection, tree and properties.
Both use IFC GlobalId for entity identity. Choosing web-ifc does not remove xeokit licensing obligations from the standard build.

## DXF and source navigation

The 2D canvas consumes Go-generated `render.json`, preserving drawing coordinates, layers and line types.
Entities with stable keys can locate their script source. The shared Design panel offers parameters, code, staging and versions.
Properties displayed in the viewer do not directly rewrite IFC geometry.

## Review and chat

Create a review issue with an entity, comment, camera and optional screenshot. Issues move through `open`, `checking` and `resolved`.
Pins restore the saved view. Change history records field-level old/new values.
Chat streams text and tool cards, asks questions and announces intermediate/committed model updates.
DXF and web-ifc refresh intermediate results directly; xeokit offers an explicit preview reload.

See [editing](/en/guide/editing), [model API](/en/reference/api-model) and [chat/SSE](/en/reference/api-chat).
