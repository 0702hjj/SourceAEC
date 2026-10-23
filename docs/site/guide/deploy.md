# 生产部署

Go 网关处理所有业务，PostgreSQL 保存业务状态。安装 Git、bubblewrap、prlimit，
准备外部几何运行时；Python 不作为 HTTP 应用部署。

构建命令：`scripts/build_server_release.sh VERSION`。Linux amd64/arm64 包包含
sourceaec-server、sourceaec-runtime、sourceaec-mcp、编译信息、示例配置和许可证。
用 SHA256SUMS 校验下载；前端与转换器依赖按匹配源码版本单独构建安装。

将配置中的路径设为正确位置，设置 PostgreSQL、网关 token 和 CORS，再启动网关。
需要 HTTPS 时放在反向代理后面。避免公开调试端口。

外部 Python 环境可通过 SOURCEAEC_PYTHON 指定。否则 uv 在 Go 控制下准备
Python 3.12 与固定依赖，生产脚本仍只在 bubblewrap 内执行。

升级前备份数据库和 data 目录，停止旧进程后更新匹配的二进制与静态资源。
旧发布 v0.1.0 的安装说明描述其发布时架构；当前源码应按本页部署。
