# Web 前端

`web/` 是 React 19 + TypeScript + Vite + zustand 的单页应用。IFC 查看用 xeokit 和 web-ifc 双引擎，DXF 查看用 Fabric Canvas。开发端口 5173，生产构建产物 `web/dist` 由 Go server 托管。界面功能本身见[界面使用](/guide/interface)。

## 命令

```bash
cd web
npm install
npm run dev        # 开发服务器，/api 与 /v1 代理到 :8090
npm test           # vitest 单测
npm run build      # tsc -b + vite build（含类型检查）
npm run lint       # oxlint
```

## 目录与组件树

```
src/
├── App.tsx                 路由：/ → LibraryPage，/view/:id → ViewerPage
├── api/client.ts           request<T> 解包 {code,message,data}；401 弹 TokenPrompt 自动重试
├── pages/LibraryPage.tsx   项目创建（kind 选择）+ 历史项目（会话）+ 模型列表（状态轮询/重试/删除/下载）
├── pages/ViewerPage.tsx    按模型 kind/引擎挂查看器 + Toolbar + 各面板；模型状态轮询
├── viewer/                 xeokit 分支 + 共享面板
│   ├── ModelTreePanel.tsx  空间树（搜索/类型过滤/显隐，默认展开 1 层）
│   ├── PropertyPanel.tsx   pset 只读展示（历史 override 带标记）+「定位脚本」按钮
│   ├── DesignPanel.tsx     PARAMS 表单（ast 提取）+ 脚本编辑器 + 版本/暂存 diff 视图（三分支共用）
│   ├── IssuePanel.tsx      Issues / 修改历史双 tab；新建 Issue（相机 + 截图）
│   ├── IssuePins.tsx       3D HTML 钉 overlay（每帧投影同步，点击定位）
│   ├── DiffPanel.tsx       版本选择 → diff 着色 + old→new 列表 + 点击定位
│   ├── ChatSidebar.tsx     AI 对话侧栏（SSE 事件流，子 agent 分组，HITL 提问）
│   ├── Toolbar.tsx / SectionControl.tsx / ...
│   └── store.ts            zustand：selectedId/tool/hiddenIds/overrides/scriptJump/viewerEngine/...
├── ifcviewer/              web-ifc 引擎分支（wasm 直读 IFC → three 场景，动态 import 独立分包）
└── dxfviewer/              DXF Canvas 分支（render.json → Fabric 2D 画布，选中取 XDATA key）
```

## IFC 双引擎

IFC 模型有两条渲染链路，用右上角开关切换，选择记在 localStorage 的 `viewerEngine`，默认 xeokit。两条链路共用同一份 store，选中状态联动：

- **xeokit**：加载预转换的 `model.xkt` 与 `metadata.json`，工具链最全——模型树、剖切、测量、Issue 钉、Diff 着色都在这条链路上。代价是要等转换完成。
- **web-ifc**：浏览器内 wasm 直读 IFC 原件，不经转换器，不依赖转换完成。提供渲染、轨道控制、空间树、属性、选中高亮的基本集，独立分包按需加载，没有高级工具。

两条链路的选中语义对齐，构件 id 都是 IFC GlobalId 级。

## 关键机制

- **DXF 画布**：拉取 `render.json` 渲染 2D 图纸，schema 见[模型与审查 API](/reference/api-model#render-json-schema)。选中带 XDATA key 的实体可以定位脚本，查的是 key 不是 guid。
- **定位脚本**：命中后写入 `store.scriptJump`，DesignPanel 切到脚本编辑器并高亮对应行。查不到、信息过期或接口失败时降级为只读提示。xeokit、web-ifc、DXF 三条分支共用这条链路。
- **中途预览**：收到 SSE 的 `viewer.staged` 事件后，DXF 和 web-ifc 分支直接刷新画布；xeokit 分支因为重转慢，改为角标提示，点击才重载。
- **Diff 着色**：diff 返回的 guid 就是场景对象 id，直接调 `entity.colorize` 上色；已删除的构件在当前 XKT 里没有几何，只在列表里呈现。
- **自动刷新**：ViewerPage 持续轮询模型状态，从 converting 变为 ready 时重挂查看器。脚本 run、save、rollback 触发的重转也能被捕获。
