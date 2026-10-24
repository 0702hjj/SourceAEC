# SourceAEC

SourceAEC 是开源、自托管、面向 Agent 的 IFC/CAD 操作接口。
Go 实现全部业务：编辑、校验、状态、沙箱编排、版本、持久化、方案和 MCP。
人和 Agent 共享 script-as-source API。

模型源码是运行时输入，可使用外部 IfcOpenShell、ezdxf 或其他配置好的几何库。
Python 不承担平台业务；仓库没有 Python 应用、SDK、CLI、hooks 或测试源码。

浏览器提供模型库、IFC 属性与三维查看、DXF 二维预览、版本及 AI 对话。
几何产物、规划声明和实际验证结果分开记录，执行成功不等于设计合规。

[快速开始](/guide/quickstart)、[编辑与版本](/guide/editing)、
[架构](/development/architecture)。
