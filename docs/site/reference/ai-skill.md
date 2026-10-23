# 声明性 skills

skills/aiplan、skills/aidxf、skills/aiifc 只维护简短的工作约定与参考。
直接以 skills 目录作为 skillsDir，不需要打包 Python CLI 或安装 skill venv。

- aiplan：规划输入、假设和检查证据；声明性方案由 Go API 版本化。
- aidxf：外部 CAD/ezdxf 能力与脚本 API 的衔接，不提供本仓画法 DSL。
- aiifc：IFC 单位、空间结构、稳定标识及脚本 API 编辑约定。

这些说明不能替代实际几何库、模型训练能力或运行验证。
不捆绑 hooks、Python SDK、自动设计 gate 或执行源码。
SimpleCADAPI 的上游声明性工作流可作为独立 CAD 能力参考，按安装版本使用。

[AI 集成](/reference/ai)、[引擎边界](/development/integration-boundaries)。
