# web — 前端（React 19 + TypeScript + Vite）

平台前端：双引擎 IFC 查看器（xeokit XKT / web-ifc 直读）+ Fabric DXF Canvas + DesignPanel 脚本编辑面板 + AI 对话侧栏（SSE 事件流）。开发端口 :5173（`/api`、`/v1` 代理到 Go server :8090）；生产构建产物 `dist/` 由 Go server 托管（单端口）。

```bash
npm install
npm run dev        # 开发服务器 :5173
npm test           # vitest 单测
npm run build      # tsc -b + vite build（含类型检查）
npm run lint       # oxlint
```

组件结构、双引擎切换与关键机制见文档站 [Web 前端](https://0702hjj.github.io/AI_IFC/development/web)。
