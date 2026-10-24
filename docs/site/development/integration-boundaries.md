# 引擎与集成边界

SourceAEC 负责项目与模型业务：编辑 API、状态、校验、执行编排、版本、Git、
持久化、Agent 和 MCP。这些业务全面由 Go 实现，不维护 Python 应用代码。

外部几何引擎负责几何操作和格式导出。Python 源码由用户或 Agent 生成，
作为运行时模型输入进入沙箱；不能承载 SourceAEC 的状态、发布或版本规则。

SimpleCADAPI 的独立 SDK 和声明性工作流说明可以作为 CAD 生成的上游参考。
SourceAEC 不将其复制成自己的 SDK，不承诺实现其全部能力。其他外部几何工具
同样应按其实际安装的能力、输入与输出格式接入。

本仓只保留 sourceaec 接入 skill 与按需读取的 IFC/DXF 参考。规划不设独立 skill，
外部 SDK 的建模知识由上游维护，Agent 角色与 skill 目录分开配置。没有 hooks、Python CLI、画法 DSL
或强制的模型推理流程。模型能否正确生成取决于实际源码、执行环境和验证证据。
