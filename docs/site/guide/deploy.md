# 生产部署与运维

生产形态是**宿主机直跑，不用 Docker**。对外只有 8090 一个端口：Go server 托管前端构建产物，浏览器只访问它；两个 Python 编辑服务只监听回环，不暴露到网络。

## 构建与启动

每行都从**仓库根目录**执行，不链式 `cd`（下面三个进程各自一个终端）：

```bash
# 1. 构建前端（产物在 web/dist）
cd web && npm ci && npm run build

# 2. 构建 server（默认托管 ../web/dist，可用 webDist / VIEWER_WEB_DIST 改路径）
cd server && go build -o server ./cmd/server

# 3. 依次启动：server 对外，两个编辑服务只绑回环
cd server && ./server
cd services/ifc && uv sync && VIEWER_DATA_DIR=/srv/sourceaec/data uv run uvicorn app.main:app --host 127.0.0.1 --port 8100
cd services/cad && uv sync && VIEWER_DATA_DIR=/srv/sourceaec/data uv run uvicorn app.main:app --host 127.0.0.1 --port 8200
```

**生产必须设置 `VIEWER_API_TOKEN`。** 编辑 API 会在服务端沙箱里执行脚本，等于开放代码执行入口。不设 token 只适用于本机单人开发，任何对外部署都必须带上。鉴权与 CORS 的行为细节见 [REST API](/reference/rest-api)，配置项见[配置说明](/guide/configuration)。

两条硬性前提：

- **沙箱依赖宿主机的 `bwrap` 与 `uv`**。缺任何一个，run/save 直接返回 503，不降级执行。安装与验证见[沙箱执行环境](/development/sandbox)。
- **两个 Python 服务的 `VIEWER_DATA_DIR` 必须与 server 的 `dataDir` 指向同一个目录**，且 server 以哪个用户运行，该目录就要对那个用户可写。配错会 404 或改错文件。

Agent 与 skill CLI 会处理用户输入和外部模型输出，两者都应视为不可信内容。请用专用、
无登录权限的低权限系统账号运行 SourceAEC，只授予代码只读权限和 `data/`、skill 工作区
的必要写权限；该账号及其环境中不要放置云凭据、SSH 私钥、其他项目源码或客户资料。
当前 API token 是单一入口凭证，不提供用户级授权、租户隔离或审计归属；需要多人或公网
服务时，应在前置网关实现独立身份认证、速率限制、请求日志脱敏和租户级数据隔离。

## systemd 最小示例

```ini
# /etc/systemd/system/sourceaec-server.service
[Unit]
Description=SourceAEC Go server
After=network.target

[Service]
WorkingDirectory=/opt/SourceAEC/server
Environment=VIEWER_API_TOKEN=换成强随机串
ExecStart=/opt/SourceAEC/server/server -config server_config.json
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```ini
# /etc/systemd/system/sourceaec-ifc.service（cad 服务同构：目录换 services/cad、端口 8200）
[Unit]
Description=SourceAEC edit-service
After=network.target

[Service]
WorkingDirectory=/opt/SourceAEC/services/ifc
Environment=VIEWER_DATA_DIR=/opt/SourceAEC/data
ExecStart=/usr/local/bin/uv run uvicorn app.main:app --host 127.0.0.1 --port 8100
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

## 反向代理（可选）

Go server 自带静态托管与 SPA fallback，**正常部署不需要 nginx**。只有在需要统一 TLS 终止、多站点复用一个入口或再加一层访问控制时才在前面挂反代，此时注意两件事：

- **SSE 不能被缓冲或掐断**。chat 事件流走 `GET /api/v1/chat/sessions/{cid}/events`，需要 `proxy_buffering off`，且读取超时要大于 `sseHeartbeatS`（默认 15 秒的心跳间隔）。心跳过不了反代 idle timeout，前端会在长任务中途静默断流。
- **只转发 8090**。8100 与 8200 没有鉴权，部署时必须限制为回环监听或受控内网；不要给它们开对外路由。
- **观测端口默认只监听 127.0.0.1:6060**，且不经过主 API token；不要把 `VIEWER_PPROF_ADDR` 配成公网地址。若必须远程观测，请使用受限管理网络或额外的认证反向代理，并确认不会把 pprof、expvar 或 token 写入公共日志。

## PostgreSQL（可选）

不配置时 Issue、override、修改记录全部落文件，零外部依赖。装好 14+ 版本后给 server 传 DSN 即可，建表自动完成：

```bash
VIEWER_PG_DSN=postgres://user:pass@127.0.0.1:5432/sourceaec
```

注意模型文件与版本快照**始终在文件系统**，PG 只承接网关侧的三张表，所以备份要覆盖两边。

## 备份与升级

- **要备份的是 `data/` 目录**。模型原件、大版本脚本（`scripts/v{n}.py` + `v{n}.map.json`）、暂存链、方案文件（`plans/{projectID}/`）、Issue 截图都在里面；历史版本的 IFC/DXF 产物可从脚本重建，丢了不致命。数据目录的完整布局见[存储与前端对接](/development/integration#数据目录布局)。
- 升级就是重新 `npm run build` 前端、重新 `go build` server，然后重启三个服务。大版本脚本与定位 map 成对追加写，跨版本可读；暂存链也是原子落盘的（`script_staging.json`），**重启会恢复而非丢失**。唯一要注意的是暂存脚本如果没跑过，它的定位 map 会和它分叉，重启后仍需要先运行一次才能定位。

## 验证

```bash
# 端到端冒烟（需 server 运行；edit-service 不可达时编辑段落自动跳过）
./scripts/smoke.sh
```

故障现象对照见[故障排查](/guide/troubleshooting)。
