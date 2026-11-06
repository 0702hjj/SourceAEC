# Web client

`web/` uses React 19, TypeScript, Vite and zustand. Development uses port 5173; the Go gateway serves `web/dist` in production.
IFC has xeokit and web-ifc paths; DXF uses a Fabric Canvas view.

```bash
cd web
npm ci
npm run dev
npm test
npm run lint
npm run build
```

The development proxy forwards `/api` and `/v1` to port 8090. `api/client.ts` unwraps `{code,message,data}` and requests a token on 401.
`LibraryPage` handles projects/models; `ViewerPage` mounts the matching viewer and shared panels.
The shared Design panel edits PARAMS/source, staging and versions. Source navigation uses GlobalId for IFC and XDATA keys for DXF.
Unmapped or stale locations remain read-only.

xeokit loads XKT/metadata with full review tools. web-ifc reads IFC directly with basic selection/tree/properties.
SSE `viewer.staged` updates DXF/web-ifc immediately and offers a reload for xeokit; committed changes refresh the normal viewer.
Diff coloring uses entity IDs; deleted geometry absent from the current model appears in the diff list.
See [interface](/en/guide/interface) and [previews](/en/architecture/previews).
