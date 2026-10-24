# Go CAD 适配

CAD 业务集成在 Go 编辑领域中，不需要单独的 CAD 服务。
模型源码可以使用外部 ezdxf 或管理员配置的几何运行时，输出 ASCII DXF。

Go 解析 DXF group code，按 AIDXF/SOURCEAEC XDATA key 比较实体。
无 key 的实体按内容匹配；移动会表示为删除和新增。
二进制 DXF 当前不支持，应在外部工具导出为 ASCII DXF。

Go 生成 `render.json` schemaVersion 2，支持 LINE、CIRCLE、ARC、TEXT、MTEXT、
LWPOLYLINE 及一层等比正缩放 INSERT。其余实体在 unsupported 中报告；
预览不等同于完整 CAD 内核或工程图验证。

SimpleCADAPI 可作为外部几何 SDK 与工作流参考；仓库不复制其实现，也不提供
旧本仓画法 DSL 或制图 CLI。具体包和导出 API 以安装的上游版本为准。

业务校验、版本与产物发布由 Go 负责，见[编辑领域](/development/edit-service)。
