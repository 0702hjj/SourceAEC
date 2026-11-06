# IFC / DXF 格式适配

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

业务校验、版本与产物发布由 Go 负责，见[编辑领域](/architecture/editing)。

## IFC 检查与格式预算

IFC 检查使用 Go STEP 遍历与规范化，按 GlobalId 匹配属性和引用；不调用完整几何内核进行合规判定。
DXF 使用组码解析、稳定 XDATA key 与受限块展开。超限时拒绝完整候选，不返回部分成功结果。
[执行资源边界](/architecture/execution#资源边界)列出输入、记录、字段、深度、引用与输出预算。

STEP 字符串外的注释按空白处理。DXF 无效/非有限浮点组码及变换、圆弧或边界计算溢出返回 422，不发布部分或空 render JSON。
