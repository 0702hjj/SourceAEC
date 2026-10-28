# Go 编辑领域

编辑实现位于 `server/internal/editing`，由网关在进程内调用；没有独立 Python
HTTP 服务，也不再需要 8100/8200 端口。旧页面地址保留用于文档链接兼容。

共享行为包括 PARAMS 字面量读写、十步暂存与撤销/重做、revision 冲突检查、
试运行、保存、回滚、版本对比、Git 分支及事务发布。IFC/DXF 差异由 Go 格式适配器处理。

一次修改读取当前脚本及 revision，然后 stage → run → save。
`baseRevision` 不匹配返回冲突；失败执行不会发布产品或创建版本。
发布通过 PostgreSQL 事务锁协调；几何执行在事务外进行，发布前重新验证输入身份。
重启恢复的是持久业务状态和文件投影，不是未完成的脚本执行。

Python 源码仅是用户/Agent 提交的模型运行时数据。执行器在临时目录生成脚本，
传入产品输出路径；环境、资源限制、版本规则与发布均由 Go 决定。
没有内置画法 SDK、规划 CLI 或 Python AST 服务。

[API 契约](/reference/edit-api)、[沙箱](/development/sandbox)、
[CAD 适配](/development/cad-service)、[执行与恢复](/development/execution-lifecycle)。
