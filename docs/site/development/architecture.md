# 架构

Go 网关是唯一业务入口。`server/internal/editing` 在进程内处理 IFC/DXF 编辑，
通过共享 Go 存储与事务模块管理 PostgreSQL、脚本版本、Git 和产物发布。

```text
浏览器 / Agent / Go MCP → Go 网关 :8090
                          ├─ PostgreSQL
                          ├─ Go 编辑、版本、方案与 Agent
                          ├─ IFC 转换器
                          └─ bubblewrap → 运行时模型源码 → 外部几何库
```

仓库不维护 Python 服务、SDK、CLI、hooks 或测试。Python 解释器与几何包是外部
执行依赖；生成的模型源码是运行时数据，不承载 SourceAEC 的业务规则。

前端使用 React；IFC 查看使用 xeokit/web-ifc，DXF 预览读取 Go 生成的 render.json。
转换器是独立的 Node.js 适配器，许可证要求见仓库 NOTICE。

相关说明：[编辑领域](/development/edit-service)、[隔离](/development/sandbox)、
[模型存储](/development/model-storage)。
