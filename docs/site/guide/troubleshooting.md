# 故障排查

按现象查表。多数问题的根因集中在三类：**数据目录不一致**、**沙箱不可用**、**LLM key 没配**。机制说明见对应页面，本页只讲怎么办。

## 模型与查看

| 现象 | 处理 |
| --- | --- |
| 模型一直停在 `converting` | 看 server 日志里的 converter stderr；手动复现：`node converter/convert.js <input.ifc> <outDir>`；核对 `nodeBin` 与 `converterScript` 配置 |
| 模型 `failed` | 调 `POST /api/v1/models/{id}/retry` 重新入队转换 |
| 编辑请求报 404 model not found | 编辑服务的 `VIEWER_DATA_DIR` 与 server 的 `dataDir` 不是同一个目录——两边必须指向同一个 `data` 绝对路径 |
| 页面报 401 | 没带 token。浏览器把 token 存在 localStorage 的 `aiifc_token` 键，弹框保存后会自动重试原请求 |
| 打开 :8090 静态路径 503，API 却正常 | `webDist` 指的前端构建产物目录不存在，先 `cd web && npm run build` |

## 编辑与版本

| 现象 | 处理 |
| --- | --- |
| 脚本编辑报 422 | 契约静态校验、沙箱构建或依赖解析失败。请求零副作用，按报错信息修正后重发 |
| 脚本编辑报 429 | 并发闸满（`SCRIPT_RUN_CONCURRENCY`），稍后重试 |
| 脚本编辑报 503 | 沙箱不可用：缺 `bwrap` 或缺 `uv`，属部署问题，不降级执行 |
| edit-call 报 409 | 暂存脚本与 ScriptMap 分叉——暂存了新脚本但没运行过。先跑一次试运行再定位或改写 |
| 定位脚本返回 `found: false` | 该构件不在当前 ScriptMap 里，这是正常响应不是错误。模型可能是 plain 态（没有构建脚本），先让 AI 复现成脚本 |
| 改了脚本前端没刷新 | 直连编辑服务的 run/save **不触发** XKT 重转（`retry` 只对 `failed` 模型有效，帮不上忙）。改走 Go 代理（:8090）的 `script/run`、`script/save`、`script/rollback` |

## AI 与存储

| 现象 | 处理 |
| --- | --- |
| AI 对话没有智能回复 | LLM 三参没配，正处于离线模式。配置 `VIEWER_LLM_API_KEY`、`VIEWER_LLM_BASE_URL`、`VIEWER_LLM_MODEL` |
| agent 的文件工具报错 | grep 工具依赖 ripgrep，装上即可（Debian/Ubuntu：`sudo apt install ripgrep`） |
| PostgreSQL 连不上 | 清空 `pgDSN`（或 `VIEWER_PG_DSN`）回退文件存储，或核对 DSN |

## 状态码去哪查

编辑请求的失败语义分散在两处，按归属看：

- **422、429、503 属沙箱**——契约或构建失败（零副作用）、并发闸满、沙箱不可用。逐条定义见[沙箱执行环境](/development/sandbox)的失败语义表。
- **409 属编辑契约**——暂存脚本与 ScriptMap 分叉，`edit-call` 拒绝执行。见 [IFC 编辑 API](/reference/edit-api#定位链路)。

这四种失败都不产生半成品版本，暂存链也不会被写坏。

验证手段见[测试与调试](/development/testing)：冒烟脚本覆盖上传到编辑的全链路，手工清单覆盖界面。
