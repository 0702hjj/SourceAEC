# 配置说明

## Go server（`server/server_config.json`）

路径相对于进程工作目录解析。

> **本地配置文件不入库。** `server/server_config.json` 在 `.gitignore` 里，因为含有 LLM key、数据库 DSN 这类敏感项。首次使用先复制模板再改：
>
> ```bash
> cp server/server_config.example.json server/server_config.json
> ```
>
> 直接改被跟踪的模板文件，`git reset --hard` 时会被覆盖。

| key | 默认 | env 覆盖 | 说明 |
| --- | --- | --- | --- |
| `host` / `port` | `127.0.0.1` / `8090` | — | 监听地址 |
| `dataDir` | `../data` | — | 数据目录，必须和编辑服务的 `VIEWER_DATA_DIR` 是同一个目录 |
| `nodeBin` / `converterScript` | `node` / `../converter/convert.js` | — | 转换器调用 |
| `maxUploadMB` | `200` | — | 上传上限 |
| `webDist` | `../web/dist` | `VIEWER_WEB_DIST` | 前端构建产物目录。存在就由 server 托管，缺失时静态路径 503、API 照常 |
| `pgDSN` | `""` | `VIEWER_PG_DSN` | 配置即启用 PostgreSQL，空则用文件存储 |
| `editServiceURL` | `http://127.0.0.1:8100` | `VIEWER_EDIT_SERVICE_URL` | IFC 编辑服务地址 |
| `cadServiceURL` | `http://127.0.0.1:8200` | `VIEWER_CAD_SERVICE_URL` | DXF 编辑服务地址 |
| `llmAPIKey` | `""` | `VIEWER_LLM_API_KEY` | chat agent 的 LLM key。留空则进入离线模式，用确定性 mock，不产生真实回复 |
| `llmBaseURL` | `""` | `VIEWER_LLM_BASE_URL` | OpenAI 兼容端点，如 `https://api.openai.com/v1` |
| `llmModel` | `""` | `VIEWER_LLM_MODEL` | 模型名，如 `gpt-4o`、`deepseek-chat` |
| `skillsDir` | `../skills/dist` | `VIEWER_SKILLS_DIR` | 正式 skill 集合目录，agent 只面对 dist，不感知开发版本 |
| `skillVenv` | `../skills/.venv` | `VIEWER_SKILLS_VENV` | skill 专用 venv，装法：`bash tools/install_skill_venv.sh` |
| `skillCLI` | `aiplan,aidxfv3,aiifc` | `VIEWER_SKILLS_CLI` | agent 可执行命令的白名单 |
| `mcpDir` | `""` | `VIEWER_MCP_DIR` | `mcp/` 目录，作为 stdio MCP server 的 cwd。留空则不接 MCP |
| `apiToken` | `""` | `VIEWER_API_TOKEN` | Bearer 鉴权。留空即关闭，只适合本机开发；生产必须设置 |
| `corsOrigins` | `http://localhost:5173,http://localhost:8080` | `VIEWER_CORS_ORIGINS` | CORS 白名单，逗号分隔 |
| `pprofAddr` | `127.0.0.1:6060` | `VIEWER_PPROF_ADDR` | 观测监听器（pprof + `/debug/vars` 并发姿态指标），只绑回环；`disable` 关闭。压测工具 `server/cmd/loadgen` 依赖它采样 |
| `llmTimeoutS` | `120` | `VIEWER_LLM_TIMEOUT_S` | 单次 LLM API 调用硬上限——上游挂起不再永久滞留对话 goroutine |
| `sseHeartbeatS` | `15` | `VIEWER_SSE_HEARTBEAT_S` | SSE 空闲心跳间隔（注释帧），过反代 idle timeout 不被静默掐断 |
| `shutdownJoinS` | `10` | `VIEWER_SHUTDOWN_JOIN_S` | 停机后台收尾预算（chat 后台 goroutine + convert 队列排干） |

最小配置示例：

```json
{
  "host": "127.0.0.1",
  "port": 8090,
  "dataDir": "../data",
  "nodeBin": "node",
  "converterScript": "../converter/convert.js",
  "maxUploadMB": 200,
  "webDist": "../web/dist",
  "pgDSN": "",
  "editServiceURL": "http://127.0.0.1:8100",
  "cadServiceURL": "http://127.0.0.1:8200",
  "skillsDir": "../skills/dist",
  "skillVenv": "../skills/.venv",
  "skillCLI": "aiplan,aidxfv3,aiifc"
}
```

## 鉴权与 CORS

鉴权默认关闭，因为编辑 API 会在沙箱里执行脚本，等于代码执行入口，所以**生产环境必须设置 `apiToken`**。CORS 走白名单。

协议侧的行为——哪四类只读文件端点豁免、401 后浏览器怎么自动重试、chat 的 SSE 为什么用 `?token=` 查询参数——见 [REST API 的鉴权一节](/reference/rest-api#鉴权与-cors)。

两个 Python 编辑服务自身没有鉴权，务必只监听 127.0.0.1。

## chat agent

- LLM 三参见上表。key 留空时回退确定性的离线 mock，界面流程能走通，但不产生真实智能回复。
- skill 三项配置：`skillsDir` 指向正式集合，`skillVenv` 提供 CLI 运行环境，`skillCLI` 是命令白名单。
- 文件工具里 grep 依赖 ripgrep，没装会报错，用 `sudo apt install ripgrep` 装上。
- 旧的 `VIEWER_OPENCODE_URL` 已退役，设置了也没有效果，可以从部署环境里删掉。
- agent 的工具面、主子编排与提问机制见 [AI 接入](/reference/ai)。

## edit-service

| 环境变量 | 默认 | 说明 |
| --- | --- | --- |
| `VIEWER_DATA_DIR` | `../data` | 数据目录。必须与 server 的 `dataDir` 一致，否则编辑请求 404 |
| `EDIT_SERVICE_PORT` | `8100` | 监听端口 |
| `AIIFC_FLOWS_DIR` | `../../skills/aiifc/references/docs/flows` | aiifc skill 的 flows 目录，沙箱契约校验依赖其中的 `script_lib.py` |
| `EDIT_SERVICE_MAX_MODELS` | `8` | 内存模型缓存上限 |
| `EDIT_SERVICE_DIFF_TIMEOUT_S` | `60` | 语义 diff 超时，经 Go 代理时映射为 `50400` |

## cad-edit-service

与 edit-service 同构：

| 环境变量 | 默认 | 说明 |
| --- | --- | --- |
| `VIEWER_DATA_DIR` | `../data` | 数据目录，三方必须一致 |
| `CAD_SERVICE_PORT` | `8200` | 监听端口 |
| `AIDXF_FLOWS_DIR` | `flows` | DXF 沙箱契约层目录 |
| `AIDXF_DRAWLIB_DIR` | 由仓库内 skill 源目录推导 | 共享画法层（archdxf + dxfkit）的 src 目录，冒号分隔 |
| `CAD_SERVICE_MAX_MODELS` | `8` | 内存模型缓存上限 |
| `CAD_SERVICE_DIFF_TIMEOUT_S` | `60` | 实体级 diff 超时 |

## 沙箱环境变量

`script/run` 和 `script/save` 在服务端沙箱里执行脚本。机制见[沙箱执行环境](/development/sandbox)，部署相关的变量如下：

| 环境变量 | 默认 | 说明 |
| --- | --- | --- |
| `SANDBOX_BACKEND` | `auto` | `auto`、`bwrap`、`rlimit` 三选一。生产只有 bwrap，rlimit 仅供测试 |
| `SCRIPT_RUN_CONCURRENCY` | `3` | run/save 并发上限 |
| `SCRIPT_MAX_FSIZE_BYTES` / `SCRIPT_MAX_OUTPUT_BYTES` / `SCRIPT_MAX_PRODUCT_BYTES` | `256MiB` / `1MiB` / `256MiB` | 沙箱资源限额 |
| `SANDBOX_ENV_CACHE_DIR` | `$XDG_CACHE_HOME/aibim-sandbox-envs` | 依赖环境缓存根。不要配到 data/ 或 /tmp 下 |
| `SCRIPT_ENV_BUILD_TIMEOUT_S` | `300` | 依赖环境构建超时 |

脚本可以用 PEP 723 块声明依赖，声明即全量替换默认集。run/save 依赖 `uv` 二进制，缺失报 503。

## PostgreSQL

不配置时，Issue、override、修改记录全部落文件，零外部依赖。配置 DSN 后自动建三张表。模型文件和版本快照始终在文件系统里，所以备份要覆盖两边。

测试要用 `VIEWER_TEST_PG_DSN` 指向专用测试库，测试会删表。

## 端口

默认端口一览：server 8090，edit-service 8100，cad-edit-service 8200，web 开发服务器 5173。
