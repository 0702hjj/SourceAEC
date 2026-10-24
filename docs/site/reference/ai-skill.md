# SourceAEC 接入 skill

本仓仅提供一个入口：`skills/sourceaec/SKILL.md`，用于让 Agent 正确调用
SourceAEC 工具和 API。`skillsDir` 配置为包含该目录的 `skills/` 根目录。

入口说明平台特有的编辑流程、运行时输入输出和能力边界。IFC、DXF 的少量特殊
约束分别放在 `references/ifc.md`、`references/dxf.md`，任务需要时再读取。
规划只是可选的声明性输入，不再有独立 aiplan skill 或强制规划前置流程。

建模方法和准确 API 由外部 SDK 自己的文档或 skill 提供，包括 SimpleCADAPI。
不在本仓复制 SDK、手册或完整建模工作流。管理员可以将上游声明性 skill
安装在配置的目录中；各 Agent 共享该目录，按任务需要加载。

skill 数量不决定角色数量。主 Agent、IFC/CAD 子 Agent 的任务分工和工具权限
独立配置，不能通过加载参考改变模型创建权限、文件访问范围或沙箱限制。
`skillsDir` 留空可以关闭 skill 接入，Go 编辑工具仍然可用。

本仓不捆绑 hooks、Python SDK、CLI 或自动设计 gate。当前模型产品是 IFC 与
ASCII DXF，不因某个上游 SDK 支持 STEP/装配而自动获得相应能力。
CI 验证接入和执行契约，不能据此宣称自然语言建模质量已与其他方案等价。

[AI 集成](/reference/ai)、[引擎边界](/development/integration-boundaries)。
