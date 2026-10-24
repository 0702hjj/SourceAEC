# Go 沙箱执行

Go 准备独立执行目录、依赖环境与进程，生产执行必须使用 Linux bubblewrap。
缺少 bubblewrap 或 `prlimit` 时拒绝执行，没有生产降级后端。

沙箱不共享宿主网络，系统和几何环境只读，工作目录可写；宿主业务数据和凭据
不挂载。默认运行时限 60 秒、地址空间 1 GiB、进程上限 256、文件与产品上限
256 MiB、累计日志上限 1 MiB。超时或日志超限终止进程组；发布拒绝符号链接和非普通文件。

`SOURCEAEC_PYTHON` 可指定已经装好外部几何包的解释器绝对路径。
未设置时 Go 使用 uv 创建按依赖摘要寻址的 Python 3.12 环境：IFC 默认使用
IfcOpenShell/numpy，DXF 默认使用 ezdxf。包版本在 Go 环境适配器中固定。

模型源码可通过 PEP 723 声明精确版本的 registry 依赖；不接受本地路径、URL、
未固定版本或命令行参数。依赖安装在隔离执行前进行，不是模型脚本中的业务步骤。

模型程序接收输出文件路径作为第一个参数。需要源定位时可选地将 JSON map 写入
`SOURCEAEC_MAP_OUT`；Go 绑定脚本摘要并检查发布。没有 SourceAEC Python SDK。

[配置](/guide/configuration)、[测试](/development/testing)。

项目上游输入由 Go 采集为受限快照，以只读目录挂载到 SOURCEAEC_INPUTS_DIR
（也可通过 VIEWER_DATA_DIR 获取同一目录），仅包含该项目的 plans 与 skill-work
输入，不挂载宿主业务目录。保存版本会归档输入，回滚和分支切换重放对应快照。
