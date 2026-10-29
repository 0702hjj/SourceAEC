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

## 模块职责

| 模块 | 责任 |
| --- | --- |
| api / agent / mcpbridge | 鉴权、协议与应用入口；MCP 经认证网关访问 |
| editing | IFC/DXF 共享的编辑、执行和发布生命周期 |
| editruntime | 事务、版本/Git 与产物读写；保留私有 stdio 兼容入口 |
| versionstore / sourcepatch | 不可变源码历史引用、head/计数，以及精确 revision 的文本修改 |
| artifact | PostgreSQL 权威内容、模型锁与文件投影恢复 |
| store / convert | 模型记录、不可变预览清单与有界转换队列 |
| 外部 Python / Node | 隔离几何执行与显示格式转换 |

## 数据与运行边界

读操作使用数据库快照；长时间几何执行不占用数据库连接。结果发布重新获取模型锁
并验证执行身份，过期结果返回冲突。当前部署以单个网关和共享工作区为边界，
进程内预算不提供多副本全局限制，运行中的脚本也没有持久续跑契约。

按问题查阅：[编辑领域](/development/edit-service)、[执行与恢复](/development/execution-lifecycle)、
[模型存储](/development/model-storage)、[预览与读取](/development/preview-storage)、
[隔离](/development/sandbox)、[性能验证](/development/performance)。
